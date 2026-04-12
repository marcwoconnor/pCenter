package alarms

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/moconnor/pcenter/internal/state"
)

// Evaluator checks alarm conditions against live state data
type Evaluator struct {
	db       *DB
	store    *state.Store
	interval time.Duration
	onChange func() // callback to broadcast state changes
}

// NewEvaluator creates an alarm evaluator
func NewEvaluator(db *DB, store *state.Store, intervalSec int, onChange func()) *Evaluator {
	if intervalSec <= 0 {
		intervalSec = 30
	}
	return &Evaluator{
		db:       db,
		store:    store,
		interval: time.Duration(intervalSec) * time.Second,
		onChange: onChange,
	}
}

// Run starts the evaluation loop (blocking, run in goroutine)
func (e *Evaluator) Run(ctx context.Context) {
	// Wait for initial data load
	time.Sleep(15 * time.Second)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	slog.Info("alarm evaluator started", "interval", e.interval)

	// Run once immediately
	e.evaluate(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("alarm evaluator stopped")
			return
		case <-ticker.C:
			e.evaluate(ctx)
		}
	}
}

func (e *Evaluator) evaluate(ctx context.Context) {
	defs, err := e.db.ListDefinitions(ctx)
	if err != nil {
		slog.Error("alarm eval: list definitions", "error", err)
		return
	}

	changed := false

	for _, def := range defs {
		if !def.Enabled {
			continue
		}

		// Get resources to check based on scope
		resources := e.getResources(def)
		for _, res := range resources {
			value, ok := e.getMetricValue(res, def.MetricType)
			if !ok {
				continue
			}

			transition := e.evaluateAlarm(ctx, def, res, value)
			if transition {
				changed = true
			}
		}
	}

	if changed && e.onChange != nil {
		e.onChange()
	}
}

type resource struct {
	cluster      string
	resourceType string
	resourceID   string
	resourceName string
}

func (e *Evaluator) getResources(def AlarmDefinition) []resource {
	var resources []resource

	switch def.ResourceType {
	case "node":
		for _, n := range e.store.GetNodes() {
			if def.Scope == "cluster" && n.Cluster != def.ScopeTarget {
				continue
			}
			if def.Scope == "resource" && n.Node != def.ScopeTarget {
				continue
			}
			resources = append(resources, resource{
				cluster: n.Cluster, resourceType: "node",
				resourceID: n.Node, resourceName: n.Node,
			})
		}
	case "vm":
		for _, g := range e.store.GetVMs() {
			if def.Scope == "cluster" && g.Cluster != def.ScopeTarget {
				continue
			}
			if g.Status != "running" {
				continue
			}
			resources = append(resources, resource{
				cluster: g.Cluster, resourceType: "vm",
				resourceID: fmt.Sprintf("%d", g.VMID), resourceName: g.Name,
			})
		}
	case "ct":
		for _, g := range e.store.GetContainers() {
			if def.Scope == "cluster" && g.Cluster != def.ScopeTarget {
				continue
			}
			if g.Status != "running" {
				continue
			}
			resources = append(resources, resource{
				cluster: g.Cluster, resourceType: "ct",
				resourceID: fmt.Sprintf("%d", g.VMID), resourceName: g.Name,
			})
		}
	}

	return resources
}

func (e *Evaluator) getMetricValue(res resource, metricType string) (float64, bool) {
	switch res.resourceType {
	case "node":
		nodes := e.store.GetNodes()
		for _, n := range nodes {
			if n.Node == res.resourceID && n.Cluster == res.cluster {
				switch metricType {
				case "cpu":
					return n.CPU * 100, true // 0-1 → 0-100
				case "mem_percent":
					if n.MaxMem > 0 {
						return float64(n.Mem) / float64(n.MaxMem) * 100, true
					}
				}
			}
		}
	case "vm":
		vms := e.store.GetVMs()
		for _, v := range vms {
			if fmt.Sprintf("%d", v.VMID) == res.resourceID && v.Cluster == res.cluster {
				switch metricType {
				case "cpu":
					return v.CPU * 100, true
				case "mem_percent":
					if v.MaxMem > 0 {
						return float64(v.Mem) / float64(v.MaxMem) * 100, true
					}
				}
			}
		}
	case "ct":
		cts := e.store.GetContainers()
		for _, c := range cts {
			if fmt.Sprintf("%d", c.VMID) == res.resourceID && c.Cluster == res.cluster {
				switch metricType {
				case "cpu":
					return c.CPU * 100, true
				case "mem_percent":
					if c.MaxMem > 0 {
						return float64(c.Mem) / float64(c.MaxMem) * 100, true
					}
				}
			}
		}
	}
	return 0, false
}

// evaluateAlarm checks one alarm definition against one resource, returns true if state changed
func (e *Evaluator) evaluateAlarm(ctx context.Context, def AlarmDefinition, res resource, value float64) bool {
	now := time.Now().Unix()

	// Determine violation level
	var violationState AlarmState
	if e.isViolation(value, def.CriticalThreshold, def.Condition) {
		violationState = StateCritical
	} else if e.isViolation(value, def.WarningThreshold, def.Condition) {
		violationState = StateWarning
	} else if e.isClear(value, def.ClearThreshold, def.Condition) {
		violationState = StateNormal
	} else {
		// Between clear and warning thresholds — maintain current state (hysteresis)
		return false
	}

	// Get or create instance
	instID := def.ID + ":" + res.cluster + ":" + res.resourceType + ":" + res.resourceID

	inst := &AlarmInstance{
		ID:              instID,
		DefinitionID:    def.ID,
		DefinitionName:  def.Name,
		Cluster:         res.cluster,
		ResourceType:    res.resourceType,
		ResourceID:      res.resourceID,
		ResourceName:    res.resourceName,
		CurrentValue:    value,
		LastEvaluatedAt: now,
	}

	// Load existing state
	var existing AlarmInstance
	err := e.db.conn.QueryRowContext(ctx,
		"SELECT state, consecutive_count, triggered_at, acknowledged_by, acknowledged_at FROM alarm_instances WHERE id = ?",
		instID).Scan(&existing.State, &existing.ConsecutiveCount, &existing.TriggeredAt, &existing.AcknowledgedBy, &existing.AcknowledgedAt)

	if err != nil {
		// No existing record
		existing.State = StateNormal
		existing.ConsecutiveCount = 0
	}

	oldState := existing.State

	// Update consecutive count
	if violationState == StateNormal {
		inst.ConsecutiveCount = 0
		inst.State = StateNormal
		inst.TriggeredAt = 0
		inst.Threshold = def.ClearThreshold
	} else {
		inst.ConsecutiveCount = existing.ConsecutiveCount + 1

		// Only transition if consecutive count meets duration requirement
		if inst.ConsecutiveCount >= def.DurationSamples {
			inst.State = violationState
			if oldState == StateNormal {
				inst.TriggeredAt = now
			} else {
				inst.TriggeredAt = existing.TriggeredAt
			}
			inst.Threshold = map[AlarmState]float64{
				StateWarning:  def.WarningThreshold,
				StateCritical: def.CriticalThreshold,
			}[violationState]
		} else {
			// Not enough consecutive violations yet — keep current state
			inst.State = oldState
			inst.TriggeredAt = existing.TriggeredAt
			inst.Threshold = def.WarningThreshold
		}
	}

	// Preserve acknowledgment if state hasn't changed
	if inst.State == oldState {
		inst.AcknowledgedBy = existing.AcknowledgedBy
		inst.AcknowledgedAt = existing.AcknowledgedAt
	}
	// Clear acknowledgment on state change
	if inst.State != oldState && oldState != "" {
		inst.AcknowledgedBy = ""
		inst.AcknowledgedAt = 0
	}

	// Save to DB
	if err := e.db.UpsertInstance(ctx, inst); err != nil {
		slog.Error("alarm eval: upsert instance", "error", err, "alarm", def.Name, "resource", res.resourceID)
		return false
	}

	// Record history on state transition
	stateChanged := oldState != "" && inst.State != oldState
	if stateChanged {
		e.db.RecordHistory(ctx, inst, oldState, inst.State)
		slog.Info("alarm state change",
			"alarm", def.Name, "resource", res.resourceName,
			"from", oldState, "to", inst.State,
			"value", fmt.Sprintf("%.1f", value))
	}

	return stateChanged
}

func (e *Evaluator) isViolation(value, threshold float64, cond Condition) bool {
	if cond == CondBelow {
		return value < threshold
	}
	return value > threshold
}

func (e *Evaluator) isClear(value, threshold float64, cond Condition) bool {
	if cond == CondBelow {
		return value >= threshold
	}
	return value <= threshold
}

