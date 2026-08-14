package sendkey

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// Errors returned by storage backends.
var (
	ErrFull     = errors.New("store is full")
	ErrExists   = errors.New("id already occupied")
	ErrNotFound = errors.New("secret not found")
)

// Secret is an opaque, encrypted blob. No backend can decrypt it: the key
// lives only in the recipient's URL fragment and is never transmitted.
type Secret struct {
	CT        []byte      // ciphertext (AES-256-GCM, sealed by the client)
	IV        []byte      // 12-byte nonce for the outer layer
	ExpiresAt time.Time   // hard TTL; reclaimed even if never read
	Views     int         // reads remaining after this one
	Opens     []time.Time // when each read happened, for the sender's receipt
}

// Meta is the non-destructive view of a stored secret.
type Meta struct {
	ExpiresAt time.Time
	Views     int
}

// Backend stores sealed secrets. Consume must be atomic: given N concurrent
// callers and one remaining view, exactly one may succeed. That guarantee is
// the whole product, so every implementation has to provide it.
type Backend interface {
	Put(ctx context.Context, ct, iv []byte, ttl time.Duration, views int) (string, error)
	// PutAt stores under a caller-chosen id with first-write-wins semantics,
	// returning ErrExists if the id is already occupied. It exists for ask
	// mailboxes, whose address both sides derive from the request id.
	PutAt(ctx context.Context, id string, ct, iv []byte, ttl time.Duration, views int) error
	Peek(ctx context.Context, id string) (Meta, error)
	Consume(ctx context.Context, id string) (*Secret, error)
	// Receipts returns when a secret was opened. Unlike Peek it still answers
	// after the last view has burned, because the whole point is to tell the
	// sender their secret was read; a burned record survives as a
	// ciphertext-stripped tombstone until its original TTL passes.
	Receipts(ctx context.Context, id string) ([]time.Time, error)
}

// MemStore is an in-memory, TTL-bounded backend with atomic burn-on-read.
// All access is guarded by one mutex, which is fast enough for this workload
// and makes the burn genuinely atomic.
//
// It is the right backend for a single long-lived process and the wrong one
// for serverless, where each invocation may run on a different instance —
// use RedisStore there.
type MemStore struct {
	mu       sync.Mutex
	items    map[string]*Secret
	maxItems int
	now      func() time.Time // injectable clock for tests
}

var _ Backend = (*MemStore)(nil)

// NewMemStore returns a store bounded to maxItems entries.
func NewMemStore(maxItems int) *MemStore {
	if maxItems <= 0 {
		maxItems = 100_000
	}
	return &MemStore{
		items:    make(map[string]*Secret),
		maxItems: maxItems,
		now:      time.Now,
	}
}

// Put stores a sealed secret and returns its random, unguessable id.
func (s *MemStore) Put(_ context.Context, ct, iv []byte, ttl time.Duration, views int) (string, error) {
	if views < 1 {
		views = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.items) >= s.maxItems {
		s.sweepLocked(s.now()) // reclaim expired entries before giving up
		if len(s.items) >= s.maxItems {
			return "", ErrFull
		}
	}

	id, err := NewID()
	if err != nil {
		return "", err
	}
	for {
		if _, exists := s.items[id]; !exists {
			break
		}
		if id, err = NewID(); err != nil { // astronomically unlikely
			return "", err
		}
	}

	s.items[id] = &Secret{
		CT:        ct,
		IV:        iv,
		ExpiresAt: s.now().Add(ttl),
		Views:     views,
	}
	return id, nil
}

// PutAt stores under id if it is free; ErrExists otherwise. An expired
// occupant does not block the slot.
func (s *MemStore) PutAt(_ context.Context, id string, ct, iv []byte, ttl time.Duration, views int) error {
	if views < 1 {
		views = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.items) >= s.maxItems {
		s.sweepLocked(s.now())
		if len(s.items) >= s.maxItems {
			return ErrFull
		}
	}
	if cur, ok := s.items[id]; ok && cur.ExpiresAt.After(s.now()) {
		return ErrExists
	}
	s.items[id] = &Secret{
		CT:        ct,
		IV:        iv,
		ExpiresAt: s.now().Add(ttl),
		Views:     views,
	}
	return nil
}

// Peek returns metadata without consuming a view, so a recipient (or a link
// preview crawler) can be told a secret is waiting without detonating it.
func (s *MemStore) Peek(_ context.Context, id string) (Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sec, ok := s.items[id]
	// Views <= 0 is a burned tombstone kept only for the sender's receipt.
	// To every recipient-facing caller it must look exactly as gone as a
	// deleted record did before receipts existed.
	if !ok || sec.Views <= 0 || !sec.ExpiresAt.After(s.now()) {
		return Meta{}, ErrNotFound
	}
	return Meta{ExpiresAt: sec.ExpiresAt, Views: sec.Views}, nil
}

// Receipts reports when this secret was opened. It deliberately ignores the
// burn state: the sender is asking about their own secret, and the answer is
// most interesting precisely after it burned.
func (s *MemStore) Receipts(_ context.Context, id string) ([]time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sec, ok := s.items[id]
	if !ok || !sec.ExpiresAt.After(s.now()) {
		return nil, ErrNotFound
	}
	return append([]time.Time(nil), sec.Opens...), nil
}

// Consume atomically returns the secret and decrements its view counter,
// deleting it once no views remain. Two concurrent readers can never both
// succeed on the last view.
func (s *MemStore) Consume(_ context.Context, id string) (*Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sec, ok := s.items[id]
	if !ok || sec.Views <= 0 || !sec.ExpiresAt.After(s.now()) {
		if ok && !sec.ExpiresAt.After(s.now()) {
			delete(s.items, id) // opportunistically reclaim if expired
		}
		return nil, ErrNotFound
	}

	// The open is recorded in the same critical section as the decrement, so
	// a receipt can never disagree with the burn that produced it.
	sec.Opens = append(sec.Opens, s.now())
	sec.Views--
	out := &Secret{CT: sec.CT, IV: sec.IV, ExpiresAt: sec.ExpiresAt, Views: sec.Views}
	if sec.Views <= 0 {
		// Keep the record as a tombstone so the sender can still collect a
		// receipt, but drop the payload: a burned secret must be
		// unrecoverable, and nothing may read ciphertext from here again.
		sec.CT, sec.IV = nil, nil
	}
	return out, nil
}

// Len reports the current number of stored secrets.
func (s *MemStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func (s *MemStore) sweepLocked(now time.Time) {
	for id, sec := range s.items {
		if !sec.ExpiresAt.After(now) {
			delete(s.items, id)
		}
	}
}

// StartSweeper runs a background ticker that evicts expired secrets. It
// returns a stop function that halts the goroutine.
func (s *MemStore) StartSweeper(every time.Duration) (stop func()) {
	t := time.NewTicker(every)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-t.C:
				s.mu.Lock()
				s.sweepLocked(s.now())
				s.mu.Unlock()
			case <-done:
				t.Stop()
				return
			}
		}
	}()
	return func() { close(done) }
}

// NewID returns a 128-bit cryptographically random, URL-safe identifier.
func NewID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
