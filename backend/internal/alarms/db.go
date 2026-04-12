package alarms

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite connection for alarm storage
type DB struct {
	conn *sql.DB
}

// Open creates or opens the alarms database
func Open(dbPath string) (*DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=1000&_foreign_keys=ON", dbPath)
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	slog.Info("alarms database opened", "path", dbPath)
	return db, nil
}

func (db *DB) Close() error { return db.conn.Close() }

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS alarm_definitions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		enabled INTEGER DEFAULT 1,
		metric_type TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		scope TEXT NOT NULL DEFAULT 'global',
		scope_target TEXT DEFAULT '',
		condition TEXT NOT NULL DEFAULT 'above',
		warning_threshold REAL NOT NULL,
		critical_threshold REAL NOT NULL,
		clear_threshold REAL NOT NULL,
		duration_samples INTEGER DEFAULT 3,
		notify_channels TEXT DEFAULT '[]',
		created_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS alarm_instances (
		id TEXT PRIMARY KEY,
		definition_id TEXT NOT NULL,
		cluster TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'normal',
		current_value REAL DEFAULT 0,
		threshold REAL DEFAULT 0,
		triggered_at INTEGER,
		last_evaluated_at INTEGER,
		acknowledged_by TEXT,
		acknowledged_at INTEGER,
		consecutive_count INTEGER DEFAULT 0,
		FOREIGN KEY (definition_id) REFERENCES alarm_definitions(id) ON DELETE CASCADE,
		UNIQUE(definition_id, cluster, resource_type, resource_id)
	);

	CREATE TABLE IF NOT EXISTS notification_channels (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'webhook',
		config TEXT NOT NULL DEFAULT '{}',
		enabled INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS alarm_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		alarm_instance_id TEXT NOT NULL,
		definition_name TEXT,
		cluster TEXT,
		resource_type TEXT,
		resource_id TEXT,
		resource_name TEXT,
		old_state TEXT,
		new_state TEXT,
		value REAL,
		threshold REAL,
		timestamp INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_alarm_history_time ON alarm_history(timestamp);
	CREATE INDEX IF NOT EXISTS idx_alarm_instances_state ON alarm_instances(state);
	`
	_, err := db.conn.Exec(schema)
	return err
}

// ── Definition CRUD ─────────────────────────────────────────────────────────

func (db *DB) CreateDefinition(ctx context.Context, def *AlarmDefinition) error {
	def.ID = uuid.New().String()
	def.CreatedAt = time.Now().Unix()
	channels, _ := json.Marshal(def.NotifyChannels)

	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO alarm_definitions (id, name, enabled, metric_type, resource_type, scope, scope_target, condition, warning_threshold, critical_threshold, clear_threshold, duration_samples, notify_channels, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		def.ID, def.Name, def.Enabled, def.MetricType, def.ResourceType,
		def.Scope, def.ScopeTarget, def.Condition,
		def.WarningThreshold, def.CriticalThreshold, def.ClearThreshold,
		def.DurationSamples, string(channels), def.CreatedAt)
	return err
}

func (db *DB) UpdateDefinition(ctx context.Context, def *AlarmDefinition) error {
	channels, _ := json.Marshal(def.NotifyChannels)
	_, err := db.conn.ExecContext(ctx,
		`UPDATE alarm_definitions SET name=?, enabled=?, metric_type=?, resource_type=?, scope=?, scope_target=?, condition=?, warning_threshold=?, critical_threshold=?, clear_threshold=?, duration_samples=?, notify_channels=? WHERE id=?`,
		def.Name, def.Enabled, def.MetricType, def.ResourceType,
		def.Scope, def.ScopeTarget, def.Condition,
		def.WarningThreshold, def.CriticalThreshold, def.ClearThreshold,
		def.DurationSamples, string(channels), def.ID)
	return err
}

func (db *DB) DeleteDefinition(ctx context.Context, id string) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM alarm_definitions WHERE id = ?", id)
	return err
}

func (db *DB) ListDefinitions(ctx context.Context) ([]AlarmDefinition, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, name, enabled, metric_type, resource_type, scope, scope_target, condition, warning_threshold, critical_threshold, clear_threshold, duration_samples, notify_channels, created_at FROM alarm_definitions ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var defs []AlarmDefinition
	for rows.Next() {
		var d AlarmDefinition
		var channels string
		var enabled int
		err := rows.Scan(&d.ID, &d.Name, &enabled, &d.MetricType, &d.ResourceType,
			&d.Scope, &d.ScopeTarget, &d.Condition,
			&d.WarningThreshold, &d.CriticalThreshold, &d.ClearThreshold,
			&d.DurationSamples, &channels, &d.CreatedAt)
		if err != nil {
			return nil, err
		}
		d.Enabled = enabled != 0
		json.Unmarshal([]byte(channels), &d.NotifyChannels)
		if d.NotifyChannels == nil {
			d.NotifyChannels = []string{}
		}
		defs = append(defs, d)
	}
	return defs, nil
}

// ── Instance CRUD ───────────────────────────────────────────────────────────

func (db *DB) UpsertInstance(ctx context.Context, inst *AlarmInstance) error {
	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO alarm_instances (id, definition_id, cluster, resource_type, resource_id, state, current_value, threshold, triggered_at, last_evaluated_at, acknowledged_by, acknowledged_at, consecutive_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(definition_id, cluster, resource_type, resource_id) DO UPDATE SET
			state=excluded.state, current_value=excluded.current_value, threshold=excluded.threshold,
			triggered_at=COALESCE(excluded.triggered_at, triggered_at),
			last_evaluated_at=excluded.last_evaluated_at,
			acknowledged_by=excluded.acknowledged_by, acknowledged_at=excluded.acknowledged_at,
			consecutive_count=excluded.consecutive_count`,
		inst.ID, inst.DefinitionID, inst.Cluster, inst.ResourceType, inst.ResourceID,
		inst.State, inst.CurrentValue, inst.Threshold, inst.TriggeredAt, inst.LastEvaluatedAt,
		inst.AcknowledgedBy, inst.AcknowledgedAt, inst.ConsecutiveCount)
	return err
}

func (db *DB) GetActiveAlarms(ctx context.Context) ([]AlarmInstance, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT ai.id, ai.definition_id, ad.name, ai.cluster, ai.resource_type, ai.resource_id,
			ai.state, ai.current_value, ai.threshold, ai.triggered_at, ai.last_evaluated_at,
			ai.acknowledged_by, ai.acknowledged_at, ai.consecutive_count
		FROM alarm_instances ai
		JOIN alarm_definitions ad ON ai.definition_id = ad.id
		WHERE ai.state != 'normal'
		ORDER BY CASE ai.state WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, ai.triggered_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstances(rows)
}

func (db *DB) AcknowledgeAlarm(ctx context.Context, id, user string) error {
	now := time.Now().Unix()
	_, err := db.conn.ExecContext(ctx,
		"UPDATE alarm_instances SET acknowledged_by = ?, acknowledged_at = ? WHERE id = ?",
		user, now, id)
	return err
}

func (db *DB) RecordHistory(ctx context.Context, inst *AlarmInstance, oldState, newState AlarmState) error {
	_, err := db.conn.ExecContext(ctx,
		`INSERT INTO alarm_history (alarm_instance_id, definition_name, cluster, resource_type, resource_id, resource_name, old_state, new_state, value, threshold, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inst.ID, inst.DefinitionName, inst.Cluster, inst.ResourceType, inst.ResourceID,
		inst.ResourceName, oldState, newState, inst.CurrentValue, inst.Threshold, time.Now().Unix())
	return err
}

func (db *DB) GetHistory(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, alarm_instance_id, definition_name, cluster, resource_type, resource_id, resource_name, old_state, new_state, value, threshold, timestamp
		FROM alarm_history ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []map[string]interface{}
	for rows.Next() {
		var id int64
		var instID, defName, cluster, resType, resID, resName, oldState, newState string
		var value, threshold float64
		var ts int64
		if err := rows.Scan(&id, &instID, &defName, &cluster, &resType, &resID, &resName, &oldState, &newState, &value, &threshold, &ts); err != nil {
			return nil, err
		}
		history = append(history, map[string]interface{}{
			"id": id, "alarm_instance_id": instID, "definition_name": defName,
			"cluster": cluster, "resource_type": resType, "resource_id": resID,
			"resource_name": resName, "old_state": oldState, "new_state": newState,
			"value": value, "threshold": threshold, "timestamp": ts,
		})
	}
	return history, nil
}

// ── Channel CRUD ────────────────────────────────────────────────────────────

func (db *DB) CreateChannel(ctx context.Context, ch *NotificationChannel) error {
	ch.ID = uuid.New().String()
	_, err := db.conn.ExecContext(ctx,
		"INSERT INTO notification_channels (id, name, type, config, enabled) VALUES (?, ?, ?, ?, ?)",
		ch.ID, ch.Name, ch.Type, ch.Config, ch.Enabled)
	return err
}

func (db *DB) ListChannels(ctx context.Context) ([]NotificationChannel, error) {
	rows, err := db.conn.QueryContext(ctx,
		"SELECT id, name, type, config, enabled FROM notification_channels ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []NotificationChannel
	for rows.Next() {
		var ch NotificationChannel
		var enabled int
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Config, &enabled); err != nil {
			return nil, err
		}
		ch.Enabled = enabled != 0
		channels = append(channels, ch)
	}
	return channels, nil
}

func (db *DB) DeleteChannel(ctx context.Context, id string) error {
	_, err := db.conn.ExecContext(ctx, "DELETE FROM notification_channels WHERE id = ?", id)
	return err
}

func (db *DB) GetChannel(ctx context.Context, id string) (*NotificationChannel, error) {
	var ch NotificationChannel
	var enabled int
	err := db.conn.QueryRowContext(ctx,
		"SELECT id, name, type, config, enabled FROM notification_channels WHERE id = ?", id).
		Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Config, &enabled)
	if err != nil {
		return nil, err
	}
	ch.Enabled = enabled != 0
	return &ch, nil
}

// ── Seed defaults ───────────────────────────────────────────────────────────

func (db *DB) SeedDefaults(ctx context.Context) error {
	// Check if any definitions exist
	var count int
	db.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM alarm_definitions").Scan(&count)
	if count > 0 {
		return nil // already seeded
	}

	defaults := []AlarmDefinition{
		{Name: "Node CPU High", MetricType: "cpu", ResourceType: "node", Scope: "global", Condition: CondAbove, WarningThreshold: 90, CriticalThreshold: 95, ClearThreshold: 85, DurationSamples: 3, Enabled: true},
		{Name: "Node Memory High", MetricType: "mem_percent", ResourceType: "node", Scope: "global", Condition: CondAbove, WarningThreshold: 85, CriticalThreshold: 95, ClearThreshold: 80, DurationSamples: 3, Enabled: true},
		{Name: "VM CPU High", MetricType: "cpu", ResourceType: "vm", Scope: "global", Condition: CondAbove, WarningThreshold: 90, CriticalThreshold: 95, ClearThreshold: 85, DurationSamples: 3, Enabled: true},
		{Name: "VM Memory High", MetricType: "mem_percent", ResourceType: "vm", Scope: "global", Condition: CondAbove, WarningThreshold: 90, CriticalThreshold: 95, ClearThreshold: 85, DurationSamples: 3, Enabled: true},
		{Name: "CT CPU High", MetricType: "cpu", ResourceType: "ct", Scope: "global", Condition: CondAbove, WarningThreshold: 90, CriticalThreshold: 95, ClearThreshold: 85, DurationSamples: 3, Enabled: true},
		{Name: "CT Memory High", MetricType: "mem_percent", ResourceType: "ct", Scope: "global", Condition: CondAbove, WarningThreshold: 90, CriticalThreshold: 95, ClearThreshold: 85, DurationSamples: 3, Enabled: true},
	}

	for _, d := range defaults {
		d.NotifyChannels = []string{}
		if err := db.CreateDefinition(ctx, &d); err != nil {
			slog.Warn("failed to seed alarm", "name", d.Name, "error", err)
		}
	}
	slog.Info("seeded default alarm definitions", "count", len(defaults))
	return nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func scanInstances(rows *sql.Rows) ([]AlarmInstance, error) {
	var instances []AlarmInstance
	for rows.Next() {
		var a AlarmInstance
		var ackBy sql.NullString
		var ackAt sql.NullInt64
		var trigAt sql.NullInt64
		err := rows.Scan(&a.ID, &a.DefinitionID, &a.DefinitionName,
			&a.Cluster, &a.ResourceType, &a.ResourceID,
			&a.State, &a.CurrentValue, &a.Threshold, &trigAt, &a.LastEvaluatedAt,
			&ackBy, &ackAt, &a.ConsecutiveCount)
		if err != nil {
			return nil, err
		}
		if ackBy.Valid {
			a.AcknowledgedBy = ackBy.String
		}
		if ackAt.Valid {
			a.AcknowledgedAt = ackAt.Int64
		}
		if trigAt.Valid {
			a.TriggeredAt = trigAt.Int64
		}
		instances = append(instances, a)
	}
	return instances, nil
}
