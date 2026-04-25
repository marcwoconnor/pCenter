package poller

import (
	"sort"
	"testing"
	"time"

	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/state"
)

// TestReconcile_DropsOrphans is the core regression test for issue #66:
// pollers whose names aren't in `expected` get removed; pollers that ARE
// in `expected` are left alone, even if they were never running (nil cancel).
func TestReconcile_DropsOrphans(t *testing.T) {
	p := New(state.New(), 1*time.Second, config.DRSConfig{})

	// Three "added" pollers, never Start()ed (cancel == nil).
	p.AddCluster(config.ClusterConfig{Name: "keep-1", DiscoveryNode: "10.0.0.1:8006"})
	p.AddCluster(config.ClusterConfig{Name: "keep-2", DiscoveryNode: "10.0.0.2:8006"})
	p.AddCluster(config.ClusterConfig{Name: "standalone:gone-host", DiscoveryNode: "10.0.0.99:8006"})

	removed := p.Reconcile([]string{"keep-1", "keep-2"})

	if len(removed) != 1 || removed[0] != "standalone:gone-host" {
		t.Errorf("Reconcile returned %v, want [standalone:gone-host]", removed)
	}

	// Verify the orphan is gone but the keepers remain.
	got := pollerNames(p)
	sort.Strings(got)
	want := []string{"keep-1", "keep-2"}
	if !equalSlices(got, want) {
		t.Errorf("after reconcile, names = %v, want %v", got, want)
	}
}

// TestReconcile_NoOpWhenAllExpected verifies that Reconcile is idempotent
// in the steady-state common case (no orphans).
func TestReconcile_NoOpWhenAllExpected(t *testing.T) {
	p := New(state.New(), 1*time.Second, config.DRSConfig{})

	p.AddCluster(config.ClusterConfig{Name: "alpha"})
	p.AddCluster(config.ClusterConfig{Name: "beta"})

	removed := p.Reconcile([]string{"alpha", "beta"})

	if len(removed) != 0 {
		t.Errorf("expected no removals, got %v", removed)
	}
	if got := pollerNames(p); len(got) != 2 {
		t.Errorf("after no-op reconcile, names = %v, want 2 entries", got)
	}
}

// TestReconcile_DoesNotAddNewEntries asserts that Reconcile is one-directional:
// names in `expected` that aren't currently registered MUST NOT cause new
// pollers to spring up. Adding is the responsibility of explicit AddCluster
// calls (the API layer + the inventory reconciler's promote callback).
func TestReconcile_DoesNotAddNewEntries(t *testing.T) {
	p := New(state.New(), 1*time.Second, config.DRSConfig{})

	p.AddCluster(config.ClusterConfig{Name: "registered"})

	p.Reconcile([]string{"registered", "something-else-pcenter-doesnt-know-about"})

	got := pollerNames(p)
	if len(got) != 1 || got[0] != "registered" {
		t.Errorf("Reconcile added entries: got %v, want [registered]", got)
	}
}

// TestReconcile_EmptyExpectedDropsAll covers the edge case of inventory
// reporting zero clusters (e.g. all hosts deleted). Every poller should go.
func TestReconcile_EmptyExpectedDropsAll(t *testing.T) {
	p := New(state.New(), 1*time.Second, config.DRSConfig{})

	p.AddCluster(config.ClusterConfig{Name: "alpha"})
	p.AddCluster(config.ClusterConfig{Name: "standalone:abc"})

	removed := p.Reconcile(nil)

	if len(removed) != 2 {
		t.Errorf("expected 2 removals, got %v", removed)
	}
	if got := pollerNames(p); len(got) != 0 {
		t.Errorf("after empty-expected reconcile, names = %v, want []", got)
	}
}

func pollerNames(p *Poller) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	names := make([]string, 0, len(p.clusters))
	for n := range p.clusters {
		names = append(names, n)
	}
	return names
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
