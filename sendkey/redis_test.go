package sendkey

// Tests for the serverless backend. They run against a stub that speaks the
// Upstash REST protocol and emulates the handful of Redis commands used,
// including the Lua burn script, so the atomic-burn guarantee is verified for
// the serverless path and not just for MemStore.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRedis is a tiny, correct-enough Redis for the commands RedisStore uses.
type fakeRedis struct {
	mu     sync.Mutex
	vals   map[string]string
	ttls   map[string]int64
	calls  int
	server *httptest.Server
}

func newFakeRedis(t *testing.T) *fakeRedis {
	t.Helper()
	f := &fakeRedis{vals: map[string]string{}, ttls: map[string]int64{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeRedis) handle(w http.ResponseWriter, r *http.Request) {
	var cmd []any
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil || len(cmd) == 0 {
		http.Error(w, `{"error":"bad command"}`, http.StatusBadRequest)
		return
	}
	if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++

	str := func(i int) string { s, _ := cmd[i].(string); return s }

	switch strings.ToUpper(str(0)) {
	case "PING":
		writeJSON(w, http.StatusOK, map[string]any{"result": "PONG"})

	case "SET":
		key, val := str(1), str(2)
		if len(cmd) > 5 && strings.EqualFold(str(5), "NX") {
			if _, exists := f.vals[key]; exists {
				writeJSON(w, http.StatusOK, map[string]any{"result": nil})
				return
			}
		}
		f.vals[key] = val
		if len(cmd) > 4 && strings.EqualFold(str(3), "EX") {
			secs, _ := strconv.ParseInt(str(4), 10, 64)
			f.ttls[key] = secs
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": "OK"})

	case "GET":
		v, ok := f.vals[str(1)]
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{"result": nil})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": v})

	case "TTL":
		if _, ok := f.vals[str(1)]; !ok {
			writeJSON(w, http.StatusOK, map[string]any{"result": -2})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": f.ttls[str(1)]})

	case "EVAL":
		// Emulate burnScript. The whole handler holds f.mu, which is exactly
		// the atomicity Redis gives a Lua script.
		key := str(2 + 1) // EVAL script numkeys key
		raw, ok := f.vals[key]
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{"result": nil})
			return
		}
		var stored storedSecret
		_ = json.Unmarshal([]byte(raw), &stored)
		stored.Views--
		if stored.Views <= 0 {
			delete(f.vals, key)
			delete(f.ttls, key)
		} else {
			b, _ := json.Marshal(stored) // KEEPTTL: f.ttls[key] untouched
			f.vals[key] = string(b)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"result": []string{raw, strconv.Itoa(stored.Views)},
		})

	default:
		writeJSON(w, http.StatusOK, map[string]any{"error": "unknown command"})
	}
}

func (f *fakeRedis) store() *RedisStore {
	return NewRedisStore(f.server.URL, "test-token")
}

func (f *fakeRedis) size() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.vals)
}

func TestRedisRoundTripAndBurn(t *testing.T) {
	f := newFakeRedis(t)
	rs := f.store()

	id, err := rs.Put(ctx, []byte("ciphertext"), make([]byte, IVLen), time.Hour, 1)
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	meta, err := rs.Peek(ctx, id)
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if meta.Views != 1 {
		t.Fatalf("views: want 1, got %d", meta.Views)
	}
	if f.size() != 1 {
		t.Fatal("peek must not burn the secret")
	}

	sec, err := rs.Consume(ctx, id)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if string(sec.CT) != "ciphertext" {
		t.Fatalf("ct mismatch: %q", sec.CT)
	}
	if f.size() != 0 {
		t.Fatal("secret should be deleted after its last view")
	}
	if _, err := rs.Consume(ctx, id); err != ErrNotFound {
		t.Fatalf("second consume: want ErrNotFound, got %v", err)
	}
}

func TestRedisMultiViewKeepsTTL(t *testing.T) {
	f := newFakeRedis(t)
	rs := f.store()

	id, _ := rs.Put(ctx, []byte("x"), make([]byte, IVLen), 2*time.Hour, 3)
	before, _ := rs.Peek(ctx, id)

	for want := 2; want >= 0; want-- {
		sec, err := rs.Consume(ctx, id)
		if err != nil {
			t.Fatalf("consume (want %d left): %v", want, err)
		}
		if sec.Views != want {
			t.Fatalf("viewsLeft: want %d, got %d", want, sec.Views)
		}
		if want == 0 {
			break
		}
		// A read must not extend the lifetime, or a frequently-read secret
		// would live forever.
		after, err := rs.Peek(ctx, id)
		if err != nil {
			t.Fatalf("peek after consume: %v", err)
		}
		if !after.ExpiresAt.Before(before.ExpiresAt.Add(time.Second)) {
			t.Fatal("consume must not extend the TTL")
		}
	}
	if _, err := rs.Consume(ctx, id); err != ErrNotFound {
		t.Fatalf("want ErrNotFound after exhausting views, got %v", err)
	}
}

func TestRedisConcurrentConsumeOnlyOneWins(t *testing.T) {
	f := newFakeRedis(t)
	rs := f.store()
	id, _ := rs.Put(ctx, []byte("once"), make([]byte, IVLen), time.Hour, 1)

	const n = 30
	var wg sync.WaitGroup
	wins := make([]bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := rs.Consume(ctx, id)
			wins[i] = err == nil
		}(i)
	}
	wg.Wait()

	count := 0
	for _, w := range wins {
		if w {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("burn must be atomic on redis too: %d winners, want 1", count)
	}
}

func TestRedisMissingKeyIsNotFound(t *testing.T) {
	rs := newFakeRedis(t).store()
	if _, err := rs.Peek(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("peek: want ErrNotFound, got %v", err)
	}
	if _, err := rs.Consume(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("consume: want ErrNotFound, got %v", err)
	}
}

func TestRedisBadTokenErrors(t *testing.T) {
	f := newFakeRedis(t)
	rs := NewRedisStore(f.server.URL, "wrong-token")
	if _, err := rs.Put(ctx, []byte("x"), make([]byte, IVLen), time.Hour, 1); err == nil {
		t.Fatal("put with a bad token should fail")
	}
}

func TestRedisServesFullHTTPStack(t *testing.T) {
	f := newFakeRedis(t)
	srv := NewServer(f.store(), Config{MaxBytes: 1024, RatePerMin: 100})

	id := createSecret(t, srv, []byte("through-the-api"))

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/secret/"+id, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("consume via HTTP: status %d", w.Code)
	}

	w = httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/secret/"+id, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("after burn: want 404, got %d", w.Code)
	}
}

func TestRedisFromEnv(t *testing.T) {
	t.Setenv("KV_REST_API_URL", "")
	t.Setenv("KV_REST_API_TOKEN", "")
	t.Setenv("UPSTASH_REDIS_REST_URL", "")
	t.Setenv("UPSTASH_REDIS_REST_TOKEN", "")
	if RedisFromEnv() != nil {
		t.Fatal("no credentials should yield no store")
	}

	t.Setenv("UPSTASH_REDIS_REST_URL", "https://example.upstash.io")
	t.Setenv("UPSTASH_REDIS_REST_TOKEN", "tok")
	if RedisFromEnv() == nil {
		t.Fatal("upstash-style env vars should be honoured")
	}

	// Vercel KV names take precedence, so a project with both set works.
	t.Setenv("KV_REST_API_URL", "https://kv.example.com")
	t.Setenv("KV_REST_API_TOKEN", "kvtok")
	rs := RedisFromEnv()
	if rs == nil || rs.url != "https://kv.example.com" {
		t.Fatalf("vercel kv names should win, got %+v", rs)
	}
}
