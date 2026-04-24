package inventory

import (
	"context"
	"testing"

	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/pve"
)

// switchableProber lets tests change what the prober returns between calls.
type switchableProber struct {
	membership *pve.ClusterMembership
}

func (s *switchableProber) ProbeClusterMembership(ctx context.Context, cfg config.ClusterConfig) (*pve.ClusterMembership, error) {
	return s.membership, nil
}

func TestReconciler_PromotesStandaloneWhenJoiningCluster(t *testing.T) {
	prober := &switchableProber{membership: &pve.ClusterMembership{IsCluster: false}}
	svc := NewServiceWithProber(openTestDB(t), prober)
	ctx := context.Background()

	// Add as standalone initially.
	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})
	result, _, err := svc.AddHostAutoRoute(ctx, dc.ID, AddHostRequest{
		Address: "10.0.0.11:8006", TokenID: "root@pam!t", TokenSecret: "s",
	})
	if err != nil {
		t.Fatalf("initial add: %v", err)
	}
	if !result.Standalone {
		t.Fatalf("expected standalone initially")
	}
	// Host must be online for the reconciler to probe it.
	svc.SetHostStatus(ctx, result.Host.ID, HostStatusOnline, "", "pve01")

	// Simulate: the PVE node has now joined a real cluster.
	prober.membership = &pve.ClusterMembership{IsCluster: true, ClusterName: "prod-a"}

	var promotedHost *InventoryHost
	var promotedCluster *Cluster
	rec := NewReconciler(svc, 0)
	rec.OnPromote(func(h *InventoryHost, c *Cluster, _ string) {
		promotedHost = h
		promotedCluster = c
	})
	rec.RunOnce(ctx)

	if promotedHost == nil || promotedCluster == nil {
		t.Fatalf("expected OnPromote to fire")
	}
	if promotedCluster.PVEClusterName != "prod-a" {
		t.Fatalf("expected cluster PVEClusterName=prod-a, got %q", promotedCluster.PVEClusterName)
	}

	// Host must no longer be standalone.
	updated, _ := svc.GetHost(ctx, result.Host.ID)
	if updated.ClusterID == "" {
		t.Fatalf("expected host moved under cluster")
	}
	if updated.ClusterID != promotedCluster.ID {
		t.Fatalf("expected host ClusterID=%s, got %s", promotedCluster.ID, updated.ClusterID)
	}

	// Standalone tree slot is empty now.
	standalones, _ := svc.ListHostsByDatacenter(ctx, dc.ID)
	if len(standalones) != 0 {
		t.Fatalf("expected 0 standalones post-promotion, got %d", len(standalones))
	}
}

func TestReconciler_AttachesToExistingClusterByPVEName(t *testing.T) {
	prober := &switchableProber{membership: &pve.ClusterMembership{IsCluster: false}}
	svc := NewServiceWithProber(openTestDB(t), prober)
	ctx := context.Background()

	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})

	// Pre-existing pcenter cluster correlated to PVE cluster "prod-a".
	existing, err := svc.db.CreateCluster(ctx, CreateClusterRequest{
		Name: "prod-a", DatacenterID: &dc.ID, PVEClusterName: "prod-a",
	})
	if err != nil {
		t.Fatalf("pre-create cluster: %v", err)
	}

	// Add a separate standalone that will later report the same PVE cluster.
	addResult, _, _ := svc.AddHostAutoRoute(ctx, dc.ID, AddHostRequest{
		Address: "10.0.0.22:8006", TokenID: "root@pam!t", TokenSecret: "s",
	})
	svc.SetHostStatus(ctx, addResult.Host.ID, HostStatusOnline, "", "pve02")

	prober.membership = &pve.ClusterMembership{IsCluster: true, ClusterName: "prod-a"}
	NewReconciler(svc, 0).RunOnce(ctx)

	updated, _ := svc.GetHost(ctx, addResult.Host.ID)
	if updated.ClusterID != existing.ID {
		t.Fatalf("expected host attached to existing cluster %s, got %s",
			existing.ID, updated.ClusterID)
	}
}

func TestReconciler_ReconcileLegacy_DemotesSingleHostFakeCluster(t *testing.T) {
	// A legacy cluster (pve_cluster_name = "") with one host that probes as
	// standalone should be demoted: host moves to datacenter-standalone, cluster deleted.
	prober := &switchableProber{membership: &pve.ClusterMembership{IsCluster: false}}
	svc := NewServiceWithProber(openTestDB(t), prober)
	ctx := context.Background()

	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})
	cluster, _ := svc.db.CreateCluster(ctx, CreateClusterRequest{Name: "default", DatacenterID: &dc.ID})
	host, _ := svc.db.AddHost(ctx, cluster.ID, AddHostRequest{
		Address: "10.0.0.50:8006", TokenID: "root@pam!t", TokenSecret: "s", Insecure: true,
	})
	svc.SetHostStatus(ctx, host.ID, HostStatusOnline, "", "pve50")

	rec := NewReconciler(svc, 0)
	result, err := rec.ReconcileLegacy(ctx)
	if err != nil {
		t.Fatalf("reconcile legacy: %v", err)
	}
	if len(result.DemotedHosts) != 1 || result.DemotedHosts[0] != host.Address {
		t.Fatalf("expected 1 demoted host %q, got %+v", host.Address, result.DemotedHosts)
	}
	if len(result.DeletedClusters) != 1 || result.DeletedClusters[0] != "default" {
		t.Fatalf("expected 'default' deleted, got %+v", result.DeletedClusters)
	}

	// Verify end state: one standalone host, no clusters.
	standalones, _ := svc.ListHostsByDatacenter(ctx, dc.ID)
	if len(standalones) != 1 {
		t.Fatalf("expected 1 standalone, got %d", len(standalones))
	}
	if standalones[0].Address != host.Address {
		t.Fatalf("standalone address mismatch: %q vs %q", standalones[0].Address, host.Address)
	}
	clusters, _ := svc.ListClusters(ctx)
	if len(clusters) != 0 {
		t.Fatalf("expected 0 clusters, got %d", len(clusters))
	}
}

func TestReconciler_ReconcileLegacy_CorrelatesRealCluster(t *testing.T) {
	// A legacy cluster whose host probes as a real PVE cluster should have
	// pve_cluster_name populated (no host movement).
	prober := &switchableProber{membership: &pve.ClusterMembership{IsCluster: true, ClusterName: "prod-a"}}
	svc := NewServiceWithProber(openTestDB(t), prober)
	ctx := context.Background()

	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})
	cluster, _ := svc.db.CreateCluster(ctx, CreateClusterRequest{Name: "legacy-named", DatacenterID: &dc.ID})
	svc.db.AddHost(ctx, cluster.ID, AddHostRequest{
		Address: "10.0.0.60:8006", TokenID: "root@pam!t", TokenSecret: "s",
	})

	rec := NewReconciler(svc, 0)
	result, err := rec.ReconcileLegacy(ctx)
	if err != nil {
		t.Fatalf("reconcile legacy: %v", err)
	}
	if len(result.CorrelatedClusters) != 1 {
		t.Fatalf("expected 1 correlated, got %+v", result.CorrelatedClusters)
	}
	if len(result.DeletedClusters) != 0 {
		t.Fatalf("expected no deletions, got %+v", result.DeletedClusters)
	}

	updated, _ := svc.GetCluster(ctx, cluster.ID)
	if updated.PVEClusterName != "prod-a" {
		t.Fatalf("expected PVEClusterName=prod-a, got %q", updated.PVEClusterName)
	}
}

func TestReconciler_LeavesStandaloneAloneWhenStillStandalone(t *testing.T) {
	prober := &switchableProber{membership: &pve.ClusterMembership{IsCluster: false}}
	svc := NewServiceWithProber(openTestDB(t), prober)
	ctx := context.Background()

	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})
	res, _, _ := svc.AddHostAutoRoute(ctx, dc.ID, AddHostRequest{
		Address: "10.0.0.33:8006", TokenID: "root@pam!t", TokenSecret: "s",
	})
	svc.SetHostStatus(ctx, res.Host.ID, HostStatusOnline, "", "pve03")

	NewReconciler(svc, 0).RunOnce(ctx)

	standalones, _ := svc.ListHostsByDatacenter(ctx, dc.ID)
	if len(standalones) != 1 {
		t.Fatalf("expected 1 standalone, got %d", len(standalones))
	}
}
