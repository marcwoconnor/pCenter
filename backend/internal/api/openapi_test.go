package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moconnor/pcenter/internal/state"
)

func TestServeOpenAPIYAML(t *testing.T) {
	rec := httptest.NewRecorder()
	serveOpenAPIYAML(rec, httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "openapi: 3.0.3") {
		t.Errorf("body missing OpenAPI version line")
	}
	if !strings.Contains(body, "/api/auth/login") {
		t.Errorf("body missing a known path (/api/auth/login)")
	}
}

func TestServeOpenAPIJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	serveOpenAPIJSON(rec, httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var spec struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title string `json:"title"`
		} `json:"info"`
		Paths      map[string]any `json:"paths"`
		Components struct {
			SecuritySchemes map[string]any `json:"securitySchemes"`
			Schemas         map[string]any `json:"schemas"`
		} `json:"components"`
	}
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&spec); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if spec.OpenAPI != "3.0.3" {
		t.Errorf("openapi = %q, want 3.0.3", spec.OpenAPI)
	}
	if spec.Info.Title != "pCenter API" {
		t.Errorf("info.title = %q", spec.Info.Title)
	}
	for _, p := range []string{"/api/auth/login", "/api/clusters", "/api/guests"} {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("paths missing %q", p)
		}
	}
	for _, s := range []string{"sessionCookie", "csrfToken"} {
		if _, ok := spec.Components.SecuritySchemes[s]; !ok {
			t.Errorf("securitySchemes missing %q", s)
		}
	}
	for _, s := range []string{"Error", "LoginRequest", "Guest"} {
		if _, ok := spec.Components.Schemas[s]; !ok {
			t.Errorf("schemas missing %q", s)
		}
	}
}

// TestOpenAPIRoutesWiredUnauthenticated proves the three doc routes are reachable
// without auth via the real NewRouter — the whole point of mounting them before
// the protectedMux block.
func TestOpenAPIRoutesWiredUnauthenticated(t *testing.T) {
	store := state.New()
	hub := NewHub(store, nil)
	handler, _ := NewRouter(store, nil, hub, nil, nil, nil)

	for _, tc := range []struct {
		path        string
		contentType string
	}{
		{"/api/openapi.yaml", "application/yaml"},
		{"/api/openapi.json", "application/json"},
		{"/api/docs", "text/html"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200; body=%s", tc.path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.contentType) {
			t.Errorf("%s: Content-Type = %q, want prefix %q", tc.path, ct, tc.contentType)
		}
	}
}

func TestServeSwaggerUI(t *testing.T) {
	rec := httptest.NewRecorder()
	serveSwaggerUI(rec, httptest.NewRequest(http.MethodGet, "/api/docs", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SwaggerUIBundle") {
		t.Errorf("HTML missing SwaggerUIBundle init")
	}
	if !strings.Contains(body, "/api/openapi.yaml") {
		t.Errorf("HTML missing spec URL")
	}
}
