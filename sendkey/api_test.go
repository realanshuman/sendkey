package sendkey

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAPIServer() *Server {
	return NewServer(NewMemStore(100), Config{MaxBytes: 4096, RatePerMin: 1000})
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "10.9.9.9:1"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestAPIPageServed(t *testing.T) {
	w := get(t, newAPIServer(), "/api")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api: got %d, want 200", w.Code)
	}
	body := w.Body.String()
	// The page's whole reason to exist is naming the endpoints, so a page
	// that renders but forgot one is still broken.
	for _, want := range []string{
		"/api/secret", "/api/secret/{id}/meta", "/api/secret/{id}/receipt", "/healthz",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("API page does not document %s", want)
		}
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

// The docs page is a page, not an endpoint: it must not pick up the API's
// cross-origin headers just because its path starts with /api.
func TestAPIPageIsNotCORSOpen(t *testing.T) {
	w := get(t, newAPIServer(), "/api")
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q on the docs page, want none", got)
	}
}

func TestAPIEndpointsAllowAnyOrigin(t *testing.T) {
	srv := newAPIServer()
	w := postSecret(t, srv, map[string]any{"ct": b64([]byte("x")), "iv": b64(make([]byte, 12))})
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("POST /api/secret is not open to other origins")
	}
	var out struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	for _, path := range []string{
		"/api/secret/" + out.ID + "/meta",
		"/api/secret/" + out.ID + "/receipt",
		"/healthz",
		"/api/secret/" + out.ID, // burns, so it goes last
	} {
		if got := get(t, srv, path).Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("%s: Access-Control-Allow-Origin = %q, want *", path, got)
		}
	}
}

// "*" plus credentials is rejected outright by browsers, which would make the
// whole API unusable from a page. There is nothing to send anyway.
func TestCORSNeverAllowsCredentials(t *testing.T) {
	srv := newAPIServer()
	if got := get(t, srv, "/healthz").Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want it unset", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	srv := newAPIServer()
	req := httptest.NewRequest("OPTIONS", "/api/secret", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.RemoteAddr = "10.9.9.9:1"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", w.Code)
	}
	h := w.Header()
	if h.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("preflight did not allow the origin")
	}
	if !strings.Contains(h.Get("Access-Control-Allow-Methods"), "POST") {
		t.Errorf("preflight methods = %q, want POST among them", h.Get("Access-Control-Allow-Methods"))
	}
	// Without this the JSON body's own Content-Type header is refused.
	if !strings.Contains(h.Get("Access-Control-Allow-Headers"), "Content-Type") {
		t.Errorf("preflight headers = %q, want Content-Type", h.Get("Access-Control-Allow-Headers"))
	}
	if w.Body.Len() != 0 {
		t.Errorf("preflight returned a body: %q", w.Body.String())
	}
}

// A preflight is answered by the CORS layer before routing, so it must not
// be able to reach past it to a handler and cause a side effect.
func TestPreflightDoesNotBurn(t *testing.T) {
	srv := newAPIServer()
	w := postSecret(t, srv, map[string]any{"ct": b64([]byte("x")), "iv": b64(make([]byte, 12))})
	var out struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	req := httptest.NewRequest("OPTIONS", "/api/secret/"+out.ID, nil)
	req.RemoteAddr = "10.9.9.9:1"
	srv.ServeHTTP(httptest.NewRecorder(), req)

	if code := get(t, srv, "/api/secret/"+out.ID).Code; code != http.StatusOK {
		t.Fatalf("secret is gone after a preflight: consume got %d, want 200", code)
	}
}

// Pages stay locked down even though the API next to them is wide open.
func TestPagesKeepStrictSecurityHeaders(t *testing.T) {
	srv := newAPIServer()
	for _, path := range []string{"/", "/send", "/ask", "/api"} {
		h := get(t, srv, path).Header()
		if !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'self'") {
			t.Errorf("%s: CSP weakened to %q", path, h.Get("Content-Security-Policy"))
		}
		if h.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: X-Frame-Options = %q, want DENY", path, h.Get("X-Frame-Options"))
		}
	}
}
