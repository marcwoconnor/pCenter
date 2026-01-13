package pve

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/moconnor/pcenter/internal/config"
)

// Client is a Proxmox VE API client for a single node
type Client struct {
	baseURL     string
	tokenID     string
	tokenSecret string
	httpClient  *http.Client
	nodeName    string
	clusterName string
}

// NewClientFromClusterConfig creates a new PVE API client for cluster discovery
func NewClientFromClusterConfig(cfg config.ClusterConfig) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.Insecure,
		},
	}

	return &Client{
		baseURL:     fmt.Sprintf("https://%s/api2/json", cfg.DiscoveryNode),
		tokenID:     cfg.TokenID,
		tokenSecret: cfg.TokenSecret,
		clusterName: cfg.Name,
		nodeName:    "", // Will be set after discovery
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// NewClientForNode creates a client for a specific node in a cluster
func NewClientForNode(cfg config.ClusterConfig, nodeName string, nodeIP string) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.Insecure,
		},
	}

	// Use node's IP if available, otherwise use discovery node
	host := cfg.DiscoveryNode
	if nodeIP != "" {
		// Extract port from discovery node
		parts := strings.Split(cfg.DiscoveryNode, ":")
		port := "8006"
		if len(parts) > 1 {
			port = parts[1]
		}
		host = fmt.Sprintf("%s:%s", nodeIP, port)
	}

	return &Client{
		baseURL:     fmt.Sprintf("https://%s/api2/json", host),
		tokenID:     cfg.TokenID,
		tokenSecret: cfg.TokenSecret,
		clusterName: cfg.Name,
		nodeName:    nodeName,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// SetNodeName sets the node name (used after cluster discovery)
func (c *Client) SetNodeName(name string) {
	c.nodeName = name
}

// ClusterName returns the cluster this client belongs to
func (c *Client) ClusterName() string {
	return c.clusterName
}

// NodeName returns the configured node name
func (c *Client) NodeName() string {
	return c.nodeName
}

// Host returns the host:port of this node
func (c *Client) Host() string {
	// baseURL is like "https://10.31.10.14:8006/api2/json"
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// request makes an authenticated API request
func (c *Client) request(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	reqURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// PVE API token auth header
	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))

	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

// get makes a GET request and unmarshals into the provided type
func get[T any](c *Client, ctx context.Context, path string) (T, error) {
	var result T

	data, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return result, err
	}

	var resp APIResponse[T]
	if err := json.Unmarshal(data, &resp); err != nil {
		return result, fmt.Errorf("unmarshal response: %w (body: %s)", err, string(data))
	}

	return resp.Data, nil
}

// post makes a POST request
func (c *Client) post(ctx context.Context, path string, params map[string]string) ([]byte, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	return c.request(ctx, http.MethodPost, path, strings.NewReader(form.Encode()))
}

// --- Node operations ---

// GetNodes returns all nodes in the cluster
func (c *Client) GetNodes(ctx context.Context) ([]Node, error) {
	return get[[]Node](c, ctx, "/nodes")
}

// GetNodeStatus returns detailed status for this node
func (c *Client) GetNodeStatus(ctx context.Context) (*Node, error) {
	nodes, err := c.GetNodes(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		if n.Node == c.nodeName {
			return &n, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", c.nodeName)
}

// --- VM operations ---

// GetVMs returns all VMs on this node
func (c *Client) GetVMs(ctx context.Context) ([]VM, error) {
	vms, err := get[[]VM](c, ctx, fmt.Sprintf("/nodes/%s/qemu", c.nodeName))
	if err != nil {
		return nil, err
	}
	// Tag with node name
	for i := range vms {
		vms[i].Node = c.nodeName
	}
	return vms, nil
}

// GetVM returns a single VM by VMID
func (c *Client) GetVM(ctx context.Context, vmid int) (*VM, error) {
	vms, err := c.GetVMs(ctx)
	if err != nil {
		return nil, err
	}
	for _, vm := range vms {
		if vm.VMID == vmid {
			return &vm, nil
		}
	}
	return nil, fmt.Errorf("VM %d not found on %s", vmid, c.nodeName)
}

// StartVM starts a VM
func (c *Client) StartVM(ctx context.Context, vmid int) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/status/start", c.nodeName, vmid), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil // returns UPID
}

// StopVM stops a VM
func (c *Client) StopVM(ctx context.Context, vmid int) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/status/stop", c.nodeName, vmid), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// ShutdownVM gracefully shuts down a VM
func (c *Client) ShutdownVM(ctx context.Context, vmid int) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/status/shutdown", c.nodeName, vmid), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// --- Container operations ---

// GetContainers returns all LXC containers on this node
func (c *Client) GetContainers(ctx context.Context) ([]Container, error) {
	cts, err := get[[]Container](c, ctx, fmt.Sprintf("/nodes/%s/lxc", c.nodeName))
	if err != nil {
		return nil, err
	}
	for i := range cts {
		cts[i].Node = c.nodeName
		cts[i].Type = "lxc"
	}
	return cts, nil
}

// StartContainer starts a container
func (c *Client) StartContainer(ctx context.Context, vmid int) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/status/start", c.nodeName, vmid), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// StopContainer stops a container
func (c *Client) StopContainer(ctx context.Context, vmid int) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/status/stop", c.nodeName, vmid), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// ShutdownContainer gracefully shuts down a container
func (c *Client) ShutdownContainer(ctx context.Context, vmid int) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/status/shutdown", c.nodeName, vmid), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// --- Storage operations ---

// GetStorage returns all storage on this node
func (c *Client) GetStorage(ctx context.Context) ([]Storage, error) {
	storage, err := get[[]Storage](c, ctx, fmt.Sprintf("/nodes/%s/storage", c.nodeName))
	if err != nil {
		return nil, err
	}
	for i := range storage {
		storage[i].Node = c.nodeName
	}
	return storage, nil
}

// --- Cluster operations ---

// GetClusterResources returns all resources across the cluster
func (c *Client) GetClusterResources(ctx context.Context, resourceType string) ([]ClusterResource, error) {
	path := "/cluster/resources"
	if resourceType != "" {
		path += "?type=" + resourceType
	}
	return get[[]ClusterResource](c, ctx, path)
}

// --- Task operations ---

// GetTasks returns recent tasks for this node
func (c *Client) GetTasks(ctx context.Context) ([]Task, error) {
	return get[[]Task](c, ctx, fmt.Sprintf("/nodes/%s/tasks", c.nodeName))
}

// GetTaskStatus returns the status of a specific task
func (c *Client) GetTaskStatus(ctx context.Context, upid string) (*Task, error) {
	// UPID format: UPID:node:pid:pstart:starttime:type:id:user:
	task, err := get[Task](c, ctx, fmt.Sprintf("/nodes/%s/tasks/%s/status", c.nodeName, url.PathEscape(upid)))
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// --- Ceph operations ---

// GetCephStatus returns Ceph cluster status (if available)
func (c *Client) GetCephStatus(ctx context.Context) (*CephStatus, error) {
	status, err := get[CephStatus](c, ctx, fmt.Sprintf("/nodes/%s/ceph/status", c.nodeName))
	if err != nil {
		return nil, err
	}
	return &status, nil
}

// --- Migration ---

// MigrateVM migrates a VM to another node
func (c *Client) MigrateVM(ctx context.Context, vmid int, targetNode string, online bool) (string, error) {
	params := map[string]string{
		"target": targetNode,
	}
	if online {
		params["online"] = "1"
	}
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/migrate", c.nodeName, vmid), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// MigrateContainer migrates a container to another node
func (c *Client) MigrateContainer(ctx context.Context, vmid int, targetNode string, online bool) (string, error) {
	params := map[string]string{
		"target": targetNode,
	}
	if online {
		params["online"] = "1"
	}
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/migrate", c.nodeName, vmid), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// --- Console/VNC operations ---

// VNCProxyResponse contains the VNC proxy ticket info
type VNCProxyResponse struct {
	Port      json.Number `json:"port"` // Can be string or int from API
	Ticket    string      `json:"ticket"`
	UPID      string      `json:"upid"`
	User      string      `json:"user"`
	Cert      string      `json:"cert,omitempty"`
}

// PortInt returns the port as an integer
func (v *VNCProxyResponse) PortInt() int {
	p, _ := v.Port.Int64()
	return int(p)
}

// GetVMVNCProxy gets a VNC proxy ticket for a VM
func (c *Client) GetVMVNCProxy(ctx context.Context, vmid int) (*VNCProxyResponse, error) {
	params := map[string]string{
		"websocket": "1",
	}
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/vncproxy", c.nodeName, vmid), params)
	if err != nil {
		return nil, err
	}
	var resp APIResponse[VNCProxyResponse]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal vncproxy response: %w", err)
	}
	return &resp.Data, nil
}

// GetContainerTermProxy gets a terminal proxy ticket for a container
// Uses vncproxy with websocket=1 which creates a terminal session
func (c *Client) GetContainerTermProxy(ctx context.Context, vmid int) (*VNCProxyResponse, error) {
	// Use vncproxy with websocket=1 - Proxmox creates terminal internally
	params := map[string]string{
		"websocket": "1",
	}
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/vncproxy", c.nodeName, vmid), params)
	if err != nil {
		return nil, err
	}
	var resp APIResponse[VNCProxyResponse]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal vncproxy response: %w", err)
	}
	return &resp.Data, nil
}

// VNCWebsocketURL returns the websocket URL for connecting to the console
func (c *Client) VNCWebsocketURL(vmType string, vmid int, port int, ticket string) string {
	// For VMs: /nodes/{node}/qemu/{vmid}/vncwebsocket?port={port}&vncticket={ticket}
	// For containers: /nodes/{node}/lxc/{vmid}/termwebsocket?port={port}&vncticket={ticket}
	wsType := "vncwebsocket"
	guestType := "qemu"
	if vmType == "lxc" {
		wsType = "termwebsocket"
		guestType = "lxc"
	}
	baseURL := strings.Replace(c.baseURL, "/api2/json", "", 1)
	return fmt.Sprintf("%s/api2/json/nodes/%s/%s/%d/%s?port=%d&vncticket=%s",
		baseURL, c.nodeName, guestType, vmid, wsType, port, url.QueryEscape(ticket))
}

// AuthHeader returns the authorization header value for this client
func (c *Client) AuthHeader() string {
	return fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret)
}

// --- Cluster Discovery ---

// ClusterStatusNode represents a node from /cluster/status
type ClusterStatusNode struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	IP     string `json:"ip"`
	Online int    `json:"online"` // 1 or 0
	Local  int    `json:"local"`  // 1 if this is the node we queried
	Type   string `json:"type"`   // "node" or "cluster"
}

// DiscoverClusterNodes returns all nodes in the cluster via /cluster/status
func (c *Client) DiscoverClusterNodes(ctx context.Context) ([]ClusterStatusNode, error) {
	items, err := get[[]ClusterStatusNode](c, ctx, "/cluster/status")
	if err != nil {
		return nil, err
	}

	// Filter to only node types
	var nodes []ClusterStatusNode
	for _, item := range items {
		if item.Type == "node" {
			nodes = append(nodes, item)
		}
	}
	return nodes, nil
}

// --- HA Operations ---

// HAManagerStatus represents the HA manager from /cluster/ha/status/manager_status
type HAManagerStatusResponse struct {
	MasterNode string `json:"master_node"`
}

// HAResourceResponse represents a resource from /cluster/ha/resources
type HAResourceResponse struct {
	SID         string `json:"sid"` // vm:100 or ct:200
	Type        string `json:"type"`
	State       string `json:"state"` // started, stopped, disabled
	Group       string `json:"group,omitempty"`
	MaxRestart  int    `json:"max_restart,omitempty"`
	MaxRelocate int    `json:"max_relocate,omitempty"`
	Comment     string `json:"comment,omitempty"`
	Digest      string `json:"digest,omitempty"`
}

// HAGroupResponse represents an HA group from /cluster/ha/groups
type HAGroupResponse struct {
	Group      string `json:"group"`
	Comment    string `json:"comment,omitempty"`
	Nodes      string `json:"nodes"` // comma-separated list
	NoFailback int    `json:"nofailback,omitempty"`
	Restricted int    `json:"restricted,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

// GetHAStatus returns the cluster's HA status
func (c *Client) GetHAStatus(ctx context.Context) (*HAStatus, error) {
	// Try to get HA manager status
	managerStatus, err := get[[]map[string]interface{}](c, ctx, "/cluster/ha/status/manager_status")
	if err != nil {
		// HA might not be configured
		return &HAStatus{Enabled: false}, nil
	}

	status := &HAStatus{
		Enabled: len(managerStatus) > 0,
	}

	// Parse manager info
	for _, item := range managerStatus {
		if itemType, ok := item["type"].(string); ok {
			if itemType == "manager" {
				if node, ok := item["node"].(string); ok {
					status.Manager.Node = node
				}
				if s, ok := item["status"].(string); ok {
					status.Manager.Status = s
				}
			}
			if itemType == "quorum" {
				if quorum, ok := item["quorate"].(float64); ok {
					status.Quorum = quorum == 1
				}
			}
		}
	}

	// Get HA resources
	resources, err := c.GetHAResources(ctx)
	if err == nil {
		for _, r := range resources {
			status.Resources = append(status.Resources, HAResourceState{
				SID:    r.SID,
				Type:   r.Type,
				State:  r.State,
			})
		}
	}

	return status, nil
}

// GetHAResources returns all HA-managed resources
func (c *Client) GetHAResources(ctx context.Context) ([]HAResourceResponse, error) {
	return get[[]HAResourceResponse](c, ctx, "/cluster/ha/resources")
}

// GetHAGroups returns all HA groups
// Note: Proxmox 8+ has migrated groups to rules - returns empty slice if not available
func (c *Client) GetHAGroups(ctx context.Context) ([]HAGroupResponse, error) {
	groups, err := get[[]HAGroupResponse](c, ctx, "/cluster/ha/groups")
	if err != nil {
		// HA groups may have been migrated to rules in newer Proxmox
		return []HAGroupResponse{}, nil
	}
	return groups, nil
}

// IsHAManaged checks if a guest is HA-managed
func (c *Client) IsHAManaged(ctx context.Context, guestType string, vmid int) (bool, string, error) {
	resources, err := c.GetHAResources(ctx)
	if err != nil {
		return false, "", nil // Assume not HA if we can't check
	}

	sid := fmt.Sprintf("%s:%d", guestType, vmid)
	for _, r := range resources {
		if r.SID == sid {
			return true, r.State, nil
		}
	}
	return false, "", nil
}

// HAResourceConfig holds configuration for enabling HA on a guest
type HAResourceConfig struct {
	State       string `json:"state,omitempty"`        // started, stopped, disabled
	Group       string `json:"group,omitempty"`        // HA group name
	MaxRestart  int    `json:"max_restart,omitempty"`  // Max restart attempts (0-10)
	MaxRelocate int    `json:"max_relocate,omitempty"` // Max relocate attempts (0-10)
	Comment     string `json:"comment,omitempty"`
}

// EnableHA adds a VM or container to HA management
func (c *Client) EnableHA(ctx context.Context, guestType string, vmid int, cfg HAResourceConfig) error {
	// guestType should be "vm" or "ct"
	sid := fmt.Sprintf("%s:%d", guestType, vmid)

	// Build form data
	data := url.Values{}
	data.Set("sid", sid)
	if cfg.State != "" {
		data.Set("state", cfg.State)
	} else {
		data.Set("state", "started") // Default to started
	}
	if cfg.Group != "" {
		data.Set("group", cfg.Group)
	}
	if cfg.MaxRestart > 0 {
		data.Set("max_restart", fmt.Sprintf("%d", cfg.MaxRestart))
	}
	if cfg.MaxRelocate > 0 {
		data.Set("max_relocate", fmt.Sprintf("%d", cfg.MaxRelocate))
	}
	if cfg.Comment != "" {
		data.Set("comment", cfg.Comment)
	}

	return c.postForm(ctx, "/cluster/ha/resources", data)
}

// DisableHA removes a VM or container from HA management
func (c *Client) DisableHA(ctx context.Context, guestType string, vmid int) error {
	sid := fmt.Sprintf("%s:%d", guestType, vmid)
	return c.delete(ctx, fmt.Sprintf("/cluster/ha/resources/%s", sid))
}

// UpdateHA updates HA settings for a managed resource
func (c *Client) UpdateHA(ctx context.Context, guestType string, vmid int, cfg HAResourceConfig) error {
	sid := fmt.Sprintf("%s:%d", guestType, vmid)

	data := url.Values{}
	if cfg.State != "" {
		data.Set("state", cfg.State)
	}
	if cfg.Group != "" {
		data.Set("group", cfg.Group)
	}
	if cfg.MaxRestart >= 0 {
		data.Set("max_restart", fmt.Sprintf("%d", cfg.MaxRestart))
	}
	if cfg.MaxRelocate >= 0 {
		data.Set("max_relocate", fmt.Sprintf("%d", cfg.MaxRelocate))
	}
	if cfg.Comment != "" {
		data.Set("comment", cfg.Comment)
	}

	return c.putForm(ctx, fmt.Sprintf("/cluster/ha/resources/%s", sid), data)
}

// postForm sends a POST request with form data
func (c *Client) postForm(ctx context.Context, path string, data url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// putForm sends a PUT request with form data
func (c *Client) putForm(ctx context.Context, path string, data url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+path, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// delete sends a DELETE request
func (c *Client) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
