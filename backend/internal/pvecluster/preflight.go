package pvecluster

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/moconnor/pcenter/internal/inventory"
	"github.com/moconnor/pcenter/internal/pve"
)

// PreflightRequest selects the founder + joiners plus the desired cluster name
// so the name itself can be validated (1-15 chars, [a-zA-Z0-9-]).
type PreflightRequest struct {
	ClusterName   string
	FounderHostID string
	JoinerHostIDs []string
}

// HostPreflightResult summarizes what we learned about a single target host.
type HostPreflightResult struct {
	HostID           string   `json:"host_id"`
	Address          string   `json:"address"`
	NodeName         string   `json:"node_name,omitempty"`
	Role             Role     `json:"role"`
	Reachable        bool     `json:"reachable"`
	AlreadyInCluster bool     `json:"already_in_cluster"`
	VMCount          int      `json:"vm_count"`
	CTCount          int      `json:"ct_count"`
	PVEVersion       string   `json:"pve_version,omitempty"`
	PVEMajor         string   `json:"pve_major,omitempty"` // "7", "8", "9"
	Blockers         []string `json:"blockers"`
}

// PreflightResponse is the API shape returned to the frontend.
type PreflightResponse struct {
	ClusterNameOK      bool                  `json:"cluster_name_ok"`
	ClusterNameMessage string                `json:"cluster_name_message,omitempty"`
	Hosts              []HostPreflightResult `json:"hosts"`
	CanProceed         bool                  `json:"can_proceed"`
}

// clusterNameRe mirrors PVE's own constraint.
var clusterNameRe = regexp.MustCompile(`^[a-zA-Z0-9-]{1,15}$`)

// validateClusterName returns empty string if OK, else a human-readable reason.
func validateClusterName(name string) string {
	if !clusterNameRe.MatchString(name) {
		return "must be 1-15 characters, letters/digits/dashes only"
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return "must not start or end with a dash"
	}
	return ""
}

// Preflight probes every target host and reports blockers without changing
// anything. Read-only on the PVE side (hits /version, /cluster/status,
// /nodes/{n}/qemu, /nodes/{n}/lxc).
//
// Takes the inventory.Service to resolve host records by ID, plus a function
// that builds a pve.Client for a given host. Tests swap that out.
func Preflight(
	ctx context.Context,
	inv *inventory.Service,
	req PreflightRequest,
	newClient func(h *inventory.InventoryHost) *pve.Client,
) (*PreflightResponse, error) {
	// Initialize slice fields as empty (not nil) so JSON marshals them as [].
	resp := &PreflightResponse{
		ClusterNameOK: true,
		CanProceed:    true,
		Hosts:         []HostPreflightResult{},
	}

	if msg := validateClusterName(req.ClusterName); msg != "" {
		resp.ClusterNameOK = false
		resp.ClusterNameMessage = msg
		resp.CanProceed = false
	}

	// Also refuse a name that's already taken in pcenter's inventory.
	if resp.ClusterNameOK {
		existing, err := inv.GetClusterByName(ctx, req.ClusterName)
		if err != nil {
			return nil, fmt.Errorf("check existing cluster name: %w", err)
		}
		if existing != nil {
			resp.ClusterNameOK = false
			resp.ClusterNameMessage = fmt.Sprintf("cluster %q already exists in pcenter", req.ClusterName)
			resp.CanProceed = false
		}
	}

	allIDs := append([]string{req.FounderHostID}, req.JoinerHostIDs...)
	seen := map[string]bool{}
	for _, id := range allIDs {
		if id == "" {
			return nil, fmt.Errorf("empty host id in request")
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate host %s", id)
		}
		seen[id] = true
	}

	majors := map[string]int{} // PVE major → count; used for homogeneity check
	for _, id := range allIDs {
		role := RoleJoiner
		if id == req.FounderHostID {
			role = RoleFounder
		}
		host, err := inv.GetHost(ctx, id)
		if err != nil || host == nil {
			return nil, fmt.Errorf("host %s not found", id)
		}
		result := probeHost(ctx, host, role, newClient)
		if len(result.Blockers) > 0 {
			resp.CanProceed = false
		}
		if result.PVEMajor != "" {
			majors[result.PVEMajor]++
		}
		resp.Hosts = append(resp.Hosts, result)
	}

	// Major-version homogeneity check. Mixed-major PVE clusters are only
	// supported as a transient state during rolling upgrade, not for new
	// cluster formation. If we see more than one major across the targets,
	// flag every host with a non-dominant major as blocked.
	if len(majors) > 1 {
		var top string
		var topCount int
		for maj, n := range majors {
			if n > topCount {
				top = maj
				topCount = n
			}
		}
		for i := range resp.Hosts {
			h := &resp.Hosts[i]
			if h.PVEMajor != "" && h.PVEMajor != top {
				h.Blockers = append(h.Blockers,
					fmt.Sprintf("PVE major version %s doesn't match majority (%s); new clusters require matching majors", h.PVEMajor, top))
			}
		}
		resp.CanProceed = false
	}

	return resp, nil
}

// probeHost issues the read-only queries for one host. On any individual
// failure we keep going — each failed probe turns into a Blocker line rather
// than aborting the whole preflight.
func probeHost(
	ctx context.Context,
	host *inventory.InventoryHost,
	role Role,
	newClient func(h *inventory.InventoryHost) *pve.Client,
) HostPreflightResult {
	// Initialize Blockers as empty slice (not nil) so JSON marshals it as []
	// rather than null — the frontend iterates .blockers unconditionally.
	result := HostPreflightResult{
		HostID:   host.ID,
		Address:  host.Address,
		NodeName: host.NodeName,
		Role:     role,
		Blockers: []string{},
	}

	client := newClient(host)
	if client == nil {
		result.Blockers = append(result.Blockers, "failed to construct PVE client (missing credentials)")
		return result
	}

	// Version probe also doubles as a reachability check.
	nodes, err := client.GetNodes(ctx)
	if err != nil {
		result.Blockers = append(result.Blockers, fmt.Sprintf("unreachable: %v", err))
		return result
	}
	result.Reachable = true

	// Pick the node that matches our stored NodeName, falling back to first.
	var target string
	for _, n := range nodes {
		if n.Node == host.NodeName {
			target = n.Node
			break
		}
	}
	if target == "" && len(nodes) > 0 {
		target = nodes[0].Node
		result.NodeName = target
	}
	if target == "" {
		result.Blockers = append(result.Blockers, "no nodes returned from /nodes")
		return result
	}

	// Make sure the client has nodeName set for the /nodes/{n}/* calls below.
	client.SetNodeName(target)

	// Version.
	details, err := client.GetNodeDetails(ctx)
	if err == nil && details != nil {
		result.PVEVersion = details.PVEVersion
		result.PVEMajor = majorVersion(details.PVEVersion)
	}

	// Cluster membership: DiscoverClusterNodes filters to type=node. On a
	// standalone PVE host this returns 0 or 1 rows; if it returns ≥2 rows,
	// the host is already in a real cluster.
	clusterNodes, err := client.DiscoverClusterNodes(ctx)
	if err == nil && len(clusterNodes) > 1 {
		result.AlreadyInCluster = true
		result.Blockers = append(result.Blockers, "already a member of a Proxmox cluster")
	}

	// Guest counts via cluster-wide /cluster/resources — works on standalone
	// nodes too because Proxmox returns this node's resources even without a
	// real cluster.
	if vms, err := client.GetClusterResources(ctx, "vm"); err == nil {
		for _, v := range vms {
			if v.Type == "qemu" {
				result.VMCount++
			} else if v.Type == "lxc" {
				result.CTCount++
			}
		}
	}

	// Per-Proxmox docs, a joining node must have ZERO guests. Founder is
	// allowed to keep its guests (it owns them into the new cluster).
	if role == RoleJoiner {
		if result.VMCount > 0 {
			result.Blockers = append(result.Blockers,
				fmt.Sprintf("joiner has %d VM(s); Proxmox refuses to join a node with guests — migrate or delete them first", result.VMCount))
		}
		if result.CTCount > 0 {
			result.Blockers = append(result.Blockers,
				fmt.Sprintf("joiner has %d container(s); Proxmox refuses to join a node with guests — migrate or delete them first", result.CTCount))
		}
	}

	return result
}

// majorVersion pulls the "8" out of "8.2.4" / "pve-manager/8.2.4/abc" / etc.
func majorVersion(v string) string {
	if v == "" {
		return ""
	}
	// Strip the "pve-manager/" prefix if present; the suffix (git hash) is
	// left attached and dropped below when we cut at the first dot.
	v = strings.TrimPrefix(v, "pve-manager/")
	// Strip any "-" suffix the version sometimes carries (e.g. "7.4-17").
	if i := strings.Index(v, "-"); i >= 0 {
		v = v[:i]
	}
	if i := strings.Index(v, "."); i >= 0 {
		return v[:i]
	}
	return v
}

