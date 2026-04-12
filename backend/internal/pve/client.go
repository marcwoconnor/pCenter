package pve

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moconnor/pcenter/internal/config"
)

// runSSHCommand executes a command on a remote host via SSH
func runSSHCommand(ctx context.Context, host string, command string) (string, error) {
	slog.Info("running SSH command", "host", host, "command", command)

	// Enforce 30s hard timeout if context has no deadline
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	// accept-new: accept on first connection (TOFU), reject if key changes (MITM).
	// Previously used StrictHostKeyChecking=no which silently accepts changed keys.
	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		fmt.Sprintf("root@%s", host),
		command,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if err != nil {
		return output, fmt.Errorf("SSH command failed: %w\nOutput: %s", err, output)
	}

	return output, nil
}

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

// AuthResult contains the result of password authentication
type AuthResult struct {
	Ticket    string
	CSRFToken string
	Username  string
}

// TokenResult contains the newly created API token
type TokenResult struct {
	TokenID string
	Secret  string
}

// AuthenticateWithPassword authenticates to a PVE host using username/password
// Returns a ticket and CSRF token for subsequent requests
func AuthenticateWithPassword(ctx context.Context, address, username, password string, insecure bool) (*AuthResult, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	reqURL := fmt.Sprintf("https://%s/api2/json/access/ticket", address)
	data := url.Values{}
	data.Set("username", username)
	data.Set("password", password)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("authentication failed: %s", string(body))
	}

	var result struct {
		Data struct {
			Ticket              string `json:"ticket"`
			CSRFPreventionToken string `json:"CSRFPreventionToken"`
			Username            string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse auth response: %w", err)
	}

	return &AuthResult{
		Ticket:    result.Data.Ticket,
		CSRFToken: result.Data.CSRFPreventionToken,
		Username:  result.Data.Username,
	}, nil
}

// CreateAPIToken creates a new API token using a ticket from password auth
// tokenName is just the suffix (e.g., "pcenter"), full token ID will be "user@realm!tokenName"
func CreateAPIToken(ctx context.Context, address string, auth *AuthResult, tokenName string, insecure bool) (*TokenResult, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	// Token endpoint: /access/users/{userid}/token/{tokenid}
	reqURL := fmt.Sprintf("https://%s/api2/json/access/users/%s/token/%s",
		address, url.PathEscape(auth.Username), tokenName)

	// Set privsep=0 so token has same permissions as user
	data := url.Values{}
	data.Set("privsep", "0")

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Use ticket cookie and CSRF token for auth
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("CSRFPreventionToken", auth.CSRFToken)
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: auth.Ticket})

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		// If token already exists, delete and recreate
		if strings.Contains(string(body), "already exists") {
			slog.Info("API token already exists, recreating", "token", tokenName)
			delURL := fmt.Sprintf("https://%s/api2/json/access/users/%s/token/%s",
				address, url.PathEscape(auth.Username), tokenName)
			delReq, _ := http.NewRequestWithContext(ctx, "DELETE", delURL, nil)
			delReq.Header.Set("CSRFPreventionToken", auth.CSRFToken)
			delReq.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: auth.Ticket})
			client.Do(delReq)
			// Retry creation
			return CreateAPIToken(ctx, address, auth, tokenName, insecure)
		}
		return nil, fmt.Errorf("create token failed: %s", string(body))
	}

	var result struct {
		Data struct {
			FullTokenID string `json:"full-tokenid"`
			Value       string `json:"value"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return &TokenResult{
		TokenID: result.Data.FullTokenID,
		Secret:  result.Data.Value,
	}, nil
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

// GetNodeDetails returns detailed node info (version, kernel, CPU model, etc.)
func (c *Client) GetNodeDetails(ctx context.Context) (*NodeStatus, error) {
	raw, err := get[NodeStatusResponse](c, ctx, fmt.Sprintf("/nodes/%s/status", c.nodeName))
	if err != nil {
		return nil, err
	}
	return &NodeStatus{
		PVEVersion:    raw.PVEVersion,
		KernelVersion: raw.KVersion,
		CPUModel:      raw.CPUInfo.Model,
		CPUCores:      raw.CPUInfo.Cores,
		CPUSockets:    raw.CPUInfo.Sockets,
		BootMode:      raw.BootInfo.Mode,
		LoadAvg:       raw.LoadAvg,
	}, nil
}

// GetNextVMID returns the next available VMID for the cluster
func (c *Client) GetNextVMID(ctx context.Context) (int, error) {
	data, err := c.request(ctx, http.MethodGet, "/cluster/nextid", nil)
	if err != nil {
		return 0, err
	}
	var resp APIResponse[string]
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("unmarshal nextid: %w", err)
	}
	var vmid int
	fmt.Sscanf(resp.Data, "%d", &vmid)
	return vmid, nil
}

// --- VM operations ---

var netKeyRe = regexp.MustCompile(`^net(\d+)$`)

// parseNICsFromConfig extracts GuestNICs from a VM/CT config map.
// VM net values look like: "virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0,tag=501"
// CT net values look like: "name=eth0,bridge=vmbr0,hwaddr=AA:BB:CC:DD:EE:FF"
func parseNICsFromConfig(cfg map[string]interface{}) []GuestNIC {
	var nics []GuestNIC
	for key, val := range cfg {
		if !netKeyRe.MatchString(key) {
			continue
		}
		s, ok := val.(string)
		if !ok {
			continue
		}
		nic := GuestNIC{Name: key}
		for _, part := range strings.Split(s, ",") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				// First token for VMs is "model=MAC" e.g. "virtio=AA:BB..."
				if strings.Contains(part, ":") {
					// bare MAC (shouldn't happen), skip
				} else {
					nic.Model = part
				}
				continue
			}
			switch kv[0] {
			case "bridge":
				nic.Bridge = kv[1]
			case "hwaddr":
				nic.MAC = kv[1]
			case "tag":
				if v, err := strconv.Atoi(kv[1]); err == nil {
					nic.Tag = v
				}
			case "name":
				// CT interface name (eth0, etc) — keep key as Name for consistency
			case "virtio", "e1000", "rtl8139", "vmxnet3":
				nic.Model = kv[0]
				nic.MAC = kv[1]
			}
		}
		if nic.Bridge != "" {
			nics = append(nics, nic)
		}
	}
	return nics
}

// GetVMs returns all VMs on this node, including NIC info from config
func (c *Client) GetVMs(ctx context.Context) ([]VM, error) {
	vms, err := get[[]VM](c, ctx, fmt.Sprintf("/nodes/%s/qemu", c.nodeName))
	if err != nil {
		return nil, err
	}
	// Tag with node name
	for i := range vms {
		vms[i].Node = c.nodeName
	}
	c.fetchVMNICs(ctx, vms)
	return vms, nil
}

// fetchVMNICs fetches config for each VM in parallel and populates NICs.
func (c *Client) fetchVMNICs(ctx context.Context, vms []VM) {
	var wg sync.WaitGroup
	for i := range vms {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cfg, err := get[map[string]interface{}](c, ctx, fmt.Sprintf("/nodes/%s/qemu/%d/config", c.nodeName, vms[idx].VMID))
			if err == nil {
				vms[idx].NICs = parseNICsFromConfig(cfg)
			}
		}(i)
	}
	wg.Wait()
}

// fetchCTNICs fetches config for each container in parallel and populates NICs.
func (c *Client) fetchCTNICs(ctx context.Context, cts []Container) {
	var wg sync.WaitGroup
	for i := range cts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cfg, err := get[map[string]interface{}](c, ctx, fmt.Sprintf("/nodes/%s/lxc/%d/config", c.nodeName, cts[idx].VMID))
			if err == nil {
				cts[idx].NICs = parseNICsFromConfig(cfg)
			}
		}(i)
	}
	wg.Wait()
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

// GetVMConfig returns the full configuration for a VM
func (c *Client) GetVMConfig(ctx context.Context, vmid int) (*VMConfig, error) {
	raw, err := get[map[string]interface{}](c, ctx, fmt.Sprintf("/nodes/%s/qemu/%d/config", c.nodeName, vmid))
	if err != nil {
		return nil, err
	}
	return parseVMConfig(raw), nil
}

// UpdateVMConfig updates VM configuration with optimistic locking via digest
func (c *Client) UpdateVMConfig(ctx context.Context, vmid int, req *ConfigUpdateRequest) error {
	data := url.Values{}
	data.Set("digest", req.Digest)

	// Add changes
	for key, value := range req.Changes {
		data.Set(key, fmt.Sprintf("%v", value))
	}

	// Add delete list if any
	if len(req.Delete) > 0 {
		data.Set("delete", strings.Join(req.Delete, ","))
	}

	return c.putForm(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/config", c.nodeName, vmid), data)
}

// parseVMConfig converts raw API response to structured VMConfig
func parseVMConfig(raw map[string]interface{}) *VMConfig {
	cfg := &VMConfig{RawConfig: raw}

	// Extract known fields
	if v, ok := raw["digest"].(string); ok {
		cfg.Digest = v
	}
	if v, ok := raw["name"].(string); ok {
		cfg.Name = v
	}
	if v, ok := raw["description"].(string); ok {
		cfg.Description = v
	}

	// Hardware
	if v, ok := raw["cores"].(float64); ok {
		cfg.Cores = int(v)
	}
	if v, ok := raw["sockets"].(float64); ok {
		cfg.Sockets = int(v)
	}
	if v, ok := raw["cpu"].(string); ok {
		cfg.CPU = v
	}
	if v, ok := raw["memory"].(float64); ok {
		cfg.Memory = int(v)
	}
	if v, ok := raw["balloon"].(float64); ok {
		cfg.Balloon = int(v)
	}
	if v, ok := raw["numa"].(float64); ok {
		cfg.Numa = int(v)
	}
	if v, ok := raw["bios"].(string); ok {
		cfg.BIOS = v
	}
	if v, ok := raw["machine"].(string); ok {
		cfg.Machine = v
	}

	// Boot
	if v, ok := raw["boot"].(string); ok {
		cfg.Boot = v
	}
	if v, ok := raw["bootdisk"].(string); ok {
		cfg.Bootdisk = v
	}

	// Options
	if v, ok := raw["onboot"].(float64); ok {
		cfg.Onboot = int(v)
	}
	if v, ok := raw["protection"].(float64); ok {
		cfg.Protection = int(v)
	}
	if v, ok := raw["agent"].(string); ok {
		cfg.Agent = v
	}
	if v, ok := raw["ostype"].(string); ok {
		cfg.Ostype = v
	}

	// Cloud-init
	if v, ok := raw["ciuser"].(string); ok {
		cfg.CIUser = v
	}
	if v, ok := raw["sshkeys"].(string); ok {
		cfg.SSHKeys = v
	}
	if v, ok := raw["ipconfig0"].(string); ok {
		cfg.IPConfig0 = v
	}
	if v, ok := raw["ipconfig1"].(string); ok {
		cfg.IPConfig1 = v
	}
	if v, ok := raw["nameserver"].(string); ok {
		cfg.Nameserver = v
	}
	if v, ok := raw["searchdomain"].(string); ok {
		cfg.Searchdomain = v
	}

	// VGA
	if v, ok := raw["vga"].(string); ok {
		cfg.VGA = v
	}

	return cfg
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

// CreateVM creates a new QEMU virtual machine
func (c *Client) CreateVM(ctx context.Context, req *CreateVMRequest) (string, error) {
	params := map[string]string{
		"vmid":   fmt.Sprintf("%d", req.VMID),
		"name":   req.Name,
		"cores":  fmt.Sprintf("%d", req.Cores),
		"memory": fmt.Sprintf("%d", req.Memory),
	}

	// Add disk if storage specified
	if req.Storage != "" && req.DiskSize > 0 {
		params["scsi0"] = fmt.Sprintf("%s:%d", req.Storage, req.DiskSize)
		params["scsihw"] = "virtio-scsi-single"
	}

	// Add ISO if specified
	if req.ISO != "" {
		params["ide2"] = req.ISO + ",media=cdrom"
		params["boot"] = "order=ide2;scsi0"
	}

	// Add OS type
	if req.OSType != "" {
		params["ostype"] = req.OSType
	} else {
		params["ostype"] = "l26" // Default to Linux
	}

	// Add network if specified
	if req.Network != "" {
		params["net0"] = fmt.Sprintf("virtio,bridge=%s", req.Network)
	} else {
		params["net0"] = "virtio,bridge=vmbr0" // Default network
	}

	// Start after creation if requested
	if req.Start {
		params["start"] = "1"
	}

	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu", c.nodeName), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil // returns UPID
}

// DeleteVM deletes a VM (must be stopped first)
func (c *Client) DeleteVM(ctx context.Context, vmid int, purge bool) (string, error) {
	path := fmt.Sprintf("/nodes/%s/qemu/%d", c.nodeName, vmid)
	if purge {
		path += "?purge=1&destroy-unreferenced-disks=1"
	}
	data, err := c.deleteWithData(ctx, path)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil // returns UPID
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
	c.fetchCTNICs(ctx, cts)
	return cts, nil
}

// GetContainerConfig returns the full configuration for a container
func (c *Client) GetContainerConfig(ctx context.Context, vmid int) (*ContainerConfig, error) {
	raw, err := get[map[string]interface{}](c, ctx, fmt.Sprintf("/nodes/%s/lxc/%d/config", c.nodeName, vmid))
	if err != nil {
		return nil, err
	}
	return parseContainerConfig(raw), nil
}

// UpdateContainerConfig updates container configuration with optimistic locking via digest
func (c *Client) UpdateContainerConfig(ctx context.Context, vmid int, req *ConfigUpdateRequest) error {
	data := url.Values{}
	data.Set("digest", req.Digest)

	// Add changes
	for key, value := range req.Changes {
		data.Set(key, fmt.Sprintf("%v", value))
	}

	// Add delete list if any
	if len(req.Delete) > 0 {
		data.Set("delete", strings.Join(req.Delete, ","))
	}

	return c.putForm(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/config", c.nodeName, vmid), data)
}

// parseContainerConfig converts raw API response to structured ContainerConfig
func parseContainerConfig(raw map[string]interface{}) *ContainerConfig {
	cfg := &ContainerConfig{RawConfig: raw}

	// Extract known fields
	if v, ok := raw["digest"].(string); ok {
		cfg.Digest = v
	}
	if v, ok := raw["hostname"].(string); ok {
		cfg.Hostname = v
	}
	if v, ok := raw["description"].(string); ok {
		cfg.Description = v
	}

	// Resources
	if v, ok := raw["cores"].(float64); ok {
		cfg.Cores = int(v)
	}
	if v, ok := raw["cpulimit"].(float64); ok {
		cfg.CPULimit = v
	}
	if v, ok := raw["cpuunits"].(float64); ok {
		cfg.CPUUnits = int(v)
	}
	if v, ok := raw["memory"].(float64); ok {
		cfg.Memory = int(v)
	}
	if v, ok := raw["swap"].(float64); ok {
		cfg.Swap = int(v)
	}

	// Root filesystem
	if v, ok := raw["rootfs"].(string); ok {
		cfg.Rootfs = v
	}

	// Options
	if v, ok := raw["onboot"].(float64); ok {
		cfg.Onboot = int(v)
	}
	if v, ok := raw["protection"].(float64); ok {
		cfg.Protection = int(v)
	}
	if v, ok := raw["unprivileged"].(float64); ok {
		cfg.Unprivileged = int(v)
	}
	if v, ok := raw["ostype"].(string); ok {
		cfg.Ostype = v
	}
	if v, ok := raw["arch"].(string); ok {
		cfg.Arch = v
	}

	// Features
	if v, ok := raw["features"].(string); ok {
		cfg.Features = v
	}

	// Startup
	if v, ok := raw["startup"].(string); ok {
		cfg.Startup = v
	}

	return cfg
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

// CreateContainer creates a new LXC container
func (c *Client) CreateContainer(ctx context.Context, req *CreateContainerRequest) (string, error) {
	params := map[string]string{
		"vmid":       fmt.Sprintf("%d", req.VMID),
		"hostname":   req.Hostname,
		"ostemplate": req.Template,
		"cores":      fmt.Sprintf("%d", req.Cores),
		"memory":     fmt.Sprintf("%d", req.Memory),
		"swap":       fmt.Sprintf("%d", req.Swap),
	}

	// Root filesystem
	if req.Storage != "" && req.DiskSize > 0 {
		params["rootfs"] = fmt.Sprintf("%s:%d", req.Storage, req.DiskSize)
	}

	// Network
	if req.Network != "" {
		params["net0"] = fmt.Sprintf("name=eth0,bridge=%s,ip=dhcp", req.Network)
	} else {
		params["net0"] = "name=eth0,bridge=vmbr0,ip=dhcp"
	}

	// Authentication
	if req.Password != "" {
		params["password"] = req.Password
	}
	if req.SSHKeys != "" {
		params["ssh-public-keys"] = req.SSHKeys
	}

	// Unprivileged container (default true for security)
	if req.Unprivileged {
		params["unprivileged"] = "1"
	} else {
		params["unprivileged"] = "0"
	}

	// Start after creation
	if req.Start {
		params["start"] = "1"
	}

	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc", c.nodeName), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil // returns UPID
}

// DeleteContainer deletes a container (must be stopped first)
func (c *Client) DeleteContainer(ctx context.Context, vmid int, purge bool) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d", c.nodeName, vmid)
	if purge {
		path += "?purge=1&destroy-unreferenced-disks=1"
	}
	data, err := c.deleteWithData(ctx, path)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil // returns UPID
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

// GetStorageContent returns all volumes on a storage
func (c *Client) GetStorageContent(ctx context.Context, storageName string) ([]StorageVolume, error) {
	return get[[]StorageVolume](c, ctx, fmt.Sprintf("/nodes/%s/storage/%s/content", c.nodeName, storageName))
}

// UploadToStorage uploads a file to storage (ISO, template, etc.)
func (c *Client) UploadToStorage(ctx context.Context, storageName, contentType, filename string, file io.Reader, size int64) (string, error) {
	path := fmt.Sprintf("/nodes/%s/storage/%s/upload", c.nodeName, storageName)

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add content type field
	if err := writer.WriteField("content", contentType); err != nil {
		return "", fmt.Errorf("failed to write content field: %w", err)
	}

	// Add file
	part, err := writer.CreateFormFile("filename", filename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("failed to copy file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, &buf)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
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
		return "", fmt.Errorf("upload failed: %s", string(data))
	}

	// Parse response to get UPID
	var apiResp struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return apiResp.Data, nil
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

// TaskLogEntry represents a single line in the task log
type TaskLogEntry struct {
	N int    `json:"n"` // line number
	T string `json:"t"` // text
}

// GetTaskLog returns the task log entries
func (c *Client) GetTaskLog(ctx context.Context, upid string, limit int) ([]TaskLogEntry, error) {
	if limit == 0 {
		limit = 50
	}
	entries, err := get[[]TaskLogEntry](c, ctx, fmt.Sprintf("/nodes/%s/tasks/%s/log?limit=%d", c.nodeName, url.PathEscape(upid), limit))
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// GetTaskError fetches the task log and extracts the error message
func (c *Client) GetTaskError(ctx context.Context, upid string) string {
	entries, err := c.GetTaskLog(ctx, upid, 20)
	if err != nil {
		return ""
	}

	// Look for ERROR: line which has the detailed message
	for _, entry := range entries {
		if strings.Contains(entry.T, "ERROR:") {
			// Extract the part after ERROR:
			parts := strings.SplitN(entry.T, "ERROR:", 2)
			if len(parts) == 2 {
				// Clean up - remove timestamp prefix if present, extract the actual error
				errMsg := strings.TrimSpace(parts[1])
				// Format: "migration aborted (duration 00:00:00): actual error message"
				if colonIdx := strings.LastIndex(errMsg, "):"); colonIdx != -1 {
					return strings.TrimSpace(errMsg[colonIdx+2:])
				}
				return errMsg
			}
		}
	}
	return ""
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

// RunCephCommand executes a whitelisted Ceph command via SSH
func (c *Client) RunCephCommand(ctx context.Context, command string, pgID string) (string, error) {
	// Build the full command based on the command type
	var fullCmd string
	switch command {
	case "pg_repair":
		if pgID == "" {
			return "", fmt.Errorf("pg_id is required for pg_repair")
		}
		// Validate PG ID format (e.g., "4.3b" - pool.pg_num in hex)
		if !isValidPgID(pgID) {
			return "", fmt.Errorf("invalid pg_id format: %s", pgID)
		}
		fullCmd = fmt.Sprintf("ceph pg repair %s", pgID)
	case "health_detail":
		fullCmd = "ceph health detail"
	case "osd_tree":
		fullCmd = "ceph osd tree"
	case "status":
		fullCmd = "ceph status"
	case "pg_query":
		if pgID == "" {
			return "", fmt.Errorf("pg_id is required for pg_query")
		}
		if !isValidPgID(pgID) {
			return "", fmt.Errorf("invalid pg_id format: %s", pgID)
		}
		fullCmd = fmt.Sprintf("ceph pg %s query | head -20", pgID)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}

	// Get the node hostname from the base URL
	host := c.getHostFromURL()
	if host == "" {
		return "", fmt.Errorf("could not determine node host")
	}

	// Execute via SSH
	return runSSHCommand(ctx, host, fullCmd)
}

// isValidPgID validates the format of a Ceph PG ID (e.g., "4.3b")
func isValidPgID(pgID string) bool {
	parts := strings.Split(pgID, ".")
	if len(parts) != 2 {
		return false
	}
	// First part should be a number (pool ID)
	for _, ch := range parts[0] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	// Second part should be hex (pg num)
	for _, ch := range parts[1] {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return len(parts[0]) > 0 && len(parts[1]) > 0
}

// getHostFromURL extracts the hostname from the client's base URL
func (c *Client) getHostFromURL() string {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

// GetSmartData fetches SMART data for all disks on this node
func (c *Client) GetSmartData(ctx context.Context) ([]SmartDisk, error) {
	host := c.getHostFromURL()
	if host == "" {
		return nil, fmt.Errorf("could not determine node host")
	}

	// Get list of SMART-capable devices
	scanOutput, err := runSSHCommand(ctx, host, "smartctl --scan -j")
	if err != nil {
		return nil, fmt.Errorf("smartctl scan failed: %w", err)
	}

	var scanResult struct {
		Devices []struct {
			Name     string `json:"name"`
			InfoName string `json:"info_name"`
			Type     string `json:"type"`
			Protocol string `json:"protocol"`
		} `json:"devices"`
	}
	if err := json.Unmarshal([]byte(scanOutput), &scanResult); err != nil {
		return nil, fmt.Errorf("failed to parse smartctl scan: %w", err)
	}

	var disks []SmartDisk
	criticalAttrs := map[int]bool{
		5:   true, // Reallocated_Sector_Ct
		10:  true, // Spin_Retry_Count
		196: true, // Reallocated_Event_Count
		197: true, // Current_Pending_Sector
		198: true, // Offline_Uncorrectable
	}

	for _, dev := range scanResult.Devices {
		// Skip rbd devices and similar
		if strings.HasPrefix(dev.Name, "/dev/rbd") {
			continue
		}

		// Get detailed SMART data for this device
		smartOutput, err := runSSHCommand(ctx, host, fmt.Sprintf("smartctl -j -a %s", dev.Name))
		if err != nil {
			slog.Warn("failed to get SMART data", "device", dev.Name, "error", err)
			continue
		}

		disk := parseSmartJSON(smartOutput, dev.Name, c.nodeName, criticalAttrs)
		if disk != nil {
			disk.Cluster = c.clusterName
			disks = append(disks, *disk)
		}
	}

	return disks, nil
}

// parseSmartJSON parses smartctl JSON output into a SmartDisk
func parseSmartJSON(jsonData string, device string, node string, criticalAttrs map[int]bool) *SmartDisk {
	var data struct {
		Device struct {
			Name     string `json:"name"`
			Protocol string `json:"protocol"`
		} `json:"device"`
		ModelName       string `json:"model_name"`
		ScsiModelName   string `json:"scsi_model_name"`
		ScsiVendor      string `json:"scsi_vendor"`
		ScsiProduct     string `json:"scsi_product"`
		SerialNumber    string `json:"serial_number"`
		ScsiSerial      string `json:"scsi_serial"`
		FirmwareVersion string `json:"firmware_version"`
		UserCapacity   struct {
			Bytes int64 `json:"bytes"`
		} `json:"user_capacity"`
		RotationRate int `json:"rotation_rate"` // 0 for SSD/NVMe
		SmartStatus  struct {
			Passed bool `json:"passed"`
		} `json:"smart_status"`
		ATASmartAttributes struct {
			Revision int `json:"revision"`
			Table    []struct {
				ID         int    `json:"id"`
				Name       string `json:"name"`
				Value      int    `json:"value"`
				Worst      int    `json:"worst"`
				Thresh     int    `json:"thresh"`
				WhenFailed string `json:"when_failed"`
				Flags      struct {
					String string `json:"string"`
				} `json:"flags"`
				Raw struct {
					Value int64 `json:"value"`
				} `json:"raw"`
			} `json:"table"`
		} `json:"ata_smart_attributes"`
		NVMeSmartHealthLog struct {
			CriticalWarning         int   `json:"critical_warning"`
			Temperature             int   `json:"temperature"`
			AvailableSpare          int   `json:"available_spare"`
			AvailableSpareThreshold int   `json:"available_spare_threshold"`
			PercentageUsed          int   `json:"percentage_used"`
			DataUnitsRead           int64 `json:"data_units_read"`
			DataUnitsWritten        int64 `json:"data_units_written"`
			PowerCycles             int64 `json:"power_cycles"`
			PowerOnHours            int64 `json:"power_on_hours"`
			UnsafeShutdowns         int64 `json:"unsafe_shutdowns"`
			MediaErrors             int64 `json:"media_and_data_integrity_errors"`
			ErrorInfoLogEntries     int64 `json:"num_err_log_entries"`
		} `json:"nvme_smart_health_information_log"`
		Temperature struct {
			Current int `json:"current"`
		} `json:"temperature"`
		PowerOnTime struct {
			Hours int64 `json:"hours"`
		} `json:"power_on_time"`
	}

	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		slog.Warn("failed to parse SMART JSON", "device", device, "error", err)
		return nil
	}

	// Use SCSI fields as fallback for model and serial
	model := data.ModelName
	if model == "" {
		model = data.ScsiModelName
	}
	if model == "" && data.ScsiVendor != "" {
		model = data.ScsiVendor + " " + data.ScsiProduct
	}

	serial := data.SerialNumber
	if serial == "" {
		serial = data.ScsiSerial
	}

	disk := &SmartDisk{
		Node:     node,
		Device:   device,
		Model:    model,
		Serial:   serial,
		Capacity: data.UserCapacity.Bytes,
		Protocol: data.Device.Protocol,
	}

	// Determine disk type
	if data.Device.Protocol == "NVMe" {
		disk.Type = "nvme"
	} else if data.RotationRate == 0 {
		disk.Type = "ssd"
	} else {
		disk.Type = "hdd"
	}

	// Health status
	if data.SmartStatus.Passed {
		disk.Health = "PASSED"
	} else {
		disk.Health = "FAILED"
	}

	// Temperature and power-on hours
	if data.Device.Protocol == "NVMe" {
		disk.Temperature = data.NVMeSmartHealthLog.Temperature
		disk.PowerOnHours = data.NVMeSmartHealthLog.PowerOnHours

		// NVMe specific health data
		disk.NVMeHealth = &NVMeHealth{
			CriticalWarning:      data.NVMeSmartHealthLog.CriticalWarning,
			AvailableSpare:       data.NVMeSmartHealthLog.AvailableSpare,
			AvailableSpareThresh: data.NVMeSmartHealthLog.AvailableSpareThreshold,
			PercentUsed:          data.NVMeSmartHealthLog.PercentageUsed,
			DataUnitsRead:        data.NVMeSmartHealthLog.DataUnitsRead,
			DataUnitsWritten:     data.NVMeSmartHealthLog.DataUnitsWritten,
			PowerCycles:          data.NVMeSmartHealthLog.PowerCycles,
			UnsafeShutdowns:      data.NVMeSmartHealthLog.UnsafeShutdowns,
			MediaErrors:          data.NVMeSmartHealthLog.MediaErrors,
			ErrorLogEntries:      data.NVMeSmartHealthLog.ErrorInfoLogEntries,
		}

		// Override health if critical warning or media errors
		if data.NVMeSmartHealthLog.CriticalWarning > 0 || data.NVMeSmartHealthLog.MediaErrors > 0 {
			disk.Health = "WARNING"
		}
	} else {
		disk.Temperature = data.Temperature.Current
		disk.PowerOnHours = data.PowerOnTime.Hours

		// Parse ATA SMART attributes
		for _, attr := range data.ATASmartAttributes.Table {
			smartAttr := SmartAttribute{
				ID:         attr.ID,
				Name:       attr.Name,
				Value:      attr.Value,
				Worst:      attr.Worst,
				Threshold:  attr.Thresh,
				Raw:        attr.Raw.Value,
				Flags:      attr.Flags.String,
				WhenFailed: attr.WhenFailed,
				Critical:   criticalAttrs[attr.ID],
			}

			// Check for critical attribute failures
			if smartAttr.Critical && smartAttr.Raw > 0 {
				disk.Health = "WARNING"
			}

			disk.Attributes = append(disk.Attributes, smartAttr)
		}
	}

	return disk
}

// --- QDevice & Maintenance ---

// GetQDeviceStatus returns the qdevice status from this node
func (c *Client) GetQDeviceStatus(ctx context.Context) (*QDeviceStatus, error) {
	host := c.getHostFromURL()
	if host == "" {
		return nil, fmt.Errorf("could not determine node host")
	}

	output, err := runSSHCommand(ctx, host, "corosync-qdevice-tool -s 2>/dev/null || echo 'NOT_CONFIGURED'")
	if err != nil {
		return nil, fmt.Errorf("failed to get qdevice status: %w", err)
	}

	status := &QDeviceStatus{
		Configured: !strings.Contains(output, "NOT_CONFIGURED"),
	}

	if !status.Configured {
		return status, nil
	}

	// Parse the output
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "QNetd host:") {
			status.QNetdAddress = strings.TrimSpace(strings.TrimPrefix(line, "QNetd host:"))
		} else if strings.HasPrefix(line, "Algorithm:") {
			status.Algorithm = strings.TrimSpace(strings.TrimPrefix(line, "Algorithm:"))
		} else if strings.HasPrefix(line, "State:") {
			status.State = strings.TrimSpace(strings.TrimPrefix(line, "State:"))
			status.Connected = status.State == "Connected"
		}
	}

	return status, nil
}

// FindQDeviceVM finds the VM running the qdevice (qnetd) server
func (c *Client) FindQDeviceVM(ctx context.Context, qnetdIP string) (*QDeviceStatus, error) {
	// We need to find which VM has this IP
	// This requires checking VM guest agent network info
	// For now, we'll identify by name pattern "osd-mon" or by checking the IP

	// Get all VMs on this node
	vms, err := c.GetVMs(ctx)
	if err != nil {
		return nil, err
	}

	for _, vm := range vms {
		// Check if VM name matches qdevice pattern
		if strings.Contains(strings.ToLower(vm.Name), "osd-mon") ||
			strings.Contains(strings.ToLower(vm.Name), "qdevice") ||
			strings.Contains(strings.ToLower(vm.Name), "quorum") {
			return &QDeviceStatus{
				HostNode:   c.nodeName,
				HostVMID:   vm.VMID,
				HostVMName: vm.Name,
			}, nil
		}
	}

	return nil, nil
}

// GetMaintenancePreflight performs pre-flight checks for maintenance mode
func (c *Client) GetMaintenancePreflight(ctx context.Context, targetNode string, allNodes []string) (*MaintenancePreflight, error) {
	preflight := &MaintenancePreflight{
		Node:     targetNode,
		CanEnter: true,
	}

	host := c.getHostFromURL()
	if host == "" {
		return nil, fmt.Errorf("could not determine node host")
	}

	// Check 1: Verify we have another node to migrate to
	var otherNode string
	for _, n := range allNodes {
		if n != targetNode {
			otherNode = n
			break
		}
	}
	if otherNode == "" {
		preflight.Checks = append(preflight.Checks, MaintenancePreflightCheck{
			Name:     "Target Node Available",
			Status:   "error",
			Message:  "No other node available for migration",
			Blocking: true,
		})
		preflight.CanEnter = false
		return preflight, nil
	}
	preflight.Checks = append(preflight.Checks, MaintenancePreflightCheck{
		Name:    "Target Node Available",
		Status:  "ok",
		Message: fmt.Sprintf("Guests will migrate to %s", otherNode),
	})

	// Check 2: Ceph health
	cephOutput, err := runSSHCommand(ctx, host, "ceph health 2>/dev/null || echo 'CEPH_NOT_AVAILABLE'")
	if err == nil && !strings.Contains(cephOutput, "CEPH_NOT_AVAILABLE") {
		cephHealth := strings.TrimSpace(cephOutput)
		if cephHealth == "HEALTH_OK" {
			preflight.Checks = append(preflight.Checks, MaintenancePreflightCheck{
				Name:    "Ceph Health",
				Status:  "ok",
				Message: "Ceph cluster is healthy",
			})
		} else if strings.Contains(cephHealth, "HEALTH_WARN") {
			preflight.Checks = append(preflight.Checks, MaintenancePreflightCheck{
				Name:    "Ceph Health",
				Status:  "warning",
				Message: fmt.Sprintf("Ceph has warnings: %s", cephHealth),
			})
		} else {
			preflight.Checks = append(preflight.Checks, MaintenancePreflightCheck{
				Name:     "Ceph Health",
				Status:   "error",
				Message:  fmt.Sprintf("Ceph is unhealthy: %s", cephHealth),
				Blocking: true,
			})
			preflight.CanEnter = false
		}
	}

	// Check 3: QDevice status
	qdeviceOutput, err := runSSHCommand(ctx, host, "corosync-qdevice-tool -s 2>/dev/null | grep -E 'State:|QNetd host:' || echo 'NO_QDEVICE'")
	if err == nil && !strings.Contains(qdeviceOutput, "NO_QDEVICE") {
		if strings.Contains(qdeviceOutput, "Connected") {
			preflight.Checks = append(preflight.Checks, MaintenancePreflightCheck{
				Name:    "QDevice Connected",
				Status:  "ok",
				Message: "Cluster qdevice is connected and will maintain quorum",
			})
		} else {
			preflight.Checks = append(preflight.Checks, MaintenancePreflightCheck{
				Name:     "QDevice Connected",
				Status:   "error",
				Message:  "QDevice is not connected - maintenance would break quorum",
				Blocking: true,
			})
			preflight.CanEnter = false
		}
	}

	// Check 4: Set Ceph noout flag reminder
	preflight.Checks = append(preflight.Checks, MaintenancePreflightCheck{
		Name:    "Ceph OSD Flags",
		Status:  "warning",
		Message: "Will set 'noout' flag to prevent OSD rebalancing during maintenance",
	})

	return preflight, nil
}

// SetCephNoout sets or unsets the Ceph noout flag
func (c *Client) SetCephNoout(ctx context.Context, enable bool) error {
	host := c.getHostFromURL()
	if host == "" {
		return fmt.Errorf("could not determine node host")
	}

	cmd := "ceph osd unset noout"
	if enable {
		cmd = "ceph osd set noout"
	}

	_, err := runSSHCommand(ctx, host, cmd)
	return err
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
		// Note: LXC live migration is not currently implemented in Proxmox
		// This will likely fail, but we pass it through for future compatibility
		params["online"] = "1"
	} else {
		// For running containers without live migration, use restart mode
		// This stops the container, migrates, and starts it on the new node
		params["restart"] = "1"
	}
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/migrate", c.nodeName, vmid), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// --- Clone operations ---

// CloneOptions contains options for cloning a VM or container
type CloneOptions struct {
	NewID       int    // Required: VMID for the clone
	Name        string // Name for the clone
	TargetNode  string // Target node (empty = same node)
	Full        bool   // Full clone (true) vs linked clone (false)
	Storage     string // Target storage (empty = same as source)
	Description string // Description for the clone
}

// CloneVM clones a VM
func (c *Client) CloneVM(ctx context.Context, vmid int, opts CloneOptions) (string, error) {
	params := map[string]string{
		"newid": fmt.Sprintf("%d", opts.NewID),
	}
	if opts.Name != "" {
		params["name"] = opts.Name
	}
	if opts.TargetNode != "" {
		params["target"] = opts.TargetNode
	}
	if opts.Full {
		params["full"] = "1"
	}
	if opts.Storage != "" {
		params["storage"] = opts.Storage
	}
	if opts.Description != "" {
		params["description"] = opts.Description
	}

	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/clone", c.nodeName, vmid), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil // returns UPID
}

// CloneContainer clones a container
func (c *Client) CloneContainer(ctx context.Context, vmid int, opts CloneOptions) (string, error) {
	params := map[string]string{
		"newid": fmt.Sprintf("%d", opts.NewID),
	}
	if opts.Name != "" {
		params["hostname"] = opts.Name // LXC uses hostname instead of name
	}
	if opts.TargetNode != "" {
		params["target"] = opts.TargetNode
	}
	if opts.Full {
		params["full"] = "1"
	}
	if opts.Storage != "" {
		params["storage"] = opts.Storage
	}
	if opts.Description != "" {
		params["description"] = opts.Description
	}

	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/clone", c.nodeName, vmid), params)
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
	// Use /cluster/ha/status/current which returns an array of status items
	haItems, err := get[[]map[string]interface{}](c, ctx, "/cluster/ha/status/current")
	if err != nil {
		// HA might not be configured
		return &HAStatus{Enabled: false}, nil
	}

	status := &HAStatus{
		Enabled: len(haItems) > 0,
	}

	// Parse status items
	for _, item := range haItems {
		itemType, _ := item["type"].(string)
		switch itemType {
		case "quorum":
			// quorate can be float64 (1) or string ("1")
			switch v := item["quorate"].(type) {
			case float64:
				status.Quorum = v == 1
			case string:
				status.Quorum = v == "1"
			}
		case "master":
			if node, ok := item["node"].(string); ok {
				status.Manager.Node = node
			}
			if s, ok := item["status"].(string); ok {
				status.Manager.Status = s
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
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("API error %d (body unreadable: %w)", resp.StatusCode, err)
		}
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
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("API error %d (body unreadable: %w)", resp.StatusCode, err)
		}
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
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("API error %d (body unreadable: %w)", resp.StatusCode, err)
		}
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// deleteWithData sends a DELETE request and returns the response body
func (c *Client) deleteWithData(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// --- Network operations ---

// GetNetworkInterfaces returns all network interfaces on this node
func (c *Client) GetNetworkInterfaces(ctx context.Context) ([]NetworkInterface, error) {
	ifaces, err := get[[]NetworkInterface](c, ctx, fmt.Sprintf("/nodes/%s/network", c.nodeName))
	if err != nil {
		return nil, err
	}
	// Tag with node name
	for i := range ifaces {
		ifaces[i].Node = c.nodeName
	}
	return ifaces, nil
}

// --- Node configuration operations ---

// GetNodeDNS returns DNS configuration for this node
func (c *Client) GetNodeDNS(ctx context.Context) (*NodeDNS, error) {
	dns, err := get[NodeDNS](c, ctx, fmt.Sprintf("/nodes/%s/dns", c.nodeName))
	if err != nil {
		return nil, err
	}
	return &dns, nil
}

// GetNodeTime returns timezone/time info for this node
func (c *Client) GetNodeTime(ctx context.Context) (*NodeTime, error) {
	t, err := get[NodeTime](c, ctx, fmt.Sprintf("/nodes/%s/time", c.nodeName))
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetNodeHosts returns /etc/hosts content for this node
func (c *Client) GetNodeHosts(ctx context.Context) (string, error) {
	hosts, err := get[NodeHosts](c, ctx, fmt.Sprintf("/nodes/%s/hosts", c.nodeName))
	if err != nil {
		return "", err
	}
	return hosts.Data, nil
}

// GetNodeSubscription returns subscription info for this node
func (c *Client) GetNodeSubscription(ctx context.Context) (*NodeSubscription, error) {
	sub, err := get[NodeSubscription](c, ctx, fmt.Sprintf("/nodes/%s/subscription", c.nodeName))
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// GetNodeAPTRepositories returns APT repository configuration for this node
func (c *Client) GetNodeAPTRepositories(ctx context.Context) (*APTRepositoryInfo, error) {
	info, err := get[APTRepositoryInfo](c, ctx, fmt.Sprintf("/nodes/%s/apt/repositories", c.nodeName))
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// GetNodeConfig returns combined host-level configuration
func (c *Client) GetNodeConfig(ctx context.Context) (*NodeConfig, error) {
	cfg := &NodeConfig{}

	// Fetch all config in parallel
	type result struct {
		field string
		err   error
	}
	ch := make(chan result, 6)

	go func() {
		dns, err := c.GetNodeDNS(ctx)
		cfg.DNS = dns
		ch <- result{"dns", err}
	}()
	go func() {
		t, err := c.GetNodeTime(ctx)
		cfg.Time = t
		ch <- result{"time", err}
	}()
	go func() {
		hosts, err := c.GetNodeHosts(ctx)
		cfg.Hosts = hosts
		ch <- result{"hosts", err}
	}()
	go func() {
		ifaces, err := c.GetNetworkInterfaces(ctx)
		cfg.Network = ifaces
		ch <- result{"network", err}
	}()
	go func() {
		sub, err := c.GetNodeSubscription(ctx)
		cfg.Subscription = sub
		ch <- result{"subscription", err}
	}()
	go func() {
		status, err := c.GetNodeDetails(ctx)
		cfg.Status = status
		ch <- result{"status", err}
	}()

	// Collect results - log errors but don't fail completely
	for i := 0; i < 6; i++ {
		r := <-ch
		if r.err != nil {
			slog.Warn("node config: failed to fetch", "field", r.field, "node", c.nodeName, "error", r.err)
		}
	}

	// APT repos can be slow and may require elevated perms - fetch separately
	repos, err := c.GetNodeAPTRepositories(ctx)
	if err != nil {
		slog.Warn("node config: failed to fetch apt repos", "node", c.nodeName, "error", err)
	} else {
		cfg.APTRepos = repos
	}

	if cfg.Network == nil {
		cfg.Network = []NetworkInterface{}
	}

	return cfg, nil
}

// --- Node configuration update operations ---

// UpdateNodeDNS updates DNS configuration for this node
func (c *Client) UpdateNodeDNS(ctx context.Context, search, dns1, dns2, dns3 string) error {
	data := url.Values{}
	data.Set("search", search)
	data.Set("dns1", dns1)
	if dns2 != "" {
		data.Set("dns2", dns2)
	}
	if dns3 != "" {
		data.Set("dns3", dns3)
	}
	return c.putForm(ctx, fmt.Sprintf("/nodes/%s/dns", c.nodeName), data)
}

// UpdateNodeTimezone updates the timezone for this node
func (c *Client) UpdateNodeTimezone(ctx context.Context, timezone string) error {
	data := url.Values{}
	data.Set("timezone", timezone)
	return c.putForm(ctx, fmt.Sprintf("/nodes/%s/time", c.nodeName), data)
}

// UpdateNodeHosts updates /etc/hosts content for this node
func (c *Client) UpdateNodeHosts(ctx context.Context, hostsContent, digest string) error {
	data := url.Values{}
	data.Set("data", hostsContent)
	if digest != "" {
		data.Set("digest", digest)
	}
	_, err := c.post(ctx, fmt.Sprintf("/nodes/%s/hosts", c.nodeName), map[string]string{
		"data":   hostsContent,
		"digest": digest,
	})
	return err
}

// CreateNetworkInterface creates a new network interface on this node
func (c *Client) CreateNetworkInterface(ctx context.Context, iface string, params map[string]string) error {
	data := url.Values{}
	data.Set("iface", iface)
	for k, v := range params {
		if v != "" {
			data.Set(k, v)
		}
	}
	_, err := c.post(ctx, fmt.Sprintf("/nodes/%s/network", c.nodeName), params)
	if err != nil {
		return err
	}
	return nil
}

// UpdateNetworkInterface updates a network interface on this node
func (c *Client) UpdateNetworkInterface(ctx context.Context, iface string, params map[string]string) error {
	data := url.Values{}
	for k, v := range params {
		data.Set(k, v)
	}
	return c.putForm(ctx, fmt.Sprintf("/nodes/%s/network/%s", c.nodeName, iface), data)
}

// DeleteNetworkInterface deletes a network interface on this node
func (c *Client) DeleteNetworkInterface(ctx context.Context, iface string) error {
	return c.delete(ctx, fmt.Sprintf("/nodes/%s/network/%s", c.nodeName, iface))
}

// ApplyNetworkConfig applies pending network changes (reloads networking)
func (c *Client) ApplyNetworkConfig(ctx context.Context) error {
	return c.putForm(ctx, fmt.Sprintf("/nodes/%s/network", c.nodeName), url.Values{})
}

// RevertNetworkConfig reverts pending network changes
func (c *Client) RevertNetworkConfig(ctx context.Context) error {
	return c.delete(ctx, fmt.Sprintf("/nodes/%s/network", c.nodeName))
}

// --- SDN operations (cluster-wide) ---

// GetSDNZones returns all SDN zones in the cluster
func (c *Client) GetSDNZones(ctx context.Context) ([]SDNZone, error) {
	zones, err := get[[]SDNZone](c, ctx, "/cluster/sdn/zones")
	if err != nil {
		// SDN might not be configured
		return []SDNZone{}, nil
	}
	return zones, nil
}

// GetSDNVNets returns all SDN virtual networks in the cluster
func (c *Client) GetSDNVNets(ctx context.Context) ([]SDNVNet, error) {
	vnets, err := get[[]SDNVNet](c, ctx, "/cluster/sdn/vnets")
	if err != nil {
		// SDN might not be configured
		return []SDNVNet{}, nil
	}
	return vnets, nil
}

// GetSDNSubnets returns all SDN subnets in the cluster
func (c *Client) GetSDNSubnets(ctx context.Context) ([]SDNSubnet, error) {
	subnets, err := get[[]SDNSubnet](c, ctx, "/cluster/sdn/subnets")
	if err != nil {
		// SDN might not be configured
		return []SDNSubnet{}, nil
	}
	return subnets, nil
}

// GetSDNControllers returns all SDN controllers in the cluster
func (c *Client) GetSDNControllers(ctx context.Context) ([]SDNController, error) {
	controllers, err := get[[]SDNController](c, ctx, "/cluster/sdn/controllers")
	if err != nil {
		// SDN controllers might not be configured
		return []SDNController{}, nil
	}
	return controllers, nil
}

// --- Snapshot operations ---

// ListVMSnapshots returns all snapshots for a VM
func (c *Client) ListVMSnapshots(ctx context.Context, vmid int) ([]Snapshot, error) {
	return get[[]Snapshot](c, ctx, fmt.Sprintf("/nodes/%s/qemu/%d/snapshot", c.nodeName, vmid))
}

// CreateVMSnapshot creates a new snapshot for a VM
func (c *Client) CreateVMSnapshot(ctx context.Context, vmid int, snapname, description string, vmstate bool) (string, error) {
	params := map[string]string{
		"snapname": snapname,
	}
	if description != "" {
		params["description"] = description
	}
	if vmstate {
		params["vmstate"] = "1"
	}
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/snapshot", c.nodeName, vmid), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// RollbackVMSnapshot rolls back a VM to a snapshot
func (c *Client) RollbackVMSnapshot(ctx context.Context, vmid int, snapname string) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/snapshot/%s/rollback", c.nodeName, vmid, snapname), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// DeleteVMSnapshot deletes a VM snapshot
func (c *Client) DeleteVMSnapshot(ctx context.Context, vmid int, snapname string) (string, error) {
	data, err := c.deleteWithData(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/snapshot/%s", c.nodeName, vmid, snapname))
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// ListContainerSnapshots returns all snapshots for a container
func (c *Client) ListContainerSnapshots(ctx context.Context, vmid int) ([]Snapshot, error) {
	return get[[]Snapshot](c, ctx, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", c.nodeName, vmid))
}

// CreateContainerSnapshot creates a new snapshot for a container
func (c *Client) CreateContainerSnapshot(ctx context.Context, vmid int, snapname, description string) (string, error) {
	params := map[string]string{
		"snapname": snapname,
	}
	if description != "" {
		params["description"] = description
	}
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", c.nodeName, vmid), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// RollbackContainerSnapshot rolls back a container to a snapshot
func (c *Client) RollbackContainerSnapshot(ctx context.Context, vmid int, snapname string) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s/rollback", c.nodeName, vmid, snapname), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// DeleteContainerSnapshot deletes a container snapshot
func (c *Client) DeleteContainerSnapshot(ctx context.Context, vmid int, snapname string) (string, error) {
	data, err := c.deleteWithData(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s", c.nodeName, vmid, snapname))
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// --- Memory stats from /proc/vmstat ---

// VmStats contains memory activity counters from /proc/vmstat
type VmStats struct {
	Node      string `json:"node"`
	PgpgIn    int64  `json:"pgpgin"`    // pages paged in from disk
	PgpgOut   int64  `json:"pgpgout"`   // pages paged out to disk
	PswpIn    int64  `json:"pswpin"`    // pages swapped in
	PswpOut   int64  `json:"pswpout"`   // pages swapped out
	PgFault   int64  `json:"pgfault"`   // page faults
	PgMajFault int64 `json:"pgmajfault"` // major page faults (require I/O)
}

// GetVmStats fetches /proc/vmstat counters from the node via SSH
func (c *Client) GetVmStats(ctx context.Context) (*VmStats, error) {
	host := c.getHostFromURL()
	if host == "" {
		return nil, fmt.Errorf("could not determine node host")
	}

	output, err := runSSHCommand(ctx, host, "cat /proc/vmstat | grep -E '^(pgpgin|pgpgout|pswpin|pswpout|pgfault|pgmajfault) '")
	if err != nil {
		return nil, fmt.Errorf("failed to get vmstat: %w", err)
	}

	stats := &VmStats{Node: c.nodeName}
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		var val int64
		fmt.Sscanf(parts[1], "%d", &val)
		switch parts[0] {
		case "pgpgin":
			stats.PgpgIn = val
		case "pgpgout":
			stats.PgpgOut = val
		case "pswpin":
			stats.PswpIn = val
		case "pswpout":
			stats.PswpOut = val
		case "pgfault":
			stats.PgFault = val
		case "pgmajfault":
			stats.PgMajFault = val
		}
	}

	return stats, nil
}
