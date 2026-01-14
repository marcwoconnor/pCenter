package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CommandData sent to agent
type CommandData struct {
	ID     string                 `json:"id"`
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params"`
}

// CommandResultData received from agent
type CommandResultData struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	UPID    string `json:"upid,omitempty"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// PendingCommand tracks an in-flight command
type PendingCommand struct {
	Command *CommandData
	SentAt  time.Time
	Result  chan *CommandResultData
}

// CommandTracker manages pending commands
type CommandTracker struct {
	pending map[string]*PendingCommand
	mu      sync.RWMutex
}

// NewCommandTracker creates a new command tracker
func NewCommandTracker() *CommandTracker {
	return &CommandTracker{
		pending: make(map[string]*PendingCommand),
	}
}

// Add registers a pending command
func (t *CommandTracker) Add(cmd *CommandData) chan *CommandResultData {
	resultCh := make(chan *CommandResultData, 1)
	t.mu.Lock()
	t.pending[cmd.ID] = &PendingCommand{
		Command: cmd,
		SentAt:  time.Now(),
		Result:  resultCh,
	}
	t.mu.Unlock()
	return resultCh
}

// Complete handles a command result
func (t *CommandTracker) Complete(result *CommandResultData) bool {
	t.mu.Lock()
	pending, ok := t.pending[result.ID]
	if ok {
		delete(t.pending, result.ID)
	}
	t.mu.Unlock()

	if !ok {
		return false
	}

	select {
	case pending.Result <- result:
	default:
	}
	return true
}

// Remove removes a pending command
func (t *CommandTracker) Remove(id string) {
	t.mu.Lock()
	delete(t.pending, id)
	t.mu.Unlock()
}

// CleanupStale removes timed-out commands
func (t *CommandTracker) CleanupStale(timeout time.Duration) int {
	now := time.Now()
	cleaned := 0

	t.mu.Lock()
	for id, pending := range t.pending {
		if now.Sub(pending.SentAt) > timeout {
			// Send timeout result
			select {
			case pending.Result <- &CommandResultData{
				ID:    id,
				Error: "command timed out",
			}:
			default:
			}
			delete(t.pending, id)
			cleaned++
		}
	}
	t.mu.Unlock()

	return cleaned
}

// SendCommand sends a command to an agent and returns a channel for the result
func (h *Hub) SendCommand(cluster, node string, cmd *CommandData) (<-chan *CommandResultData, error) {
	key := cluster + "/" + node

	h.mu.RLock()
	agent, ok := h.agents[key]
	h.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent not connected: %s", key)
	}

	// Register pending command
	resultCh := h.commands.Add(cmd)

	// Serialize message
	msg := Message{
		Type:      MsgTypeCommand,
		Timestamp: time.Now().Unix(),
	}
	cmdData, _ := json.Marshal(cmd)
	msg.Data = cmdData

	msgData, err := json.Marshal(msg)
	if err != nil {
		h.commands.Remove(cmd.ID)
		return nil, err
	}

	// Send to agent
	select {
	case agent.send <- msgData:
		slog.Info("command sent to agent", "id", cmd.ID, "action", cmd.Action, "agent", key)
	default:
		h.commands.Remove(cmd.ID)
		return nil, fmt.Errorf("agent send buffer full")
	}

	return resultCh, nil
}

// handleCommandResult processes command results from agents
func (a *AgentConn) handleCommandResult(data json.RawMessage) {
	var result CommandResultData
	if err := json.Unmarshal(data, &result); err != nil {
		slog.Error("failed to parse command result", "error", err)
		return
	}

	if a.hub.commands.Complete(&result) {
		slog.Info("command result received", "id", result.ID, "success", result.Success)
	} else {
		slog.Warn("received result for unknown command", "id", result.ID)
	}
}

// StartCleanupLoop runs periodic cleanup of stale commands
func (h *Hub) StartCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cleaned := h.commands.CleanupStale(60 * time.Second); cleaned > 0 {
				slog.Warn("cleaned up stale commands", "count", cleaned)
			}
		}
	}
}
