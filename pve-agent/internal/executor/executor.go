package executor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/moconnor/pve-agent/internal/collector"
	"github.com/moconnor/pve-agent/internal/types"
)

// AllowedActions is the whitelist of executable actions
var AllowedActions = map[string]bool{
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

	// Storage queries
	"storage_content": true,
}

// Executor handles command execution on the agent
type Executor struct {
	api *collector.PVEClient
}

// NewExecutor creates a new command executor
func NewExecutor(api *collector.PVEClient) *Executor {
	return &Executor{api: api}
}

// Execute runs a command and returns the result
func (e *Executor) Execute(ctx context.Context, cmd *types.CommandData) *types.CommandResultData {
	result := &types.CommandResultData{ID: cmd.ID}

	// Validate
	if err := e.validate(cmd); err != nil {
		result.Error = err.Error()
		slog.Warn("command validation failed", "id", cmd.ID, "action", cmd.Action, "error", err)
		return result
	}

	slog.Info("executing command", "id", cmd.ID, "action", cmd.Action, "params", cmd.Params)

	// Route to handler
	switch {
	case strings.HasPrefix(cmd.Action, "vm_"):
		e.executeVM(ctx, cmd, result)
	case strings.HasPrefix(cmd.Action, "ct_"):
		e.executeCT(ctx, cmd, result)
	case strings.HasPrefix(cmd.Action, "ceph_"):
		e.executeCeph(ctx, cmd, result)
	case strings.HasPrefix(cmd.Action, "storage_"):
		e.executeStorage(ctx, cmd, result)
	default:
		result.Error = "unknown action type"
	}

	if result.Success {
		slog.Info("command completed", "id", cmd.ID, "upid", result.UPID)
	} else {
		slog.Error("command failed", "id", cmd.ID, "error", result.Error)
	}

	return result
}

func (e *Executor) validate(cmd *types.CommandData) error {
	if cmd.ID == "" {
		return fmt.Errorf("command ID required")
	}
	if !AllowedActions[cmd.Action] {
		return fmt.Errorf("action not allowed: %s", cmd.Action)
	}

	// Action-specific validation
	switch {
	case strings.HasPrefix(cmd.Action, "vm_"), strings.HasPrefix(cmd.Action, "ct_"):
		if cmd.Action == "vm_migrate" || cmd.Action == "ct_migrate" {
			if _, ok := cmd.Params["target"]; !ok {
				return fmt.Errorf("target required for migration")
			}
		}
		vmid, ok := cmd.Params["vmid"]
		if !ok {
			return fmt.Errorf("vmid required")
		}
		// JSON numbers come as float64
		switch v := vmid.(type) {
		case float64:
			if v <= 0 {
				return fmt.Errorf("vmid must be positive")
			}
		case int:
			if v <= 0 {
				return fmt.Errorf("vmid must be positive")
			}
		default:
			return fmt.Errorf("vmid must be a number")
		}

	case cmd.Action == "ceph_pg_repair" || cmd.Action == "ceph_pg_query":
		pgID, ok := cmd.Params["pg_id"].(string)
		if !ok || !isValidPgID(pgID) {
			return fmt.Errorf("valid pg_id required (format: pool.hex)")
		}
	}

	return nil
}

// getVMID extracts vmid from params as int
func getVMID(params map[string]interface{}) int {
	switch v := params["vmid"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
