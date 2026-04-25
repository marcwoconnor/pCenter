package pve

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClusterCreate_PostsExpectedParams(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":"UPID:pve01:1234:ABCD:CLUSTER_CREATE::root@pam:"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv, "pve01")
	upid, err := c.ClusterCreate(context.Background(), ClusterCreateOptions{
		ClusterName: "homelab1",
		NodeID:      1,
		Link0:       "10.0.0.11",
	})
	if err != nil {
		t.Fatalf("ClusterCreate: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("want UPID, got %q", upid)
	}
	if gotPath != "/api2/json/cluster/config" {
		t.Errorf("path = %q, want /api2/json/cluster/config", gotPath)
	}
	form, _ := url.ParseQuery(gotBody)
	if got := form.Get("clustername"); got != "homelab1" {
		t.Errorf("clustername = %q, want homelab1", got)
	}
	if got := form.Get("nodeid"); got != "1" {
		t.Errorf("nodeid = %q, want 1", got)
	}
	if got := form.Get("link0"); got != "10.0.0.11" {
		t.Errorf("link0 = %q, want 10.0.0.11", got)
	}
}

func TestClusterCreate_RejectsEmptyName(t *testing.T) {
	c := newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not hit the network with empty name")
	})), "pve01")
	_, err := c.ClusterCreate(context.Background(), ClusterCreateOptions{})
	if err == nil {
		t.Fatal("expected error for empty cluster name")
	}
}

func TestGetClusterJoinInfo_Unmarshals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/cluster/config/join" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{
		  "ipAddress": "10.0.0.11",
		  "fingerprint": "AB:CD:EF:01:23:45",
		  "totem": {"cluster_name":"homelab1","version":"2"},
		  "config_digest": "deadbeef",
		  "nodelist": [
		    {"name":"pve01","nodeid":"1","quorum_votes":"1","ring0_addr":"10.0.0.11"}
		  ]
		}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv, "pve01")
	info, err := c.GetClusterJoinInfo(context.Background())
	if err != nil {
		t.Fatalf("GetClusterJoinInfo: %v", err)
	}
	if info.IPAddress != "10.0.0.11" {
		t.Errorf("IPAddress = %q, want 10.0.0.11", info.IPAddress)
	}
	if info.Fingerprint != "AB:CD:EF:01:23:45" {
		t.Errorf("Fingerprint = %q", info.Fingerprint)
	}
	if len(info.Nodelist) != 1 || info.Nodelist[0].Name != "pve01" {
		t.Errorf("Nodelist = %+v", info.Nodelist)
	}
}

// TestGetClusterJoinInfoWithPassword_PerNodeFingerprint covers PVE 8+ shape
// where ipAddress/fingerprint live on each nodelist entry as ring0_addr/pve_fp
// instead of at the top level. We expect the helper to lift them onto the
// top-level fields based on `preferred_node`.
func TestGetClusterJoinInfoWithPassword_PerNodeFingerprint(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{
		  "config_digest": "abc123",
		  "preferred_node": "pmnode01",
		  "totem": {"cluster_name":"CL1","version":"2"},
		  "nodelist": [
		    {"name":"pmnode01","nodeid":"1","quorum_votes":"1","ring0_addr":"10.0.0.11","pve_fp":"AA:BB:CC:DD"}
		  ]
		}}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	auth := &AuthResult{Ticket: "x", CSRFToken: "y", Username: "root@pam"}
	info, err := GetClusterJoinInfoWithPassword(context.Background(), addr, auth, "pmnode01", true)
	if err != nil {
		t.Fatalf("GetClusterJoinInfoWithPassword: %v", err)
	}
	if info.Fingerprint != "AA:BB:CC:DD" {
		t.Errorf("Fingerprint = %q, want AA:BB:CC:DD (lifted from nodelist[].pve_fp)", info.Fingerprint)
	}
	if info.IPAddress != "10.0.0.11" {
		t.Errorf("IPAddress = %q, want 10.0.0.11 (lifted from nodelist[].ring0_addr)", info.IPAddress)
	}
}

// TestGetClusterJoinInfoWithPassword_NoFingerprintAnywhere covers the failure
// path so we surface a useful error.
func TestGetClusterJoinInfoWithPassword_NoFingerprintAnywhere(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{
		  "nodelist": [{"name":"pmnode01","ring0_addr":"10.0.0.11"}]
		}}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")
	auth := &AuthResult{Ticket: "x", CSRFToken: "y"}
	_, err := GetClusterJoinInfoWithPassword(context.Background(), addr, auth, "pmnode01", true)
	if err == nil {
		t.Fatal("expected error when no fingerprint is present")
	}
	if !strings.Contains(err.Error(), "without a fingerprint") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestClusterJoin_PasswordAuth verifies the package-level ClusterJoin sends
// cookie+CSRF (not a token auth header) and submits the expected form fields.
func TestClusterJoin_PasswordAuth(t *testing.T) {
	var (
		gotAuthHeader string
		gotCSRF       string
		gotCookie     string
		gotBody       string
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/cluster/config/join" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotAuthHeader = r.Header.Get("Authorization")
		gotCSRF = r.Header.Get("CSRFPreventionToken")
		if cookie, err := r.Cookie("PVEAuthCookie"); err == nil {
			gotCookie = cookie.Value
		}
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":"UPID:pve02:1:ABCD:CLUSTER_JOIN::root@pam:"}`))
	}))
	defer srv.Close()

	// Extract host:port for the `address` arg.
	addr := strings.TrimPrefix(srv.URL, "https://")

	auth := &AuthResult{
		Ticket:    "PVE:root@pam:DEADBEEF",
		CSRFToken: "csrf-abc",
		Username:  "root@pam",
	}
	upid, err := ClusterJoin(context.Background(), addr, auth, ClusterJoinRequest{
		Hostname:    "10.0.0.11",
		Fingerprint: "AB:CD:EF",
		Password:    "s3cret",
		Link0:       "10.0.0.12",
	}, true /* insecure */)
	if err != nil {
		t.Fatalf("ClusterJoin: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("want UPID, got %q", upid)
	}
	// Must not send the API-token Authorization header (PVE rejects it here).
	if gotAuthHeader != "" {
		t.Errorf("should not send Authorization header, got %q", gotAuthHeader)
	}
	if gotCSRF != "csrf-abc" {
		t.Errorf("CSRF header = %q, want csrf-abc", gotCSRF)
	}
	if gotCookie != "PVE:root@pam:DEADBEEF" {
		t.Errorf("cookie = %q, want ticket", gotCookie)
	}
	form, _ := url.ParseQuery(gotBody)
	for _, pair := range []struct{ k, want string }{
		{"hostname", "10.0.0.11"},
		{"fingerprint", "AB:CD:EF"},
		{"password", "s3cret"},
		{"link0", "10.0.0.12"},
	} {
		if got := form.Get(pair.k); got != pair.want {
			t.Errorf("form[%s] = %q, want %q", pair.k, got, pair.want)
		}
	}
}

// TestClusterJoin_NullDataUPID exercises the PVE-version quirk where join
// returns {"data":null} and the caller must accept an empty UPID.
func TestClusterJoin_NullDataUPID(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	upid, err := ClusterJoin(context.Background(), addr, &AuthResult{CSRFToken: "x", Ticket: "y"},
		ClusterJoinRequest{Hostname: "10.0.0.11", Fingerprint: "aa:bb", Password: "p"}, true)
	if err != nil {
		t.Fatalf("ClusterJoin: %v", err)
	}
	if upid != "" {
		t.Errorf("want empty UPID, got %q", upid)
	}
}

func TestClusterJoin_HTTPError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":{"password":"authentication failure"}}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	_, err := ClusterJoin(context.Background(), addr, &AuthResult{CSRFToken: "x", Ticket: "y"},
		ClusterJoinRequest{Hostname: "h", Fingerprint: "f", Password: "p"}, true)
	if err == nil {
		t.Fatal("expected error on HTTP 400")
	}
	if !strings.Contains(err.Error(), "authentication failure") {
		t.Errorf("error should include server body, got: %v", err)
	}
}

func TestClusterJoin_ValidatesInputs(t *testing.T) {
	auth := &AuthResult{CSRFToken: "x", Ticket: "y"}
	cases := []struct {
		name string
		req  ClusterJoinRequest
	}{
		{"missing hostname", ClusterJoinRequest{Fingerprint: "f", Password: "p"}},
		{"missing fingerprint", ClusterJoinRequest{Hostname: "h", Password: "p"}},
		{"missing password", ClusterJoinRequest{Hostname: "h", Fingerprint: "f"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ClusterJoin(context.Background(), "127.0.0.1:8006", auth, tc.req, true)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
	// nil auth
	_, err := ClusterJoin(context.Background(), "127.0.0.1:8006", nil,
		ClusterJoinRequest{Hostname: "h", Fingerprint: "f", Password: "p"}, true)
	if err == nil {
		t.Error("expected error for nil auth")
	}
}

// TestWaitForTask_Success stops after 3 polls when task transitions to stopped/OK.
func TestWaitForTask_Success(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			_, _ = w.Write([]byte(`{"data":{"upid":"UPID:x","status":"running"}}`))
		} else {
			_, _ = w.Write([]byte(`{"data":{"upid":"UPID:x","status":"stopped","exitstatus":"OK"}}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv, "pve01")
	task, err := c.WaitForTask(context.Background(), "UPID:x", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForTask: %v", err)
	}
	if task.ExitCode != "OK" {
		t.Errorf("ExitCode = %q, want OK", task.ExitCode)
	}
	if got := calls.Load(); got < 3 {
		t.Errorf("expected ≥3 polls, got %d", got)
	}
}

// TestWaitForTask_Failure reports non-OK exit with log tail.
func TestWaitForTask_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/log") {
			_, _ = w.Write([]byte(`{"data":[{"n":1,"t":"starting cluster create"},{"n":2,"t":"ERROR: cluster already exists"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"upid":"UPID:x","status":"stopped","exitstatus":"cluster already exists"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv, "pve01")
	_, err := c.WaitForTask(context.Background(), "UPID:x", 5*time.Millisecond)
	if err == nil {
		t.Fatal("expected error on non-OK exit")
	}
	if !strings.Contains(err.Error(), "cluster already exists") {
		t.Errorf("error should carry task log, got: %v", err)
	}
}

func TestWaitForTask_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"upid":"UPID:x","status":"running"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv, "pve01")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := c.WaitForTask(ctx, "UPID:x", 5*time.Millisecond)
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

// TestWaitForTask_ValidatesInputs guards the "empty UPID" path.
func TestWaitForTask_ValidatesInputs(t *testing.T) {
	c := newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})), "pve01")
	if _, err := c.WaitForTask(context.Background(), "", time.Second); err == nil {
		t.Error("expected error for empty UPID")
	}
}

// sanity: ensure ClusterJoin's test server's insecure=true path actually works
// by reusing a self-signed TLS server and verifying our transport handshakes.
func TestClusterJoin_InsecureTransport(t *testing.T) {
	// A plain HTTPS server with a self-signed cert — would fail without insecure=true.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer srv.Close()

	// Sanity check: without insecure, a stock client would fail TLS verification.
	// We don't repeat the test here; just assert httptest.NewTLSServer is reachable
	// via our code path.
	addr := strings.TrimPrefix(srv.URL, "https://")
	_, err := ClusterJoin(context.Background(), addr,
		&AuthResult{CSRFToken: "x", Ticket: "y"},
		ClusterJoinRequest{Hostname: "h", Fingerprint: "f", Password: "p"},
		true)
	if err != nil {
		t.Fatalf("ClusterJoin over TLS: %v", err)
	}
	// And just to exercise the tls package import (avoid unused import if the
	// file evolves).
	_ = &tls.Config{InsecureSkipVerify: true}
	_ = fmt.Sprint("")
}
