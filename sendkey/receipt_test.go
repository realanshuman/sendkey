package sendkey

// Tests for live burn receipts: a consumed secret leaves a
// ciphertext-stripped tombstone that answers Receipts until its original TTL
// passes, while every recipient-facing path keeps seeing a burned secret as
// gone.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMemReceiptRecordsOpen(t *testing.T) {
	s := NewMemStore(10)
	at := time.Date(2026, 3, 4, 15, 4, 5, 0, time.UTC)
	s.now = func() time.Time { return at }

	id, _ := s.Put(ctx, []byte("cipher"), make([]byte, IVLen), time.Hour, 1)

	opens, err := s.Receipts(ctx, id)
	if err != nil || len(opens) != 0 {
		t.Fatalf("unopened secret: want empty receipt, got %v err %v", opens, err)
	}

	if _, err := s.Consume(ctx, id); err != nil {
		t.Fatalf("consume: %v", err)
	}

	opens, err = s.Receipts(ctx, id)
	if err != nil {
		t.Fatalf("receipts after burn: %v", err)
	}
	if len(opens) != 1 || !opens[0].Equal(at) {
		t.Fatalf("want one open at %v, got %v", at, opens)
	}
}

func TestMemReceiptOutlivesTheBurn(t *testing.T) {
	s := NewMemStore(10)
	id, _ := s.Put(ctx, []byte("cipher"), make([]byte, IVLen), time.Hour, 1)
	_, _ = s.Consume(ctx, id)

	// Recipient-facing paths must not see the tombstone at all.
	if _, err := s.Peek(ctx, id); err != ErrNotFound {
		t.Errorf("peek: want ErrNotFound, got %v", err)
	}
	if _, err := s.Consume(ctx, id); err != ErrNotFound {
		t.Errorf("consume: want ErrNotFound, got %v", err)
	}
	// The sender's path still answers.
	if _, err := s.Receipts(ctx, id); err != nil {
		t.Errorf("receipts: want the tombstone to answer, got %v", err)
	}
}

func TestMemReceiptMultiViewAccumulates(t *testing.T) {
	s := NewMemStore(10)
	now := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	id, _ := s.Put(ctx, []byte("x"), make([]byte, IVLen), time.Hour, 3)
	for i := 0; i < 3; i++ {
		now = now.Add(time.Minute)
		if _, err := s.Consume(ctx, id); err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
	}

	opens, err := s.Receipts(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(opens) != 3 {
		t.Fatalf("want 3 opens, got %d", len(opens))
	}
	for i := 1; i < len(opens); i++ {
		if !opens[i].After(opens[i-1]) {
			t.Errorf("opens must be in order: %v then %v", opens[i-1], opens[i])
		}
	}
}

func TestMemReceiptExpiresWithTheSecret(t *testing.T) {
	s := NewMemStore(10)
	now := time.Now()
	s.now = func() time.Time { return now }

	id, _ := s.Put(ctx, []byte("x"), make([]byte, IVLen), time.Minute, 1)
	_, _ = s.Consume(ctx, id)
	if _, err := s.Receipts(ctx, id); err != nil {
		t.Fatalf("receipt before expiry: %v", err)
	}

	now = now.Add(2 * time.Minute) // past the original TTL
	if _, err := s.Receipts(ctx, id); err != ErrNotFound {
		t.Fatalf("receipt after expiry: want ErrNotFound, got %v", err)
	}
}

func TestRedisReceiptRoundTrip(t *testing.T) {
	f := newFakeRedis(t)
	rs := f.store()

	id, _ := rs.Put(ctx, []byte("cipher"), make([]byte, IVLen), time.Hour, 2)

	opens, err := rs.Receipts(ctx, id)
	if err != nil || len(opens) != 0 {
		t.Fatalf("unopened: want empty receipt, got %v err %v", opens, err)
	}

	before := time.Now().Add(-time.Second)
	if _, err := rs.Consume(ctx, id); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := rs.Consume(ctx, id); err != nil {
		t.Fatalf("second consume: %v", err)
	}

	opens, err = rs.Receipts(ctx, id)
	if err != nil {
		t.Fatalf("receipts: %v", err)
	}
	if len(opens) != 2 {
		t.Fatalf("want 2 opens, got %d", len(opens))
	}
	if opens[0].Before(before) {
		t.Errorf("timestamp looks wrong: %v", opens[0])
	}
	// Burned to the recipient, readable to the sender.
	if _, err := rs.Peek(ctx, id); err != ErrNotFound {
		t.Errorf("peek after burn: want ErrNotFound, got %v", err)
	}
}

func TestRedisReceiptMissingKey(t *testing.T) {
	rs := newFakeRedis(t).store()
	if _, err := rs.Receipts(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func getReceipt(t *testing.T, srv *Server, id string) (int, []string) {
	t.Helper()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/secret/"+id+"/receipt", nil))
	var out struct {
		Opens []string `json:"opens"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out.Opens
}

func TestReceiptEndpoint(t *testing.T) {
	srv := NewServer(NewMemStore(100), Config{MaxBytes: 4096, RatePerMin: 1000})
	id := createSecret(t, srv, []byte("watch me"))

	code, opens := getReceipt(t, srv, id)
	if code != http.StatusOK || len(opens) != 0 {
		t.Fatalf("before opening: want 200 with no opens, got %d %v", code, opens)
	}

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/api/secret/"+id, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("consume: %d", w.Code)
	}

	code, opens = getReceipt(t, srv, id)
	if code != http.StatusOK || len(opens) != 1 {
		t.Fatalf("after opening: want 200 with one open, got %d %v", code, opens)
	}
	if _, err := time.Parse(time.RFC3339, opens[0]); err != nil {
		t.Errorf("timestamp %q is not RFC3339: %v", opens[0], err)
	}

	if code, _ := getReceipt(t, srv, "neverExisted"); code != http.StatusNotFound {
		t.Errorf("unknown id: want 404, got %d", code)
	}
}

func TestReceiptIsNotRateLimited(t *testing.T) {
	// The sender's page polls this endpoint every few seconds for as long as
	// the tab is open; the limiter covers creation only.
	srv := NewServer(NewMemStore(100), Config{MaxBytes: 4096, RatePerMin: 1})
	id := createSecret(t, srv, []byte("poll me"))

	for i := 0; i < 40; i++ {
		if code, _ := getReceipt(t, srv, id); code != http.StatusOK {
			t.Fatalf("poll %d: want 200, got %d", i, code)
		}
	}
}
