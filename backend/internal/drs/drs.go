package drs

import (
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

// Scheduler analyzes cluster load and generates migration recommendations
type Scheduler struct {
	cfg   config.DRSConfig
	store *state.Store
}

// NewScheduler creates a new DRS scheduler
func NewScheduler(cfg config.DRSConfig, store *state.Store) *Scheduler {
	return &Scheduler{
		cfg:   cfg,
		store: store,
	}
}

// NodeLoad holds calculated load metrics for a node
type NodeLoad struct {
	Node     string
	CPUUsage float64 // 0.0-1.0
	MemUsage float64 // 0.0-1.0
	VMs      []pve.VM
	CTs      []pve.Container
}

// AnalyzeCluster analyzes a cluster and generates recommendations
func (s *Scheduler) AnalyzeCluster(clusterName string) []pve.DRSRecommendation {
	cs, ok := s.store.GetCluster(clusterName)
	if !ok {
		return nil
	}

	nodes := cs.GetNodes()
	if len(nodes) < 2 {
		// Need at least 2 nodes for load balancing
		return nil
	}

	// Calculate per-node load
	loads := s.calculateNodeLoads(cs)
	if len(loads) < 2 {
		return nil
	}

	// Check for imbalance
	cpuStdDev := s.calculateStdDev(loads, func(l NodeLoad) float64 { return l.CPUUsage })
	memStdDev := s.calculateStdDev(loads, func(l NodeLoad) float64 { return l.MemUsage })

	slog.Debug("DRS analysis",
		"cluster", clusterName,
		"cpu_stddev", cpuStdDev,
		"mem_stddev", memStdDev,
	)

	// If cluster is well-balanced, no recommendations needed
	const imbalanceThreshold = 0.15 // 15% standard deviation
	if cpuStdDev < imbalanceThreshold && memStdDev < imbalanceThreshold {
		return nil
	}

	// Generate recommendations
	return s.generateRecommendations(clusterName, loads)
}

// calculateNodeLoads computes load metrics for each node
func (s *Scheduler) calculateNodeLoads(cs *state.ClusterStore) []NodeLoad {
	nodes := cs.GetNodes()
	vms := cs.GetVMs()
	cts := cs.GetContainers()

	// Build node -> guests maps
	vmsByNode := make(map[string][]pve.VM)
	ctsByNode := make(map[string][]pve.Container)

	for _, vm := range vms {
		if vm.Status == "running" {
			vmsByNode[vm.Node] = append(vmsByNode[vm.Node], vm)
		}
	}
	for _, ct := range cts {
		if ct.Status == "running" {
			ctsByNode[ct.Node] = append(ctsByNode[ct.Node], ct)
		}
	}

	// Calculate load for each online node
	var loads []NodeLoad
	for _, node := range nodes {
		if node.Status != "online" {
			continue
		}

		load := NodeLoad{
			Node: node.Node,
			VMs:  vmsByNode[node.Node],
			CTs:  ctsByNode[node.Node],
		}

		// CPU usage (node.CPU is already 0.0-1.0)
		load.CPUUsage = node.CPU

		// Memory usage
		if node.MaxMem > 0 {
			load.MemUsage = float64(node.Mem) / float64(node.MaxMem)
		}

		loads = append(loads, load)
	}

	return loads
}

// calculateStdDev computes standard deviation of a metric across nodes
func (s *Scheduler) calculateStdDev(loads []NodeLoad, getValue func(NodeLoad) float64) float64 {
	if len(loads) == 0 {
		return 0
	}

	// Calculate mean
	var sum float64
	for _, l := range loads {
		sum += getValue(l)
	}
	mean := sum / float64(len(loads))

	// Calculate variance
	var variance float64
	for _, l := range loads {
		diff := getValue(l) - mean
		variance += diff * diff
	}
	variance /= float64(len(loads))

	return math.Sqrt(variance)
}

// generateRecommendations creates migration suggestions to balance load
func (s *Scheduler) generateRecommendations(clusterName string, loads []NodeLoad) []pve.DRSRecommendation {
	var recommendations []pve.DRSRecommendation

	// Find overloaded and underloaded nodes
	var overloaded, underloaded []NodeLoad

	for _, load := range loads {
		if load.CPUUsage > s.cfg.CPUThreshold || load.MemUsage > s.cfg.MemThreshold {
			overloaded = append(overloaded, load)
		} else if load.CPUUsage < s.cfg.CPUThreshold*0.5 && load.MemUsage < s.cfg.MemThreshold*0.5 {
			underloaded = append(underloaded, load)
		}
	}

	if len(overloaded) == 0 || len(underloaded) == 0 {
		return nil
	}

	// Sort overloaded by load (highest first)
	sort.Slice(overloaded, func(i, j int) bool {
		return overloaded[i].CPUUsage+overloaded[i].MemUsage > overloaded[j].CPUUsage+overloaded[j].MemUsage
	})

	// Sort underloaded by available capacity (most capacity first)
	sort.Slice(underloaded, func(i, j int) bool {
		return underloaded[i].CPUUsage+underloaded[i].MemUsage < underloaded[j].CPUUsage+underloaded[j].MemUsage
	})

	recID := 0
	now := time.Now()

	// Generate recommendations from overloaded nodes
	for _, src := range overloaded {
		// Try to find a suitable guest to migrate
		candidates := s.selectMigrationCandidates(src)

		for _, candidate := range candidates {
			// Find best target node
			target := s.selectTargetNode(candidate, underloaded)
			if target == nil {
				continue
			}

			recID++
			reason := s.formatReason(src, *target)
			priority := s.calculatePriority(src)

			if candidate.isVM {
				recommendations = append(recommendations, pve.DRSRecommendation{
					ID:        generateRecID(clusterName, recID),
					Cluster:   clusterName,
					GuestType: "vm",
					VMID:      candidate.vmid,
					GuestName: candidate.name,
					FromNode:  src.Node,
					ToNode:    target.Node,
					Reason:    reason,
					Priority:  priority,
					CreatedAt: now,
				})
			} else {
				recommendations = append(recommendations, pve.DRSRecommendation{
					ID:        generateRecID(clusterName, recID),
					Cluster:   clusterName,
					GuestType: "ct",
					VMID:      candidate.vmid,
					GuestName: candidate.name,
					FromNode:  src.Node,
					ToNode:    target.Node,
					Reason:    reason,
					Priority:  priority,
					CreatedAt: now,
				})
			}

			// Limit recommendations per run
			if len(recommendations) >= s.cfg.MigrationRate {
				return recommendations
			}
		}
	}

	return recommendations
}

// migrationCandidate represents a guest that could be migrated
type migrationCandidate struct {
	vmid    int
	name    string
	isVM    bool
	cpuLoad float64
	memLoad float64
}

// selectMigrationCandidates picks guests suitable for migration
func (s *Scheduler) selectMigrationCandidates(src NodeLoad) []migrationCandidate {
	var candidates []migrationCandidate

	// Add VMs (prefer smaller VMs for quicker migration)
	for _, vm := range src.VMs {
		if vm.Template {
			continue // Skip templates
		}
		candidates = append(candidates, migrationCandidate{
			vmid:    vm.VMID,
			name:    vm.Name,
			isVM:    true,
			cpuLoad: vm.CPU,
			memLoad: float64(vm.Mem),
		})
	}

	// Add containers
	for _, ct := range src.CTs {
		candidates = append(candidates, migrationCandidate{
			vmid:    ct.VMID,
			name:    ct.Name,
			isVM:    false,
			cpuLoad: ct.CPU,
			memLoad: float64(ct.Mem),
		})
	}

	// Sort by memory (smaller first - easier to migrate)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].memLoad < candidates[j].memLoad
	})

	// Return top candidates
	if len(candidates) > 5 {
		return candidates[:5]
	}
	return candidates
}

// selectTargetNode picks the best destination for a guest
func (s *Scheduler) selectTargetNode(candidate migrationCandidate, targets []NodeLoad) *NodeLoad {
	for i := range targets {
		target := &targets[i]
		// Simple check: ensure target won't become overloaded
		if target.CPUUsage < s.cfg.CPUThreshold*0.8 && target.MemUsage < s.cfg.MemThreshold*0.8 {
			return target
		}
	}
	return nil
}

// formatReason creates a human-readable reason
func (s *Scheduler) formatReason(src, target NodeLoad) string {
	if src.CPUUsage > s.cfg.CPUThreshold {
		return "High CPU: " + src.Node + " at " + formatPercent(src.CPUUsage) + ", " + target.Node + " at " + formatPercent(target.CPUUsage)
	}
	if src.MemUsage > s.cfg.MemThreshold {
		return "High memory: " + src.Node + " at " + formatPercent(src.MemUsage) + ", " + target.Node + " at " + formatPercent(target.MemUsage)
	}
	return "Load balancing"
}

// calculatePriority returns priority (1-5, higher = more urgent)
func (s *Scheduler) calculatePriority(src NodeLoad) int {
	maxUsage := math.Max(src.CPUUsage, src.MemUsage)
	switch {
	case maxUsage > 0.95:
		return 5
	case maxUsage > 0.9:
		return 4
	case maxUsage > 0.85:
		return 3
	case maxUsage > 0.8:
		return 2
	default:
		return 1
	}
}

func generateRecID(cluster string, n int) string {
	return cluster + "-drs-" + time.Now().Format("20060102-150405") + "-" + string(rune('a'+n))
}

func formatPercent(v float64) string {
	return fmt.Sprintf("%.0f%%", v*100)
}
