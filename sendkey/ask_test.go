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
