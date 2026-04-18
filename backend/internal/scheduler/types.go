package scheduler

import "time"

// TaskType defines the type of scheduled task
type TaskType string

const (
	TaskPowerOn         TaskType = "power_on"
	TaskPowerOff        TaskType = "power_off"
	TaskShutdown        TaskType = "shutdown"
	TaskSnapshotCreate  TaskType = "snapshot_create"
	TaskSnapshotCleanup TaskType = "snapshot_cleanup" // delete a specific named snapshot
	TaskSnapshotRotate  TaskType = "snapshot_rotate"  // create a new snapshot + prune auto-* beyond retention
	TaskBackupCreate    TaskType = "backup_create"    // vzdump to target storage
	TaskACMERenew       TaskType = "acme_renew"       // renew ACME cert across all online nodes in cluster
	TaskMigrate         TaskType = "migrate"
)

// TargetType defines what the task targets
type TargetType string

const (
	TargetVM        TargetType = "vm"
	TargetContainer TargetType = "ct"
)

// ScheduledTask defines a recurring task
type ScheduledTask struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Enabled     bool       `json:"enabled"`
	TaskType    TaskType   `json:"task_type"`
	TargetType  TargetType `json:"target_type"` // vm or ct
	TargetID    int        `json:"target_id"`   // VMID
	Cluster     string     `json:"cluster"`
	CronExpr    string     `json:"cron_expr"`       // cron expression (minute hour dom month dow)
	Params      string     `json:"params,omitempty"` // JSON: e.g. {"target_node":"pve05","retention":3}
	LastRun     *time.Time `json:"last_run,omitempty"`
	NextRun     *time.Time `json:"next_run,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TaskRun records a single execution of a scheduled task
type TaskRun struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	TaskName  string    `json:"task_name,omitempty"` // populated on read
	StartedAt time.Time `json:"started_at"`
	Duration  int       `json:"duration_ms"` // milliseconds
	Success   bool      `json:"success"`
	UPID      string    `json:"upid,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// --- Request types ---

type CreateTaskRequest struct {
	Name       string     `json:"name"`
	TaskType   TaskType   `json:"task_type"`
	TargetType TargetType `json:"target_type"`
	TargetID   int        `json:"target_id"`
	Cluster    string     `json:"cluster"`
	CronExpr   string     `json:"cron_expr"`
	Params     string     `json:"params,omitempty"`
	Enabled    bool       `json:"enabled"`
}

type UpdateTaskRequest struct {
	Name     string   `json:"name"`
	CronExpr string   `json:"cron_expr"`
	Params   string   `json:"params,omitempty"`
	Enabled  bool     `json:"enabled"`
}
