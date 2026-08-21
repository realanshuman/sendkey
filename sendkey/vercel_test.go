package sendkey

import (
	"encoding/json"
	"os"
	"testing"
)

// TestVercelRewritesCoverEveryRoute pins vercel.json to the mux. On Vercel
// the CDN serves sendkey/public and vercel.json is the router; the Go mux
// only ever sees what the /api rewrite forwards. A route added to the mux
// without a rewrite works locally, passes every browser suite, and 404s in
// production, which is exactly how /numbers shipped broken: path-to-regexp
// gives /api/(.*) no claim on bare /api, so page routes need their own line.
func TestVercelRewritesCoverEveryRoute(t *testing.T) {
	raw, err := os.ReadFile("../vercel.json")
	if err != nil {
		t.Fatalf("vercel.json unreadable: %v", err)
	}
	var cfg struct {
		Rewrites []struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
		} `json:"rewrites"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("vercel.json is not valid JSON: %v", err)
	}
	sources := map[string]string{}
	for _, r := range cfg.Rewrites {
		sources[r.Source] = r.Destination
	}

	// Every non-root path the mux serves, and the vercel.json source that
	// must exist for it. The root needs no line: index.html is the CDN
	// default. New page routes get added here when they get added to Routes.
	want := map[string]string{
		"/send":     "/send.html",
		"/drop":     "/send.html",
		"/ask":      "/a.html",
		"/a/(.*)":   "/a.html",
		"/s/(.*)":   "/s.html",
		"/api":      "/api.html",
		"/numbers":  "/numbers.html",
		"/healthz":  "/api/index",
		"/api/(.*)": "/api/index",
	}
	for src, dst := range want {
		got, ok := sources[src]
		if !ok {
			t.Errorf("vercel.json is missing a rewrite for %q: the page works locally and 404s on Vercel", src)
			continue
		}
		if got != dst {
			t.Errorf("rewrite %q points at %q, want %q", src, got, dst)
		}
	}

	// The order trap: /api must be rewritten to the docs page, never
	// swallowed by a function rule that would shadow it if patterns change.
	if sources["/api"] != "/api.html" {
		t.Error("/api must serve the docs page, not the function")
	}
}
