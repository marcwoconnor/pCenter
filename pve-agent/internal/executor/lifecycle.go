package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/moconnor/pve-agent/internal/types"
)

// --- VM Lifecycle ---

func (e *Executor) cloneVM(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()
	newid := getParamInt(cmd.Params, "newid")
	if newid == 0 {
		result.Error = "newid required"
		return
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%d/clone", node, vmid)
	params := map[string]string{
		"newid": fmt.Sprintf("%d", newid),
	}
	if name, ok := cmd.Params["name"].(string); ok && name != "" {
		params["name"] = name
	}
	if target, ok := cmd.Params["target"].(string); ok && target != "" {
		params["target"] = target
	}
	if full, ok := cmd.Params["full"].(bool); ok && full {
		params["full"] = "1"
	}

	upid, err := e.api.PostWithParams(ctx, path, params)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) deleteVM(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	upid, err := e.api.Delete(ctx, fmt.Sprintf("/nodes/%s/qemu/%d", node, vmid))
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) vmConfigGet(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	data, err := e.api.GetRaw(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid))
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.Output = string(data)
}

func (e *Executor) vmConfigSet(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	configParams := extractStringParams(cmd.Params, "vmid")
	err := e.api.PutWithParams(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid), configParams)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
}

// --- Container Lifecycle ---

func (e *Executor) cloneCT(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()
	newid := getParamInt(cmd.Params, "newid")
	if newid == 0 {
		result.Error = "newid required"
		return
	}

	path := fmt.Sprintf("/nodes/%s/lxc/%d/clone", node, vmid)
	params := map[string]string{
		"newid": fmt.Sprintf("%d", newid),
	}
	if name, ok := cmd.Params["hostname"].(string); ok && name != "" {
		params["hostname"] = name
	}
	if target, ok := cmd.Params["target"].(string); ok && target != "" {
		params["target"] = target
	}
	if full, ok := cmd.Params["full"].(bool); ok && full {
		params["full"] = "1"
	}

	upid, err := e.api.PostWithParams(ctx, path, params)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) deleteCT(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	upid, err := e.api.Delete(ctx, fmt.Sprintf("/nodes/%s/lxc/%d", node, vmid))
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) ctConfigGet(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	data, err := e.api.GetRaw(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/config", node, vmid))
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.Output = string(data)
}

func (e *Executor) ctConfigSet(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	configParams := extractStringParams(cmd.Params, "vmid")
	err := e.api.PutWithParams(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/config", node, vmid), configParams)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
}

// --- Convert to Template ---

func (e *Executor) convertVMToTemplate(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	upid, err := e.api.PostWithParams(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/template", node, vmid), nil)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) convertCTToTemplate(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	upid, err := e.api.PostWithParams(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/template", node, vmid), nil)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

// --- Snapshots ---

func (e *Executor) vmSnapshotList(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	data, err := e.api.GetRaw(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/snapshot", node, vmid))
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.Output = string(data)
}

func (e *Executor) vmSnapshotCreate(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	params := map[string]string{}
	if name, ok := cmd.Params["snapname"].(string); ok {
		params["snapname"] = name
	}
	if desc, ok := cmd.Params["description"].(string); ok {
		params["description"] = desc
	}
	if vmstate, ok := cmd.Params["vmstate"].(bool); ok && vmstate {
		params["vmstate"] = "1"
	}

	upid, err := e.api.PostWithParams(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/snapshot", node, vmid), params)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) vmSnapshotDelete(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()
	snapname, _ := cmd.Params["snapname"].(string)
	if snapname == "" {
		result.Error = "snapname required"
		return
	}

	upid, err := e.api.Delete(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/snapshot/%s", node, vmid, url.PathEscape(snapname)))
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) vmSnapshotRollback(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()
	snapname, _ := cmd.Params["snapname"].(string)
	if snapname == "" {
		result.Error = "snapname required"
		return
	}

	upid, err := e.api.PostWithParams(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/snapshot/%s/rollback", node, vmid, url.PathEscape(snapname)), nil)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) ctSnapshotList(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	data, err := e.api.GetRaw(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", node, vmid))
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.Output = string(data)
}

func (e *Executor) ctSnapshotCreate(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()

	params := map[string]string{}
	if name, ok := cmd.Params["snapname"].(string); ok {
		params["snapname"] = name
	}
	if desc, ok := cmd.Params["description"].(string); ok {
		params["description"] = desc
	}

	upid, err := e.api.PostWithParams(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", node, vmid), params)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) ctSnapshotDelete(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()
	snapname, _ := cmd.Params["snapname"].(string)
	if snapname == "" {
		result.Error = "snapname required"
		return
	}

	upid, err := e.api.Delete(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s", node, vmid, url.PathEscape(snapname)))
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

func (e *Executor) ctSnapshotRollback(ctx context.Context, cmd *types.CommandData, result *types.CommandResultData) {
	vmid := getVMID(cmd.Params)
	node := e.api.NodeName()
	snapname, _ := cmd.Params["snapname"].(string)
	if snapname == "" {
		result.Error = "snapname required"
		return
	}

	upid, err := e.api.PostWithParams(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s/rollback", node, vmid, url.PathEscape(snapname)), nil)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.Success = true
	result.UPID = upid
}

// --- Helpers ---

// getParamInt extracts an int param (JSON numbers arrive as float64)
func getParamInt(params map[string]interface{}, key string) int {
	switch v := params[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// extractStringParams converts params map to string map, excluding specified keys
func extractStringParams(params map[string]interface{}, exclude ...string) map[string]string {
	excludeSet := make(map[string]bool, len(exclude))
	for _, k := range exclude {
		excludeSet[k] = true
	}

	result := make(map[string]string)
	for k, v := range params {
		if excludeSet[k] {
			continue
		}
		switch val := v.(type) {
		case string:
			result[k] = val
		case float64:
			if val == float64(int(val)) {
				result[k] = fmt.Sprintf("%d", int(val))
			} else {
				result[k] = fmt.Sprintf("%f", val)
			}
		case bool:
			if val {
				result[k] = "1"
			} else {
				result[k] = "0"
			}
		default:
			data, err := json.Marshal(val)
			if err == nil {
				result[k] = string(data)
			}
		}
	}
	return result
}
