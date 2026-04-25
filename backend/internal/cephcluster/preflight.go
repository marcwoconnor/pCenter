package cephcluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/moconnor/pcenter/internal/pve"
)

// HostPreflightResult is the per-host outcome of a Ceph install preflight.
type HostPreflightResult struct {
	Node       string   `json:"node"`
	Reachable  bool     `json:"reachable"`
	PVEVersion string   `json:"pve_version,omitempty"`
	PVEMajor   string   `json:"pve_major,omitempty"`
	CephAlready bool    `json:"ceph_already_installed"`
	Blockers   []string `json:"blockers"`
}

// PreflightResponse is what the API returns to the wizard.
type PreflightResponse struct {
	Hosts      []HostPreflightResult `json:"hosts"`
	CanProceed bool                  `json:"can_proceed"`
	NetworkOK  bool                  `json:"network_ok"`
	Message    string                `json:"message,omitempty"`
}

// RunInstallPreflight checks each target host and returns a structured
// report the wizard renders. Non-blocking checks become warnings in the UI;
// blockers prevent the install.
//
// Checks performed:
//   - reachable: the host responds to /nodes/{node}/version
//   - pve_version: captured for homogeneity check
//   - ceph_already_installed: refuse to clobber an existing install
//   - PVE major-version homogeneity across all targets (warn-level for now)
//
// Network CIDR validation is left to PVE (`pveceph init` rejects bad input);
// pre-checking it here would duplicate logic and rot.
func RunInstallPreflight(
	ctx context.Context,
	clientFor func(node string) (*pve.Client, bool),
	hosts []string,
) PreflightResponse {
	resp := PreflightResponse{
		CanProceed: true,
		NetworkOK:  true, // PVE validates this on init
		Hosts:      make([]HostPreflightResult, 0, len(hosts)),
	}
	majors := map[string]bool{}

	for _, node := range hosts {
		// Blockers is initialized non-nil so it serializes as [] rather
		// than null — the wizard renders h.blockers.length and a null
		// here crashes the React tree (blank screen on success path).
		hr := HostPreflightResult{Node: node, Blockers: []string{}}
		c, ok := clientFor(node)
		if !ok {
			hr.Blockers = append(hr.Blockers, "no PVE client available for this node")
			resp.Hosts = append(resp.Hosts, hr)
			resp.CanProceed = false
			continue
		}

		// Reachability + version. GetNodeDetails hits /nodes/{node}/status
		// which doubles as a "are we reachable + authenticated" check.
		details, err := c.GetNodeDetails(ctx)
		if err != nil {
			hr.Reachable = false
			hr.Blockers = append(hr.Blockers, fmt.Sprintf("unreachable: %v", err))
			resp.Hosts = append(resp.Hosts, hr)
			resp.CanProceed = false
			continue
		}
		hr.Reachable = true
		hr.PVEVersion = details.PVEVersion
		hr.PVEMajor = majorVersion(details.PVEVersion)
		if hr.PVEMajor != "" {
			majors[hr.PVEMajor] = true
		}

		// Ceph already installed?
		if status, err := c.GetCephStatus(ctx); err == nil && status != nil {
			hr.CephAlready = true
			hr.Blockers = append(hr.Blockers, "Ceph is already installed on this node")
			resp.CanProceed = false
		}

		resp.Hosts = append(resp.Hosts, hr)
	}

	if len(majors) > 1 {
		resp.Message = fmt.Sprintf("PVE major-version mismatch across targets: %v. All hosts should be on the same major release before installing Ceph.", keys(majors))
		resp.CanProceed = false
	}

	return resp
}

// majorVersion extracts the major number from a PVE version string like
// "8.3.5". Returns "" if the format is unexpected.
func majorVersion(v string) string {
	if v == "" {
		return ""
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
