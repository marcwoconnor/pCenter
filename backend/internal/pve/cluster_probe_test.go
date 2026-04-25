package pve

import "testing"

func TestClassifyClusterStatus_StandaloneNoClusterEntry(t *testing.T) {
	// A fresh standalone PVE node with no cluster formed: /cluster/status
	// returns only a single node entry, no type=cluster entry.
	items := []ClusterStatusNode{
		{Type: "node", Name: "pve01", ID: "node/pve01", Online: 1, Local: 1},
	}
	m := ClassifyClusterStatus(items)
	if m.IsCluster {
		t.Fatalf("expected standalone, got IsCluster=true")
	}
	if m.ClusterName != "" {
		t.Fatalf("expected empty cluster name, got %q", m.ClusterName)
	}
	if m.LocalNode != "pve01" {
		t.Fatalf("expected LocalNode=pve01, got %q", m.LocalNode)
	}
	if len(m.Nodes) != 1 {
		t.Fatalf("expected 1 node entry, got %d", len(m.Nodes))
	}
}

func TestClassifyClusterStatus_StandaloneWithSingleNodeClusterEntry(t *testing.T) {
	// Some single-node installs still return a type=cluster entry reporting nodes=1.
	// That's still a standalone from a cluster-semantics perspective.
	items := []ClusterStatusNode{
		{Type: "cluster", Name: "solo", Nodes: 1, Quorate: 1},
		{Type: "node", Name: "pve01", Online: 1, Local: 1},
	}
	m := ClassifyClusterStatus(items)
	if m.IsCluster {
		t.Fatalf("expected standalone (single-node cluster entry), got IsCluster=true")
	}
	if m.LocalNode != "pve01" {
		t.Fatalf("expected LocalNode=pve01, got %q", m.LocalNode)
	}
}

func TestClassifyClusterStatus_RealMultiNodeCluster(t *testing.T) {
	items := []ClusterStatusNode{
		{Type: "cluster", Name: "prod-a", Nodes: 3, Quorate: 1},
		{Type: "node", Name: "pve01", Online: 1, Local: 1},
		{Type: "node", Name: "pve02", Online: 1},
		{Type: "node", Name: "pve03", Online: 0},
	}
	m := ClassifyClusterStatus(items)
	if !m.IsCluster {
		t.Fatalf("expected IsCluster=true")
	}
	if m.ClusterName != "prod-a" {
		t.Fatalf("expected ClusterName=prod-a, got %q", m.ClusterName)
	}
	if !m.Quorate {
		t.Fatalf("expected Quorate=true")
	}
	if len(m.Nodes) != 3 {
		t.Fatalf("expected 3 node entries, got %d", len(m.Nodes))
	}
	if m.LocalNode != "pve01" {
		t.Fatalf("expected LocalNode=pve01, got %q", m.LocalNode)
	}
}

func TestClassifyClusterStatus_TwoNodeClusterNotQuorate(t *testing.T) {
	// Two-node cluster where one is offline — still a real cluster, but not quorate.
	items := []ClusterStatusNode{
		{Type: "cluster", Name: "edge", Nodes: 2, Quorate: 0},
		{Type: "node", Name: "pve01", Online: 1, Local: 1},
		{Type: "node", Name: "pve02", Online: 0},
	}
	m := ClassifyClusterStatus(items)
	if !m.IsCluster {
		t.Fatalf("expected IsCluster=true")
	}
	if m.Quorate {
		t.Fatalf("expected Quorate=false")
	}
}
