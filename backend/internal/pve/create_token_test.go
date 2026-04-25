package pve

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestCreateAPIToken_ReusesValidExistingSecret covers issue #59: when the
// caller knows a previously-stored secret, CreateAPIToken should probe before
// touching token.cfg. A successful probe means the secret is still good — and
// recreating would invalidate every sibling host in the same PVE cluster
// (token.cfg lives in /etc/pve, which is shared via pmxcfs).
func TestCreateAPIToken_ReusesValidExistingSecret(t *testing.T) {
	const wantSecret = "abc123-still-valid"
	var probedAuth string
	var createCalled, deleteCalled atomic.Bool

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api2/json/version":
			probedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"version":"8.2.4","release":"8.2"}}`))
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api2/json/access/users/"):
			createCalled.Store(true)
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/api2/json/access/users/"):
			deleteCalled.Store(true)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "https://")
	auth := &AuthResult{Username: "root@pam", Ticket: "t", CSRFToken: "c"}

	tok, err := CreateAPIToken(context.Background(), addr, auth, "pcenter", wantSecret, true)
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if tok.Secret != wantSecret {
		t.Errorf("Secret = %q, want %q", tok.Secret, wantSecret)
	}
	if tok.TokenID != "root@pam!pcenter" {
		t.Errorf("TokenID = %q, want root@pam!pcenter", tok.TokenID)
	}
	wantHeader := fmt.Sprintf("PVEAPIToken=root@pam!pcenter=%s", wantSecret)
	if probedAuth != wantHeader {
		t.Errorf("probe Authorization = %q, want %q", probedAuth, wantHeader)
	}
	if createCalled.Load() {
		t.Error("create endpoint called despite valid existing secret — would invalidate sibling hosts")
	}
	if deleteCalled.Load() {
		t.Error("delete endpoint called despite valid existing secret")
	}
}

// TestCreateAPIToken_RecreatesWhenProbeFails verifies that a stale stored
// secret does NOT block recreation: probe → 401, fall through to create.
func TestCreateAPIToken_RecreatesWhenProbeFails(t *testing.T) {
	var createCalled atomic.Bool

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api2/json/version":
			w.WriteHeader(http.StatusUnauthorized)
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api2/json/access/users/"):
			createCalled.Store(true)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"full-tokenid":"root@pam!pcenter","value":"fresh-secret"}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "https://")
	auth := &AuthResult{Username: "root@pam", Ticket: "t", CSRFToken: "c"}

	tok, err := CreateAPIToken(context.Background(), addr, auth, "pcenter", "stale-secret", true)
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if !createCalled.Load() {
		t.Fatal("expected create endpoint to be called when probe failed")
	}
	if tok.Secret != "fresh-secret" {
		t.Errorf("Secret = %q, want fresh-secret", tok.Secret)
	}
}

// TestCreateAPIToken_NoExistingSecretSkipsProbe verifies that a first-time
// add path (no stored secret) goes straight to create without a /version probe.
func TestCreateAPIToken_NoExistingSecretSkipsProbe(t *testing.T) {
	var probeCalled, createCalled atomic.Bool

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api2/json/version":
			probeCalled.Store(true)
			w.WriteHeader(http.StatusOK)
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api2/json/access/users/"):
			createCalled.Store(true)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"full-tokenid":"root@pam!pcenter","value":"new-secret"}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "https://")
	auth := &AuthResult{Username: "root@pam", Ticket: "t", CSRFToken: "c"}

	if _, err := CreateAPIToken(context.Background(), addr, auth, "pcenter", "", true); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if probeCalled.Load() {
		t.Error("probe should be skipped when no existing secret is supplied")
	}
	if !createCalled.Load() {
		t.Error("create endpoint should be called for first-time add")
	}
}
