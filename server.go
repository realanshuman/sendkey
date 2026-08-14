// Package sendkey implements a zero-knowledge, burn-after-reading secret
// sharing service. Secrets are sealed by the client; the server only ever
// holds ciphertext it cannot read, and destroys it on first retrieval.
package sendkey

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed all:public
var webFS embed.FS

// Config holds tunable server limits.
type Config struct {
	MaxBytes   int  // maximum ciphertext size accepted
	RatePerMin int  // secret creations per minute per client
	TrustProxy bool // honor X-Forwarded-For for client IP
}

// Server wires the HTTP routes, static assets and rate limiter together.
type Server struct {
	store   Backend
	cfg     Config
	assets  fs.FS
	limiter *rateLimiter
	mux     *http.ServeMux
}

// NewServer builds a ready-to-serve handler over any Backend.
func NewServer(store Backend, cfg Config) *Server {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 128 * 1024
	}
	assets, err := fs.Sub(webFS, "public")
	if err != nil {
		panic(err) // embedded FS is known at build time
	}

	s := &Server{
		store:   store,
		cfg:     cfg,
		assets:  assets,
		limiter: newRateLimiter(cfg.RatePerMin),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /s/{id}", s.handleViewPage)
	mux.HandleFunc("GET /assets/{path...}", s.handleAssets)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /api/secret", s.handleCreate)
	mux.HandleFunc("GET /api/secret/{id}/meta", s.handleMeta)
	mux.HandleFunc("GET /api/secret/{id}", s.handleConsume)
	s.mux = mux
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	s.mux.ServeHTTP(w, r)
}

// --- page + asset handlers ---------------------------------------------------

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.serveAsset(w, "index.html")
}

func (s *Server) handleViewPage(w http.ResponseWriter, r *http.Request) {
	// The id in the path is only a lookup handle; decryption is entirely
	// client-side, so one static page serves every secret.
	s.serveAsset(w, "s.html")
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if p == "" || strings.Contains(p, "..") {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	s.serveAsset(w, "assets/"+p)
}

func (s *Server) serveAsset(w http.ResponseWriter, name string) {
	data, err := fs.ReadFile(s.assets, name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType(name))
	_, _ = w.Write(data)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"ok": true}
	if m, isMem := s.store.(*MemStore); isMem {
		out["secrets"] = m.Len()
	}
	writeJSON(w, http.StatusOK, out)
}

// --- API handlers ------------------------------------------------------------

// CreateRequest is the body accepted by POST /api/secret. Only ciphertext
// crosses the wire; the key stays with the sender.
type CreateRequest struct {
	CT    string `json:"ct"`    // base64url ciphertext
	IV    string `json:"iv"`    // base64url 12-byte nonce
	TTL   int    `json:"ttl"`   // seconds
	Views int    `json:"views"` // reads before burn
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.allow(s.clientIP(r)) {
		writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded, slow down")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(s.cfg.MaxBytes)*2+4096)
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ct, err1 := base64.RawURLEncoding.DecodeString(req.CT)
	iv, err2 := base64.RawURLEncoding.DecodeString(req.IV)
	if err1 != nil || err2 != nil || len(ct) == 0 || len(iv) != IVLen {
		writeJSONError(w, http.StatusBadRequest, "invalid ciphertext or iv")
		return
	}
	if len(ct) > s.cfg.MaxBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "secret too large")
		return
	}

	id, err := s.store.Put(r.Context(), ct, iv, ClampTTL(req.TTL), ClampViews(req.Views))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"id": id})
	case err == ErrFull:
		writeJSONError(w, http.StatusInsufficientStorage, "server at capacity, try again later")
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal error")
	}
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	meta, err := s.store.Peek(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "this secret has expired or already been viewed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"exists":    true,
		"expiresAt": meta.ExpiresAt.UTC().Format(time.RFC3339),
		"views":     meta.Views,
	})
}

func (s *Server) handleConsume(w http.ResponseWriter, r *http.Request) {
	sec, err := s.store.Consume(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "this secret has expired or already been viewed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ct":        base64.RawURLEncoding.EncodeToString(sec.CT),
		"iv":        base64.RawURLEncoding.EncodeToString(sec.IV),
		"viewsLeft": sec.Views,
	})
}

// --- helpers -----------------------------------------------------------------

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ClampTTL bounds a requested lifetime to [1m, 7d], defaulting to 24h.
func ClampTTL(sec int) time.Duration {
	const def, minS, maxS = 24 * 3600, 60, 7 * 24 * 3600
	if sec <= 0 {
		sec = def
	}
	if sec < minS {
		sec = minS
	}
	if sec > maxS {
		sec = maxS
	}
	return time.Duration(sec) * time.Second
}

// ClampViews bounds a requested read count to [1, 10].
func ClampViews(v int) int {
	if v < 1 {
		return 1
	}
	if v > 10 {
		return 10
	}
	return v
}

func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	// Everything is same-origin: no CDNs, no inline scripts, no framing.
	h.Set("Content-Security-Policy",
		"default-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Robots-Tag", "noindex, nofollow")
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".webmanifest"):
		return "application/manifest+json"
	default:
		return "application/octet-stream"
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// --- rate limiter ------------------------------------------------------------

// rateLimiter is a per-key token bucket. Buckets refill continuously and the
// map is swept on a timer so idle clients don't leak memory.
//
// Sweeping is time-gated rather than size-gated: a purely size-gated sweep
// degrades to an O(n) scan on *every* request once the map passes its
// threshold, because a burst of fresh clients has nothing old enough to
// evict. Gating on time amortises the scan to O(n) per sweepEvery instead.
type rateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	rate      float64 // tokens per second
	burst     float64
	lastSweep time.Time
	now       func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

const (
	sweepEvery   = 30 * time.Second // minimum interval between map scans
	sweepAfter   = 4096             // start sweeping past this many buckets
	idleTimeout  = 10 * time.Minute // evict buckets untouched for this long
	hardCapRatio = 4                // above sweepAfter*this, evict aggressively
)

func newRateLimiter(perMin int) *rateLimiter {
	if perMin <= 0 {
		perMin = 30
	}
	return &rateLimiter{
		buckets: make(map[string]*bucket),
		rate:    float64(perMin) / 60.0,
		burst:   float64(perMin),
		now:     time.Now,
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	rl.maybeSweepLocked(now)

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	}
	b.tokens = min(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// maybeSweepLocked evicts stale buckets at most once per sweepEvery.
func (rl *rateLimiter) maybeSweepLocked(now time.Time) {
	if len(rl.buckets) <= sweepAfter || now.Sub(rl.lastSweep) < sweepEvery {
		return
	}
	rl.lastSweep = now

	for k, b := range rl.buckets {
		if now.Sub(b.last) > idleTimeout {
			delete(rl.buckets, k)
		}
	}

	// A bucket refilled to burst is indistinguishable from one that was never
	// created, so dropping it is always safe — it just gets re-made on the
	// next request. That keeps memory bounded under a flood of unique IPs,
	// which the idle sweep alone cannot do.
	if len(rl.buckets) > sweepAfter*hardCapRatio {
		for k, b := range rl.buckets {
			if b.tokens+now.Sub(b.last).Seconds()*rl.rate >= rl.burst {
				delete(rl.buckets, k)
			}
		}
	}
}
