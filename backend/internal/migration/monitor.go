package migration

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

// Monitor watches active migrations and updates their status
type Monitor struct {
	store    *state.Store
	clusters []config.ClusterConfig
	clients  map[string]*pve.Client // keyed by "cluster/node"
	clientMu sync.RWMutex
	interval time.Duration
	onChange func()
	activity *activity.Service
}

// NewMonitor creates a new migration monitor
func NewMonitor(store *state.Store, clusters []config.ClusterConfig, interval time.Duration) *Monitor {
	if interval == 0 {
		interval = 3 * time.Second
	}
	return &Monitor{
		store:    store,
		clusters: clusters,
		clients:  make(map[string]*pve.Client),
		interval: interval,
	}
}

// OnChange sets a callback for when migration status changes
func (m *Monitor) OnChange(fn func()) {
	m.onChange = fn
}

// SetActivity sets the activity logging service
func (m *Monitor) SetActivity(a *activity.Service) {
	m.activity = a
}

// Start begins monitoring migrations
func (m *Monitor) Start(ctx context.Context) {
	slog.Info("migration monitor started", "interval", m.interval)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("migration monitor stopped")
			return
		case <-ticker.C:
			m.checkMigrations(ctx)
			m.checkDiskMoves(ctx)
		}
	}
}

// checkMigrations checks status of all active migrations
func (m *Monitor) checkMigrations(ctx context.Context) {
	migrations := m.store.GetMigrations()
	if len(migrations) == 0 {
		return
	}

	changed := false
	for _, mig := range migrations {
		if mig.Status != "running" {
			continue
		}

		// Parse node from UPID (format: UPID:node:pid:pstart:starttime:type:id:user:)
		node := parseNodeFromUPID(mig.UPID)
		if node == "" {
			slog.Warn("could not parse node from UPID", "upid", mig.UPID)
			continue
		}

		// Get or create client for this node
		client := m.getClient(mig.Cluster, node)
		if client == nil {
			slog.Debug("no client available for migration check", "cluster", mig.Cluster, "node", node)
			continue
		}

		// Check task status
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		task, err := client.GetTaskStatus(checkCtx, mig.UPID)
		cancel()

		if err != nil {
			// Task might have been cleaned up - check if it's been running too long
			elapsed := time.Since(mig.StartedAt)
			if elapsed > 10*time.Minute {
				slog.Warn("migration task not found after timeout, marking failed",
					"vmid", mig.VMID, "elapsed", elapsed)
				m.store.UpdateMigration(mig.UPID, 0, "failed", "task not found")
				changed = true
			}
			continue
		}

		// Update based on task status
		switch task.Status {
		case "running":
			// Parse task log to determine progress
			progress := m.parseProgress(ctx, client, &mig)
			if progress > mig.Progress {
				m.store.UpdateMigration(mig.UPID, progress, "running", "")
				changed = true
			}
			continue

		case "stopped":
			// Task completed - check exit status
			if task.ExitCode == "OK" {
				slog.Info("migration completed successfully",
					"vmid", mig.VMID, "from", mig.FromNode, "to", mig.ToNode)
				m.store.UpdateMigration(mig.UPID, 100, "completed", "")

				// Log activity - completed
				if m.activity != nil {
					details, _ := json.Marshal(map[string]interface{}{
						"from_node": mig.FromNode,
						"to_node":   mig.ToNode,
						"duration":  time.Since(mig.StartedAt).String(),
					})
					m.activity.Log(activity.Entry{
						Action:       activity.ActionMigrate,
						ResourceType: mig.GuestType,
						ResourceID:   strconv.Itoa(mig.VMID),
						ResourceName: mig.GuestName,
						Cluster:      mig.Cluster,
						Details:      string(details),
						Status:       activity.StatusSuccess,
					})
				}

				// Remove after a short delay so UI can show completion
				go func(upid string) {
					time.Sleep(5 * time.Second)
					m.store.RemoveMigration(upid)
					if m.onChange != nil {
						m.onChange()
					}
				}(mig.UPID)
			} else {
				// Get detailed error from task log
				detailedError := client.GetTaskError(ctx, mig.UPID)
				if detailedError == "" {
					detailedError = task.ExitCode // fallback to exit code
				}

				slog.Warn("migration failed",
					"vmid", mig.VMID, "error", detailedError)
				m.store.UpdateMigration(mig.UPID, 0, "failed", detailedError)

				// Log activity - failed
				if m.activity != nil {
					details, _ := json.Marshal(map[string]interface{}{
						"from_node": mig.FromNode,
						"to_node":   mig.ToNode,
						"error":     detailedError,
					})
					m.activity.Log(activity.Entry{
						Action:       activity.ActionMigrate,
						ResourceType: mig.GuestType,
						ResourceID:   strconv.Itoa(mig.VMID),
						ResourceName: mig.GuestName,
						Cluster:      mig.Cluster,
						Details:      string(details),
						Status:       activity.StatusError,
					})
				}

				// Remove failed migrations after showing error
				go func(upid string) {
					time.Sleep(30 * time.Second)
					m.store.RemoveMigration(upid)
					if m.onChange != nil {
						m.onChange()
					}
				}(mig.UPID)
			}
			changed = true

		default:
			// Unknown status
			slog.Debug("unknown task status", "status", task.Status, "upid", mig.UPID)
		}
	}

	if changed && m.onChange != nil {
		m.onChange()
	}
}

// parseProgress parses the task log to determine migration progress
func (m *Monitor) parseProgress(ctx context.Context, client *pve.Client, mig *pve.MigrationProgress) int {
	logCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	entries, err := client.GetTaskLog(logCtx, mig.UPID, 50)
	if err != nil {
		return mig.Progress
	}

	progress := 0
	for _, entry := range entries {
		text := strings.ToLower(entry.T)

		// Container migration phases (restart mode)
		if strings.Contains(text, "shutdown ct") || strings.Contains(text, "stop ct") {
			if progress < 15 {
				progress = 15
			}
		}
		if strings.Contains(text, "starting migration") {
			if progress < 33 {
				progress = 33
			}
		}
		if strings.Contains(text, "start container on target") || strings.Contains(text, "pct start") {
			if progress < 66 {
				progress = 66
			}
		}
		if strings.Contains(text, "migration finished") {
			if progress < 90 {
				progress = 90
			}
		}

		// VM migration phases (live migration)
		if strings.Contains(text, "starting migration of vm") {
			if progress < 20 {
				progress = 20
			}
		}
		if strings.Contains(text, "start migrate") || strings.Contains(text, "migration active") {
			if progress < 50 {
				progress = 50
			}
		}
		if strings.Contains(text, "migration finished") || strings.Contains(text, "task ok") {
			if progress < 90 {
				progress = 90
			}
		}
	}

	return progress
}

// checkDiskMoves checks status of all active storage vMotion tasks. Mirrors
// checkMigrations; simpler because there's no from/to-node concept and
// move_disk/move_volume task logs don't expose fine-grained percentage in a
// form worth parsing — we flip 0 → 100 on success.
func (m *Monitor) checkDiskMoves(ctx context.Context) {
	moves := m.store.GetDiskMoves()
	if len(moves) == 0 {
		return
	}

	changed := false
	for _, mv := range moves {
		if mv.Status != "running" {
			continue
		}

		node := parseNodeFromUPID(mv.UPID)
		if node == "" {
			slog.Warn("could not parse node from disk-move UPID", "upid", mv.UPID)
			continue
		}

		client := m.getClient(mv.Cluster, node)
		if client == nil {
			continue
		}

		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		task, err := client.GetTaskStatus(checkCtx, mv.UPID)
		cancel()

		if err != nil {
			if time.Since(mv.StartedAt) > 30*time.Minute {
				slog.Warn("disk-move task not found after timeout, marking failed",
					"vmid", mv.VMID, "disk", mv.Disk)
				m.store.UpdateDiskMove(mv.UPID, 0, "failed", "task not found")
				changed = true
			}
			continue
		}

		switch task.Status {
		case "running":
			// PVE task logs for move_disk emit "transferred X MiB of Y GiB"
			// lines. Rather than parse them, we just leave progress at whatever
			// the caller set (typically 0) until completion.
			continue
		case "stopped":
			if task.ExitCode == "OK" {
				slog.Info("disk move completed",
					"vmid", mv.VMID, "disk", mv.Disk, "to", mv.ToStorage)
				m.store.UpdateDiskMove(mv.UPID, 100, "completed", "")
				if m.activity != nil {
					details, _ := json.Marshal(map[string]interface{}{
						"disk":          mv.Disk,
						"from_storage":  mv.FromStorage,
						"to_storage":    mv.ToStorage,
						"delete_source": mv.DeleteSource,
						"duration":      time.Since(mv.StartedAt).String(),
					})
					m.activity.Log(activity.Entry{
						Action:       activity.ActionMoveDisk,
						ResourceType: mv.GuestType,
						ResourceID:   strconv.Itoa(mv.VMID),
						ResourceName: mv.GuestName,
						Cluster:      mv.Cluster,
						Details:      string(details),
						Status:       activity.StatusSuccess,
					})
				}
				go func(upid string) {
					time.Sleep(5 * time.Second)
					m.store.RemoveDiskMove(upid)
					if m.onChange != nil {
						m.onChange()
					}
				}(mv.UPID)
			} else {
				detailedError := client.GetTaskError(ctx, mv.UPID)
				if detailedError == "" {
					detailedError = task.ExitCode
				}
				slog.Warn("disk move failed", "vmid", mv.VMID, "disk", mv.Disk, "error", detailedError)
				m.store.UpdateDiskMove(mv.UPID, 0, "failed", detailedError)
				if m.activity != nil {
					details, _ := json.Marshal(map[string]interface{}{
						"disk":         mv.Disk,
						"from_storage": mv.FromStorage,
						"to_storage":   mv.ToStorage,
						"error":        detailedError,
					})
					m.activity.Log(activity.Entry{
						Action:       activity.ActionMoveDisk,
						ResourceType: mv.GuestType,
						ResourceID:   strconv.Itoa(mv.VMID),
						ResourceName: mv.GuestName,
						Cluster:      mv.Cluster,
						Details:      string(details),
						Status:       activity.StatusError,
					})
				}
				go func(upid string) {
					time.Sleep(30 * time.Second)
					m.store.RemoveDiskMove(upid)
					if m.onChange != nil {
						m.onChange()
					}
				}(mv.UPID)
			}
			changed = true
		}
	}

	if changed && m.onChange != nil {
		m.onChange()
	}
}

// getClient returns a PVE client for the given cluster/node, creating one if needed
func (m *Monitor) getClient(clusterName, node string) *pve.Client {
	key := clusterName + "/" + node

	m.clientMu.RLock()
	client, ok := m.clients[key]
	m.clientMu.RUnlock()
	if ok {
		return client
	}

	// Create new client
	var clusterCfg *config.ClusterConfig
	for _, c := range m.clusters {
		if c.Name == clusterName {
			clusterCfg = &c
			break
		}
	}
	if clusterCfg == nil {
		return nil
	}

	client = pve.NewClientForNode(*clusterCfg, node, "")

	m.clientMu.Lock()
	m.clients[key] = client
	m.clientMu.Unlock()

	return client
}

// parseNodeFromUPID extracts the node name from a UPID
// UPID format: UPID:node:pid:pstart:starttime:type:id:user:
func parseNodeFromUPID(upid string) string {
	// URL decode first in case it was encoded
	decoded, err := url.PathUnescape(upid)
	if err != nil {
		decoded = upid
	}

	parts := strings.Split(decoded, ":")
	if len(parts) < 2 {
		return ""
	}
	// First part is "UPID", second is node
	return parts[1]
}
