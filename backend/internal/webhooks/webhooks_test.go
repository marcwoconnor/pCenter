package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moconnor/pcenter/internal/activity"
)

func TestSignRoundTrip(t *testing.T) {
	secret := "dead-beef-feed-face"
	body := []byte(`{"event":"vm.create"}`)
	ts := time.Unix(1_700_000_000, 0)

	sig := Sign(secret, body, ts)

	// Shape: "t=<unix>,v1=<hex>"
	parts := strings.Split(sig, ",")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "t=") || !strings.HasPrefix(parts[1], "v1=") {
		t.Fatalf("unexpected signature shape: %q", sig)
	}

	// Verify by recomputing.
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts.Unix())
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	got := strings.TrimPrefix(parts[1], "v1=")
	if got != want {
		t.Errorf("signature mismatch:\n  got  %s\n  want %s", got, want)
	}
}

func TestSignTamperedBodyFailsVerification(t *testing.T) {
	secret := "s3cr3t"
	ts := time.Now()
	sigA := Sign(secret, []byte(`{"a":1}`), ts)
	sigB := Sign(secret, []byte(`{"a":2}`), ts)
	if sigA == sigB {
		t.Error("signatures should differ for different bodies at same timestamp")
	}
}

func TestMatches(t *testing.T) {
	cases := []struct {
		filter []string
		event  string
		want   bool
	}{
		{nil, "vm.create", true},                               // nil filter = all
		{[]string{}, "vm.create", true},                        // empty filter = all
		{[]string{"vm.create"}, "vm.create", true},             // exact match
		{[]string{"vm.create", "vm.delete"}, "vm.delete", true},
		{[]string{"vm.create"}, "vm.delete", false},            // wrong event
		{[]string{"VM.CREATE"}, "vm.create", true},             // case-insensitive
		{[]string{"ct.start"}, "vm.start", false},              // wrong resource
	}
	for _, tc := range cases {
		if got := matches(tc.filter, tc.event); got != tc.want {
			t.Errorf("matches(%v, %q) = %v, want %v", tc.filter, tc.event, got, tc.want)
		}
	}
}

func TestEventName(t *testing.T) {
	cases := []struct {
		entry activity.Entry
		want  string
	}{
		{activity.Entry{ResourceType: "vm", Action: "vm_create"}, "vm.create"}, // prefix stripped
		{activity.Entry{ResourceType: "ct", Action: "migrate"}, "ct.migrate"},  // no prefix
		{activity.Entry{ResourceType: "folder", Action: "folder_rename"}, "folder.rename"},
		{activity.Entry{ResourceType: "", Action: "config_update"}, "activity.config_update"}, // fallback
		{activity.Entry{ResourceType: "VM", Action: "VM_Create"}, "vm.create"},                // normalisation
	}
	for _, tc := range cases {
		if got := eventName(tc.entry); got != tc.want {
			t.Errorf("eventName(%+v) = %q, want %q", tc.entry, got, tc.want)
		}
	}
}

func TestValidateRequest(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"ok", "https://example.com/hook", false},
		{"ok-http", "http://localhost:9000/h", false},
		{"empty name", "https://example.com/hook", true},
		{"bad scheme", "ftp://example.com/hook", true},
		{"no host", "https:///hook", true},
		{"not a url", "not a url", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := "n"
			if tc.name == "empty name" {
				name = ""
			}
			err := validateRequest(name, tc.url)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateRequest(%q, %q) err=%v, wantErr=%v", name, tc.url, err, tc.wantErr)
			}
		})
	}
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"a", "b", "a", " c ", "", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v, want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("index %d: got %q, want %q", i, got[i], v)
		}
	}
	if dedupe(nil) != nil {
		t.Errorf("dedupe(nil) should be nil")
	}
}

// TestDispatcherRetriesThenSucceeds spins up a fake receiver that fails the
// first two attempts and accepts the third, then checks the dispatcher
// eventually reports success with a verifiable signature.
func TestDispatcherRetriesThenSucceeds(t *testing.T) {
	// Shrink retry delays so the test runs quickly.
	orig := retrySchedule
	retrySchedule = []time.Duration{50 * time.Millisecond, 50 * time.Millisecond, 50 * time.Millisecond}
	t.Cleanup(func() { retrySchedule = orig })

	var attempts int32
	var capturedSig string
	var capturedBody []byte
	secret := "super-secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		capturedSig = r.Header.Get(SignatureHeader)
		capturedBody, _ = io.ReadAll(r.Body)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	db := newTestDB(t)
	t.Cleanup(func() { db.Close() })

	d := NewDispatcher(db)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	d.Start(ctx)

	evt := Event{
		ID:        "test-1",
		Timestamp: time.Unix(1_700_000_000, 0),
		Event:     "webhook.test",
		Data:      map[string]any{"hello": "world"},
	}
	d.Enqueue(job{
		event: evt,
		targets: []dispatchTarget{{
			endpointID:  "eid-1",
			url:         srv.URL,
			secretPlain: secret,
		}},
		queuedAt: time.Now(),
	})

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&attempts) < 3 {
		select {
		case <-deadline:
			t.Fatalf("dispatcher did not reach 3rd attempt; saw %d", atomic.LoadInt32(&attempts))
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Verify captured signature matches what receivers would verify.
	bodyJSON, _ := json.Marshal(evt)
	if string(capturedBody) != string(bodyJSON) {
		t.Errorf("body mismatch: got %q, want %q", capturedBody, bodyJSON)
	}
	expected := Sign(secret, bodyJSON, evt.Timestamp)
	if capturedSig != expected {
		t.Errorf("signature mismatch:\n  got  %s\n  want %s", capturedSig, expected)
	}
}

// TestDispatcherGivesUpAfterAllRetries checks we bail cleanly on persistent failure.
func TestDispatcherGivesUpAfterAllRetries(t *testing.T) {
	orig := retrySchedule
	retrySchedule = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}
	t.Cleanup(func() { retrySchedule = orig })

	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	db := newTestDB(t)
	t.Cleanup(func() { db.Close() })

	d := NewDispatcher(db)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	d.Start(ctx)

	d.Enqueue(job{
		event: Event{ID: "x", Event: "webhook.test", Timestamp: time.Now(), Data: map[string]any{}},
		targets: []dispatchTarget{{
			endpointID:  "eid-2",
			url:         srv.URL,
			secretPlain: "s",
		}},
		queuedAt: time.Now(),
	})

	// Wait for all 1 + len(retrySchedule) attempts.
	want := int32(1 + len(retrySchedule))
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&attempts) < want {
		select {
		case <-deadline:
			t.Fatalf("expected %d attempts, saw %d", want, atomic.LoadInt32(&attempts))
		case <-time.After(5 * time.Millisecond):
		}
	}
	// Let the dispatcher finish its loop (record delivery).
	time.Sleep(50 * time.Millisecond)
}

func TestServiceCRUDRoundTrip(t *testing.T) {
	db := newTestDB(t)
	t.Cleanup(func() { db.Close() })

	svc := NewService(db, nil) // no crypto = plaintext for test

	resp, err := svc.Create(CreateRequest{
		Name:    "test",
		URL:     "https://example.com/h",
		Events:  []string{"vm.create", "vm.create", " "}, // dupes + whitespace
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.Secret == "" || len(resp.Secret) < 32 {
		t.Errorf("expected non-trivial secret, got %q", resp.Secret)
	}
	if len(resp.Endpoint.Events) != 1 || resp.Endpoint.Events[0] != "vm.create" {
		t.Errorf("events not deduped: %v", resp.Endpoint.Events)
	}

	list, err := svc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List: got %d endpoints, err=%v", len(list), err)
	}

	updated, err := svc.Update(resp.Endpoint.ID, UpdateRequest{
		Name: "renamed", URL: "https://example.com/h2", Events: nil, Enabled: false,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "renamed" || updated.Enabled {
		t.Errorf("update didn't stick: %+v", updated)
	}

	if err := svc.Delete(resp.Endpoint.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(resp.Endpoint.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

// newTestDB creates a fresh SQLite file in the test's temp dir.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "webhooks.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}
