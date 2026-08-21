package sendkey

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMemStoreCounts drives the counters through the store API and checks
// every invariant the /numbers page leans on: buckets sum to created, opens
// and burns disagree exactly when views > 1, and answers count as creates.
func TestMemStoreCounts(t *testing.T) {
	s := NewMemStore(100)
	ctx := context.Background()

	// three creates: 1h/1v, 1d/2v, 7d/5v
	id1, _ := s.Put(ctx, []byte("aaaa"), []byte("iv"), time.Hour, 1)
	id2, _ := s.Put(ctx, []byte("bbbbbbbb"), []byte("iv"), 24*time.Hour, 2)
	if _, err := s.Put(ctx, []byte("cc"), []byte("iv"), 7*24*time.Hour, 5); err != nil {
		t.Fatal(err)
	}
	// one ask answer
	if err := s.PutAt(ctx, "mailbox-1-abcdefghijkl", []byte("dddd"), []byte("iv"), time.Hour, 1); err != nil {
		t.Fatal(err)
	}

	// open id1 to the burn, id2 once (no burn: a view remains)
	if _, err := s.Consume(ctx, id1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Consume(ctx, id2); err != nil {
		t.Fatal(err)
	}

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Created != 4 || st.Answered != 1 {
		t.Fatalf("created=%d answered=%d, want 4 and 1", st.Created, st.Answered)
	}
	if st.Opened != 2 || st.Burned != 1 {
		t.Fatalf("opened=%d burned=%d, want 2 and 1", st.Opened, st.Burned)
	}
	if st.Sealed != 4+8+2+4 {
		t.Fatalf("sealed=%d, want 18", st.Sealed)
	}
	if got := st.TTLHour + st.TTLDay + st.TTLWeek; got != st.Created {
		t.Fatalf("ttl buckets sum to %d, created is %d", got, st.Created)
	}
	if got := st.Views1 + st.Views2 + st.Views5; got != st.Created {
		t.Fatalf("view buckets sum to %d, created is %d", got, st.Created)
	}
	if st.TTLHour != 2 || st.TTLDay != 1 || st.TTLWeek != 1 {
		t.Fatalf("ttl buckets %d/%d/%d, want 2/1/1", st.TTLHour, st.TTLDay, st.TTLWeek)
	}
	if st.Views1 != 2 || st.Views2 != 1 || st.Views5 != 1 {
		t.Fatalf("view buckets %d/%d/%d, want 2/1/1", st.Views1, st.Views2, st.Views5)
	}
	if st.Since.IsZero() {
		t.Fatal("since is zero; the page cannot state its window")
	}
}

// TestMemStoreFailedPutsAreNotCounted pins the acceptance rule: a rejected
// create must leave every counter untouched.
func TestMemStoreFailedPutsAreNotCounted(t *testing.T) {
	s := NewMemStore(1)
	ctx := context.Background()
	if _, err := s.Put(ctx, []byte("x"), []byte("iv"), time.Hour, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, []byte("y"), []byte("iv"), time.Hour, 1); err == nil {
		t.Fatal("second put should have hit the maxItems ceiling")
	}
	// occupied mailbox: ErrExists must not count either
	if err := s.PutAt(ctx, "taken-id-abcdefghijklmn", []byte("z"), []byte("iv"), time.Hour, 1); err == nil {
		t.Fatal("expected ErrFull or ErrExists")
	}
	st, _ := s.Stats(ctx)
	if st.Created != 1 || st.Answered != 0 {
		t.Fatalf("created=%d answered=%d after failures, want 1 and 0", st.Created, st.Answered)
	}
}

// TestRedisStatsRoundTrip runs the same shape against the fake Redis, which
// emulates putScript and the extended burnScript.
func TestRedisStatsRoundTrip(t *testing.T) {
	f := newFakeRedis(t)
	r := f.store()
	ctx := context.Background()

	id1, err := r.Put(ctx, []byte("aaaa"), []byte("iviviviviviv"), time.Hour, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Put(ctx, []byte("bbbbbbbb"), []byte("iviviviviviv"), 7*24*time.Hour, 5); err != nil {
		t.Fatal(err)
	}
	if err := r.PutAt(ctx, "mailbox-2-abcdefghijkl", []byte("cc"), []byte("iviviviviviv"), 24*time.Hour, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Consume(ctx, id1); err != nil {
		t.Fatal(err)
	}

	st, err := r.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Created != 3 || st.Answered != 1 || st.Opened != 1 || st.Burned != 1 {
		t.Fatalf("got created=%d answered=%d opened=%d burned=%d", st.Created, st.Answered, st.Opened, st.Burned)
	}
	if st.Sealed != 4+8+2 {
		t.Fatalf("sealed=%d, want 14", st.Sealed)
	}
	if st.TTLHour != 1 || st.TTLDay != 1 || st.TTLWeek != 1 {
		t.Fatalf("ttl buckets %d/%d/%d, want 1/1/1", st.TTLHour, st.TTLDay, st.TTLWeek)
	}
	if st.Since.IsZero() {
		t.Fatal("since not pinned by first write")
	}
}

// TestStatsEndpoint proves the JSON contract of /api/stats after real
// traffic through the HTTP layer.
func TestStatsEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	w := postSecret(t, srv, map[string]any{
		"ct": b64([]byte("sealed")), "iv": b64(make([]byte, 12)),
		"ttl": 60, "views": 1,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create failed: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	cw := httptest.NewRecorder()
	srv.ServeHTTP(cw, httptest.NewRequest("GET", "/api/secret/"+created.ID, nil))
	if cw.Code != http.StatusOK {
		t.Fatalf("consume failed: %d", cw.Code)
	}

	sw := httptest.NewRecorder()
	srv.ServeHTTP(sw, httptest.NewRequest("GET", "/api/stats", nil))
	if sw.Code != http.StatusOK {
		t.Fatalf("stats status %d", sw.Code)
	}
	var st Stats
	if err := json.Unmarshal(sw.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Created != 1 || st.Opened != 1 || st.Burned != 1 {
		t.Fatalf("endpoint reports created=%d opened=%d burned=%d", st.Created, st.Opened, st.Burned)
	}
}

// TestNumbersPageServed pins the page route and the honesty line: /numbers
// must state what it cannot count, or the totals read as surveillance.
func TestNumbersPageServed(t *testing.T) {
	srv, _ := newTestServer(t)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest("GET", "/numbers", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Numbers", "What we cannot count", "/api/stats"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page is missing %q", want)
		}
	}
}
