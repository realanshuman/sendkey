package sendkey

// Tests for ask mailboxes: caller-chosen ids with first-write-wins.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mailboxID(b byte) string {
	raw := bytes.Repeat([]byte{b}, 16)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestMemPutAtFirstWriteWins(t *testing.T) {
	s := NewMemStore(10)
	ctx := context.Background()
	id := mailboxID(1)

	if err := s.PutAt(ctx, id, []byte("first"), make([]byte, 12), time.Hour, 1); err != nil {
		t.Fatalf("first PutAt: %v", err)
	}
	if err := s.PutAt(ctx, id, []byte("second"), make([]byte, 12), time.Hour, 1); err != ErrExists {
		t.Fatalf("second PutAt: want ErrExists, got %v", err)
	}
	sec, err := s.Consume(ctx, id)
	if err != nil || string(sec.CT) != "first" {
		t.Fatalf("mailbox content: want first writer kept, got %q err %v", sec.CT, err)
	}
}

func TestMemPutAtReclaimsExpiredSlot(t *testing.T) {
	s := NewMemStore(10)
	ctx := context.Background()
	id := mailboxID(2)

	now := time.Now()
	s.now = func() time.Time { return now }
	if err := s.PutAt(ctx, id, []byte("old"), make([]byte, 12), time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute) // occupant expires
	if err := s.PutAt(ctx, id, []byte("new"), make([]byte, 12), time.Hour, 1); err != nil {
		t.Fatalf("PutAt over expired slot: %v", err)
	}
	sec, err := s.Consume(ctx, id)
	if err != nil || string(sec.CT) != "new" {
		t.Fatalf("want new occupant, got %q err %v", sec.CT, err)
	}
}

func TestRedisPutAtFirstWriteWins(t *testing.T) {
	f := newFakeRedis(t)
	store := NewRedisStore(f.server.URL, "test-token")
	ctx := context.Background()
	id := mailboxID(3)

	if err := store.PutAt(ctx, id, []byte("first"), make([]byte, 12), time.Hour, 1); err != nil {
		t.Fatalf("first PutAt: %v", err)
	}
	if err := store.PutAt(ctx, id, []byte("second"), make([]byte, 12), time.Hour, 1); err != ErrExists {
		t.Fatalf("second PutAt: want ErrExists, got %v", err)
	}
}

func postSecret(t *testing.T, srv *Server, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/secret", bytes.NewReader(raw))
	req.RemoteAddr = "10.9.9.9:1"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestCreateWithMailboxID(t *testing.T) {
	srv := NewServer(NewMemStore(100), Config{MaxBytes: 4096, RatePerMin: 1000})
	id := mailboxID(4)
	body := map[string]any{
		"ct": b64([]byte("sealed")), "iv": b64(make([]byte, 12)),
		"ttl": 3600, "views": 1, "id": id,
	}

	if w := postSecret(t, srv, body); w.Code != http.StatusOK {
		t.Fatalf("create at mailbox: want 200, got %d %s", w.Code, w.Body)
	}
	// the slot is taken now
	if w := postSecret(t, srv, body); w.Code != http.StatusConflict {
		t.Fatalf("second create: want 409, got %d %s", w.Code, w.Body)
	}
	// and it is served under exactly that id
	req := httptest.NewRequest("GET", "/api/secret/"+id, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("consume mailbox: want 200, got %d", w.Code)
	}
}

func TestCreateRejectsMalformedMailboxID(t *testing.T) {
	srv := NewServer(NewMemStore(100), Config{MaxBytes: 4096, RatePerMin: 1000})
	for _, bad := range []string{
		"short",
		base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 8)),  // 8 bytes
		base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)), // 32 bytes
		"not!!valid@@b64",
	} {
		w := postSecret(t, srv, map[string]any{
			"ct": b64([]byte("x")), "iv": b64(make([]byte, 12)), "id": bad,
		})
		if w.Code != http.StatusBadRequest {
			t.Errorf("id %q: want 400, got %d", bad, w.Code)
		}
	}
}

func TestAskPageServes(t *testing.T) {
	srv := NewServer(NewMemStore(10), Config{})
	for _, path := range []string{"/ask", "/a/someRequestId"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("GET %s: want 200, got %d", path, w.Code)
		}
	}
}

func TestMailboxProbe(t *testing.T) {
	srv := NewServer(NewMemStore(100), Config{MaxBytes: 4096, RatePerMin: 1000})
	id := mailboxID(7)

	probe := func() string {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/mailbox/"+id, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("probe: want 200 always, got %d", w.Code)
		}
		return w.Body.String()
	}

	if got := probe(); !bytes.Contains([]byte(got), []byte("false")) {
		t.Fatalf("empty mailbox: want ready false, got %s", got)
	}
	postSecret(t, srv, map[string]any{
		"ct": b64([]byte("sealed")), "iv": b64(make([]byte, 12)), "id": id,
	})
	if got := probe(); !bytes.Contains([]byte(got), []byte("true")) {
		t.Fatalf("full mailbox: want ready true, got %s", got)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/secret/"+id, nil))
	if got := probe(); !bytes.Contains([]byte(got), []byte("false")) {
		t.Fatalf("burned mailbox: want ready false again, got %s", got)
	}
}

func TestFileSizedChunkAccepted(t *testing.T) {
	srv := NewServer(NewMemStore(100), Config{RatePerMin: 1000})
	// a full 512KiB chunk plus GCM tag must pass the default MaxBytes
	big := bytes.Repeat([]byte{7}, 512*1024+16)
	w := postSecret(t, srv, map[string]any{"ct": b64(big), "iv": b64(make([]byte, 12))})
	if w.Code != http.StatusOK {
		t.Fatalf("512KiB chunk: want 200, got %d %s", w.Code, w.Body.String()[:80])
	}
	over := bytes.Repeat([]byte{7}, 640*1024+1)
	w = postSecret(t, srv, map[string]any{"ct": b64(over), "iv": b64(make([]byte, 12))})
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over MaxBytes: want 413, got %d", w.Code)
	}
}

func TestHealthzExposesMaxBytes(t *testing.T) {
	srv := NewServer(NewMemStore(10), Config{MaxBytes: 4096})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	var out struct {
		MaxBytes int `json:"maxBytes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.MaxBytes != 4096 {
		t.Fatalf("healthz maxBytes: got %d err %v body %s", out.MaxBytes, err, w.Body)
	}
}

func TestLimiterBurstCoversFileUpload(t *testing.T) {
	// rate 30/min sustained must still allow a 17-request burst (16 chunks
	// and a head) plus a second file soon after, via the 2x burst bucket.
	srv := NewServer(NewMemStore(1000), Config{MaxBytes: 4096, RatePerMin: 30})
	ok := 0
	for i := 0; i < 60; i++ {
		w := postSecret(t, srv, map[string]any{"ct": b64([]byte("x")), "iv": b64(make([]byte, 12))})
		if w.Code == http.StatusOK {
			ok++
		}
	}
	if ok < 60 {
		t.Fatalf("burst of 60 (three files back to back) should pass with burst=2x30, got %d", ok)
	}
	w := postSecret(t, srv, map[string]any{"ct": b64([]byte("x")), "iv": b64(make([]byte, 12))})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("61st immediate create should be limited, got %d", w.Code)
	}
}

func TestConsumeIsNotRateLimited(t *testing.T) {
	// downloads are 17 fetches; the limiter applies to creation only, so a
	// file download can never 429
	srv := NewServer(NewMemStore(1000), Config{MaxBytes: 4096, RatePerMin: 1000})
	ids := make([]string, 40)
	for i := range ids {
		w := postSecret(t, srv, map[string]any{"ct": b64([]byte("x")), "iv": b64(make([]byte, 12))})
		var out struct{ ID string }
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		ids[i] = out.ID
	}
	for _, id := range ids {
		req := httptest.NewRequest("GET", "/api/secret/"+id, nil)
		req.RemoteAddr = "10.1.1.1:1"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("consume %s: got %d, downloads must never be limited", id, w.Code)
		}
	}
}
