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
	// VM operations
	"vm_start":    true,
	"vm_stop":     true,
	"vm_shutdown": true,
	"vm_reboot":   true,

	// Container operations
	"ct_start":    true,
	"ct_stop":     true,
	"ct_shutdown": true,
	"ct_reboot":   true,

	// Migration
	"vm_migrate": true,
	"ct_migrate": true,

	// Ceph commands
	"ceph_pg_repair":     true,
	"ceph_health_detail": true,
	"ceph_osd_tree":      true,
	"ceph_status":        true,
	"ceph_pg_query":      true,
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

// GetConnectedAgents returns the list of connected agents
func (h *Handler) GetConnectedAgents(w http.ResponseWriter, r *http.Request) {
	if h.agentHub == nil {
		writeJSON(w, []string{})
		return
	}
	writeJSON(w, h.agentHub.GetConnectedAgents())
}
