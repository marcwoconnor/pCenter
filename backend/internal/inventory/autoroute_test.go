package inventory

import (
	"context"
	"testing"

	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/pve"
)

// fakeProber returns a canned ClusterMembership for tests.
type fakeProber struct {
	membership *pve.ClusterMembership
	err        error
}

func (f *fakeProber) ProbeClusterMembership(ctx context.Context, cfg config.ClusterConfig) (*pve.ClusterMembership, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.membership, nil
}

func newAutoRouteService(t *testing.T, prober ClusterProber) *Service {
	t.Helper()
	return NewServiceWithProber(openTestDB(t), prober)
}

func TestAddHostAutoRoute_StandaloneFilesUnderDatacenter(t *testing.T) {
	prober := &fakeProber{membership: &pve.ClusterMembership{IsCluster: false}}
	svc := newAutoRouteService(t, prober)
	ctx := context.Background()

	dc, err := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})
	if err != nil {
		t.Fatalf("create dc: %v", err)
	}

	result, _, err := svc.AddHostAutoRoute(ctx, dc.ID, AddHostRequest{
		Address:     "10.0.0.10:8006",
		TokenID:     "root@pam!t",
		TokenSecret: "secret",
		Insecure:    true,
	})
	if err != nil {
		t.Fatalf("auto-route: %v", err)
	}
	if !result.Standalone {
		t.Fatalf("expected standalone routing")
	}
	if result.Cluster != nil {
		t.Fatalf("expected no cluster, got %+v", result.Cluster)
	}
	if result.Host.DatacenterID != dc.ID {
		t.Fatalf("expected host DatacenterID=%s, got %s", dc.ID, result.Host.DatacenterID)
	}
	if result.Host.ClusterID != "" {
		t.Fatalf("expected no ClusterID on standalone, got %q", result.Host.ClusterID)
	}

	// No cluster record should have been created.
	clusters, _ := svc.ListClusters(ctx)
	if len(clusters) != 0 {
		t.Fatalf("expected 0 clusters, got %d", len(clusters))
	}
}

func TestAddHostAutoRoute_RealClusterCreatesClusterRecord(t *testing.T) {
	prober := &fakeProber{membership: &pve.ClusterMembership{
		IsCluster:   true,
		ClusterName: "prod-a",
		Quorate:     true,
	}}
	svc := newAutoRouteService(t, prober)
	ctx := context.Background()

	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})

	result, _, err := svc.AddHostAutoRoute(ctx, dc.ID, AddHostRequest{
		Address:     "10.0.0.11:8006",
		TokenID:     "root@pam!t",
		TokenSecret: "secret",
	})
	if err != nil {
		t.Fatalf("auto-route: %v", err)
	}
	if result.Standalone {
		t.Fatalf("expected cluster routing, got standalone")
	}
	if result.Cluster == nil {
		t.Fatalf("expected cluster record")
	}
	if result.Cluster.PVEClusterName != "prod-a" {
		t.Fatalf("expected PVEClusterName=prod-a, got %q", result.Cluster.PVEClusterName)
	}
	if result.Cluster.Name != "prod-a" {
		t.Fatalf("expected cluster display name=prod-a, got %q", result.Cluster.Name)
	}
	if result.Cluster.DatacenterID == nil || *result.Cluster.DatacenterID != dc.ID {
		t.Fatalf("expected cluster under dc1")
	}
	if result.Host.ClusterID != result.Cluster.ID {
		t.Fatalf("expected host ClusterID=%s, got %s", result.Cluster.ID, result.Host.ClusterID)
	}
	if result.DetectedPVECluster != "prod-a" {
		t.Fatalf("expected DetectedPVECluster=prod-a, got %q", result.DetectedPVECluster)
	}
}

func TestAddHostAutoRoute_SecondHostAttachesToExistingCluster(t *testing.T) {
	prober := &fakeProber{membership: &pve.ClusterMembership{
		IsCluster:   true,
		ClusterName: "prod-a",
	}}
	svc := newAutoRouteService(t, prober)
	ctx := context.Background()

	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})

	first, _, err := svc.AddHostAutoRoute(ctx, dc.ID, AddHostRequest{
		Address: "10.0.0.11:8006", TokenID: "root@pam!t", TokenSecret: "s",
	})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}

	second, _, err := svc.AddHostAutoRoute(ctx, dc.ID, AddHostRequest{
		Address: "10.0.0.12:8006", TokenID: "root@pam!t", TokenSecret: "s",
	})
	if err != nil {
		t.Fatalf("second add: %v", err)
	}

	if first.Cluster.ID != second.Cluster.ID {
		t.Fatalf("expected same cluster on second add, got %s vs %s",
			first.Cluster.ID, second.Cluster.ID)
	}

	// Only one cluster record in total.
	clusters, _ := svc.ListClusters(ctx)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster total, got %d", len(clusters))
	}
	if len(clusters[0].Hosts) != 2 {
		t.Fatalf("expected 2 hosts under cluster, got %d", len(clusters[0].Hosts))
	}
}

func TestAddHostAutoRoute_NameCollisionFallsBackToSuffixedName(t *testing.T) {
	// Pre-existing cluster named "default" (e.g. legacy migration) blocks the
	// display-name path; the PVE-correlated cluster must still be created under
	// a disambiguated name.
	prober := &fakeProber{membership: &pve.ClusterMembership{
		IsCluster:   true,
		ClusterName: "default",
	}}
	svc := newAutoRouteService(t, prober)
	ctx := context.Background()

	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})
	// Existing cluster named "default" with no pve_cluster_name (legacy-style).
	_, err := svc.CreateCluster(ctx, CreateClusterRequest{Name: "default", DatacenterID: &dc.ID})
	if err != nil {
		t.Fatalf("precreate: %v", err)
	}

	result, _, err := svc.AddHostAutoRoute(ctx, dc.ID, AddHostRequest{
		Address: "10.0.0.11:8006", TokenID: "root@pam!t", TokenSecret: "s",
	})
	if err != nil {
		t.Fatalf("auto-route: %v", err)
	}
	if result.Cluster == nil {
		t.Fatalf("expected cluster record")
	}
	if result.Cluster.Name == "default" {
		t.Fatalf("expected disambiguated name, got collision with legacy cluster")
	}
	if result.Cluster.PVEClusterName != "default" {
		t.Fatalf("expected PVEClusterName=default, got %q", result.Cluster.PVEClusterName)
	}
}
