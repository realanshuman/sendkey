// Package handler is the Vercel serverless entry point. Vercel builds every
// file under /api into its own function; the exported Handler receives the
// request after vercel.json rewrites /api/* to this file.
package handler

import (
	"net/http"
	"os"
	"strconv"
	"sync"

	sendkey "github.com/realanshuman/sendkey"
)

var (
	once sync.Once
	srv  *sendkey.Server
	// initErr is reported per-request rather than panicking at import time,
	// so a misconfigured deployment returns a readable 503 instead of an
	// opaque function crash.
	initErr string
)

func build() {
	// Serverless functions do not share memory between invocations, so an
	// in-process store would lose every secret the moment the instance
	// rotated. Redis is required here, not optional.
	store := sendkey.RedisFromEnv()
	if store == nil {
		initErr = "storage is not configured: set KV_REST_API_URL and KV_REST_API_TOKEN " +
			"(or UPSTASH_REDIS_REST_URL / UPSTASH_REDIS_REST_TOKEN)"
		return
	}

	srv = sendkey.NewServer(store, sendkey.Config{
		MaxBytes:   envInt("SENDKEY_MAX_BYTES", 128*1024),
		RatePerMin: envInt("SENDKEY_RATE", 30),
		// Vercel terminates TLS and always sets X-Forwarded-For, so the
		// client IP is only knowable through it. It is safe to trust here
		// precisely because the platform overwrites any client-supplied value.
		TrustProxy: true,
	})
}

// Handler serves every /api/* route.
func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(build)
	if srv == nil {
		http.Error(w, `{"error":"`+initErr+`"}`, http.StatusServiceUnavailable)
		return
	}
	srv.ServeHTTP(w, r)
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}
