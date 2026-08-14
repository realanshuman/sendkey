package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	sendkey "github.com/realanshuman/sendkey"
)

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", envOr("SENDKEY_ADDR", ":8080"), "listen address")
	maxItems := fs.Int("max-items", 100_000, "maximum stored secrets, in-memory backend only")
	maxBytes := fs.Int("max-bytes", 128*1024, "maximum ciphertext size in bytes")
	rate := fs.Int("rate", 30, "max secret creations per minute per client IP")
	trustProxy := fs.Bool("trust-proxy", false, "trust X-Forwarded-For for client IP (only behind a proxy you control)")
	_ = fs.Parse(args)

	// Redis is picked up automatically when configured, which is what makes
	// the same binary work behind a load balancer or on serverless; otherwise
	// the process keeps secrets in memory.
	var store sendkey.Backend
	if rs := sendkey.RedisFromEnv(); rs != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := rs.Ping(ctx); err != nil {
			cancel()
			log.Fatalf("redis configured but unreachable: %v", err)
		}
		cancel()
		store = rs
		log.Print("storage: redis")
	} else {
		mem := sendkey.NewMemStore(*maxItems)
		defer mem.StartSweeper(time.Minute)()
		store = mem
		log.Print("storage: in-memory (secrets are lost on restart)")
	}

	handler := sendkey.NewServer(store, sendkey.Config{
		MaxBytes:   *maxBytes,
		RatePerMin: *rate,
		TrustProxy: *trustProxy,
	})

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("sendkey %s listening on %s", version, *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	<-sigc

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
