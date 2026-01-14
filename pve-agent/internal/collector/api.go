package collector

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/moconnor/pve-agent/internal/types"
)

// PVEClient talks to the local Proxmox API
type PVEClient struct {
	nodeName    string
	baseURL     string
	tokenID     string
	tokenSecret string
	httpClient  *http.Client
}

// NewPVEClient creates a client for the local PVE API
func NewPVEClient(nodeName, tokenID, tokenSecret string) *PVEClient {
	return &PVEClient{
		nodeName:    nodeName,
		baseURL:     "https://127.0.0.1:8006/api2/json",
		tokenID:     tokenID,
		tokenSecret: tokenSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

// APIResponse wraps PVE API responses
type APIResponse[T any] struct {
	Data T `json:"data"`
}

// get makes a GET request to the local PVE API
func get[T any](c *PVEClient, ctx context.Context, path string) (T, error) {
	var result T

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return result, err
	}

	// Add PVE API token auth if configured
	if c.tokenID != "" && c.tokenSecret != "" {
		req.Header.Set("Authorization", "PVEAPIToken="+c.tokenID+"="+c.tokenSecret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var apiResp APIResponse[T]
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return result, err
	}

	return apiResp.Data, nil
}

// Post makes a POST request to the local PVE API and returns the UPID
func (c *PVEClient) Post(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, nil)
	if err != nil {
		return "", err
	}

	if c.tokenID != "" && c.tokenSecret != "" {
		req.Header.Set("Authorization", "PVEAPIToken="+c.tokenID+"="+c.tokenSecret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var apiResp APIResponse[string]
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return "", err
	}

	return apiResp.Data, nil
}

// PostWithParams makes a POST with form params
func (c *PVEClient) PostWithParams(ctx context.Context, path string, params map[string]string) (string, error) {
	form := make([]byte, 0)
	for k, v := range params {
		if len(form) > 0 {
			form = append(form, '&')
		}
		form = append(form, []byte(k+"="+v)...)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(form))
	if err != nil {
		return "", err
	}

	if c.tokenID != "" && c.tokenSecret != "" {
		req.Header.Set("Authorization", "PVEAPIToken="+c.tokenID+"="+c.tokenSecret)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var apiResp APIResponse[string]
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return "", err
	}

	return apiResp.Data, nil
}

// NodeName returns the configured node name
func (c *PVEClient) NodeName() string {
	return c.nodeName
}

// Raw node data from API
type rawNode struct {
	Status    string  `json:"status"`
	CPU       float64 `json:"cpu"`
	MaxCPU    int     `json:"maxcpu"`
	Mem       int64   `json:"mem"`
	MaxMem    int64   `json:"maxmem"`
	Disk      int64   `json:"disk"`
	MaxDisk   int64   `json:"maxdisk"`
	Uptime    int64   `json:"uptime"`
}

type rawNodeStatus struct {
	PVEVersion string   `json:"pveversion"`
	KVersion   string   `json:"kversion"`
	LoadAvg    []string `json:"loadavg"`
	CPUInfo    struct {
		Cores   int    `json:"cores"`
		Sockets int    `json:"sockets"`
		Model   string `json:"model"`
	} `json:"cpuinfo"`
}

// GetNodeStatus returns the local node's status
func (c *PVEClient) GetNodeStatus(ctx context.Context) (*types.NodeStatus, error) {
	// Get basic node info
	nodes, err := get[[]rawNode](c, ctx, "/nodes")
	if err != nil {
		return nil, err
	}

	var node *rawNode
	for i := range nodes {
		if nodes[i].Status != "" {
			node = &nodes[i]
			break
		}
	}
	if node == nil {
		return nil, fmt.Errorf("node not found")
	}

	// Get detailed status
	status, err := get[rawNodeStatus](c, ctx, fmt.Sprintf("/nodes/%s/status", c.nodeName))
	if err != nil {
		// Non-fatal, continue with basic info
		return &types.NodeStatus{
			Status:  "online",
			CPU:     node.CPU,
			MaxCPU:  node.MaxCPU,
			Mem:     node.Mem,
			MaxMem:  node.MaxMem,
			Disk:    node.Disk,
			MaxDisk: node.MaxDisk,
			Uptime:  node.Uptime,
		}, nil
	}

	return &types.NodeStatus{
		Status:     "online",
		CPU:        node.CPU,
		MaxCPU:     node.MaxCPU,
		Mem:        node.Mem,
		MaxMem:     node.MaxMem,
		Disk:       node.Disk,
		MaxDisk:    node.MaxDisk,
		Uptime:     node.Uptime,
		PVEVersion: status.PVEVersion,
		KVersion:   status.KVersion,
		LoadAvg:    status.LoadAvg,
	}, nil
}

// Raw VM data from API
type rawVM struct {
	VMID      int     `json:"vmid"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	CPU       float64 `json:"cpu"`
	CPUs      int     `json:"cpus"`
	Mem       int64   `json:"mem"`
	MaxMem    int64   `json:"maxmem"`
	Disk      int64   `json:"disk"`
	MaxDisk   int64   `json:"maxdisk"`
	NetIn     int64   `json:"netin"`
	NetOut    int64   `json:"netout"`
	DiskRead  int64   `json:"diskread"`
	DiskWrite int64   `json:"diskwrite"`
	Uptime    int64   `json:"uptime"`
	Template  int     `json:"template"`
}

// GetVMs returns all VMs on this node
func (c *PVEClient) GetVMs(ctx context.Context) ([]types.VMStatus, error) {
	vms, err := get[[]rawVM](c, ctx, fmt.Sprintf("/nodes/%s/qemu", c.nodeName))
	if err != nil {
		return nil, err
	}

	result := make([]types.VMStatus, len(vms))
	for i, vm := range vms {
		result[i] = types.VMStatus{
			VMID:      vm.VMID,
			Name:      vm.Name,
			Status:    vm.Status,
			CPU:       vm.CPU,
			CPUs:      vm.CPUs,
			Mem:       vm.Mem,
			MaxMem:    vm.MaxMem,
			Disk:      vm.Disk,
			MaxDisk:   vm.MaxDisk,
			NetIn:     vm.NetIn,
			NetOut:    vm.NetOut,
			DiskRead:  vm.DiskRead,
			DiskWrite: vm.DiskWrite,
			Uptime:    vm.Uptime,
			Template:  vm.Template == 1,
		}
	}

	return result, nil
}

// Raw container data from API
type rawCT struct {
	VMID      int     `json:"vmid"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	CPU       float64 `json:"cpu"`
	CPUs      int     `json:"cpus"`
	Mem       int64   `json:"mem"`
	MaxMem    int64   `json:"maxmem"`
	Disk      int64   `json:"disk"`
	MaxDisk   int64   `json:"maxdisk"`
	Swap      int64   `json:"swap"`
	MaxSwap   int64   `json:"maxswap"`
	NetIn     int64   `json:"netin"`
	NetOut    int64   `json:"netout"`
	DiskRead  int64   `json:"diskread"`
	DiskWrite int64   `json:"diskwrite"`
	Uptime    int64   `json:"uptime"`
	Template  int     `json:"template"`
}

// GetContainers returns all containers on this node
func (c *PVEClient) GetContainers(ctx context.Context) ([]types.CTStatus, error) {
	cts, err := get[[]rawCT](c, ctx, fmt.Sprintf("/nodes/%s/lxc", c.nodeName))
	if err != nil {
		return nil, err
	}

	result := make([]types.CTStatus, len(cts))
	for i, ct := range cts {
		result[i] = types.CTStatus{
			VMID:      ct.VMID,
			Name:      ct.Name,
			Status:    ct.Status,
			CPU:       ct.CPU,
			CPUs:      ct.CPUs,
			Mem:       ct.Mem,
			MaxMem:    ct.MaxMem,
			Disk:      ct.Disk,
			MaxDisk:   ct.MaxDisk,
			Swap:      ct.Swap,
			MaxSwap:   ct.MaxSwap,
			NetIn:     ct.NetIn,
			NetOut:    ct.NetOut,
			DiskRead:  ct.DiskRead,
			DiskWrite: ct.DiskWrite,
			Uptime:    ct.Uptime,
			Template:  ct.Template == 1,
		}
	}

	return result, nil
}

// Raw storage data from API
type rawStorage struct {
	Storage string `json:"storage"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	Total   int64  `json:"total"`
	Used    int64  `json:"used"`
	Avail   int64  `json:"avail"`
	Shared  int    `json:"shared"`
	Content string `json:"content"`
}

// GetStorage returns storage status for this node
func (c *PVEClient) GetStorage(ctx context.Context) ([]types.StorageStatus, error) {
	storage, err := get[[]rawStorage](c, ctx, fmt.Sprintf("/nodes/%s/storage", c.nodeName))
	if err != nil {
		return nil, err
	}

	result := make([]types.StorageStatus, len(storage))
	for i, s := range storage {
		result[i] = types.StorageStatus{
			Storage: s.Storage,
			Type:    s.Type,
			Status:  s.Status,
			Total:   s.Total,
			Used:    s.Used,
			Avail:   s.Avail,
			Shared:  s.Shared == 1,
			Content: s.Content,
		}
	}

	return result, nil
}

// Raw Ceph status from API
type rawCephStatus struct {
	Health struct {
		Status string `json:"status"`
		Checks map[string]struct {
			Severity string `json:"severity"`
			Summary  struct {
				Message string `json:"message"`
			} `json:"summary"`
		} `json:"checks"`
	} `json:"health"`
	PGMap struct {
		BytesTotal int64 `json:"bytes_total"`
		BytesUsed  int64 `json:"bytes_used"`
		BytesAvail int64 `json:"bytes_avail"`
	} `json:"pgmap"`
	OSDMap struct {
		NumOSDs   int `json:"num_osds"`
		NumUpOSDs int `json:"num_up_osds"`
		NumInOSDs int `json:"num_in_osds"`
	} `json:"osdmap"`
	MonMap struct {
		NumMons int `json:"num_mons"`
	} `json:"monmap"`
}

// GetCephStatus returns Ceph cluster status
func (c *PVEClient) GetCephStatus(ctx context.Context) (*types.CephStatus, error) {
	raw, err := get[rawCephStatus](c, ctx, fmt.Sprintf("/nodes/%s/ceph/status", c.nodeName))
	if err != nil {
		return nil, err
	}

	status := &types.CephStatus{
		Health: raw.Health.Status,
		PGMap: types.CephPGMap{
			BytesTotal: raw.PGMap.BytesTotal,
			BytesUsed:  raw.PGMap.BytesUsed,
			BytesAvail: raw.PGMap.BytesAvail,
		},
		OSDMap: types.CephOSDMap{
			NumOSDs:   raw.OSDMap.NumOSDs,
			NumUpOSDs: raw.OSDMap.NumUpOSDs,
			NumInOSDs: raw.OSDMap.NumInOSDs,
		},
		MonMap: types.CephMonMap{
			NumMons: raw.MonMap.NumMons,
		},
	}

	// Convert health checks
	for name, check := range raw.Health.Checks {
		status.HealthChecks = append(status.HealthChecks, types.HealthCheck{
			Name:     name,
			Severity: check.Severity,
			Summary:  check.Summary.Message,
		})
	}

	return status, nil
}
