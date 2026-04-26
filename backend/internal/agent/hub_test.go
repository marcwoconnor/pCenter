package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/moconnor/pcenter/internal/state"
)

// TestAgentHub_NoToken_RejectsAll verifies that when auth_token is empty
// (not configured), ALL agent connections are rejected. This is the
// fail-closed behavior — the safe default.
func TestAgentHub_NoToken_RejectsAll(t *testing.T) {
	store := state.New()
	hub := NewHub(store, "") // empty token = reject all

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	// Try to connect — should get 503 Service Unavailable
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when no token configured, got %d", resp.StatusCode)
	}
}

// TestAgentHub_BadToken_Rejected verifies that an incorrect token is rejected
// with 401.
func TestAgentHub_BadToken_Rejected(t *testing.T) {
	store := state.New()
	hub := NewHub(store, "correct-secret-token")

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	// Try with wrong token
	resp, err := http.Get(server.URL + "?token=wrong-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad token, got %d", resp.StatusCode)
	}
}

// TestAgentHub_MissingToken_Rejected verifies that a request without any
// token param is rejected.
func TestAgentHub_MissingToken_Rejected(t *testing.T) {
	store := state.New()
	hub := NewHub(store, "my-secret")

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing token, got %d", resp.StatusCode)
	}
}

// TestAgentHub_ValidToken_Connects verifies that a correct token allows
// the WebSocket connection to upgrade successfully.
func TestAgentHub_ValidToken_Connects(t *testing.T) {
	store := state.New()
	hub := NewHub(store, "my-secret")

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=my-secret"

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v (status: %d)", err, resp.StatusCode)
	}
	defer conn.Close()

	// Connection succeeded — agent is connected
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}
}

// TestAgentHub_TimingAttackResistance verifies that token comparison doesn't
// leak length information. We can't fully test constant-time behavior in a
// unit test, but we verify both short and long wrong tokens are rejected.
func TestAgentHub_TimingAttackResistance(t *testing.T) {
	store := state.New()
	hub := NewHub(store, "correct-32-byte-secret-token-ok!")

	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	tokens := []string{
		"",                                 // empty
		"a",                                // very short
		"correct-32-byte-secret-token-ok",  // off by one char
		"correct-32-byte-secret-token-ok!x", // one char too long
		"CORRECT-32-BYTE-SECRET-TOKEN-OK!", // wrong case
	}

	for _, tok := range tokens {
		resp, err := http.Get(server.URL + "?token=" + tok)
		if err != nil {
			t.Fatalf("request failed for token %q: %v", tok, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("token %q should be rejected, got status %d", tok, resp.StatusCode)
		}
	}
}

// TestAgentHub_RegisterAndUnregister verifies the basic agent lifecycle.
func TestAgentHub_RegisterAndUnregister(t *testing.T) {
	store := state.New()
	hub := NewHub(store, "test-token")

	// Initially no agents
	agents := hub.GetConnectedAgents()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}

	// Simulate agent registration
	agent := &AgentConn{
		hub:     hub,
		node:    "pve01",
		cluster: "test-cluster",
		send:    make(chan []byte, 64),
		done:    make(chan struct{}),
	}

	hub.register(agent)

	agents = hub.GetConnectedAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0] != "test-cluster/pve01" {
		t.Errorf("expected key 'test-cluster/pve01', got %q", agents[0])
	}

	// Unregister
	hub.unregister(agent)

	agents = hub.GetConnectedAgents()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents after unregister, got %d", len(agents))
	}
}

// TestConvertAgentSmart verifies the wire-format SmartReport from the agent
// converts cleanly into a domain pve.SmartReport: parseable scrapes become
// disks, scrapes with Error become DeviceErrors, and unparseable RawJSON
// also becomes a DeviceError (instead of being silently dropped).
func TestConvertAgentSmart(t *testing.T) {
	// Minimal valid smartctl JSON output. ParseSmartJSON only requires the
	// structure to decode — even mostly-empty values yield a SmartDisk.
	validJSON := `{"device":{"name":"/dev/sda","protocol":"ATA"},"smart_status":{"passed":true},"model_name":"TEST","serial_number":"SN1"}`

	in := &AgentSmartReport{
		Node:        "pve01",
		Cluster:     "test",
		CollectedAt: 1700000000,
		DurationMs:  1234,
		Scrapes: []AgentSmartScrape{
			{Device: "/dev/sda", Type: "sat", RawJSON: validJSON},
			{Device: "/dev/sdb", Type: "sat", Error: "smartctl: not installed"},
			{Device: "/dev/sdc", Type: "sat", RawJSON: "not-valid-json"},
		},
	}

	out := convertAgentSmart(in)

	if out.Source != "agent" {
		t.Errorf("Source = %q, want %q", out.Source, "agent")
	}
	if out.DurationMs != 1234 {
		t.Errorf("DurationMs = %d, want 1234", out.DurationMs)
	}
	if len(out.Disks) != 1 {
		t.Fatalf("Disks = %d, want 1", len(out.Disks))
	}
	if out.Disks[0].Device != "/dev/sda" || out.Disks[0].Cluster != "test" {
		t.Errorf("Disk metadata wrong: %+v", out.Disks[0])
	}
	if len(out.DeviceErrors) != 2 {
		t.Fatalf("DeviceErrors = %d, want 2", len(out.DeviceErrors))
	}
	// Error from agent passed through verbatim
	foundAgentErr := false
	foundParseErr := false
	for _, de := range out.DeviceErrors {
		if de.Device == "/dev/sdb" && de.Error == "smartctl: not installed" {
			foundAgentErr = true
		}
		if de.Device == "/dev/sdc" && de.Error == "unparseable smartctl JSON" {
			foundParseErr = true
		}
	}
	if !foundAgentErr {
		t.Error("expected agent-provided error to pass through verbatim")
	}
	if !foundParseErr {
		t.Error("expected unparseable JSON to surface as DeviceError, not silently drop")
	}
}

// TestAgentHub_ReplaceExistingAgent verifies that when a new agent connects
// with the same cluster/node key, the old one is signaled to stop.
func TestAgentHub_ReplaceExistingAgent(t *testing.T) {
	store := state.New()
	hub := NewHub(store, "test-token")

	oldAgent := &AgentConn{
		hub:     hub,
		node:    "pve01",
		cluster: "test-cluster",
		send:    make(chan []byte, 64),
		done:    make(chan struct{}),
	}
	hub.register(oldAgent)

	newAgent := &AgentConn{
		hub:     hub,
		node:    "pve01",
		cluster: "test-cluster",
		send:    make(chan []byte, 64),
		done:    make(chan struct{}),
	}
	hub.register(newAgent)

	agents := hub.GetConnectedAgents()
	if len(agents) != 1 {
		t.Errorf("expected 1 agent after replacement, got %d", len(agents))
	}

	// Old agent's done channel should be closed (signals writePump to stop)
	select {
	case <-oldAgent.done:
		// Good — done channel is closed
	default:
		t.Error("old agent's done channel should be closed after replacement")
	}
}
