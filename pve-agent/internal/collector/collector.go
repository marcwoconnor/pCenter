package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/moconnor/pve-agent/internal/client"
	"github.com/moconnor/pve-agent/internal/config"
	"github.com/moconnor/pve-agent/internal/types"
)

// Collector gathers data from the local Proxmox node
type Collector struct {
	cfg      *config.Config
	client   *client.Client
	api      *PVEClient
	interval time.Duration
}

// NewCollector creates a new collector
func NewCollector(cfg *config.Config, wsClient *client.Client) *Collector {
	return &Collector{
		cfg:      cfg,
		client:   wsClient,
		api:      NewPVEClient(cfg.Node.Name, cfg.PVE.TokenID, cfg.PVE.TokenSecret),
		interval: time.Duration(cfg.Collection.Interval) * time.Second,
	}
}

// Start begins the collection loop
func (c *Collector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	slog.Info("collector started", "interval", c.interval, "node", c.cfg.Node.Name)

	// Initial collection
	c.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("collector stopped")
			return
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

// collect gathers all data and sends to pCenter
func (c *Collector) collect(ctx context.Context) {
	status := &types.StatusData{
		Node:    c.cfg.Node.Name,
		Cluster: c.cfg.Node.Cluster,
	}

	// Collect node status
	nodeStatus, err := c.api.GetNodeStatus(ctx)
	if err != nil {
		slog.Error("failed to get node status", "error", err)
	} else {
		status.NodeStatus = nodeStatus
	}

	// Collect VMs
	vms, err := c.api.GetVMs(ctx)
	if err != nil {
		slog.Error("failed to get VMs", "error", err)
	} else {
		status.VMs = vms
	}

	// Collect containers
	containers, err := c.api.GetContainers(ctx)
	if err != nil {
		slog.Error("failed to get containers", "error", err)
	} else {
		status.Containers = containers
	}

	// Collect storage
	storage, err := c.api.GetStorage(ctx)
	if err != nil {
		slog.Error("failed to get storage", "error", err)
	} else {
		status.Storage = storage
	}

	// Collect network interfaces
	networks, err := c.api.GetNetworkInterfaces(ctx)
	if err != nil {
		slog.Error("failed to get network interfaces", "error", err)
	} else {
		status.Networks = networks
	}

	// Collect Ceph if enabled
	if c.cfg.Collection.IncludeCeph {
		ceph, err := c.api.GetCephStatus(ctx)
		if err != nil {
			// Ceph might not be available, that's OK
			slog.Debug("ceph not available", "error", err)
		} else {
			status.Ceph = ceph
		}
	}

	// Collect system metrics from /proc
	metrics, err := c.api.GetSystemMetrics()
	if err != nil {
		slog.Error("failed to get system metrics", "error", err)
	} else {
		status.Metrics = metrics
	}

	// Send to pCenter
	c.client.SendStatus(status)

	slog.Debug("collected status",
		"vms", len(status.VMs),
		"containers", len(status.Containers),
		"storage", len(status.Storage))
}

// API returns the PVE client for executor use
func (c *Collector) API() *PVEClient {
	return c.api
}
