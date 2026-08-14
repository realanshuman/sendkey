package sendkey

import (
	"context"
	"testing"
	"time"
)

var ctx = context.Background()

func TestBurnAfterReading(t *testing.T) {
	s := NewMemStore(10)
	id, err := s.Put(ctx, []byte("cipher"), make([]byte, 12), time.Hour, 1)
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := s.Consume(ctx, id)
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if string(got.CT) != "cipher" {
		t.Fatalf("got ct %q", got.CT)
	}

	if _, err := s.Consume(ctx, id); err != ErrNotFound {
		t.Fatalf("second consume: want ErrNotFound, got %v", err)
	}
	if _, err := s.Peek(ctx, id); err != ErrNotFound {
		t.Fatalf("peek after burn: want ErrNotFound, got %v", err)
	}
	// The record lives on as a receipt tombstone until its TTL, but the
	// payload is gone: burned means unrecoverable, not merely unreadable.
	s.mu.Lock()
	tomb := s.items[id]
	s.mu.Unlock()
	if tomb == nil {
		t.Fatal("receipt tombstone should outlive the burn")
	}
	if tomb.CT != nil || tomb.IV != nil {
		t.Fatal("burned tombstone must not retain ciphertext")
	}
}

func TestMultiViewCountsDown(t *testing.T) {
	s := NewMemStore(10)
	id, _ := s.Put(ctx, []byte("x"), make([]byte, 12), time.Hour, 3)

	for i := 3; i >= 1; i-- {
		got, err := s.Consume(ctx, id)
		if err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
		if got.Views != i-1 {
			t.Fatalf("viewsLeft: want %d got %d", i-1, got.Views)
		}
	}
	if _, err := s.Consume(ctx, id); err != ErrNotFound {
		t.Fatalf("want ErrNotFound after exhausting views, got %v", err)
	}
}

func TestExpiry(t *testing.T) {
	s := NewMemStore(10)
	now := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return now }

	id, _ := s.Put(ctx, []byte("x"), make([]byte, 12), 60*time.Second, 5)

	if _, err := s.Peek(ctx, id); err != nil {
		t.Fatalf("peek before expiry: %v", err)
	}

	now = now.Add(61 * time.Second)
	if _, err := s.Peek(ctx, id); err != ErrNotFound {
		t.Fatalf("peek after expiry: want ErrNotFound, got %v", err)
	}
	if _, err := s.Consume(ctx, id); err != ErrNotFound {
		t.Fatalf("consume after expiry: want ErrNotFound, got %v", err)
	}
}

func TestPeekIsNonDestructive(t *testing.T) {
	s := NewMemStore(10)
	id, _ := s.Put(ctx, []byte("x"), make([]byte, 12), time.Hour, 1)

	for i := 0; i < 5; i++ {
		if _, err := s.Peek(ctx, id); err != nil {
			t.Fatalf("peek %d: %v", i, err)
		}
	}
	if _, err := s.Consume(ctx, id); err != nil {
		t.Fatalf("consume after peeks should still work: %v", err)
	}
}

func TestFull(t *testing.T) {
	s := NewMemStore(2)
	if _, err := s.Put(ctx, []byte("a"), make([]byte, 12), time.Hour, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, []byte("b"), make([]byte, 12), time.Hour, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, []byte("c"), make([]byte, 12), time.Hour, 1); err != ErrFull {
		t.Fatalf("want ErrFull, got %v", err)
	}
}

func TestFullReclaimsExpired(t *testing.T) {
	s := NewMemStore(1)
	now := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return now }

	if _, err := s.Put(ctx, []byte("a"), make([]byte, 12), 30*time.Second, 1); err != nil {
		t.Fatal(err)
	}
	now = now.Add(31 * time.Second) // first entry now expired
	if _, err := s.Put(ctx, []byte("b"), make([]byte, 12), time.Hour, 1); err != nil {
		t.Fatalf("put should reclaim expired slot: %v", err)
	}
}

func TestIDsAreUniqueAndURLSafe(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
		for _, c := range id {
			ok := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
			if !ok {
				t.Fatalf("id %q contains non-URL-safe char %q", id, c)
			}
		}
	}
}
