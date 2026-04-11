package drs

import (
	"testing"

	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

func defaultDRSConfig() config.DRSConfig {
	return config.DRSConfig{
		Enabled:       true,
		Mode:          "manual",
		CheckInterval: 60,
		CPUThreshold:  0.80,
		MemThreshold:  0.80,
		MigrationRate: 2,
	}
}

func disabledDRSConfig() config.DRSConfig {
	cfg := defaultDRSConfig()
	cfg.Enabled = false
	return cfg
}

// populateBalancedCluster creates a cluster with even load across nodes.
func populateBalancedCluster(t *testing.T, store *state.Store, clusterName string) {
	t.Helper()
	cs := store.GetOrCreateCluster(clusterName)

	// Two nodes, both at ~30% CPU, ~30% mem
	for _, name := range []string{"node1", "node2"} {
		node := pve.Node{
			Node:   name,
			Status: "online",
			CPU:    0.30,
			MaxCPU: 16,
			Mem:    3 * 1024 * 1024 * 1024,  // 3GB
			MaxMem: 10 * 1024 * 1024 * 1024, // 10GB
		}
		vms := []pve.VM{
			{VMID: 100, Name: "vm-" + name, Node: name, Status: "running", CPU: 0.1, Mem: 512 * 1024 * 1024},
		}
		cs.UpdateNode(name, node, vms, nil, nil, nil)
	}
}

// populateImbalancedCluster creates a cluster with one overloaded and two underloaded nodes.
func populateImbalancedCluster(t *testing.T, store *state.Store, clusterName string) {
	t.Helper()
	cs := store.GetOrCreateCluster(clusterName)

	// Overloaded node: 90% CPU
	hotNode := pve.Node{
		Node:   "hot-node",
		Status: "online",
		CPU:    0.90,
		MaxCPU: 16,
		Mem:    9 * 1024 * 1024 * 1024,
		MaxMem: 10 * 1024 * 1024 * 1024,
	}
	hotVMs := []pve.VM{
		{VMID: 200, Name: "busy-vm-1", Node: "hot-node", Status: "running", CPU: 0.4, Mem: 2 * 1024 * 1024 * 1024},
		{VMID: 201, Name: "busy-vm-2", Node: "hot-node", Status: "running", CPU: 0.3, Mem: 1024 * 1024 * 1024},
	}
	cs.UpdateNode("hot-node", hotNode, hotVMs, nil, nil, nil)

	// Underloaded nodes: 10% CPU
	for _, name := range []string{"cold-node-1", "cold-node-2"} {
		node := pve.Node{
			Node:   name,
			Status: "online",
			CPU:    0.10,
			MaxCPU: 16,
			Mem:    1 * 1024 * 1024 * 1024,
			MaxMem: 10 * 1024 * 1024 * 1024,
		}
		cs.UpdateNode(name, node, nil, nil, nil, nil)
	}
}

func TestNewScheduler(t *testing.T) {
	store := state.New()
	cfg := defaultDRSConfig()
	s := NewScheduler(cfg, store)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if s.cfg.CPUThreshold != 0.80 {
		t.Fatalf("expected CPU threshold 0.80, got %f", s.cfg.CPUThreshold)
	}
}

func TestAnalyzeBalancedClusterReturnsNil(t *testing.T) {
	store := state.New()
	populateBalancedCluster(t, store, "balanced")

	s := NewScheduler(defaultDRSConfig(), store)
	recs := s.AnalyzeCluster("balanced")
	if recs != nil {
		t.Fatalf("expected nil recommendations for balanced cluster, got %d", len(recs))
	}
}

func TestAnalyzeImbalancedClusterReturnsRecommendations(t *testing.T) {
	store := state.New()
	populateImbalancedCluster(t, store, "imbalanced")

	s := NewScheduler(defaultDRSConfig(), store)
	recs := s.AnalyzeCluster("imbalanced")
	if len(recs) == 0 {
		t.Fatal("expected recommendations for imbalanced cluster")
	}
}

func TestAnalyzeNonexistentClusterReturnsNil(t *testing.T) {
	store := state.New()
	s := NewScheduler(defaultDRSConfig(), store)
	recs := s.AnalyzeCluster("no-such-cluster")
	if recs != nil {
		t.Fatal("expected nil for nonexistent cluster")
	}
}

func TestRecommendationsMoveFromOverloadedToUnderloaded(t *testing.T) {
	store := state.New()
	populateImbalancedCluster(t, store, "move-test")

	s := NewScheduler(defaultDRSConfig(), store)
	recs := s.AnalyzeCluster("move-test")
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}

	for _, rec := range recs {
		if rec.FromNode != "hot-node" {
			t.Fatalf("expected migration from hot-node, got %s", rec.FromNode)
		}
		if rec.ToNode != "cold-node-1" && rec.ToNode != "cold-node-2" {
			t.Fatalf("expected migration to a cold node, got %s", rec.ToNode)
		}
		if rec.Cluster != "move-test" {
			t.Fatalf("expected cluster move-test, got %s", rec.Cluster)
		}
		if rec.GuestType != "vm" {
			t.Fatalf("expected guest type vm, got %s", rec.GuestType)
		}
		if rec.Priority < 1 || rec.Priority > 5 {
			t.Fatalf("expected priority 1-5, got %d", rec.Priority)
		}
	}
}

func TestStdDevDetectsImbalance(t *testing.T) {
	store := state.New()
	s := NewScheduler(defaultDRSConfig(), store)

	// Balanced loads: stddev should be near 0
	balanced := []NodeLoad{
		{Node: "a", CPUUsage: 0.30},
		{Node: "b", CPUUsage: 0.30},
		{Node: "c", CPUUsage: 0.30},
	}
	sd := s.calculateStdDev(balanced, func(l NodeLoad) float64 { return l.CPUUsage })
	if sd > 0.01 {
		t.Fatalf("expected near-zero stddev for balanced loads, got %f", sd)
	}

	// Imbalanced loads: stddev should be significant
	imbalanced := []NodeLoad{
		{Node: "a", CPUUsage: 0.90},
		{Node: "b", CPUUsage: 0.10},
		{Node: "c", CPUUsage: 0.10},
	}
	sd = s.calculateStdDev(imbalanced, func(l NodeLoad) float64 { return l.CPUUsage })
	if sd < 0.15 {
		t.Fatalf("expected significant stddev for imbalanced loads, got %f", sd)
	}
}

func TestStdDevEmptyLoads(t *testing.T) {
	store := state.New()
	s := NewScheduler(defaultDRSConfig(), store)

	sd := s.calculateStdDev(nil, func(l NodeLoad) float64 { return l.CPUUsage })
	if sd != 0 {
		t.Fatalf("expected 0 stddev for empty loads, got %f", sd)
	}
}

func TestMigrationRateLimit(t *testing.T) {
	store := state.New()
	cs := store.GetOrCreateCluster("rate-limit")

	// Overloaded node with many VMs
	hotNode := pve.Node{
		Node: "hot", Status: "online",
		CPU: 0.95, MaxCPU: 16,
		Mem: 9 * 1024 * 1024 * 1024, MaxMem: 10 * 1024 * 1024 * 1024,
	}
	vms := make([]pve.VM, 10)
	for i := range vms {
		vms[i] = pve.VM{
			VMID: 300 + i, Name: "vm", Node: "hot",
			Status: "running", CPU: 0.05, Mem: 256 * 1024 * 1024,
		}
	}
	cs.UpdateNode("hot", hotNode, vms, nil, nil, nil)

	coldNode := pve.Node{
		Node: "cold", Status: "online",
		CPU: 0.05, MaxCPU: 16,
		Mem: 1 * 1024 * 1024 * 1024, MaxMem: 10 * 1024 * 1024 * 1024,
	}
	cs.UpdateNode("cold", coldNode, nil, nil, nil, nil)

	cfg := defaultDRSConfig()
	cfg.MigrationRate = 2
	s := NewScheduler(cfg, store)

	recs := s.AnalyzeCluster("rate-limit")
	if len(recs) > 2 {
		t.Fatalf("expected at most 2 recommendations (migration rate limit), got %d", len(recs))
	}
}

func TestSingleNodeClusterReturnsNil(t *testing.T) {
	store := state.New()
	cs := store.GetOrCreateCluster("single")
	cs.UpdateNode("only-node", pve.Node{
		Node: "only-node", Status: "online",
		CPU: 0.90, MaxCPU: 8,
		Mem: 7 * 1024 * 1024 * 1024, MaxMem: 8 * 1024 * 1024 * 1024,
	}, nil, nil, nil, nil)

	s := NewScheduler(defaultDRSConfig(), store)
	recs := s.AnalyzeCluster("single")
	if recs != nil {
		t.Fatal("expected nil for single-node cluster")
	}
}
