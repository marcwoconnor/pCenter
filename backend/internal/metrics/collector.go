package metrics

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/moconnor/pcenter/internal/state"
)

// Collector extracts metrics from state and writes them to the database
type Collector struct {
	store    *state.Store
	db       *DB
	interval time.Duration

	// For rate calculations (network, disk I/O)
	mu          sync.Mutex
	lastSamples map[string]lastSample
}

type lastSample struct {
	timestamp int64
	netin     int64
	netout    int64
	diskread  int64
	diskwrite int64
	pgpgin    int64
	pgpgout   int64
}

// NewCollector creates a new metrics collector
func NewCollector(store *state.Store, db *DB, intervalSecs int) *Collector {
	return &Collector{
		store:       store,
		db:          db,
		interval:    time.Duration(intervalSecs) * time.Second,
		lastSamples: make(map[string]lastSample),
	}
}

// Start begins the collection loop
func (c *Collector) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	slog.Info("metrics collector started", "interval", c.interval)

	// Initial collection
	c.collect()

	for {
		select {
		case <-ctx.Done():
			slog.Info("metrics collector stopped")
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// collect gathers all current metrics and writes them to the database
func (c *Collector) collect() {
	now := time.Now().Unix()
	var metrics []RawMetric

	// Collect from all clusters
	for _, clusterName := range c.store.GetClusterNames() {
		cluster, ok := c.store.GetCluster(clusterName)
		if !ok {
			continue
		}

		// Collect node metrics
		metrics = append(metrics, c.collectNodes(clusterName, cluster, now)...)

		// Collect VM metrics
		metrics = append(metrics, c.collectVMs(clusterName, cluster, now)...)

		// Collect container metrics
		metrics = append(metrics, c.collectContainers(clusterName, cluster, now)...)

		// Collect storage metrics
		metrics = append(metrics, c.collectStorage(clusterName, cluster, now)...)

		// Collect Ceph metrics
		metrics = append(metrics, c.collectCeph(clusterName, cluster, now)...)
	}

	// Write batch to database
	if len(metrics) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := c.db.InsertRawMetricsBatch(ctx, metrics); err != nil {
			slog.Error("failed to write metrics", "error", err, "count", len(metrics))
		} else {
			slog.Debug("collected metrics", "count", len(metrics))
		}
	}
}

// collectNodes extracts metrics from nodes
func (c *Collector) collectNodes(cluster string, cs *state.ClusterStore, ts int64) []RawMetric {
	var metrics []RawMetric

	nodes := cs.GetNodes()
	nodeDetails := cs.GetNodeDetails()

	for _, node := range nodes {
		resID := node.Node

		// CPU usage (convert 0-1 to 0-100)
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "node",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("cpu"),
			Value:        node.CPU * 100,
		})

		// Memory bytes
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "node",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("mem"),
			Value:        float64(node.Mem),
		})

		// Memory percent
		if node.MaxMem > 0 {
			metrics = append(metrics, RawMetric{
				Timestamp:    ts,
				Cluster:      cluster,
				ResourceType: "node",
				ResourceID:   resID,
				MetricTypeID: MetricTypeByName("mem_percent"),
				Value:        float64(node.Mem) / float64(node.MaxMem) * 100,
			})
		}

		// Disk usage
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "node",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("disk"),
			Value:        float64(node.Disk),
		})

		// Disk percent
		if node.MaxDisk > 0 {
			metrics = append(metrics, RawMetric{
				Timestamp:    ts,
				Cluster:      cluster,
				ResourceType: "node",
				ResourceID:   resID,
				MetricTypeID: MetricTypeByName("disk_percent"),
				Value:        float64(node.Disk) / float64(node.MaxDisk) * 100,
			})
		}

		// Uptime
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "node",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("uptime"),
			Value:        float64(node.Uptime),
		})

		// Load averages from node details
		if details, ok := nodeDetails[node.Node]; ok && details != nil && len(details.LoadAvg) >= 3 {
			if la1, err := strconv.ParseFloat(details.LoadAvg[0], 64); err == nil {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "node",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("loadavg_1m"),
					Value:        la1,
				})
			}
			if la5, err := strconv.ParseFloat(details.LoadAvg[1], 64); err == nil {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "node",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("loadavg_5m"),
					Value:        la5,
				})
			}
			if la15, err := strconv.ParseFloat(details.LoadAvg[2], 64); err == nil {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "node",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("loadavg_15m"),
					Value:        la15,
				})
			}
		}
	}

	// Memory paging rates from vmstats
	vmStats := cs.GetVmStats()
	for nodeName, stats := range vmStats {
		if stats == nil {
			continue
		}

		key := cluster + ":vmstat:" + nodeName
		c.mu.Lock()
		last, hasLast := c.lastSamples[key]
		c.lastSamples[key] = lastSample{
			timestamp: ts,
			pgpgin:    stats.PgpgIn,
			pgpgout:   stats.PgpgOut,
		}
		c.mu.Unlock()

		if hasLast && ts > last.timestamp {
			elapsed := float64(ts - last.timestamp)

			// pgpgin rate (pages/sec) - convert to KB/sec (*4 since page size is 4KB)
			if stats.PgpgIn >= last.pgpgin {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "node",
					ResourceID:   nodeName,
					MetricTypeID: MetricTypeByName("pgpgin"),
					Value:        float64(stats.PgpgIn-last.pgpgin) / elapsed,
				})
			}

			// pgpgout rate (pages/sec) - convert to KB/sec (*4 since page size is 4KB)
			if stats.PgpgOut >= last.pgpgout {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "node",
					ResourceID:   nodeName,
					MetricTypeID: MetricTypeByName("pgpgout"),
					Value:        float64(stats.PgpgOut-last.pgpgout) / elapsed,
				})
			}
		}
	}

	return metrics
}

// collectVMs extracts metrics from VMs
func (c *Collector) collectVMs(cluster string, cs *state.ClusterStore, ts int64) []RawMetric {
	var metrics []RawMetric

	for _, vm := range cs.GetVMs() {
		if vm.Status != "running" {
			continue // Only collect from running VMs
		}

		resID := strconv.Itoa(vm.VMID)
		key := cluster + ":vm:" + resID

		// CPU usage
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "vm",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("cpu"),
			Value:        vm.CPU * 100,
		})

		// Memory
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "vm",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("mem"),
			Value:        float64(vm.Mem),
		})

		if vm.MaxMem > 0 {
			metrics = append(metrics, RawMetric{
				Timestamp:    ts,
				Cluster:      cluster,
				ResourceType: "vm",
				ResourceID:   resID,
				MetricTypeID: MetricTypeByName("mem_percent"),
				Value:        float64(vm.Mem) / float64(vm.MaxMem) * 100,
			})
		}

		// Disk
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "vm",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("disk"),
			Value:        float64(vm.Disk),
		})

		if vm.MaxDisk > 0 {
			metrics = append(metrics, RawMetric{
				Timestamp:    ts,
				Cluster:      cluster,
				ResourceType: "vm",
				ResourceID:   resID,
				MetricTypeID: MetricTypeByName("disk_percent"),
				Value:        float64(vm.Disk) / float64(vm.MaxDisk) * 100,
			})
		}

		// Network and disk I/O rates
		c.mu.Lock()
		last, hasLast := c.lastSamples[key]
		c.lastSamples[key] = lastSample{
			timestamp: ts,
			netin:     vm.NetIn,
			netout:    vm.NetOut,
			diskread:  vm.DiskRead,
			diskwrite: vm.DiskWrite,
		}
		c.mu.Unlock()

		if hasLast && ts > last.timestamp {
			elapsed := float64(ts - last.timestamp)

			// Network in bytes/sec
			if vm.NetIn >= last.netin {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "vm",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("netin"),
					Value:        float64(vm.NetIn-last.netin) / elapsed,
				})
			}

			// Network out bytes/sec
			if vm.NetOut >= last.netout {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "vm",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("netout"),
					Value:        float64(vm.NetOut-last.netout) / elapsed,
				})
			}

			// Disk read bytes/sec
			if vm.DiskRead >= last.diskread {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "vm",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("diskread"),
					Value:        float64(vm.DiskRead-last.diskread) / elapsed,
				})
			}

			// Disk write bytes/sec
			if vm.DiskWrite >= last.diskwrite {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "vm",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("diskwrite"),
					Value:        float64(vm.DiskWrite-last.diskwrite) / elapsed,
				})
			}
		}

		// Uptime
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "vm",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("uptime"),
			Value:        float64(vm.Uptime),
		})
	}

	return metrics
}

// collectContainers extracts metrics from containers
func (c *Collector) collectContainers(cluster string, cs *state.ClusterStore, ts int64) []RawMetric {
	var metrics []RawMetric

	for _, ct := range cs.GetContainers() {
		if ct.Status != "running" {
			continue
		}

		resID := strconv.Itoa(ct.VMID)
		key := cluster + ":ct:" + resID

		// CPU usage
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "ct",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("cpu"),
			Value:        ct.CPU * 100,
		})

		// Memory
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "ct",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("mem"),
			Value:        float64(ct.Mem),
		})

		if ct.MaxMem > 0 {
			metrics = append(metrics, RawMetric{
				Timestamp:    ts,
				Cluster:      cluster,
				ResourceType: "ct",
				ResourceID:   resID,
				MetricTypeID: MetricTypeByName("mem_percent"),
				Value:        float64(ct.Mem) / float64(ct.MaxMem) * 100,
			})
		}

		// Disk
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "ct",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("disk"),
			Value:        float64(ct.Disk),
		})

		if ct.MaxDisk > 0 {
			metrics = append(metrics, RawMetric{
				Timestamp:    ts,
				Cluster:      cluster,
				ResourceType: "ct",
				ResourceID:   resID,
				MetricTypeID: MetricTypeByName("disk_percent"),
				Value:        float64(ct.Disk) / float64(ct.MaxDisk) * 100,
			})
		}

		// Swap
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "ct",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("swap"),
			Value:        float64(ct.Swap),
		})

		if ct.MaxSwap > 0 {
			metrics = append(metrics, RawMetric{
				Timestamp:    ts,
				Cluster:      cluster,
				ResourceType: "ct",
				ResourceID:   resID,
				MetricTypeID: MetricTypeByName("swap_percent"),
				Value:        float64(ct.Swap) / float64(ct.MaxSwap) * 100,
			})
		}

		// Network and Disk I/O rates
		c.mu.Lock()
		last, hasLast := c.lastSamples[key]
		c.lastSamples[key] = lastSample{
			timestamp: ts,
			netin:     ct.NetIn,
			netout:    ct.NetOut,
			diskread:  ct.DiskRead,
			diskwrite: ct.DiskWrite,
		}
		c.mu.Unlock()

		if hasLast && ts > last.timestamp {
			elapsed := float64(ts - last.timestamp)

			if ct.NetIn >= last.netin {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "ct",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("netin"),
					Value:        float64(ct.NetIn-last.netin) / elapsed,
				})
			}

			if ct.NetOut >= last.netout {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "ct",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("netout"),
					Value:        float64(ct.NetOut-last.netout) / elapsed,
				})
			}

			if ct.DiskRead >= last.diskread {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "ct",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("diskread"),
					Value:        float64(ct.DiskRead-last.diskread) / elapsed,
				})
			}

			if ct.DiskWrite >= last.diskwrite {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "ct",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("diskwrite"),
					Value:        float64(ct.DiskWrite-last.diskwrite) / elapsed,
				})
			}
		}

		// Uptime
		metrics = append(metrics, RawMetric{
			Timestamp:    ts,
			Cluster:      cluster,
			ResourceType: "ct",
			ResourceID:   resID,
			MetricTypeID: MetricTypeByName("uptime"),
			Value:        float64(ct.Uptime),
		})
	}

	return metrics
}

// collectStorage extracts metrics from storage
func (c *Collector) collectStorage(cluster string, cs *state.ClusterStore, ts int64) []RawMetric {
	var metrics []RawMetric

	// Get storage from all nodes
	for _, node := range cs.GetNodes() {
		for _, st := range cs.GetStorage(node.Node) {
			if st.Status != "available" {
				continue
			}

			resID := st.Storage

			// Used bytes
			metrics = append(metrics, RawMetric{
				Timestamp:    ts,
				Cluster:      cluster,
				ResourceType: "storage",
				ResourceID:   resID,
				MetricTypeID: MetricTypeByName("disk"),
				Value:        float64(st.Used),
			})

			// Used percent
			if st.Total > 0 {
				metrics = append(metrics, RawMetric{
					Timestamp:    ts,
					Cluster:      cluster,
					ResourceType: "storage",
					ResourceID:   resID,
					MetricTypeID: MetricTypeByName("disk_percent"),
					Value:        float64(st.Used) / float64(st.Total) * 100,
				})
			}
		}
	}

	return metrics
}

// collectCeph extracts metrics from Ceph
func (c *Collector) collectCeph(cluster string, cs *state.ClusterStore, ts int64) []RawMetric {
	var metrics []RawMetric

	ceph := cs.GetCeph()
	if ceph == nil {
		return metrics
	}

	resID := "cluster"

	// Ceph used bytes
	metrics = append(metrics, RawMetric{
		Timestamp:    ts,
		Cluster:      cluster,
		ResourceType: "ceph",
		ResourceID:   resID,
		MetricTypeID: MetricTypeByName("ceph_used"),
		Value:        float64(ceph.PGMap.BytesUsed),
	})

	// Ceph available bytes
	metrics = append(metrics, RawMetric{
		Timestamp:    ts,
		Cluster:      cluster,
		ResourceType: "ceph",
		ResourceID:   resID,
		MetricTypeID: MetricTypeByName("ceph_avail"),
		Value:        float64(ceph.PGMap.BytesAvail),
	})

	// Ceph health (0=OK, 1=WARN, 2=ERR)
	var healthValue float64
	switch ceph.Health.Status {
	case "HEALTH_OK":
		healthValue = 0
	case "HEALTH_WARN":
		healthValue = 1
	default:
		healthValue = 2
	}
	metrics = append(metrics, RawMetric{
		Timestamp:    ts,
		Cluster:      cluster,
		ResourceType: "ceph",
		ResourceID:   resID,
		MetricTypeID: MetricTypeByName("ceph_health"),
		Value:        healthValue,
	})

	return metrics
}
