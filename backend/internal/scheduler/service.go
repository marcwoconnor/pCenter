package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// ActionFunc executes a task action. Returns (upid, error).
// This is injected by the API layer so the scheduler can use
// the same agent-first routing as interactive actions.
type ActionFunc func(ctx context.Context, cluster, node, action string, params map[string]interface{}) (string, error)

// NodeResolver finds which node a VM/CT is on
type NodeResolver func(cluster string, targetType TargetType, targetID int) string

// Service manages scheduled task execution
type Service struct {
	db           *DB
	executeAction ActionFunc
	resolveNode   NodeResolver
}

// NewService creates a scheduler service
func NewService(db *DB, executeAction ActionFunc, resolveNode NodeResolver) *Service {
	return &Service{
		db:            db,
		executeAction: executeAction,
		resolveNode:   resolveNode,
	}
}

// DB returns the underlying database
func (s *Service) DB() *DB {
	return s.db
}

// Start begins the scheduler loop, checking for due tasks every minute
func (s *Service) Start(ctx context.Context) {
	// Compute next run times for all enabled tasks on startup
	s.computeAllNextRuns()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Also cleanup old runs daily
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDueTasks(ctx)
		case <-cleanupTicker.C:
			if cleaned, err := s.db.CleanupOldRuns(30 * 24 * time.Hour); err == nil && cleaned > 0 {
				slog.Info("scheduler: cleaned old task runs", "count", cleaned)
			}
		}
	}
}

// computeAllNextRuns sets next_run for all enabled tasks
func (s *Service) computeAllNextRuns() {
	tasks, err := s.db.ListTasks()
	if err != nil {
		slog.Error("scheduler: failed to list tasks", "error", err)
		return
	}

	now := time.Now()
	for _, t := range tasks {
		if !t.Enabled {
			continue
		}
		next, err := nextCronTime(t.CronExpr, now)
		if err != nil {
			slog.Warn("scheduler: invalid cron expression", "task", t.Name, "cron", t.CronExpr, "error", err)
			continue
		}
		s.db.SetNextRun(t.ID, next)
	}
}

// ComputeNextRun computes and stores the next run time for a single task
func (s *Service) ComputeNextRun(taskID string) error {
	task, err := s.db.GetTask(taskID)
	if err != nil || task == nil {
		return fmt.Errorf("task not found")
	}
	if !task.Enabled {
		return nil
	}
	next, err := nextCronTime(task.CronExpr, time.Now())
	if err != nil {
		return fmt.Errorf("invalid cron: %w", err)
	}
	return s.db.SetNextRun(taskID, next)
}

// runDueTasks finds and executes all tasks whose next_run has passed
func (s *Service) runDueTasks(ctx context.Context) {
	now := time.Now()
	tasks, err := s.db.GetDueTasks(now)
	if err != nil {
		slog.Error("scheduler: failed to get due tasks", "error", err)
		return
	}

	for _, task := range tasks {
		go s.executeTask(ctx, task)
	}
}

// executeTask runs a single scheduled task
func (s *Service) executeTask(ctx context.Context, task ScheduledTask) {
	startedAt := time.Now()
	slog.Info("scheduler: executing task", "id", task.ID, "name", task.Name, "type", task.TaskType,
		"target", fmt.Sprintf("%s/%d", task.TargetType, task.TargetID))

	// Resolve node
	node := s.resolveNode(task.Cluster, task.TargetType, task.TargetID)
	if node == "" {
		errMsg := "could not resolve target node"
		slog.Error("scheduler: "+errMsg, "task", task.Name)
		s.recordRun(task.ID, startedAt, false, "", errMsg)
		s.advanceNextRun(task)
		return
	}

	// Build action name
	action := s.taskTypeToAction(task)
	if action == "" {
		errMsg := "unknown task type: " + string(task.TaskType)
		slog.Error("scheduler: "+errMsg, "task", task.Name)
		s.recordRun(task.ID, startedAt, false, "", errMsg)
		s.advanceNextRun(task)
		return
	}

	// Build params
	params := map[string]interface{}{
		"vmid": task.TargetID,
	}

	// Merge task-specific params from JSON
	if task.Params != "" && task.Params != "{}" {
		var extra map[string]interface{}
		if err := json.Unmarshal([]byte(task.Params), &extra); err == nil {
			for k, v := range extra {
				params[k] = v
			}
		}
	}

	// Execute via the shared action function (agent-first, poller fallback)
	taskCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	upid, err := s.executeAction(taskCtx, task.Cluster, node, action, params)

	duration := int(time.Since(startedAt).Milliseconds())

	if err != nil {
		slog.Error("scheduler: task failed", "task", task.Name, "error", err)
		s.recordRun(task.ID, startedAt, false, "", err.Error())
	} else {
		slog.Info("scheduler: task completed", "task", task.Name, "upid", upid, "duration_ms", duration)
		s.recordRun(task.ID, startedAt, true, upid, "")
	}

	s.advanceNextRun(task)
}

func (s *Service) recordRun(taskID string, startedAt time.Time, success bool, upid, errMsg string) {
	duration := int(time.Since(startedAt).Milliseconds())
	s.db.AddRun(taskID, startedAt, duration, success, upid, errMsg)
	s.db.SetLastRun(taskID, startedAt)
}

func (s *Service) advanceNextRun(task ScheduledTask) {
	next, err := nextCronTime(task.CronExpr, time.Now())
	if err != nil {
		slog.Warn("scheduler: failed to compute next run", "task", task.Name, "error", err)
		return
	}
	s.db.SetNextRun(task.ID, next)
}

// taskTypeToAction maps task type to agent/PVE action name
func (s *Service) taskTypeToAction(task ScheduledTask) string {
	prefix := string(task.TargetType) // "vm" or "ct"
	switch task.TaskType {
	case TaskPowerOn:
		return prefix + "_start"
	case TaskPowerOff:
		return prefix + "_stop"
	case TaskShutdown:
		return prefix + "_shutdown"
	case TaskSnapshotCreate:
		return prefix + "_snapshot_create"
	case TaskSnapshotCleanup:
		return prefix + "_snapshot_delete"
	case TaskSnapshotRotate:
		return prefix + "_snapshot_rotate"
	case TaskBackupCreate:
		return prefix + "_backup"
	case TaskMigrate:
		return prefix + "_migrate"
	default:
		return ""
	}
}

// --- Simple cron parser ---
// Supports: minute hour dom month dow (5 fields)
// Supports: *, */N, N, N-M, N,M,O

// nextCronTime returns the next time after 'after' that matches the cron expression
func nextCronTime(expr string, after time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron expression must have 5 fields")
	}

	minuteSet, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("minute: %w", err)
	}
	hourSet, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("hour: %w", err)
	}
	domSet, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("dom: %w", err)
	}
	monthSet, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("month: %w", err)
	}
	dowSet, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return time.Time{}, fmt.Errorf("dow: %w", err)
	}

	// Start from the next minute
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Search up to 1 year ahead
	limit := t.Add(366 * 24 * time.Hour)
	for t.Before(limit) {
		if monthSet[int(t.Month())] &&
			domSet[t.Day()] &&
			dowSet[int(t.Weekday())] &&
			hourSet[t.Hour()] &&
			minuteSet[t.Minute()] {
			return t, nil
		}
		t = t.Add(time.Minute)
	}

	return time.Time{}, fmt.Errorf("no matching time found within 1 year")
}

// parseCronField parses a single cron field into a set of matching values
func parseCronField(field string, min, max int) (map[int]bool, error) {
	set := make(map[int]bool)

	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)

		// Handle */N (step)
		if strings.HasPrefix(part, "*/") {
			step, err := strconv.Atoi(part[2:])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step: %s", part)
			}
			for i := min; i <= max; i += step {
				set[i] = true
			}
			continue
		}

		// Handle * (all)
		if part == "*" {
			for i := min; i <= max; i++ {
				set[i] = true
			}
			continue
		}

		// Handle N-M (range)
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			start, err1 := strconv.Atoi(rangeParts[0])
			end, err2 := strconv.Atoi(rangeParts[1])
			if err1 != nil || err2 != nil || start < min || end > max || start > end {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			for i := start; i <= end; i++ {
				set[i] = true
			}
			continue
		}

		// Handle single value
		val, err := strconv.Atoi(part)
		if err != nil || val < min || val > max {
			return nil, fmt.Errorf("invalid value: %s", part)
		}
		set[val] = true
	}

	if len(set) == 0 {
		return nil, fmt.Errorf("empty field")
	}
	return set, nil
}
