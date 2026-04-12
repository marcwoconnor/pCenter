package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/moconnor/pcenter/internal/agent"
)

// AllowedAgentActions is the whitelist of actions that can be executed via agents
var AllowedAgentActions = map[string]bool{
	// VM power operations
	"vm_start":    true,
	"vm_stop":     true,
	"vm_shutdown": true,
	"vm_reboot":   true,

	// VM lifecycle
	"vm_clone":      true,
	"vm_delete":     true,
	"vm_config_get": true,
	"vm_config_set": true,

	// VM snapshots
	"vm_snapshot_list":     true,
	"vm_snapshot_create":   true,
	"vm_snapshot_delete":   true,
	"vm_snapshot_rollback": true,

	// Container power operations
	"ct_start":    true,
	"ct_stop":     true,
	"ct_shutdown": true,
	"ct_reboot":   true,

	// Container lifecycle
	"ct_clone":      true,
	"ct_delete":     true,
	"ct_config_get": true,
	"ct_config_set": true,

	// Container snapshots
	"ct_snapshot_list":     true,
	"ct_snapshot_create":   true,
	"ct_snapshot_delete":   true,
	"ct_snapshot_rollback": true,

	// Migration
	"vm_migrate": true,
	"ct_migrate": true,

	// Ceph commands
	"ceph_pg_repair":     true,
	"ceph_health_detail": true,
	"ceph_osd_tree":      true,
	"ceph_status":        true,
	"ceph_pg_query":      true,

	// Storage queries
	"storage_content": true,
}

// AgentCommandRequest is the request body for /api/agent/command
type AgentCommandRequest struct {
	Cluster string                 `json:"cluster"`
	Node    string                 `json:"node"`
	Action  string                 `json:"action"`
	Params  map[string]interface{} `json:"params"`
}

// AgentCommandResponse is the response from /api/agent/command
type AgentCommandResponse struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	UPID    string `json:"upid,omitempty"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// AgentCommand handles command execution via agents
func (h *Handler) AgentCommand(w http.ResponseWriter, r *http.Request) {
	if h.agentHub == nil {
		writeError(w, http.StatusServiceUnavailable, "agent hub not configured")
		return
	}

	var req AgentCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.Cluster == "" || req.Node == "" || req.Action == "" {
		writeError(w, http.StatusBadRequest, "cluster, node, and action required")
		return
	}

	// Validate action is whitelisted
	if !AllowedAgentActions[req.Action] {
		writeError(w, http.StatusBadRequest, "action not allowed: "+req.Action)
		return
	}

	// Generate command ID
	cmdID := fmt.Sprintf("cmd-%d-%s", time.Now().UnixNano(), req.Action)

	cmd := &agent.CommandData{
		ID:     cmdID,
		Action: req.Action,
		Params: req.Params,
	}

	slog.Info("sending agent command", "id", cmdID, "cluster", req.Cluster, "node", req.Node, "action", req.Action)

	// Send command to agent
	resultCh, err := h.agentHub.SendCommand(req.Cluster, req.Node, cmd)
	if err != nil {
		slog.Error("failed to send command", "error", err)
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Wait for result with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	select {
	case result := <-resultCh:
		resp := AgentCommandResponse{
			ID:      result.ID,
			Success: result.Success,
			UPID:    result.UPID,
			Output:  result.Output,
			Error:   result.Error,
		}
		if !result.Success {
			slog.Error("agent command failed", "id", result.ID, "error", result.Error)
		}
		writeJSON(w, resp)

	case <-ctx.Done():
		slog.Error("agent command timed out", "id", cmdID)
		writeError(w, http.StatusGatewayTimeout, "command timed out")
	}
}

// tryAgentAction attempts to execute an action via the agent.
// Returns (upid, true, nil) if agent handled it successfully.
// Returns ("", true, err) if agent handled it but failed.
// Returns ("", false, nil) if no agent available — caller should fall back to poller.
func (h *Handler) tryAgentAction(ctx context.Context, cluster, node, action string, params map[string]interface{}) (string, bool, error) {
	if h.agentHub == nil {
		return "", false, nil
	}

	// Check if action is agent-supported
	if !AllowedAgentActions[action] {
		return "", false, nil
	}

	// Check if agent is connected for this cluster/node
	key := cluster + "/" + node
	agents := h.agentHub.GetConnectedAgents()
	connected := false
	for _, a := range agents {
		if a == key {
			connected = true
			break
		}
	}
	if !connected {
		return "", false, nil
	}

	cmdID := fmt.Sprintf("cmd-%d-%s", time.Now().UnixNano(), action)
	cmd := &agent.CommandData{
		ID:     cmdID,
		Action: action,
		Params: params,
	}

	slog.Info("routing action via agent", "id", cmdID, "cluster", cluster, "node", node, "action", action)

	resultCh, err := h.agentHub.SendCommand(cluster, node, cmd)
	if err != nil {
		slog.Warn("agent command send failed, falling back to poller", "error", err)
		return "", false, nil // fall back
	}

	// Wait for result
	select {
	case result := <-resultCh:
		if !result.Success {
			return "", true, fmt.Errorf("agent: %s", result.Error)
		}
		return result.UPID, true, nil

	case <-ctx.Done():
		return "", true, fmt.Errorf("agent command timed out")
	}
}

// GetConnectedAgents returns the list of connected agents
func (h *Handler) GetConnectedAgents(w http.ResponseWriter, r *http.Request) {
	if h.agentHub == nil {
		writeJSON(w, []string{})
		return
	}
	writeJSON(w, h.agentHub.GetConnectedAgents())
}
