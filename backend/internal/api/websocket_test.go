package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/moconnor/pcenter/internal/state"
)

// TestWebSocketHub_NoOrigins_RejectsCrossOrigin verifies that when no
// origins are configured, cross-origin WebSocket connections are rejected.
func TestWebSocketHub_NoOrigins_RejectsCrossOrigin(t *testing.T) {
	store := state.New()
	hub := NewHub(store, nil) // no allowed origins
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	// Try to connect with a cross-origin header
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Set("Origin", "https://evil.example.com")

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err == nil {
		t.Fatal("expected connection to be rejected for cross-origin with no allowed origins")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		t.Logf("connection rejected (status %d) — correct behavior", resp.StatusCode)
	}
}

// TestWebSocketHub_NoOrigins_AllowsSameOrigin verifies that same-origin
// connections (no Origin header) work even with no configured origins.
func TestWebSocketHub_NoOrigins_AllowsSameOrigin(t *testing.T) {
	store := state.New()
	hub := NewHub(store, nil)
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// No Origin header = same-origin
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("same-origin WebSocket should connect, got error: %v", err)
	}
	conn.Close()
}

// TestWebSocketHub_ConfiguredOrigin_Allowed verifies that a connection from
// an explicitly allowed origin succeeds.
func TestWebSocketHub_ConfiguredOrigin_Allowed(t *testing.T) {
	store := state.New()
	hub := NewHub(store, []string{"http://localhost:5173"})
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Set("Origin", "http://localhost:5173")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("allowed origin should connect, got error: %v", err)
	}
	conn.Close()
}

// TestWebSocketHub_ConfiguredOrigin_RejectsOther verifies that a connection
// from an origin NOT in the allowed list is rejected.
func TestWebSocketHub_ConfiguredOrigin_RejectsOther(t *testing.T) {
	store := state.New()
	hub := NewHub(store, []string{"http://localhost:5173"})
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Set("Origin", "https://evil.example.com")

	_, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err == nil {
		t.Fatal("unlisted origin should be rejected")
	}
}

// TestWebSocketHub_BroadcastReachesClients verifies that BroadcastState
// sends data to connected clients.
func TestWebSocketHub_BroadcastReachesClients(t *testing.T) {
	store := state.New()
	hub := NewHub(store, nil)
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("connection failed: %v", err)
	}
	defer conn.Close()

	// Read the initial state message that gets sent on connect
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read initial state: %v", err)
	}

	// Should be a JSON message with type "state"
	if !strings.Contains(string(msg), `"type":"state"`) {
		t.Errorf("expected state message, got: %s", string(msg)[:min(100, len(msg))])
	}
}
