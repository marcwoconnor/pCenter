package collector

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/moconnor/pve-agent/internal/types"
)

// scanTimeout caps `smartctl --scan -j`. The scan should take well under a
// second on healthy hardware; a long hang means the controller is stuck.
const scanTimeout = 10 * time.Second

// scrapeTimeout caps a single `smartctl -j -a /dev/X` call. Values can hang
// when a disk is failing — we'd rather report an error than block the
// collector goroutine forever.
const scrapeTimeout = 15 * time.Second

// SmartCollector runs smartctl on a slow loop and caches the latest report.
//
// Cadence: SMART attribute values change on the order of hours/days, so the
// collector ticks at smartInterval (default 300s) independently of the main
// status push interval (default 5s). The most recent report is cached and
// attached to every outbound StatusData.
//
// On startup the cache is empty until the first scan completes, which can
// take 10-30s on a host with many disks. Status pushes during that window
// omit the SmartReport entirely (omitempty) so the backend can render a
// "first scan in progress" hint rather than a misleading empty disk list.
type SmartCollector struct {
	node     string
	cluster  string
	interval time.Duration

	mu      sync.RWMutex
	latest  *types.SmartReport
}

func NewSmartCollector(node, cluster string, interval time.Duration) *SmartCollector {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &SmartCollector{
		node:     node,
		cluster:  cluster,
		interval: interval,
	}
}

// Start runs the collection loop until ctx is cancelled. The first scan kicks
// off immediately so the cache is warm by the second collection tick (5s).
func (s *SmartCollector) Start(ctx context.Context) {
	slog.Info("smart collector started", "interval", s.interval, "node", s.node)
	s.runOnce(ctx)

	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("smart collector stopped")
			return
		case <-t.C:
			s.runOnce(ctx)
		}
	}
}

// Latest returns the most recent successful or partially-successful report,
// or nil if no scan has completed yet.
func (s *SmartCollector) Latest() *types.SmartReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest
}

func (s *SmartCollector) runOnce(ctx context.Context) {
	start := time.Now()
	report := &types.SmartReport{
		Node:        s.node,
		Cluster:     s.cluster,
		CollectedAt: start.Unix(),
	}

	devices, err := scanDevices(ctx)
	if err != nil {
		report.ScanError = err.Error()
		slog.Warn("smartctl scan failed", "error", err)
		s.publish(report, start)
		return
	}

	for _, dev := range devices {
		if shouldSkipDevice(dev.Name) {
			continue
		}
		raw, err := scrapeDevice(ctx, dev.Name)
		scrape := types.SmartScrape{
			Device: dev.Name,
			Type:   dev.Type,
		}
		if err != nil {
			scrape.Error = err.Error()
		} else {
			scrape.RawJSON = raw
		}
		report.Scrapes = append(report.Scrapes, scrape)
	}

	s.publish(report, start)
	slog.Debug("smart collected",
		"devices", len(report.Scrapes),
		"duration_ms", report.DurationMs)
}

func (s *SmartCollector) publish(r *types.SmartReport, start time.Time) {
	r.DurationMs = time.Since(start).Milliseconds()
	s.mu.Lock()
	s.latest = r
	s.mu.Unlock()
}

type scannedDevice struct {
	Name string
	Type string
}

func scanDevices(parent context.Context) ([]scannedDevice, error) {
	ctx, cancel := context.WithTimeout(parent, scanTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "smartctl", "--scan", "-j").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, errors.New(strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}

	var parsed struct {
		Devices []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}

	devs := make([]scannedDevice, len(parsed.Devices))
	for i, d := range parsed.Devices {
		devs[i] = scannedDevice{Name: d.Name, Type: d.Type}
	}
	return devs, nil
}

func scrapeDevice(parent context.Context, device string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, scrapeTimeout)
	defer cancel()

	// smartctl exits with a bitmask; non-zero is informational (e.g. bit 0
	// set = command-line parse error, bits 2-7 = various health flags).
	// We accept any output that decodes as JSON — only treat it as a hard
	// error if there's no JSON at all.
	out, err := exec.CommandContext(ctx, "smartctl", "-j", "-a", device).Output()
	if len(out) > 0 && json.Valid(out) {
		return string(out), nil
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", errors.New(strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return "", errors.New("smartctl returned no JSON output")
}

// shouldSkipDevice filters out devices that won't yield meaningful SMART
// data. RBD volumes are Ceph-backed network block devices; loop and dm
// devices are virtual mappings; nbd is iSCSI/network. None of these have
// underlying SMART attributes worth reporting.
func shouldSkipDevice(name string) bool {
	for _, prefix := range []string{"/dev/rbd", "/dev/loop", "/dev/dm-", "/dev/nbd", "/dev/zd"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
