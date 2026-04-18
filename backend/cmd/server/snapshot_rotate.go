package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/moconnor/pcenter/internal/pve"
)

// snapshotAutoPrefix is the name prefix used for snapshots created by
// the scheduled rotation task. Only snapshots starting with this prefix
// are eligible for automatic pruning.
const snapshotAutoPrefix = "auto-"

// rotateSnapshots creates a new timestamp-named snapshot and prunes old
// `auto-*` snapshots beyond the retention count.
//
// Returns the UPID of the create operation (or an error string prefixed
// with "prune:" if only the prune phase failed after a successful create).
//
// Params:
//   - retention (int, default 7): number of auto-* snapshots to keep AFTER this run
//   - description (string, optional): snapshot description
//   - vmstate (bool, VM only, optional): include RAM state
func rotateSnapshots(ctx context.Context, client *pve.Client, isVM bool, vmid int, params map[string]interface{}) (string, error) {
	retention := 7
	if v, ok := params["retention"].(float64); ok && v > 0 {
		retention = int(v)
	} else if v, ok := params["retention"].(int); ok && v > 0 {
		retention = v
	}
	description, _ := params["description"].(string)
	if description == "" {
		description = "pCenter scheduled snapshot"
	}
	vmstate := false
	if v, ok := params["vmstate"].(bool); ok {
		vmstate = v
	}

	// Generate snapshot name with timestamp (Proxmox allows letters/digits/dashes/underscores)
	name := snapshotAutoPrefix + time.Now().UTC().Format("20060102-150405")

	// Create new snapshot first so a failure doesn't leave us under retention
	var upid string
	var err error
	if isVM {
		upid, err = client.CreateVMSnapshot(ctx, vmid, name, description, vmstate)
	} else {
		upid, err = client.CreateContainerSnapshot(ctx, vmid, name, description)
	}
	if err != nil {
		return "", fmt.Errorf("create snapshot: %w", err)
	}

	// Prune old auto-* snapshots. Best-effort: if pruning fails, we still
	// return the successful create UPID so the run isn't marked as a failure.
	pruneErr := pruneAutoSnapshots(ctx, client, isVM, vmid, retention)
	if pruneErr != nil {
		// Encode prune error in UPID suffix so it appears in task history.
		return upid, fmt.Errorf("snapshot created (%s) but prune failed: %w", upid, pruneErr)
	}
	return upid, nil
}

// pruneAutoSnapshots lists all `auto-*` snapshots, sorts them newest-first,
// and deletes any beyond index `retention`.
func pruneAutoSnapshots(ctx context.Context, client *pve.Client, isVM bool, vmid, retention int) error {
	var snaps []pve.Snapshot
	var err error
	if isVM {
		snaps, err = client.ListVMSnapshots(ctx, vmid)
	} else {
		snaps, err = client.ListContainerSnapshots(ctx, vmid)
	}
	if err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}

	// Filter to auto-prefix only; exclude the special "current" pseudo-snapshot
	// which some PVE responses include.
	var eligible []pve.Snapshot
	for _, s := range snaps {
		if s.Name == "current" {
			continue
		}
		if strings.HasPrefix(s.Name, snapshotAutoPrefix) {
			eligible = append(eligible, s)
		}
	}

	// Sort newest first (by SnapTime; fall back to name which is timestamp-sortable)
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].SnapTime != eligible[j].SnapTime {
			return eligible[i].SnapTime > eligible[j].SnapTime
		}
		return eligible[i].Name > eligible[j].Name
	})

	if len(eligible) <= retention {
		return nil
	}

	// Delete everything beyond the retention count
	var deleteErrs []string
	for _, s := range eligible[retention:] {
		var derr error
		if isVM {
			_, derr = client.DeleteVMSnapshot(ctx, vmid, s.Name)
		} else {
			_, derr = client.DeleteContainerSnapshot(ctx, vmid, s.Name)
		}
		if derr != nil {
			deleteErrs = append(deleteErrs, fmt.Sprintf("%s: %v", s.Name, derr))
		}
	}
	if len(deleteErrs) > 0 {
		return fmt.Errorf("failed to delete %d: %s", len(deleteErrs), strings.Join(deleteErrs, "; "))
	}
	return nil
}
