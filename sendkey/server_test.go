package sendkey

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*Server, *MemStore) {
	t.Helper()
	store := NewMemStore(100)
	srv := NewServer(store, Config{MaxBytes: 1024, RatePerMin: 1000})
	return srv, store
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func createSecret(t *testing.T, srv *Server, ct []byte) string {
	t.Helper()
	body, _ := json.Marshal(CreateRequest{CT: b64(ct), IV: b64(make([]byte, 12)), TTL: 3600, Views: 1})
	req := httptest.NewRequest("POST", "/api/secret", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create: status %d body %s", w.Code, w.Body)
	}
	var resp struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("create: bad json: %v", err)
	}
	return resp.ID
}

func TestCreateAndConsumeRoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	ct := []byte("sealed-bytes")
	id := createSecret(t, srv, ct)

	// Meta peek does not burn.
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/secret/"+id+"/meta", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("meta: status %d", w.Code)
	}

	// Consume returns the ciphertext.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/secret/"+id, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("consume: status %d", w.Code)
	}
	var resp struct{ CT string }
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	got, _ := base64.RawURLEncoding.DecodeString(resp.CT)
	if !bytes.Equal(got, ct) {
		t.Fatalf("roundtrip mismatch: %q != %q", got, ct)
	}

	// Second consume must 404: the secret burned.
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/secret/"+id, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("after burn: want 404, got %d", w.Code)
	}
}

func TestConcurrentConsumeOnlyOneWins(t *testing.T) {
	srv, _ := newTestServer(t)
	id := createSecret(t, srv, []byte("once"))

	const n = 50
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/secret/"+id, nil))
			codes[i] = w.Code
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, c := range codes {
		if c == http.StatusOK {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("burn must be atomic: %d readers succeeded, want exactly 1", wins)
	}
}

func TestRejectsOversizedSecret(t *testing.T) {
	srv, _ := newTestServer(t) // MaxBytes: 1024
	body, _ := json.Marshal(CreateRequest{CT: b64(make([]byte, 2048)), IV: b64(make([]byte, 12)), TTL: 60, Views: 1})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("POST", "/api/secret", bytes.NewReader(body)))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", w.Code)
	}
}

func TestRejectsBadInput(t *testing.T) {
	srv, _ := newTestServer(t)
	cases := []string{
		`{"ct":"","iv":"AAAAAAAAAAAAAAAA","ttl":60,"views":1}`,     // empty ct
		`{"ct":"AAAA","iv":"AAAA","ttl":60,"views":1}`,             // iv wrong length
		`{"ct":"!!not-base64!!","iv":"AAAAAAAAAAAAAAAA","ttl":60}`, // bad encoding
		`not json at all`, // not json
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest("POST", "/api/secret", strings.NewReader(c)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("case %q: want 400, got %d", c, w.Code)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(5)
	now := time.Unix(1_700_000_000, 0)
	rl.now = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed (burst)", i)
		}
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("6th request should be blocked")
	}
	if !rl.allow("5.6.7.8") {
		t.Fatal("different client should not be affected")
	}

	now = now.Add(13 * time.Second) // ~1 token refilled at 5/min
	if !rl.allow("1.2.3.4") {
		t.Fatal("token should have refilled")
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("only one token should have refilled")
	}
}

func TestLimiterEvictsIdleBuckets(t *testing.T) {
	rl := newRateLimiter(60)
	now := time.Unix(1_700_000_000, 0)
	rl.now = func() time.Time { return now }

	for i := 0; i < sweepAfter+100; i++ {
		rl.allow(fmt.Sprintf("10.0.%d.%d", i>>8&255, i&255))
	}
	if len(rl.buckets) <= sweepAfter {
		t.Fatalf("setup: want >%d buckets, got %d", sweepAfter, len(rl.buckets))
	}

	// Past both the idle timeout and the sweep interval, one call reclaims.
	now = now.Add(idleTimeout + time.Minute)
	rl.allow("fresh-client")
	if len(rl.buckets) != 1 {
		t.Fatalf("idle buckets should be evicted, got %d", len(rl.buckets))
	}
}

func TestLimiterSweepIsTimeGated(t *testing.T) {
	rl := newRateLimiter(60)
	now := time.Unix(1_700_000_000, 0)
	rl.now = func() time.Time { return now }

	for i := 0; i < sweepAfter+10; i++ {
		rl.allow(fmt.Sprintf("10.0.%d.%d", i>>8&255, i&255))
	}
	first := rl.lastSweep

	// Within the interval no further sweep runs, so the O(n) scan cannot
	// fire on every request — that was the bug this guards against.
	now = now.Add(time.Second)
	rl.allow("another")
	if !rl.lastSweep.Equal(first) {
		t.Fatal("sweep ran twice inside the sweep interval")
	}
}

func TestLimiterStillLimitsAfterSweep(t *testing.T) {
	rl := newRateLimiter(2)
	now := time.Unix(1_700_000_000, 0)
	rl.now = func() time.Time { return now }

	// Exhaust one client, then force a sweep with many other clients.
	rl.allow("victim")
	rl.allow("victim")
	if rl.allow("victim") {
		t.Fatal("victim should be limited")
	}
	for i := 0; i < sweepAfter+10; i++ {
		rl.allow(fmt.Sprintf("10.0.%d.%d", i>>8&255, i&255))
	}
	// A drained bucket must survive the sweep, or the limit is trivially
	// bypassable by flooding the map with new keys.
	if rl.allow("victim") {
		t.Fatal("drained bucket was evicted: rate limit bypassable")
	}
}

func TestRateLimitEndpoint(t *testing.T) {
	store := NewMemStore(100)
	srv := NewServer(store, Config{MaxBytes: 1024, RatePerMin: 2})

	mk := func() int {
		body, _ := json.Marshal(CreateRequest{CT: b64([]byte("x")), IV: b64(make([]byte, 12)), TTL: 60, Views: 1})
		req := httptest.NewRequest("POST", "/api/secret", bytes.NewReader(body))
		req.RemoteAddr = "9.9.9.9:1234"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		return w.Code
	}

	if mk() != http.StatusOK || mk() != http.StatusOK {
		t.Fatal("first two creates should pass")
	}
	if got := mk(); got != http.StatusTooManyRequests {
		t.Fatalf("third create: want 429, got %d", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv, _ := newTestServer(t)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	for header, want := range map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s: want %q, got %q", header, want, got)
		}
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP missing default-src 'self': %q", csp)
	}
}

func TestPagesServe(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, path := range []string{"/", "/s/someid", "/assets/app.js", "/assets/brand.css", "/healthz"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("GET %s: want 200, got %d", path, w.Code)
		}
	}
}

func TestAssetTraversalBlocked(t *testing.T) {
	srv, _ := newTestServer(t)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/assets/"+"%2e%2e/main.go", nil))
	if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "package main") {
		t.Fatal("path traversal leaked source")
	}
}

func TestTTLAndViewClamps(t *testing.T) {
	if ClampTTL(0) != 24*time.Hour {
		t.Error("default TTL should be 24h")
	}
	if ClampTTL(5) != time.Minute {
		t.Error("TTL floor should be 60s")
	}
	if ClampTTL(10_000_000) != 7*24*time.Hour {
		t.Error("TTL ceiling should be 7d")
	}
	if ClampViews(0) != 1 || ClampViews(-3) != 1 {
		t.Error("views floor should be 1")
	}
	if ClampViews(99) != 10 {
		t.Error("views ceiling should be 10")
	}
}

func BenchmarkCreateConsume(b *testing.B) {
	store := NewMemStore(1_000_000)
	srv := NewServer(store, Config{MaxBytes: 4096, RatePerMin: 1 << 30})
	body, _ := json.Marshal(CreateRequest{CT: b64(bytes.Repeat([]byte("k"), 256)), IV: b64(make([]byte, 12)), TTL: 3600, Views: 1})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/api/secret", bytes.NewReader(body))
		req.RemoteAddr = fmt.Sprintf("10.0.%d.%d:1", i>>8&255, i&255)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		var resp struct{ ID string }
		_ = json.Unmarshal(w.Body.Bytes(), &resp)

		w2 := httptest.NewRecorder()
		srv.ServeHTTP(w2, httptest.NewRequest("GET", "/api/secret/"+resp.ID, nil))
	}
}

// TestNoInlineStylesOrScripts guards the strict CSP. `default-src 'self'`
// blocks inline <style> blocks and style="" attributes outright, and a
// blocked style fails silently — the page renders unstyled rather than
// erroring — so this is caught here instead of in production.
func TestNoInlineStylesOrScripts(t *testing.T) {
	for _, page := range []string{"index.html", "s.html", "a.html"} {
		data, err := fs.ReadFile(webFS, "public/"+page)
		if err != nil {
			t.Fatalf("read %s: %v", page, err)
		}
		body := string(data)
		for _, banned := range []string{`style="`, "<style>", "<style ", "javascript:"} {
			if strings.Contains(body, banned) {
				t.Errorf("%s contains %q, which the CSP blocks", page, banned)
			}
		}
		// Inline event handlers are blocked for the same reason.
		for _, attr := range []string{"onclick=", "onload=", "onerror=", "onsubmit="} {
			if strings.Contains(body, attr) {
				t.Errorf("%s contains inline handler %q, which the CSP blocks", page, attr)
			}
		}
	}
}

// TestAssetsReferencedByPagesExist catches a stylesheet or module renamed on
// one side of the split but not the other; a missing asset is another silent
// visual failure.
func TestAssetsReferencedByPagesExist(t *testing.T) {
	ref := regexp.MustCompile(`(?:href|src)="(/assets/[^"]+)"`)
	for _, page := range []string{"index.html", "s.html", "a.html"} {
		data, err := fs.ReadFile(webFS, "public/"+page)
		if err != nil {
			t.Fatalf("read %s: %v", page, err)
		}
		for _, m := range ref.FindAllStringSubmatch(string(data), -1) {
			path := strings.TrimPrefix(m[1], "/")
			if _, err := fs.ReadFile(webFS, "public/"+path); err != nil {
				t.Errorf("%s references %s, which does not exist", page, m[1])
			}
		}
	}
}
