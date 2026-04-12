package scheduler

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// DB handles scheduler persistence
type DB struct {
	db *sql.DB
}

// Open opens or creates the scheduler database
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open scheduler db: %w", err)
	}

	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("init scheduler schema: %w", err)
	}

	return &DB{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS scheduled_tasks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		enabled INTEGER DEFAULT 1,
		task_type TEXT NOT NULL,
		target_type TEXT NOT NULL,
		target_id INTEGER NOT NULL,
		cluster TEXT NOT NULL,
		cron_expr TEXT NOT NULL,
		params TEXT DEFAULT '{}',
		last_run INTEGER,
		next_run INTEGER,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS task_runs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
		started_at INTEGER NOT NULL,
		duration_ms INTEGER DEFAULT 0,
		success INTEGER DEFAULT 0,
		upid TEXT DEFAULT '',
		error TEXT DEFAULT '',
		FOREIGN KEY (task_id) REFERENCES scheduled_tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_task_runs_task ON task_runs(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_runs_started ON task_runs(started_at);
	CREATE INDEX IF NOT EXISTS idx_tasks_next_run ON scheduled_tasks(next_run);
	`
	_, err := db.Exec(schema)
	return err
}

// ListTasks returns all scheduled tasks
func (d *DB) ListTasks() ([]ScheduledTask, error) {
	rows, err := d.db.Query(`
		SELECT id, name, enabled, task_type, target_type, target_id, cluster,
			cron_expr, params, last_run, next_run, created_at, updated_at
		FROM scheduled_tasks ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []ScheduledTask
	for rows.Next() {
		var t ScheduledTask
		var enabled int
		var lastRun, nextRun sql.NullInt64
		var createdAt, updatedAt int64

		if err := rows.Scan(&t.ID, &t.Name, &enabled, &t.TaskType, &t.TargetType,
			&t.TargetID, &t.Cluster, &t.CronExpr, &t.Params,
			&lastRun, &nextRun, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.Enabled = enabled == 1
		if lastRun.Valid {
			lr := time.Unix(lastRun.Int64, 0)
			t.LastRun = &lr
		}
		if nextRun.Valid {
			nr := time.Unix(nextRun.Int64, 0)
			t.NextRun = &nr
		}
		t.CreatedAt = time.Unix(createdAt, 0)
		t.UpdatedAt = time.Unix(updatedAt, 0)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// GetTask returns a single task
func (d *DB) GetTask(id string) (*ScheduledTask, error) {
	var t ScheduledTask
	var enabled int
	var lastRun, nextRun sql.NullInt64
	var createdAt, updatedAt int64

	err := d.db.QueryRow(`
		SELECT id, name, enabled, task_type, target_type, target_id, cluster,
			cron_expr, params, last_run, next_run, created_at, updated_at
		FROM scheduled_tasks WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &enabled, &t.TaskType, &t.TargetType,
			&t.TargetID, &t.Cluster, &t.CronExpr, &t.Params,
			&lastRun, &nextRun, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled == 1
	if lastRun.Valid {
		lr := time.Unix(lastRun.Int64, 0)
		t.LastRun = &lr
	}
	if nextRun.Valid {
		nr := time.Unix(nextRun.Int64, 0)
		t.NextRun = &nr
	}
	t.CreatedAt = time.Unix(createdAt, 0)
	t.UpdatedAt = time.Unix(updatedAt, 0)
	return &t, nil
}

// CreateTask creates a new scheduled task
func (d *DB) CreateTask(req CreateTaskRequest) (*ScheduledTask, error) {
	now := time.Now()
	id := uuid.New().String()[:12]
	enabled := 0
	if req.Enabled {
		enabled = 1
	}

	_, err := d.db.Exec(`
		INSERT INTO scheduled_tasks (id, name, enabled, task_type, target_type, target_id,
			cluster, cron_expr, params, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.Name, enabled, string(req.TaskType), string(req.TargetType),
		req.TargetID, req.Cluster, req.CronExpr, req.Params,
		now.Unix(), now.Unix())
	if err != nil {
		return nil, err
	}

	return &ScheduledTask{
		ID:         id,
		Name:       req.Name,
		Enabled:    req.Enabled,
		TaskType:   req.TaskType,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		Cluster:    req.Cluster,
		CronExpr:   req.CronExpr,
		Params:     req.Params,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// UpdateTask updates a task
func (d *DB) UpdateTask(id string, req UpdateTaskRequest) error {
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	result, err := d.db.Exec(`
		UPDATE scheduled_tasks SET name = ?, cron_expr = ?, params = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		req.Name, req.CronExpr, req.Params, enabled, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found")
	}
	return nil
}

// DeleteTask deletes a task and its run history
func (d *DB) DeleteTask(id string) error {
	result, err := d.db.Exec(`DELETE FROM scheduled_tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task not found")
	}
	return nil
}

// SetNextRun updates the next run time for a task
func (d *DB) SetNextRun(id string, nextRun time.Time) error {
	_, err := d.db.Exec(`UPDATE scheduled_tasks SET next_run = ? WHERE id = ?`, nextRun.Unix(), id)
	return err
}

// SetLastRun records that a task ran
func (d *DB) SetLastRun(id string, lastRun time.Time) error {
	_, err := d.db.Exec(`UPDATE scheduled_tasks SET last_run = ? WHERE id = ?`, lastRun.Unix(), id)
	return err
}

// AddRun records a task execution
func (d *DB) AddRun(taskID string, startedAt time.Time, durationMs int, success bool, upid, errMsg string) error {
	id := uuid.New().String()[:12]
	successInt := 0
	if success {
		successInt = 1
	}
	_, err := d.db.Exec(`
		INSERT INTO task_runs (id, task_id, started_at, duration_ms, success, upid, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, taskID, startedAt.Unix(), durationMs, successInt, upid, errMsg)
	return err
}

// ListRuns returns recent task runs, optionally filtered by task ID
func (d *DB) ListRuns(taskID string, limit int) ([]TaskRun, error) {
	query := `
		SELECT r.id, r.task_id, t.name, r.started_at, r.duration_ms, r.success, r.upid, r.error
		FROM task_runs r
		JOIN scheduled_tasks t ON t.id = r.task_id
	`
	var args []interface{}
	if taskID != "" {
		query += ` WHERE r.task_id = ?`
		args = append(args, taskID)
	}
	query += ` ORDER BY r.started_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []TaskRun
	for rows.Next() {
		var r TaskRun
		var success int
		var startedAt int64
		if err := rows.Scan(&r.ID, &r.TaskID, &r.TaskName, &startedAt,
			&r.Duration, &success, &r.UPID, &r.Error); err != nil {
			return nil, err
		}
		r.Success = success == 1
		r.StartedAt = time.Unix(startedAt, 0)
		runs = append(runs, r)
	}
	return runs, nil
}

// GetDueTasks returns enabled tasks whose next_run is at or before now
func (d *DB) GetDueTasks(now time.Time) ([]ScheduledTask, error) {
	rows, err := d.db.Query(`
		SELECT id, name, enabled, task_type, target_type, target_id, cluster,
			cron_expr, params, last_run, next_run, created_at, updated_at
		FROM scheduled_tasks
		WHERE enabled = 1 AND next_run IS NOT NULL AND next_run <= ?
		ORDER BY next_run
	`, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []ScheduledTask
	for rows.Next() {
		var t ScheduledTask
		var enabled int
		var lastRun, nextRun sql.NullInt64
		var createdAt, updatedAt int64

		if err := rows.Scan(&t.ID, &t.Name, &enabled, &t.TaskType, &t.TargetType,
			&t.TargetID, &t.Cluster, &t.CronExpr, &t.Params,
			&lastRun, &nextRun, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.Enabled = enabled == 1
		if lastRun.Valid {
			lr := time.Unix(lastRun.Int64, 0)
			t.LastRun = &lr
		}
		if nextRun.Valid {
			nr := time.Unix(nextRun.Int64, 0)
			t.NextRun = &nr
		}
		t.CreatedAt = time.Unix(createdAt, 0)
		t.UpdatedAt = time.Unix(updatedAt, 0)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// CleanupOldRuns deletes runs older than the specified duration
func (d *DB) CleanupOldRuns(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).Unix()
	result, err := d.db.Exec(`DELETE FROM task_runs WHERE started_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// Close closes the database
func (d *DB) Close() error {
	return d.db.Close()
}
