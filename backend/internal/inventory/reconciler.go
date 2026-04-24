package inventory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/moconnor/pcenter/internal/config"
)

// Reconciler periodically probes standalone hosts and promotes them when
// they've joined a real PVE cluster. It also flags mismatches for hosts
// already filed under a cluster (different pve_cluster_name than the pcenter
// cluster's recorded one) — those are logged, not auto-moved, matching the
// "demotion is a user action" rule.
//
// Runs as a separate goroutine rather than as part of the per-cluster poll
// loop: (a) cluster-membership rarely changes, so per-tick probing is
// wasteful; (b) this is tolerant of minutes of latency; (c) it doesn't need
// any per-cluster state to do its job.
type Reconciler struct {
	svc      *Service
	interval time.Duration

	// onPromote is called when a standalone host is promoted into a cluster;
	// callers use it to rewire poller/secrets bookkeeping (the "standalone:<id>"
	// key goes away, the cluster's key takes over).
	onPromote func(host *InventoryHost, cluster *Cluster, tokenSecret string)
}

// NewReconciler builds a Reconciler. Pass 0 for interval to use the default (10 min).
func NewReconciler(svc *Service, interval time.Duration) *Reconciler {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &Reconciler{svc: svc, interval: interval}
}

// OnPromote sets a callback invoked when a host is promoted from standalone to
// a cluster. Safe to call before Start.
func (r *Reconciler) OnPromote(fn func(host *InventoryHost, cluster *Cluster, tokenSecret string)) {
	r.onPromote = fn
}

// Start runs the reconciler loop until ctx is cancelled.
func (r *Reconciler) Start(ctx context.Context) {
	slog.Info("cluster reconciler started", "interval", r.interval)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// First pass after a short delay so startup isn't thrashy.
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}
	r.RunOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("cluster reconciler stopped")
			return
		case <-ticker.C:
			r.RunOnce(ctx)
		}
	}
}

// RunOnce performs one reconciliation pass. Exposed for tests and for the
// opt-in /api/inventory/reconcile endpoint.
func (r *Reconciler) RunOnce(ctx context.Context) {
	if err := r.reconcileStandalones(ctx); err != nil {
		slog.Warn("reconcile standalones failed", "error", err)
	}
	if err := r.checkExistingClusters(ctx); err != nil {
		slog.Warn("reconcile existing clusters failed", "error", err)
	}
}

// reconcileStandalones promotes any standalone host that now reports itself
// as part of a real PVE cluster.
func (r *Reconciler) reconcileStandalones(ctx context.Context) error {
	dcs, _, err := r.svc.GetDatacenterTree(ctx)
	if err != nil {
		return err
	}

	for _, dc := range dcs {
		for _, host := range dc.Hosts {
			if host.Status != HostStatusOnline {
				continue // skip flaky hosts; we'll retry next tick
			}
			if host.TokenSecret == "" {
				continue // can't probe without creds
			}

			cfg := config.ClusterConfig{
				DiscoveryNode: host.Address,
				TokenID:       host.TokenID,
				TokenSecret:   host.TokenSecret,
				Insecure:      host.Insecure,
			}
			membership, err := r.svc.prober.ProbeClusterMembership(ctx, cfg)
			if err != nil {
				slog.Debug("reconciler probe failed", "host", host.ID, "address", host.Address, "error", err)
				continue
			}
			if !membership.IsCluster {
				continue // still standalone
			}

			if err := r.promoteStandalone(ctx, host, dc.ID, membership.ClusterName); err != nil {
				slog.Warn("promote failed", "host", host.ID, "pve_cluster", membership.ClusterName, "error", err)
			}
		}
	}
	return nil
}

// promoteStandalone moves a standalone host under the pcenter cluster
// correlated to pveClusterName (creating the cluster if needed), and notifies
// the OnPromote callback so the poller can be rewired.
func (r *Reconciler) promoteStandalone(ctx context.Context, host InventoryHost, datacenterID, pveClusterName string) error {
	cluster, err := r.svc.db.GetClusterByPVEName(ctx, pveClusterName)
	if err != nil {
		return err
	}

	if cluster == nil {
		// Disambiguate the display name if it collides with a legacy cluster.
		name := pveClusterName
		if existing, _ := r.svc.db.GetClusterByName(ctx, name); existing != nil {
			name = pveClusterName + " (" + host.Address + ")"
		}
		dcID := datacenterID
		cluster, err = r.svc.db.CreateCluster(ctx, CreateClusterRequest{
			Name:           name,
			DatacenterID:   &dcID,
			PVEClusterName: pveClusterName,
		})
		if err != nil {
			return err
		}
		slog.Info("reconciler created cluster", "cluster", cluster.Name, "pve_cluster", pveClusterName)
	}

	if err := r.svc.db.MoveHostToCluster(ctx, host.ID, cluster.ID); err != nil {
		return err
	}
	if cluster.Status == ClusterStatusEmpty {
		r.svc.db.SetClusterStatus(ctx, cluster.ID, ClusterStatusActive)
	}

	slog.Info("promoted standalone to cluster", "host", host.ID, "address", host.Address, "cluster", cluster.Name)

	if r.onPromote != nil {
		r.onPromote(&host, cluster, host.TokenSecret)
	}
	return nil
}

// LegacyReconcileResult summarizes the opt-in cleanup of pre-existing clusters
// that were never correlated to a real PVE cluster (PVEClusterName == "").
type LegacyReconcileResult struct {
	CorrelatedClusters []string `json:"correlated_clusters,omitempty"` // cluster names now bound to their real PVE name
	DemotedHosts       []string `json:"demoted_hosts,omitempty"`       // hosts moved from cluster to standalone (addresses)
	DeletedClusters    []string `json:"deleted_clusters,omitempty"`    // fake clusters deleted after demotion
	SkippedClusters    []string `json:"skipped_clusters,omitempty"`    // clusters left as-is (e.g. multi-host but no PVE probe match)
	Errors             []string `json:"errors,omitempty"`
}

// ReconcileLegacy is the opt-in cleanup pass for pcenter clusters that pre-date
// the PVE-correlation model (PVEClusterName == ""). For each such cluster:
//
//   - If exactly one host and it probes as standalone → move host to its
//     datacenter as standalone, delete the now-empty cluster.
//   - If any host probes as a real cluster → set PVEClusterName on the
//     pcenter cluster (no host movement, no rename).
//   - Otherwise leave alone (multi-host with no consistent probe result is a
//     user-intent situation, not a cleanup case).
//
// Triggered from POST /api/inventory/reconcile.
func (r *Reconciler) ReconcileLegacy(ctx context.Context) (*LegacyReconcileResult, error) {
	out := &LegacyReconcileResult{}

	clusters, err := r.svc.db.ListClusters(ctx)
	if err != nil {
		return nil, err
	}

	for _, c := range clusters {
		if c.PVEClusterName != "" {
			continue // already correlated
		}
		hosts, err := r.svc.db.ListHostsByCluster(ctx, c.ID)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: list hosts: %v", c.Name, err))
			continue
		}
		if len(hosts) == 0 {
			out.SkippedClusters = append(out.SkippedClusters, c.Name+" (empty)")
			continue
		}

		// Probe the first host with credentials.
		var probeHost *InventoryHost
		for i := range hosts {
			if hosts[i].TokenSecret != "" {
				probeHost = &hosts[i]
				break
			}
		}
		if probeHost == nil {
			out.SkippedClusters = append(out.SkippedClusters, c.Name+" (no probe credentials)")
			continue
		}

		cfg := config.ClusterConfig{
			DiscoveryNode: probeHost.Address,
			TokenID:       probeHost.TokenID,
			TokenSecret:   probeHost.TokenSecret,
			Insecure:      probeHost.Insecure,
		}
		membership, err := r.svc.prober.ProbeClusterMembership(ctx, cfg)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: probe failed: %v", c.Name, err))
			continue
		}

		if membership.IsCluster {
			if err := r.svc.db.SetPVEClusterName(ctx, c.ID, membership.ClusterName); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("%s: set pve name: %v", c.Name, err))
				continue
			}
			out.CorrelatedClusters = append(out.CorrelatedClusters, c.Name+" → "+membership.ClusterName)
			continue
		}

		// Standalone probe result. Only demote the single-host case: a cluster
		// with multiple hosts that reports standalone is inconsistent (users may
		// have hand-populated it) — leave it alone.
		if len(hosts) != 1 {
			out.SkippedClusters = append(out.SkippedClusters, c.Name+" (multi-host, standalone probe)")
			continue
		}

		if c.DatacenterID == nil {
			out.SkippedClusters = append(out.SkippedClusters, c.Name+" (orphan cluster, no datacenter)")
			continue
		}

		// Move the host: delete the host row, re-add as standalone under the
		// datacenter (preserving credentials). Do it in that order so we don't
		// leave orphan rows on partial failure.
		host := hosts[0]
		if err := r.svc.db.DeleteHost(ctx, host.ID); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: delete host: %v", c.Name, err))
			continue
		}
		newHost, err := r.svc.db.AddDatacenterHost(ctx, *c.DatacenterID, AddHostRequest{
			Address:     host.Address,
			TokenID:     host.TokenID,
			TokenSecret: host.TokenSecret,
			Insecure:    host.Insecure,
		})
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: re-add as standalone: %v", c.Name, err))
			continue
		}
		// Restore status so the poller keeps it live.
		_ = r.svc.db.SetHostStatus(ctx, newHost.ID, host.Status, host.Error, host.NodeName)

		if err := r.svc.db.DeleteCluster(ctx, c.ID); err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: delete empty cluster: %v", c.Name, err))
			continue
		}
		out.DemotedHosts = append(out.DemotedHosts, host.Address)
		out.DeletedClusters = append(out.DeletedClusters, c.Name)
	}

	return out, nil
}

// checkExistingClusters logs (does not fix) hosts whose probe reports a
// different PVE cluster than their pcenter cluster's recorded pve_cluster_name.
// Auto-moving would step on user intent — the rule is user-triggered demotion.
func (r *Reconciler) checkExistingClusters(ctx context.Context) error {
	clusters, err := r.svc.db.ListClusters(ctx)
	if err != nil {
		return err
	}

	for _, c := range clusters {
		if c.PVEClusterName == "" {
			continue // not correlated to a real PVE cluster; /api/inventory/reconcile handles these
		}
		hosts, err := r.svc.db.ListHostsByCluster(ctx, c.ID)
		if err != nil {
			continue
		}
		for _, host := range hosts {
			if host.Status != HostStatusOnline || host.TokenSecret == "" {
				continue
			}
			cfg := config.ClusterConfig{
				DiscoveryNode: host.Address,
				TokenID:       host.TokenID,
				TokenSecret:   host.TokenSecret,
				Insecure:      host.Insecure,
			}
			membership, err := r.svc.prober.ProbeClusterMembership(ctx, cfg)
			if err != nil {
				continue
			}
			if !membership.IsCluster {
				slog.Warn("cluster member no longer in a PVE cluster",
					"cluster", c.Name, "pve_cluster", c.PVEClusterName,
					"host", host.Address)
				continue
			}
			if membership.ClusterName != c.PVEClusterName {
				slog.Warn("cluster member reports different PVE cluster",
					"cluster", c.Name, "expected", c.PVEClusterName,
					"reported", membership.ClusterName, "host", host.Address)
			}
		}
	}
	return nil
}
