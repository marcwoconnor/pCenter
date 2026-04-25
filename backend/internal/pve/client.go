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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moconnor/pcenter/internal/config"
)

// runSSHCommand executes a command on a remote host via SSH
// RunSSHCommand executes a command on a remote host via SSH.
//
// Uses explicit -i and UserKnownHostsFile paths derived from $HOME so we don't
// rely on OpenSSH's pw_dir lookup, which under systemd's ProtectHome=true
// resolves to a read-only /root for uid 0 even when $HOME is overridden.
// BatchMode=yes prevents ssh from blocking on a password prompt (which would
// hang indefinitely since there's no terminal attached).
func RunSSHCommand(ctx context.Context, host string, command string) (string, error) {
	slog.Info("running SSH command", "host", host, "command", command)

	// Enforce 30s hard timeout if context has no deadline
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}
	sshDir := filepath.Join(home, ".ssh")
	keyPath := filepath.Join(sshDir, "id_ed25519")
	knownHostsPath := filepath.Join(sshDir, "known_hosts")

	// accept-new: accept on first connection (TOFU), reject if key changes (MITM).
	// Previously used StrictHostKeyChecking=no which silently accepts changed keys.
	cmd := exec.CommandContext(ctx, "ssh",
		"-i", keyPath,
		"-o", "IdentitiesOnly=yes",
		"-o", "UserKnownHostsFile="+knownHostsPath,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
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

// probeAPIToken returns true if a GET against /version succeeds with the given
// token credentials. PVE invalidates revoked tokens immediately on token.cfg
// rewrite, so a 200 here proves the secret is current.
func probeAPIToken(ctx context.Context, address, tokenID, secret string, insecure bool) bool {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	reqURL := fmt.Sprintf("https://%s/api2/json/version", address)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", tokenID, secret))

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// CreateAPIToken creates a new API token using a ticket from password auth.
// tokenName is just the suffix (e.g., "pcenter"); full token ID will be
// "user@realm!tokenName".
//
// If existingSecret is non-empty, the function probes the token first and
// reuses it on success — this avoids a delete-and-recreate cycle that would
// invalidate sibling hosts sharing the same /etc/pve/priv/token.cfg in a PVE
// cluster (see issue #59). Pass empty string for first-time creation.
func CreateAPIToken(ctx context.Context, address string, auth *AuthResult, tokenName string, existingSecret string, insecure bool) (*TokenResult, error) {
	fullID := auth.Username + "!" + tokenName

	if existingSecret != "" && probeAPIToken(ctx, address, fullID, existingSecret, insecure) {
		slog.Info("existing API token still valid, reusing", "token", tokenName)
		return &TokenResult{TokenID: fullID, Secret: existingSecret}, nil
	}

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
		// If token already exists, delete and recreate. The probe above (when
		// existingSecret was supplied) already proved the stored secret is
		// stale, so recreating is the right call.
		if strings.Contains(string(body), "already exists") {
			slog.Info("API token already exists, recreating", "token", tokenName)
			delURL := fmt.Sprintf("https://%s/api2/json/access/users/%s/token/%s",
				address, url.PathEscape(auth.Username), tokenName)
			delReq, _ := http.NewRequestWithContext(ctx, "DELETE", delURL, nil)
			delReq.Header.Set("CSRFPreventionToken", auth.CSRFToken)
			delReq.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: auth.Ticket})
			client.Do(delReq)
			// Retry creation; pass empty existingSecret to skip another probe.
			return CreateAPIToken(ctx, address, auth, tokenName, "", insecure)
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

// put sends a PUT and returns the response body. Mirrors post().
func (c *Client) put(ctx context.Context, path string, params map[string]string) ([]byte, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	return c.request(ctx, http.MethodPut, path, strings.NewReader(form.Encode()))
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
	return RunSSHCommand(ctx, host, fullCmd)
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
	scanOutput, err := RunSSHCommand(ctx, host, "smartctl --scan -j")
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
		smartOutput, err := RunSSHCommand(ctx, host, fmt.Sprintf("smartctl -j -a %s", dev.Name))
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

// GetQDeviceStatus returns the qdevice status via Proxmox API (no SSH required)
func (c *Client) GetQDeviceStatus(ctx context.Context) (*QDeviceStatus, error) {
	data, err := c.request(ctx, "GET", "/cluster/config/qdevice", nil)
	if err != nil {
		return &QDeviceStatus{Configured: false}, nil
	}

	var resp struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil || len(resp.Data) == 0 {
		return &QDeviceStatus{Configured: false}, nil
	}

	state := resp.Data["State"]
	return &QDeviceStatus{
		Configured:   true,
		Connected:    state == "Connected",
		State:        state,
		QNetdAddress: resp.Data["QNetd host"],
		Algorithm:    resp.Data["Algorithm"],
	}, nil
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

// GetVMIPAddress returns the first non-loopback IPv4 address from the QEMU guest agent
func (c *Client) GetVMIPAddress(ctx context.Context, vmid int) (string, error) {
	data, err := c.request(ctx, "GET", fmt.Sprintf("/nodes/%s/qemu/%d/agent/network-get-interfaces", c.nodeName, vmid), nil)
	if err != nil {
		return "", err
	}

	var resp struct {
		Data struct {
			Result []struct {
				IPAddresses []struct {
					IPAddress   string `json:"ip-address"`
					IPType      string `json:"ip-address-type"`
				} `json:"ip-addresses"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	for _, iface := range resp.Data.Result {
		for _, addr := range iface.IPAddresses {
			if addr.IPType == "ipv4" && addr.IPAddress != "127.0.0.1" {
				return addr.IPAddress, nil
			}
		}
	}
	return "", fmt.Errorf("no IPv4 address found for VM %d", vmid)
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
	cephOutput, err := RunSSHCommand(ctx, host, "ceph health 2>/dev/null || echo 'CEPH_NOT_AVAILABLE'")
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
	qdeviceOutput, err := RunSSHCommand(ctx, host, "corosync-qdevice-tool -s 2>/dev/null | grep -E 'State:|QNetd host:' || echo 'NO_QDEVICE'")
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

	_, err := RunSSHCommand(ctx, host, cmd)
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

// --- Storage vMotion (per-disk / per-volume move across storages) ---

// MoveVMDisk moves a single VM disk to a different storage pool while the
// VM can stay running. Returns the UPID of the PVE task.
//
//   disk:          qemu disk key (scsi0, virtio0, ide0, sata0, ...)
//   targetStorage: destination storage pool name
//   delete:        when true, removes the source disk after the copy completes
//                  (PVE default is false — source is kept as an unused disk)
//   format:        optional target format override ("raw", "qcow2", "vmdk");
//                  empty string lets PVE pick the right default for the target
func (c *Client) MoveVMDisk(ctx context.Context, vmid int, disk, targetStorage string, delete bool, format string) (string, error) {
	params := map[string]string{
		"disk":    disk,
		"storage": targetStorage,
	}
	if delete {
		params["delete"] = "1"
	}
	if format != "" {
		params["format"] = format
	}
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/move_disk", c.nodeName, vmid), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// MoveContainerVolume moves a single LXC volume (rootfs or mpN) to a different
// storage pool. PVE requires the container to be stopped for this op — it
// does not support online volume migration for LXC today. Returns UPID.
//
//   volume:        lxc volume key ("rootfs" or "mp0", "mp1", ...)
//   targetStorage: destination storage pool name
//   delete:        when true, removes the source volume after copy completes
func (c *Client) MoveContainerVolume(ctx context.Context, vmid int, volume, targetStorage string, delete bool) (string, error) {
	params := map[string]string{
		"volume":  volume,
		"storage": targetStorage,
	}
	if delete {
		params["delete"] = "1"
	}
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/move_volume", c.nodeName, vmid), params)
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

// ConvertVMToTemplate marks a VM as a template (POST /nodes/{node}/qemu/{vmid}/template).
// The VM must be stopped. Returns UPID for task tracking (empty string if PVE returns no task).
func (c *Client) ConvertVMToTemplate(ctx context.Context, vmid int) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/template", c.nodeName, vmid), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// ConvertContainerToTemplate marks a container as a template.
// The container must be stopped. Returns UPID (empty if PVE returns no task).
func (c *Client) ConvertContainerToTemplate(ctx context.Context, vmid int) (string, error) {
	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/template", c.nodeName, vmid), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// --- ACME / certificate operations ---

// GetNodeCertificates returns the certificates installed on this node.
// Each cert is enriched with server-side parsed fields (serial, signature
// algorithm, key usage, etc.) by decoding its PEM block.
func (c *Client) GetNodeCertificates(ctx context.Context) ([]NodeCertificate, error) {
	certs, err := get[[]NodeCertificate](c, ctx, fmt.Sprintf("/nodes/%s/certificates/info", c.nodeName))
	if err != nil {
		return nil, err
	}
	for i := range certs {
		EnrichCertificateFromPEM(&certs[i])
	}
	return certs, nil
}

// ListACMEAccounts returns ACME accounts configured at the cluster level.
func (c *Client) ListACMEAccounts(ctx context.Context) ([]ACMEAccount, error) {
	return get[[]ACMEAccount](c, ctx, "/cluster/acme/account")
}

// ListACMEPlugins returns ACME DNS/standalone plugins configured at the cluster level.
func (c *Client) ListACMEPlugins(ctx context.Context) ([]ACMEPlugin, error) {
	return get[[]ACMEPlugin](c, ctx, "/cluster/acme/plugins")
}

// RenewNodeACMECertificate triggers a renewal of the node's existing ACME certificate.
// Returns UPID for task tracking. Requires the node already has an ACME-issued cert.
func (c *Client) RenewNodeACMECertificate(ctx context.Context) (string, error) {
	data, err := c.put(ctx, fmt.Sprintf("/nodes/%s/certificates/acme/certificate", c.nodeName), nil)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// UploadNodeCustomCertificate installs a non-ACME certificate on a node.
// `certificates` is the PEM-encoded cert (chain). `key` is optional if reusing existing private key.
// Returns the list of certificate info entries now installed.
func (c *Client) UploadNodeCustomCertificate(ctx context.Context, certificates, key string, force, restart bool) error {
	params := map[string]string{"certificates": certificates}
	if key != "" {
		params["key"] = key
	}
	if force {
		params["force"] = "1"
	}
	if restart {
		params["restart"] = "1"
	}
	_, err := c.post(ctx, fmt.Sprintf("/nodes/%s/certificates/custom", c.nodeName), params)
	return err
}

// DeleteNodeCustomCertificate removes the custom certificate from a node.
func (c *Client) DeleteNodeCustomCertificate(ctx context.Context, restart bool) error {
	path := fmt.Sprintf("/nodes/%s/certificates/custom", c.nodeName)
	if restart {
		path += "?restart=1"
	}
	return c.delete(ctx, path)
}

// --- Backup (vzdump) operations ---

// VzdumpOptions configures a backup operation.
type VzdumpOptions struct {
	VMIDs    []int  // one or more guest IDs
	Storage  string // target storage ID (required)
	Mode     string // snapshot (default), suspend, stop
	Compress string // 0, gzip, lzo, zstd
	Remove   int    // -1 to keep current defaults, otherwise number of backups to keep
	Notes    string // notes-template
}

// CreateVzdump triggers a backup task on a node.
// Returns UPID for task tracking. Multi-VMID requests return a single task UPID.
func (c *Client) CreateVzdump(ctx context.Context, opts VzdumpOptions) (string, error) {
	if len(opts.VMIDs) == 0 {
		return "", fmt.Errorf("at least one VMID required")
	}
	if opts.Storage == "" {
		return "", fmt.Errorf("storage required")
	}

	vmidStrs := make([]string, 0, len(opts.VMIDs))
	for _, id := range opts.VMIDs {
		vmidStrs = append(vmidStrs, fmt.Sprintf("%d", id))
	}

	params := map[string]string{
		"vmid":    strings.Join(vmidStrs, ","),
		"storage": opts.Storage,
	}
	if opts.Mode != "" {
		params["mode"] = opts.Mode
	}
	if opts.Compress != "" {
		params["compress"] = opts.Compress
	}
	if opts.Notes != "" {
		params["notes-template"] = opts.Notes
	}

	data, err := c.post(ctx, fmt.Sprintf("/nodes/%s/vzdump", c.nodeName), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// --- Resource Pool operations (cluster-wide) ---

// ListPools returns all resource pools in the cluster.
func (c *Client) ListPools(ctx context.Context) ([]Pool, error) {
	return get[[]Pool](c, ctx, "/pools")
}

// GetPool returns full pool details including members.
func (c *Client) GetPool(ctx context.Context, poolID string) (*PoolDetail, error) {
	d, err := get[PoolDetail](c, ctx, "/pools/"+url.PathEscape(poolID))
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// CreatePool creates a new resource pool.
func (c *Client) CreatePool(ctx context.Context, poolID, comment string) error {
	params := map[string]string{"poolid": poolID}
	if comment != "" {
		params["comment"] = comment
	}
	_, err := c.post(ctx, "/pools", params)
	return err
}

// UpdatePool modifies a pool. Can update comment, add members via `vms` (comma-sep VMIDs)
// or `storage` (comma-sep storage IDs), or remove members by setting `delete=1`.
func (c *Client) UpdatePool(ctx context.Context, poolID, comment string, vms, storage []string, deleteMembers bool) error {
	params := map[string]string{}
	if comment != "" {
		params["comment"] = comment
	}
	if len(vms) > 0 {
		params["vms"] = strings.Join(vms, ",")
	}
	if len(storage) > 0 {
		params["storage"] = strings.Join(storage, ",")
	}
	if deleteMembers {
		params["delete"] = "1"
	}
	_, err := c.put(ctx, "/pools/"+url.PathEscape(poolID), params)
	return err
}

// DeletePool removes an empty pool.
func (c *Client) DeletePool(ctx context.Context, poolID string) error {
	return c.delete(ctx, "/pools/"+url.PathEscape(poolID))
}

// ListACMEDirectories returns the published ACME directories (LE prod, LE staging, etc.).
func (c *Client) ListACMEDirectories(ctx context.Context) ([]ACMEDirectory, error) {
	return get[[]ACMEDirectory](c, ctx, "/cluster/acme/directories")
}

// GetACMETOSURL returns the terms-of-service URL for the specified ACME directory.
func (c *Client) GetACMETOSURL(ctx context.Context, directoryURL string) (string, error) {
	path := "/cluster/acme/tos"
	if directoryURL != "" {
		path += "?directory=" + url.QueryEscape(directoryURL)
	}
	return get[string](c, ctx, path)
}

// GetACMEAccount returns full details for a single account.
func (c *Client) GetACMEAccount(ctx context.Context, name string) (*ACMEAccountDetail, error) {
	detail, err := get[ACMEAccountDetail](c, ctx, "/cluster/acme/account/"+url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	return &detail, nil
}

// CreateACMEAccount registers a new ACME account. Returns UPID.
func (c *Client) CreateACMEAccount(ctx context.Context, name, contact, directory, tosURL string) (string, error) {
	params := map[string]string{"name": name, "contact": contact}
	if directory != "" {
		params["directory"] = directory
	}
	if tosURL != "" {
		params["tos_url"] = tosURL
	}
	data, err := c.post(ctx, "/cluster/acme/account", params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// UpdateACMEAccount updates an account's contact. Returns UPID.
func (c *Client) UpdateACMEAccount(ctx context.Context, name, contact string) (string, error) {
	params := map[string]string{}
	if contact != "" {
		params["contact"] = contact
	}
	data, err := c.put(ctx, "/cluster/acme/account/"+url.PathEscape(name), params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

// DeleteACMEAccount deregisters an ACME account. Returns UPID.
func (c *Client) DeleteACMEAccount(ctx context.Context, name string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/cluster/acme/account/"+url.PathEscape(name), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var apiResp APIResponse[string]
	json.Unmarshal(body, &apiResp)
	return apiResp.Data, nil
}

// ListACMEChallengeSchemas returns all available DNS/standalone plugin schemas.
func (c *Client) ListACMEChallengeSchemas(ctx context.Context) ([]ACMEChallengeSchema, error) {
	return get[[]ACMEChallengeSchema](c, ctx, "/cluster/acme/challenge-schema")
}

// GetACMEPlugin returns full details for a single plugin.
func (c *Client) GetACMEPlugin(ctx context.Context, id string) (*ACMEPlugin, error) {
	plugin, err := get[ACMEPlugin](c, ctx, "/cluster/acme/plugins/"+url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	return &plugin, nil
}

// CreateACMEPlugin creates a new DNS/standalone plugin.
// `data` contains the provider-specific fields (e.g. {"CF_Token": "..."}).
func (c *Client) CreateACMEPlugin(ctx context.Context, id, pluginType, api string, data map[string]string) error {
	params := map[string]string{"id": id, "type": pluginType}
	if api != "" {
		params["api"] = api
	}
	// Encode data[] as semicolon-separated key=value pairs
	if len(data) > 0 {
		pairs := make([]string, 0, len(data))
		for k, v := range data {
			pairs = append(pairs, k+"="+v)
		}
		params["data"] = strings.Join(pairs, "\n")
	}
	_, err := c.post(ctx, "/cluster/acme/plugins", params)
	return err
}

// UpdateACMEPlugin updates an existing plugin. Only non-empty fields are sent.
func (c *Client) UpdateACMEPlugin(ctx context.Context, id string, data map[string]string) error {
	params := map[string]string{}
	if len(data) > 0 {
		pairs := make([]string, 0, len(data))
		for k, v := range data {
			pairs = append(pairs, k+"="+v)
		}
		params["data"] = strings.Join(pairs, "\n")
	}
	_, err := c.put(ctx, "/cluster/acme/plugins/"+url.PathEscape(id), params)
	return err
}

// DeleteACMEPlugin removes a plugin.
func (c *Client) DeleteACMEPlugin(ctx context.Context, id string) error {
	return c.delete(ctx, "/cluster/acme/plugins/"+url.PathEscape(id))
}

// GetNodeACMEDomains parses ACME domain config from the raw node config.
// Reads `acme` (if present as "domains=...;plugin=...") plus `acmedomain0..4` fields.
func (c *Client) GetNodeACMEDomains(ctx context.Context) ([]NodeACMEDomain, error) {
	raw, err := get[map[string]interface{}](c, ctx, fmt.Sprintf("/nodes/%s/config", c.nodeName))
	if err != nil {
		return nil, err
	}
	return parseACMEDomains(raw), nil
}

// SetNodeACMEDomains writes ACME domain config back. All domains share `plugin` on the `acme` field.
// Passing an empty list clears the acme + acmedomain[0-4] fields.
func (c *Client) SetNodeACMEDomains(ctx context.Context, domains []NodeACMEDomain) error {
	params := map[string]string{}

	if len(domains) == 0 {
		// Clear everything via the `delete` param
		params["delete"] = "acme,acmedomain0,acmedomain1,acmedomain2,acmedomain3,acmedomain4"
	} else {
		// Group domains by plugin. If they all share one plugin, use the `acme` field.
		// Otherwise, fall back to acmedomain[0-4] per-domain.
		pluginGroups := map[string][]string{}
		for _, d := range domains {
			pluginGroups[d.Plugin] = append(pluginGroups[d.Plugin], d.Domain)
		}
		// Always clear the slots we don't touch to avoid leftovers.
		params["delete"] = "acmedomain0,acmedomain1,acmedomain2,acmedomain3,acmedomain4"

		if len(pluginGroups) == 1 {
			for plugin, doms := range pluginGroups {
				val := "domains=" + strings.Join(doms, ",")
				if plugin != "" {
					val += ";plugin=" + plugin
				}
				params["acme"] = val
			}
		} else {
			// Use acmedomain[0-4] for per-domain plugin (up to 5 entries)
			params["delete"] = "acme"
			slot := 0
			for _, d := range domains {
				if slot > 4 {
					return fmt.Errorf("more than 5 domains with different plugins is not supported")
				}
				val := d.Domain
				if d.Plugin != "" {
					val += ",plugin=" + d.Plugin
				}
				params[fmt.Sprintf("acmedomain%d", slot)] = val
				slot++
			}
		}
	}

	return c.putForm(ctx, fmt.Sprintf("/nodes/%s/config", c.nodeName), mapToValues(params))
}

// parseACMEDomains extracts domain entries from a raw /nodes/{node}/config response.
func parseACMEDomains(raw map[string]interface{}) []NodeACMEDomain {
	var out []NodeACMEDomain
	// Top-level `acme` field: "domains=a,b,c;plugin=x"
	if s, ok := raw["acme"].(string); ok && s != "" {
		domains, plugin := parseACMEField(s)
		for _, d := range domains {
			out = append(out, NodeACMEDomain{Domain: d, Plugin: plugin})
		}
	}
	// acmedomain0..4: "domain,plugin=x"
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("acmedomain%d", i)
		s, ok := raw[key].(string)
		if !ok || s == "" {
			continue
		}
		parts := strings.Split(s, ",")
		if len(parts) == 0 {
			continue
		}
		entry := NodeACMEDomain{Domain: parts[0]}
		for _, kv := range parts[1:] {
			if strings.HasPrefix(kv, "plugin=") {
				entry.Plugin = strings.TrimPrefix(kv, "plugin=")
			}
		}
		out = append(out, entry)
	}
	return out
}

// parseACMEField parses "domains=a,b,c;plugin=x" into ([a,b,c], "x").
func parseACMEField(s string) ([]string, string) {
	var domains []string
	var plugin string
	for _, part := range strings.Split(s, ";") {
		if strings.HasPrefix(part, "domains=") {
			domains = strings.Split(strings.TrimPrefix(part, "domains="), ",")
		} else if strings.HasPrefix(part, "plugin=") {
			plugin = strings.TrimPrefix(part, "plugin=")
		}
	}
	return domains, plugin
}

// mapToValues converts a string map to url.Values for form submission.
func mapToValues(m map[string]string) url.Values {
	v := url.Values{}
	for k, val := range m {
		v.Set(k, val)
	}
	return v
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

// ClusterStatusNode represents an entry from /cluster/status (can be type=node or type=cluster)
type ClusterStatusNode struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	IP      string `json:"ip"`
	Online  int    `json:"online"` // 1 or 0 (node entries)
	Local   int    `json:"local"`  // 1 if this is the node we queried (node entries)
	Type    string `json:"type"`   // "node" or "cluster"
	Nodes   int    `json:"nodes"`  // number of configured nodes (cluster entries)
	Quorate int    `json:"quorate"` // 1 if quorate (cluster entries)
}

// ClusterMembership represents whether a PVE host is in a real cluster, and which one.
// Determined from the /cluster/status response: a real multi-node cluster has a
// type=cluster entry AND ≥2 type=node entries. Standalone single-node installs
// either have no type=cluster entry or a cluster entry reporting nodes=1.
type ClusterMembership struct {
	IsCluster   bool                // true iff PVE reports a real multi-node cluster
	ClusterName string              // from the type=cluster entry; "" if standalone
	Quorate     bool                // quorum state from the type=cluster entry
	Nodes       []ClusterStatusNode // all type=node entries
	LocalNode   string              // name of the node we queried (local=1)
}

// ProbeClusterMembership queries /cluster/status and classifies the response.
// Used at host-add time and by the promotion reconciler to decide whether to
// file a host under a real cluster or as a standalone.
func (c *Client) ProbeClusterMembership(ctx context.Context) (*ClusterMembership, error) {
	items, err := get[[]ClusterStatusNode](c, ctx, "/cluster/status")
	if err != nil {
		return nil, err
	}
	return ClassifyClusterStatus(items), nil
}

// ClassifyClusterStatus turns raw /cluster/status entries into a ClusterMembership.
// Exposed for unit testing.
func ClassifyClusterStatus(items []ClusterStatusNode) *ClusterMembership {
	m := &ClusterMembership{}
	var clusterEntry *ClusterStatusNode
	for i := range items {
		item := items[i]
		switch item.Type {
		case "node":
			m.Nodes = append(m.Nodes, item)
			if item.Local == 1 {
				m.LocalNode = item.Name
			}
		case "cluster":
			clusterEntry = &items[i]
		}
	}

	if clusterEntry != nil && len(m.Nodes) >= 2 {
		m.IsCluster = true
		m.ClusterName = clusterEntry.Name
		m.Quorate = clusterEntry.Quorate == 1
	}

	return m
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

// --- Cluster Formation ---

// ClusterCreateOptions are params for creating a new PVE cluster on this node.
type ClusterCreateOptions struct {
	ClusterName string // required, 1-15 chars, [a-zA-Z0-9-]
	NodeID      int    // optional 1..N; 0 = let PVE assign
	Link0       string // optional ring0 addr; empty = node's default IP
	Link1       string // reserved; non-goal for initial release
}

// ClusterCreate forms a 1-node cluster on this client's node.
//
// NOTE: on most Proxmox versions /cluster/config rejects API tokens with
// "Permission check failed (user != root@pam)". Use ClusterCreateWithPassword
// instead — pass an AuthResult obtained from AuthenticateWithPassword. This
// method is kept for test coverage and compatibility with any PVE build that
// does accept tokens here.
func (c *Client) ClusterCreate(ctx context.Context, opts ClusterCreateOptions) (string, error) {
	if opts.ClusterName == "" {
		return "", fmt.Errorf("cluster_name is required")
	}
	params := map[string]string{"clustername": opts.ClusterName}
	if opts.NodeID > 0 {
		params["nodeid"] = fmt.Sprintf("%d", opts.NodeID)
	}
	if opts.Link0 != "" {
		params["link0"] = opts.Link0
	}
	if opts.Link1 != "" {
		params["link1"] = opts.Link1
	}

	data, err := c.post(ctx, "/cluster/config", params)
	if err != nil {
		return "", err
	}
	var resp APIResponse[string]
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse cluster-create response: %w (body: %s)", err, string(data))
	}
	return resp.Data, nil
}

// ClusterJoinInfo is what a prospective joiner needs from the founder's
// GET /cluster/config/join endpoint.
//
// Newer PVE versions (8.x+) return Fingerprint and IPAddress as per-node
// fields inside Nodelist (`pve_fp`, `ring0_addr`) rather than at the top
// level. GetClusterJoinInfoWithPassword normalizes both shapes.
type ClusterJoinInfo struct {
	IPAddress     string                 `json:"ipAddress"`
	Fingerprint   string                 `json:"fingerprint"`
	Totem         map[string]interface{} `json:"totem"`
	ConfigDigest  string                 `json:"config_digest"`
	Nodelist      []ClusterJoinNode      `json:"nodelist"`
	PreferredNode string                 `json:"preferred_node"`
}

// ClusterJoinNode describes a cluster member in the join-info payload.
type ClusterJoinNode struct {
	Name        string `json:"name"`
	Nodeid      string `json:"nodeid"`
	QuorumVotes string `json:"quorum_votes"`
	Ring0Addr   string `json:"ring0_addr"`
	PveFP       string `json:"pve_fp"` // SSL fingerprint, set per-node in PVE 8+
}

// GetClusterJoinInfo fetches the join parameters a new node will need.
// Must be called against the founder (or any existing member). API token auth
// is fine here — only /cluster/config (the create endpoint) demands password.
//
// nodeName selects which member to fetch info ABOUT (used by PVE 8+ to pick
// the per-node fingerprint and ring0 address). Pass empty string to let PVE
// use the connected node.
func (c *Client) GetClusterJoinInfo(ctx context.Context, nodeName string) (*ClusterJoinInfo, error) {
	path := "/cluster/config/join"
	if nodeName != "" {
		path = path + "?node=" + url.QueryEscape(nodeName)
	}
	info, err := get[ClusterJoinInfo](c, ctx, path)
	if err != nil {
		return nil, err
	}
	normalizeJoinInfo(&info, nodeName)
	if info.Fingerprint == "" {
		return &info, fmt.Errorf("PVE returned join info without a fingerprint (top-level or per-node)")
	}
	return &info, nil
}

// normalizeJoinInfo lifts per-node `pve_fp` / `ring0_addr` (PVE 8+) onto the
// top-level Fingerprint / IPAddress fields when the older top-level fields are
// empty, so callers don't have to care about the version difference.
func normalizeJoinInfo(info *ClusterJoinInfo, preferredNode string) {
	if info == nil {
		return
	}
	if info.Fingerprint != "" && info.IPAddress != "" {
		return
	}
	target := info.PreferredNode
	if target == "" {
		target = preferredNode
	}
	var entry *ClusterJoinNode
	for i := range info.Nodelist {
		if info.Nodelist[i].Name == target {
			entry = &info.Nodelist[i]
			break
		}
	}
	if entry == nil && len(info.Nodelist) > 0 {
		entry = &info.Nodelist[0]
	}
	if entry != nil {
		if info.Fingerprint == "" {
			info.Fingerprint = entry.PveFP
		}
		if info.IPAddress == "" {
			info.IPAddress = entry.Ring0Addr
		}
	}
}

// ClusterJoinRequest are params for a node joining an existing cluster.
// The target `address` is the joining node itself; Hostname is the founder's
// address that the joiner will contact via SSH to fetch /etc/pve bits.
type ClusterJoinRequest struct {
	Hostname    string // founder IP (what PVE calls "hostname" in the form)
	Fingerprint string // founder's SSL fingerprint from GetClusterJoinInfo
	Password    string // root@pam password — PVE requires it in the body here
	Link0       string // joining node's ring0 addr (usually its own IP)
	Votes       int    // optional, default 1
	Force       bool   // false — only true if overriding an existing /etc/pve
	Nodeid      int    // 0 = let PVE assign
}

// ClusterJoin joins the target node (at `address`) to an existing cluster.
//
// Must use password auth: PVE's /cluster/config/join explicitly rejects API
// tokens. Callers authenticate via AuthenticateWithPassword first and pass the
// resulting AuthResult in.
//
// Returns the task UPID. On some PVE versions the endpoint returns `null` in
// `data` rather than a UPID; callers should treat an empty return as "join
// accepted, watch /cluster/status on the founder" and not as an error.
func ClusterJoin(ctx context.Context, address string, auth *AuthResult, req ClusterJoinRequest, insecure bool) (string, error) {
	if req.Hostname == "" {
		return "", fmt.Errorf("hostname (founder address) is required")
	}
	if req.Fingerprint == "" {
		return "", fmt.Errorf("fingerprint is required")
	}
	if req.Password == "" {
		return "", fmt.Errorf("password is required")
	}
	if auth == nil {
		return "", fmt.Errorf("auth ticket is required (call AuthenticateWithPassword first)")
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	httpClient := &http.Client{
		Transport: transport,
		// Join rewrites /etc/pve and restarts pveproxy; the HTTP request itself
		// commonly drops mid-response. Bound the wait generously.
		Timeout: 240 * time.Second,
	}

	reqURL := fmt.Sprintf("https://%s/api2/json/cluster/config/join", address)
	form := url.Values{}
	form.Set("hostname", req.Hostname)
	form.Set("fingerprint", req.Fingerprint)
	form.Set("password", req.Password)
	if req.Link0 != "" {
		form.Set("link0", req.Link0)
	}
	if req.Votes > 0 {
		form.Set("votes", fmt.Sprintf("%d", req.Votes))
	}
	if req.Force {
		form.Set("force", "1")
	}
	if req.Nodeid > 0 {
		form.Set("nodeid", fmt.Sprintf("%d", req.Nodeid))
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("CSRFPreventionToken", auth.CSRFToken)
	httpReq.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: auth.Ticket})

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		// pveproxy restart during join can surface as a connection reset. Caller
		// compensates by watching the founder's /cluster/status for membership.
		return "", fmt.Errorf("join request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("join failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Response is either {"data":"UPID:..."} or {"data":null}.
	var parsed struct {
		Data *string `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse join response: %w (body: %s)", err, string(body))
	}
	if parsed.Data == nil {
		return "", nil
	}
	return *parsed.Data, nil
}

// ClusterCreateWithPassword forms a cluster using cookie+CSRF auth, which PVE
// requires for /cluster/config — API tokens are rejected there ("Permission
// check failed (user != root@pam)"). Caller obtains `auth` from
// AuthenticateWithPassword.
//
// Returns the task UPID for polling (via WaitForTask against a token-auth
// Client — task status endpoints do accept API tokens).
func ClusterCreateWithPassword(ctx context.Context, address string, auth *AuthResult, opts ClusterCreateOptions, insecure bool) (string, error) {
	if auth == nil {
		return "", fmt.Errorf("auth ticket is required (call AuthenticateWithPassword first)")
	}
	if opts.ClusterName == "" {
		return "", fmt.Errorf("cluster_name is required")
	}

	form := url.Values{}
	form.Set("clustername", opts.ClusterName)
	if opts.NodeID > 0 {
		form.Set("nodeid", fmt.Sprintf("%d", opts.NodeID))
	}
	if opts.Link0 != "" {
		form.Set("link0", opts.Link0)
	}
	if opts.Link1 != "" {
		form.Set("link1", opts.Link1)
	}

	body, err := doPasswordAuthPost(ctx, address, auth, "/api2/json/cluster/config", form, insecure, 90*time.Second)
	if err != nil {
		return "", err
	}

	var resp APIResponse[string]
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse cluster-create response: %w (body: %s)", err, string(body))
	}
	return resp.Data, nil
}

// GetClusterJoinInfoWithPassword fetches the founder's join info using
// password auth. Mirrors the GET side of ClusterCreateWithPassword — some PVE
// versions also gate this endpoint behind the user-not-token check.
//
// nodeName is the PVE node we want join info ABOUT (typically the founder
// itself). On a freshly-created 1-node cluster, omitting this parameter has
// been observed to return a payload without a fingerprint; passing it
// explicitly is the same path pvecm uses internally.
func GetClusterJoinInfoWithPassword(ctx context.Context, address string, auth *AuthResult, nodeName string, insecure bool) (*ClusterJoinInfo, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth ticket is required")
	}
	path := "/api2/json/cluster/config/join"
	if nodeName != "" {
		path = path + "?node=" + url.QueryEscape(nodeName)
	}
	body, err := doPasswordAuthGet(ctx, address, auth, path, insecure, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("%w (body so far unread)", err)
	}
	var resp APIResponse[ClusterJoinInfo]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse join-info response: %w (body: %s)", err, string(body))
	}
	info := &resp.Data
	normalizeJoinInfo(info, nodeName)
	if info.Fingerprint == "" {
		raw := string(body)
		if len(raw) > 400 {
			raw = raw[:400] + "…"
		}
		return info, fmt.Errorf("PVE returned join info without a fingerprint (top-level or per-node); body=%s", raw)
	}
	return info, nil
}

// doPasswordAuthPost is the shared HTTP plumbing for the cluster-formation
// endpoints that demand cookie+CSRF auth.
func doPasswordAuthPost(ctx context.Context, address string, auth *AuthResult, path string, form url.Values, insecure bool, timeout time.Duration) ([]byte, error) {
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}}
	client := &http.Client{Transport: transport, Timeout: timeout}

	reqURL := fmt.Sprintf("https://%s%s", address, path)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("CSRFPreventionToken", auth.CSRFToken)
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: auth.Ticket})

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
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

// doPasswordAuthGet mirrors doPasswordAuthPost for read endpoints.
func doPasswordAuthGet(ctx context.Context, address string, auth *AuthResult, path string, insecure bool, timeout time.Duration) ([]byte, error) {
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}}
	client := &http.Client{Transport: transport, Timeout: timeout}

	reqURL := fmt.Sprintf("https://%s%s", address, path)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("CSRFPreventionToken", auth.CSRFToken)
	req.AddCookie(&http.Cookie{Name: "PVEAuthCookie", Value: auth.Ticket})

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
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

// WaitForTask polls /nodes/{node}/tasks/{upid}/status until the task reaches a
// terminal state or ctx is done. Returns the final Task. If the task failed,
// the returned error message includes the task log error (via GetTaskError).
//
// pollInterval is clamped to a 1s minimum.
func (c *Client) WaitForTask(ctx context.Context, upid string, pollInterval time.Duration) (*Task, error) {
	if upid == "" {
		return nil, fmt.Errorf("upid is required")
	}
	if pollInterval < time.Second {
		pollInterval = time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		task, err := c.GetTaskStatus(ctx, upid)
		if err == nil && task != nil && task.Status != "running" {
			if task.Status == "stopped" && task.ExitCode != "OK" {
				// Non-OK exit: include log tail for context.
				logErr := c.GetTaskError(ctx, upid)
				if logErr != "" {
					return task, fmt.Errorf("task %s failed: %s", task.ExitCode, logErr)
				}
				return task, fmt.Errorf("task %s exited with status %q", upid, task.ExitCode)
			}
			return task, nil
		}
		// Transient GetTaskStatus errors (e.g. pveproxy restart during join) are
		// tolerated: we keep polling until ctx deadline.

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
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

	output, err := RunSSHCommand(ctx, host, "cat /proc/vmstat | grep -E '^(pgpgin|pgpgout|pswpin|pswpout|pgfault|pgmajfault) '")
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
