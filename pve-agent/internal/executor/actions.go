package executor

import (
	"context"
	"fmt"

	"github.com/moconnor/pve-agent/internal/types"
)

// executeVM handles VM operations
func (e *Executor) executeVM(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	var path string
	switch cmd.Action {
	case "vm_start":
		path = fmt.Sprintf("/nodes/%s/qemu/%d/status/start", node, vmid)
	case "vm_stop":
		path = fmt.Sprintf("/nodes/%s/qemu/%d/status/stop", node, vmid)
	case "vm_shutdown":
		path = fmt.Sprintf("/nodes/%s/qemu/%d/status/shutdown", node, vmid)
	case "vm_reboot":
		path = fmt.Sprintf("/nodes/%s/qemu/%d/status/reboot", node, vmid)
	case "vm_migrate":
		e.migrateVM(ctx, cmd, result)
		return
	default:
		result.Error = "unknown VM action"
		return
	}

	upid, err := e.api.Post(ctx, path)
	if err != nil {
		result.Error = err.Error()
		return
	}

	result.Success = true
	result.UPID = upid
}

// executeCT handles container operations
func (e *Executor) executeCT(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	var path string
	switch cmd.Action {
	case "ct_start":
		path = fmt.Sprintf("/nodes/%s/lxc/%d/status/start", node, vmid)
	case "ct_stop":
		path = fmt.Sprintf("/nodes/%s/lxc/%d/status/stop", node, vmid)
	case "ct_shutdown":
		path = fmt.Sprintf("/nodes/%s/lxc/%d/status/shutdown", node, vmid)
	case "ct_reboot":
		path = fmt.Sprintf("/nodes/%s/lxc/%d/status/reboot", node, vmid)
	case "ct_migrate":
		e.migrateCT(ctx, cmd, result)
		return
	default:
		result.Error = "unknown container action"
		return
	}

	upid, err := e.api.Post(ctx, path)
	if err != nil {
		result.Error = err.Error()
		return
	}

	result.Success = true
	result.UPID = upid
}

// migrateVM handles VM migration
func (e *Executor) migrateVM(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()
	target, _ := cmd.Params["target"].(string)
	online, _ := cmd.Params["online"].(bool)

	path := fmt.Sprintf("/nodes/%s/qemu/%d/migrate", node, vmid)
	params := map[string]string{
		"target": target,
	}
	if online {
		params["online"] = "1"
	}

	upid, err := e.api.PostWithParams(ctx, path, params)
	if err != nil {
		result.Error = err.Error()
		return
	}

	result.Success = true
	result.UPID = upid
}

// migrateCT handles container migration
func (e *Executor) migrateCT(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()
	target, _ := cmd.Params["target"].(string)
	online, _ := cmd.Params["online"].(bool)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/migrate", node, vmid)
	params := map[string]string{
		"target": target,
	}
	if online {
		params["online"] = "1"
		params["restart"] = "1" // LXC needs restart for online
	}

	upid, err := e.api.PostWithParams(ctx, path, params)
	if err != nil {
		result.Error = err.Error()
		return
	}

	result.Success = true
	result.UPID = upid
}
