package inventory

import (
	"context"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open :memory: db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(openTestDB(t))
}

func TestOpenMemoryCreatessTables(t *testing.T) {
	db := openTestDB(t)

	// Verify tables exist by querying them
	ctx := context.Background()
	var count int
	if err := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM datacenters").Scan(&count); err != nil {
		t.Fatalf("datacenters table missing: %v", err)
	}
	if err := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM clusters").Scan(&count); err != nil {
		t.Fatalf("clusters table missing: %v", err)
	}
	if err := db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_hosts").Scan(&count); err != nil {
		t.Fatalf("inventory_hosts table missing: %v", err)
	}
}

func TestCreateDatacenter(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	dc, err := svc.CreateDatacenter(ctx, CreateDatacenterRequest{
		Name:        "dc-east",
		Description: "East coast DC",
	})
	if err != nil {
		t.Fatalf("create datacenter: %v", err)
	}
	if dc.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if dc.Name != "dc-east" {
		t.Fatalf("expected name dc-east, got %s", dc.Name)
	}
	if dc.Description != "East coast DC" {
		t.Fatalf("expected description, got %s", dc.Description)
	}
}

func TestCreateDatacenterRejectsDuplicate(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestCreateCluster(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Without datacenter
	c, err := svc.CreateCluster(ctx, CreateClusterRequest{Name: "cluster-a"})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if c.Name != "cluster-a" {
		t.Fatalf("expected cluster-a, got %s", c.Name)
	}
	if c.DatacenterID != nil {
		t.Fatal("expected nil datacenter_id for orphan cluster")
	}
	if c.Status != ClusterStatusEmpty {
		t.Fatalf("expected empty status, got %s", c.Status)
	}

	// With datacenter
	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc1"})
	c2, err := svc.CreateCluster(ctx, CreateClusterRequest{Name: "cluster-b", DatacenterID: &dc.ID})
	if err != nil {
		t.Fatalf("create cluster with dc: %v", err)
	}
	if c2.DatacenterID == nil || *c2.DatacenterID != dc.ID {
		t.Fatal("expected datacenter_id to match")
	}
}

func TestCreateClusterRejectsDuplicate(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.CreateCluster(ctx, CreateClusterRequest{Name: "dup"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = svc.CreateCluster(ctx, CreateClusterRequest{Name: "dup"})
	if err == nil {
		t.Fatal("expected error for duplicate cluster name")
	}
}

func TestAddHostToCluster(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	c, _ := svc.CreateCluster(ctx, CreateClusterRequest{Name: "mycluster"})

	host, err := svc.AddHost(ctx, c.ID, AddHostRequest{
		Address:     "10.0.0.1:8006",
		TokenID:     "root@pam!test",
		TokenSecret: "secret123",
		Insecure:    true,
	})
	if err != nil {
		t.Fatalf("add host: %v", err)
	}
	if host.Address != "10.0.0.1:8006" {
		t.Fatalf("expected address 10.0.0.1:8006, got %s", host.Address)
	}
	if host.TokenID != "root@pam!test" {
		t.Fatalf("expected token_id root@pam!test, got %s", host.TokenID)
	}
	if host.Status != HostStatusStaged {
		t.Fatalf("expected staged status, got %s", host.Status)
	}

	// Adding first host should move cluster from empty to pending
	updated, _ := svc.GetCluster(ctx, c.ID)
	if updated.Status != ClusterStatusPending {
		t.Fatalf("expected pending status after adding host, got %s", updated.Status)
	}
}

func TestListDatacenters(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc-a"})
	svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc-b"})

	dcs, err := svc.ListDatacenters(ctx)
	if err != nil {
		t.Fatalf("list datacenters: %v", err)
	}
	if len(dcs) != 2 {
		t.Fatalf("expected 2 datacenters, got %d", len(dcs))
	}
}

func TestListClusters(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	svc.CreateCluster(ctx, CreateClusterRequest{Name: "c1"})
	svc.CreateCluster(ctx, CreateClusterRequest{Name: "c2"})
	svc.CreateCluster(ctx, CreateClusterRequest{Name: "c3"})

	clusters, err := svc.ListClusters(ctx)
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if len(clusters) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(clusters))
	}
}

func TestDeleteDatacenterCascadesToClusters(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "dc-delete"})
	svc.CreateCluster(ctx, CreateClusterRequest{Name: "child-cluster", DatacenterID: &dc.ID})

	err := svc.DeleteDatacenter(ctx, dc.ID)
	if err != nil {
		t.Fatalf("delete datacenter: %v", err)
	}

	// Cluster should still exist but with nil datacenter_id (ON DELETE SET NULL)
	c, err := svc.GetClusterByName(ctx, "child-cluster")
	if err != nil {
		t.Fatalf("get cluster after dc delete: %v", err)
	}
	if c == nil {
		t.Fatal("cluster should still exist after datacenter delete")
	}
	if c.DatacenterID != nil {
		t.Fatal("cluster datacenter_id should be nil after datacenter delete")
	}
}

func TestDeleteClusterCascadesToHosts(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	c, _ := svc.CreateCluster(ctx, CreateClusterRequest{Name: "to-delete"})
	host, _ := svc.AddHost(ctx, c.ID, AddHostRequest{
		Address:     "10.0.0.5:8006",
		TokenID:     "root@pam!tok",
		TokenSecret: "s",
		Insecure:    true,
	})

	err := svc.DeleteCluster(ctx, c.ID)
	if err != nil {
		t.Fatalf("delete cluster: %v", err)
	}

	// Host should be gone (CASCADE)
	h, err := svc.GetHost(ctx, host.ID)
	if err != nil {
		t.Fatalf("get host after cluster delete: %v", err)
	}
	if h != nil {
		t.Fatal("host should be deleted after cluster cascade delete")
	}
}

func TestGetDatacenterTree(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	dc, _ := svc.CreateDatacenter(ctx, CreateDatacenterRequest{Name: "tree-dc"})
	c1, _ := svc.CreateCluster(ctx, CreateClusterRequest{Name: "tree-c1", DatacenterID: &dc.ID})
	svc.CreateCluster(ctx, CreateClusterRequest{Name: "orphan-cluster"}) // no datacenter
	svc.AddHost(ctx, c1.ID, AddHostRequest{
		Address:     "10.0.0.1:8006",
		TokenID:     "root@pam!t",
		TokenSecret: "s",
		Insecure:    true,
	})

	datacenters, orphans, err := svc.GetDatacenterTree(ctx)
	if err != nil {
		t.Fatalf("get tree: %v", err)
	}

	if len(datacenters) != 1 {
		t.Fatalf("expected 1 datacenter, got %d", len(datacenters))
	}
	if len(datacenters[0].Clusters) != 1 {
		t.Fatalf("expected 1 cluster under dc, got %d", len(datacenters[0].Clusters))
	}
	if len(datacenters[0].Clusters[0].Hosts) != 1 {
		t.Fatalf("expected 1 host under cluster, got %d", len(datacenters[0].Clusters[0].Hosts))
	}
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan cluster, got %d", len(orphans))
	}
	if orphans[0].Name != "orphan-cluster" {
		t.Fatalf("expected orphan-cluster, got %s", orphans[0].Name)
	}
}

func TestSetHostStatus(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	c, _ := svc.CreateCluster(ctx, CreateClusterRequest{Name: "status-test"})
	host, _ := svc.AddHost(ctx, c.ID, AddHostRequest{
		Address:     "10.0.0.1:8006",
		TokenID:     "root@pam!t",
		TokenSecret: "s",
	})

	err := svc.SetHostStatus(ctx, host.ID, HostStatusOnline, "", "pve04")
	if err != nil {
		t.Fatalf("set host status: %v", err)
	}

	h, _ := svc.GetHost(ctx, host.ID)
	if h.Status != HostStatusOnline {
		t.Fatalf("expected online, got %s", h.Status)
	}
	if h.NodeName != "pve04" {
		t.Fatalf("expected pve04, got %s", h.NodeName)
	}

	// Set error status
	err = svc.SetHostStatus(ctx, host.ID, HostStatusError, "connection refused", "")
	if err != nil {
		t.Fatalf("set error status: %v", err)
	}
	h, _ = svc.GetHost(ctx, host.ID)
	if h.Status != HostStatusError {
		t.Fatalf("expected error, got %s", h.Status)
	}
	if h.Error != "connection refused" {
		t.Fatalf("expected error msg, got %s", h.Error)
	}
}

func TestSetClusterStatus(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	c, _ := svc.CreateCluster(ctx, CreateClusterRequest{Name: "cs-test"})

	err := svc.SetClusterStatus(ctx, c.ID, ClusterStatusActive)
	if err != nil {
		t.Fatalf("set cluster status: %v", err)
	}

	updated, _ := svc.GetCluster(ctx, c.ID)
	if updated.Status != ClusterStatusActive {
		t.Fatalf("expected active, got %s", updated.Status)
	}

	err = svc.SetClusterStatus(ctx, c.ID, ClusterStatusError)
	if err != nil {
		t.Fatalf("set error status: %v", err)
	}
	updated, _ = svc.GetCluster(ctx, c.ID)
	if updated.Status != ClusterStatusError {
		t.Fatalf("expected error, got %s", updated.Status)
	}
}
