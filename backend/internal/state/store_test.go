package state

import (
	"testing"

	"github.com/moconnor/pcenter/internal/pve"
)

// TestStore_NewEmpty verifies a fresh store has no data.
func TestStore_NewEmpty(t *testing.T) {
	store := New()

	if names := store.GetClusterNames(); len(names) != 0 {
		t.Errorf("new store should have 0 clusters, got %d", len(names))
	}
	if vms := store.GetVMs(); len(vms) != 0 {
		t.Errorf("new store should have 0 VMs, got %d", len(vms))
	}
	if cts := store.GetContainers(); len(cts) != 0 {
		t.Errorf("new store should have 0 containers, got %d", len(cts))
	}
	if nodes := store.GetNodes(); len(nodes) != 0 {
		t.Errorf("new store should have 0 nodes, got %d", len(nodes))
	}
}

// TestStore_GetOrCreateCluster verifies cluster creation and retrieval.
func TestStore_GetOrCreateCluster(t *testing.T) {
	store := New()

	cs := store.GetOrCreateCluster("prod")
	if cs == nil {
		t.Fatal("GetOrCreateCluster should return non-nil")
	}

	// Same name returns same instance
	cs2 := store.GetOrCreateCluster("prod")
	if cs != cs2 {
		t.Error("GetOrCreateCluster should return same instance for same name")
	}

	// GetCluster should find it
	found, ok := store.GetCluster("prod")
	if !ok || found == nil {
		t.Error("GetCluster should find created cluster")
	}

	// Non-existent cluster
	_, ok = store.GetCluster("nonexistent")
	if ok {
		t.Error("GetCluster should return false for non-existent cluster")
	}
}

// TestStore_UpdateNode verifies that updating node data stores VMs,
// containers, and storage correctly.
func TestStore_UpdateNode(t *testing.T) {
	store := New()
	cs := store.GetOrCreateCluster("test")

	node := pve.Node{
		Node:    "pve01",
		Cluster: "test",
		Status:  "online",
		CPU:     0.25,
		MaxCPU:  4,
		Mem:     4 * 1024 * 1024 * 1024,
		MaxMem:  16 * 1024 * 1024 * 1024,
	}

	vms := []pve.VM{
		{VMID: 100, Name: "web-server", Node: "pve01", Cluster: "test", Status: "running"},
		{VMID: 101, Name: "db-server", Node: "pve01", Cluster: "test", Status: "stopped"},
	}

	cts := []pve.Container{
		{VMID: 200, Name: "redis", Node: "pve01", Cluster: "test", Status: "running"},
	}

	storage := []pve.Storage{
		{Storage: "local-lvm", Node: "pve01", Cluster: "test", Type: "lvmthin", Total: 100 * 1024 * 1024 * 1024},
	}

	cs.UpdateNode("pve01", node, vms, cts, storage, nil)

	// Check global store
	allVMs := store.GetVMs()
	if len(allVMs) != 2 {
		t.Errorf("expected 2 VMs, got %d", len(allVMs))
	}

	allCTs := store.GetContainers()
	if len(allCTs) != 1 {
		t.Errorf("expected 1 container, got %d", len(allCTs))
	}

	allNodes := store.GetNodes()
	if len(allNodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(allNodes))
	}

	// Check per-cluster
	vm, ok := cs.GetVM(100)
	if !ok {
		t.Fatal("GetVM(100) should find the VM")
	}
	if vm.Name != "web-server" {
		t.Errorf("expected VM name 'web-server', got %q", vm.Name)
	}

	ct, ok := cs.GetContainer(200)
	if !ok {
		t.Fatal("GetContainer(200) should find the container")
	}
	if ct.Name != "redis" {
		t.Errorf("expected container name 'redis', got %q", ct.Name)
	}

	// Non-existent guest
	_, ok = cs.GetVM(999)
	if ok {
		t.Error("GetVM(999) should return false")
	}
}

// TestStore_GetVM_SearchesAllClusters verifies that the global GetVM
// searches across all clusters.
func TestStore_GetVM_SearchesAllClusters(t *testing.T) {
	store := New()

	cs1 := store.GetOrCreateCluster("cluster-a")
	cs1.UpdateNode("node1", pve.Node{Node: "node1", Cluster: "cluster-a"}, []pve.VM{
		{VMID: 100, Name: "vm-a", Cluster: "cluster-a"},
	}, nil, nil, nil)

	cs2 := store.GetOrCreateCluster("cluster-b")
	cs2.UpdateNode("node2", pve.Node{Node: "node2", Cluster: "cluster-b"}, []pve.VM{
		{VMID: 200, Name: "vm-b", Cluster: "cluster-b"},
	}, nil, nil, nil)

	// Find VM in cluster-a
	vm, ok := store.GetVM(100)
	if !ok {
		t.Fatal("GetVM(100) should find VM across clusters")
	}
	if vm.Cluster != "cluster-a" {
		t.Errorf("expected cluster 'cluster-a', got %q", vm.Cluster)
	}

	// Find VM in cluster-b
	vm, ok = store.GetVM(200)
	if !ok {
		t.Fatal("GetVM(200) should find VM across clusters")
	}
	if vm.Cluster != "cluster-b" {
		t.Errorf("expected cluster 'cluster-b', got %q", vm.Cluster)
	}
}

// TestStore_Summary verifies that global summary aggregates correctly.
func TestStore_Summary(t *testing.T) {
	store := New()
	cs := store.GetOrCreateCluster("prod")

	cs.UpdateNode("pve01",
		pve.Node{Node: "pve01", Cluster: "prod", Status: "online"},
		[]pve.VM{
			{VMID: 100, Status: "running", Cluster: "prod", Node: "pve01"},
			{VMID: 101, Status: "stopped", Cluster: "prod", Node: "pve01"},
		},
		[]pve.Container{
			{VMID: 200, Status: "running", Cluster: "prod", Node: "pve01"},
		},
		nil, nil,
	)

	summary := store.GetGlobalSummary()

	if summary.Total.TotalNodes != 1 {
		t.Errorf("expected 1 node, got %d", summary.Total.TotalNodes)
	}
	if summary.Total.TotalVMs != 2 {
		t.Errorf("expected 2 VMs, got %d", summary.Total.TotalVMs)
	}
	if summary.Total.RunningVMs != 1 {
		t.Errorf("expected 1 running VM, got %d", summary.Total.RunningVMs)
	}
	if summary.Total.TotalContainers != 1 {
		t.Errorf("expected 1 container, got %d", summary.Total.TotalContainers)
	}
	if summary.Total.RunningCTs != 1 {
		t.Errorf("expected 1 running container, got %d", summary.Total.RunningCTs)
	}
}

// TestStore_ConcurrentAccess verifies that concurrent reads and writes
// don't panic (basic race condition check — run with -race flag).
func TestStore_ConcurrentAccess(t *testing.T) {
	store := New()
	cs := store.GetOrCreateCluster("test")

	done := make(chan bool, 4)

	// Writer
	go func() {
		for i := 0; i < 100; i++ {
			cs.UpdateNode("pve01",
				pve.Node{Node: "pve01", Cluster: "test", Status: "online"},
				[]pve.VM{{VMID: 100, Status: "running"}},
				nil, nil, nil,
			)
		}
		done <- true
	}()

	// Readers
	for range 3 {
		go func() {
			for range 100 {
				store.GetVMs()
				store.GetNodes()
				store.GetGlobalSummary()
				store.GetVM(100)
			}
			done <- true
		}()
	}

	for range 4 {
		<-done
	}
}

// TestClusterStore_CephTopology verifies the cluster-wide Ceph topology
// can be set, retrieved, and cleared independently of the per-node ceph
// status map.
func TestClusterStore_CephTopology(t *testing.T) {
	store := New()
	cs := store.GetOrCreateCluster("test")

	// Default is nil — topology not yet populated.
	if got := cs.GetCephTopology(); got != nil {
		t.Errorf("new ClusterStore should have nil CephTopology, got %+v", got)
	}

	topology := &pve.CephCluster{
		MONs:  []pve.CephMON{{Name: "pve1", Quorum: true, State: "leader"}},
		OSDs:  []pve.CephOSD{{ID: 0, Name: "osd.0", Status: "up", In: true}},
		Pools: []pve.CephPool{{Name: "rbd", Size: 3}},
	}
	cs.SetCephTopology(topology)

	got := cs.GetCephTopology()
	if got == nil {
		t.Fatal("after SetCephTopology, GetCephTopology returned nil")
	}
	if len(got.MONs) != 1 || len(got.OSDs) != 1 || len(got.Pools) != 1 {
		t.Errorf("topology lost data after roundtrip: %+v", got)
	}

	// Setting nil clears (e.g. after Ceph uninstall).
	cs.SetCephTopology(nil)
	if got := cs.GetCephTopology(); got != nil {
		t.Errorf("SetCephTopology(nil) should clear, got %+v", got)
	}
}

// TestStore_GetCephTopology verifies the Store-level accessor by cluster name,
// including the nil cases (unknown cluster, cluster with no topology yet).
func TestStore_GetCephTopology(t *testing.T) {
	store := New()

	// Unknown cluster returns nil, doesn't panic.
	if got := store.GetCephTopology("nonexistent"); got != nil {
		t.Errorf("unknown cluster should return nil, got %+v", got)
	}

	// Known cluster with no topology returns nil.
	cs := store.GetOrCreateCluster("prod")
	if got := store.GetCephTopology("prod"); got != nil {
		t.Errorf("cluster without topology should return nil, got %+v", got)
	}

	cs.SetCephTopology(&pve.CephCluster{
		Pools: []pve.CephPool{{Name: "rbd"}},
	})

	got := store.GetCephTopology("prod")
	if got == nil || len(got.Pools) != 1 || got.Pools[0].Name != "rbd" {
		t.Errorf("expected topology with rbd pool, got %+v", got)
	}
}
