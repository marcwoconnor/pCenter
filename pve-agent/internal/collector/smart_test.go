package collector

import (
	"context"
	"testing"
	"time"
)

func TestShouldSkipDevice(t *testing.T) {
	cases := map[string]bool{
		"/dev/sda":   false,
		"/dev/nvme0n1": false,
		"/dev/rbd0":  true,
		"/dev/loop0": true,
		"/dev/dm-3":  true,
		"/dev/nbd1":  true,
		"/dev/zd16":  true,
	}
	for dev, want := range cases {
		if got := shouldSkipDevice(dev); got != want {
			t.Errorf("shouldSkipDevice(%q) = %v, want %v", dev, got, want)
		}
	}
}

// TestSmartCollector_NoSmartctl exercises the failure path when smartctl is
// missing entirely (the typical containerized-test environment). The
// collector must still publish a report — with ScanError set — rather than
// hang or panic. This is the observability guarantee: agent-side failures
// surface to the backend instead of becoming silent zero-disk reports.
func TestSmartCollector_NoSmartctl(t *testing.T) {
	c := NewSmartCollector("test-node", "test-cluster", time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.runOnce(ctx)

	rep := c.Latest()
	if rep == nil {
		t.Fatal("Latest() = nil; collector should publish a report even on failure")
	}
	// On systems without smartctl installed (or where exec fails for any
	// reason), ScanError must be non-empty so the backend can surface why.
	// On systems WITH smartctl installed, this still passes — the test just
	// verifies a report was published; non-empty disks are also acceptable.
	if rep.ScanError == "" && len(rep.Scrapes) == 0 {
		t.Error("expected either ScanError or at least one Scrape; got neither")
	}
	if rep.Node != "test-node" || rep.Cluster != "test-cluster" {
		t.Errorf("identity wrong: node=%q cluster=%q", rep.Node, rep.Cluster)
	}
	if rep.CollectedAt == 0 {
		t.Error("CollectedAt not set")
	}
}
