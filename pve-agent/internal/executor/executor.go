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
	switch cmd.Action {
	// VM power
	case "vm_start", "vm_stop", "vm_shutdown", "vm_reboot", "vm_migrate":
		e.executeVM(ctx, cmd, result)
	// VM lifecycle
	case "vm_clone":
		e.cloneVM(ctx, cmd, result)
	case "vm_delete":
		e.deleteVM(ctx, cmd, result)
	case "vm_config_get":
		e.vmConfigGet(ctx, cmd, result)
	case "vm_config_set":
		e.vmConfigSet(ctx, cmd, result)
	// VM snapshots
	case "vm_snapshot_list":
		e.vmSnapshotList(ctx, cmd, result)
	case "vm_snapshot_create":
		e.vmSnapshotCreate(ctx, cmd, result)
	case "vm_snapshot_delete":
		e.vmSnapshotDelete(ctx, cmd, result)
	case "vm_snapshot_rollback":
		e.vmSnapshotRollback(ctx, cmd, result)
	// CT power
	case "ct_start", "ct_stop", "ct_shutdown", "ct_reboot", "ct_migrate":
		e.executeCT(ctx, cmd, result)
	// CT lifecycle
	case "ct_clone":
		e.cloneCT(ctx, cmd, result)
	case "ct_delete":
		e.deleteCT(ctx, cmd, result)
	case "ct_config_get":
		e.ctConfigGet(ctx, cmd, result)
	case "ct_config_set":
		e.ctConfigSet(ctx, cmd, result)
	// CT snapshots
	case "ct_snapshot_list":
		e.ctSnapshotList(ctx, cmd, result)
	case "ct_snapshot_create":
		e.ctSnapshotCreate(ctx, cmd, result)
	case "ct_snapshot_delete":
		e.ctSnapshotDelete(ctx, cmd, result)
	case "ct_snapshot_rollback":
		e.ctSnapshotRollback(ctx, cmd, result)
	// Ceph
	case "ceph_pg_repair", "ceph_health_detail", "ceph_osd_tree", "ceph_status", "ceph_pg_query":
		e.executeCeph(ctx, cmd, result)
	// Storage
	case "storage_content":
		e.executeStorage(ctx, cmd, result)
	default:
		result.Error = "unknown action: " + cmd.Action
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

	case cmd.Action == "vm_clone" || cmd.Action == "ct_clone":
		if getParamInt(cmd.Params, "newid") == 0 {
			return fmt.Errorf("newid required for clone")
		}

	case cmd.Action == "vm_snapshot_delete" || cmd.Action == "vm_snapshot_rollback" ||
		cmd.Action == "ct_snapshot_delete" || cmd.Action == "ct_snapshot_rollback":
		if _, ok := cmd.Params["snapname"].(string); !ok {
			return fmt.Errorf("snapname required")
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
