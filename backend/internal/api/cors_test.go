package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCORSMiddleware_EmptyOrigins_DeniesAllCrossOrigin verifies that when no
// origins are configured (the default), ALL cross-origin requests are blocked.
// This was the critical bug: previously, empty origins = allow everything.
func TestCORSMiddleware_EmptyOrigins_DeniesAllCrossOrigin(t *testing.T) {
	handler := CORSMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/summary", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("empty origins should NOT set Access-Control-Allow-Origin, got %q",
			rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

// TestCORSMiddleware_EmptyOrigins_AllowsSameOrigin verifies that same-origin
// requests (no Origin header) pass through even with no configured origins.
func TestCORSMiddleware_EmptyOrigins_AllowsSameOrigin(t *testing.T) {
	called := false
	handler := CORSMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/summary", nil)
	// No Origin header = same-origin request
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("same-origin request should pass through to handler")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// TestCORSMiddleware_ConfiguredOrigin_Allowed verifies explicitly configured
// origins get proper CORS headers.
func TestCORSMiddleware_ConfiguredOrigin_Allowed(t *testing.T) {
	origins := []string{"https://pcenter.local:3000", "https://dev.local:5173"}
	handler := CORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/summary", nil)
	req.Header.Set("Origin", "https://pcenter.local:3000")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	got := rr.Header().Get("Access-Control-Allow-Origin")
	if got != "https://pcenter.local:3000" {
		t.Errorf("expected origin 'https://pcenter.local:3000', got %q", got)
	}
	if rr.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("expected Allow-Credentials: true")
	}
}

// TestCORSMiddleware_ConfiguredOrigin_RejectsUnlisted verifies that origins
// NOT in the configured list are denied CORS headers.
func TestCORSMiddleware_ConfiguredOrigin_RejectsUnlisted(t *testing.T) {
	origins := []string{"https://pcenter.local:3000"}
	handler := CORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/summary", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("unlisted origin should NOT get CORS headers, got %q",
			rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

// TestCORSMiddleware_PreflightOptions verifies OPTIONS preflight returns
// 204 with no body.
func TestCORSMiddleware_PreflightOptions(t *testing.T) {
	origins := []string{"https://pcenter.local:3000"}
	called := false
	handler := CORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("OPTIONS", "/api/summary", nil)
	req.Header.Set("Origin", "https://pcenter.local:3000")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("preflight should return 204, got %d", rr.Code)
	}
	if called {
		t.Error("preflight should NOT call the next handler")
	}
}

// TestCORSMiddleware_NoHeadersOnDeniedOrigin verifies that when a cross-origin
// request is denied, we don't leak any CORS headers at all.
func TestCORSMiddleware_NoHeadersOnDeniedOrigin(t *testing.T) {
	origins := []string{"https://good.example.com"}
	handler := CORSMiddleware(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/summary", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	corsHeaders := []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Credentials",
	}
	for _, h := range corsHeaders {
		if v := rr.Header().Get(h); v != "" {
			t.Errorf("denied origin should NOT have %s header, got %q", h, v)
		}
	}
}
