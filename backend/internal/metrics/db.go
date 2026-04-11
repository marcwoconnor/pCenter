package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite database connection
type DB struct {
	conn *sql.DB
	mu   sync.RWMutex

	// Prepared statements for performance
	insertRawStmt    *sql.Stmt
	insertHourlyStmt *sql.Stmt
	insertDailyStmt  *sql.Stmt
	insertWeeklyStmt *sql.Stmt
	insertMonthlyStmt *sql.Stmt
}

// Open creates or opens the metrics database
func Open(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// Open with WAL mode for better concurrent performance
	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Set connection pool settings
	conn.SetMaxOpenConns(1) // SQLite only supports one writer
	conn.SetMaxIdleConns(1)

	db := &DB{conn: conn}

	// Run migrations
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Prepare statements
	if err := db.prepareStatements(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("prepare statements: %w", err)
	}

	slog.Info("metrics database opened", "path", dbPath)
	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.insertRawStmt != nil {
		db.insertRawStmt.Close()
	}
	if db.insertHourlyStmt != nil {
		db.insertHourlyStmt.Close()
	}
	if db.insertDailyStmt != nil {
		db.insertDailyStmt.Close()
	}
	if db.insertWeeklyStmt != nil {
		db.insertWeeklyStmt.Close()
	}
	if db.insertMonthlyStmt != nil {
		db.insertMonthlyStmt.Close()
	}
	return db.conn.Close()
}

// migrate runs database migrations
func (db *DB) migrate() error {
	schema := `
	-- Metric type lookup table
	CREATE TABLE IF NOT EXISTS metric_types (
		id INTEGER PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		unit TEXT NOT NULL
	);

	-- Raw metrics (30-sec samples, kept 24h)
	CREATE TABLE IF NOT EXISTS metrics_raw (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		cluster TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		metric_type_id INTEGER NOT NULL,
		value REAL NOT NULL
	);

	-- Hourly rollups (kept 7 days)
	CREATE TABLE IF NOT EXISTS metrics_hourly (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		bucket_timestamp INTEGER NOT NULL,
		cluster TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		metric_type_id INTEGER NOT NULL,
		min_value REAL NOT NULL,
		max_value REAL NOT NULL,
		avg_value REAL NOT NULL,
		sample_count INTEGER NOT NULL,
		UNIQUE(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id)
	);

	-- Daily rollups (kept 30 days)
	CREATE TABLE IF NOT EXISTS metrics_daily (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		bucket_timestamp INTEGER NOT NULL,
		cluster TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		metric_type_id INTEGER NOT NULL,
		min_value REAL NOT NULL,
		max_value REAL NOT NULL,
		avg_value REAL NOT NULL,
		sample_count INTEGER NOT NULL,
		UNIQUE(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id)
	);

	-- Weekly rollups (kept 1 year)
	CREATE TABLE IF NOT EXISTS metrics_weekly (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		bucket_timestamp INTEGER NOT NULL,
		cluster TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		metric_type_id INTEGER NOT NULL,
		min_value REAL NOT NULL,
		max_value REAL NOT NULL,
		avg_value REAL NOT NULL,
		sample_count INTEGER NOT NULL,
		UNIQUE(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id)
	);

	-- Monthly rollups (kept indefinitely)
	CREATE TABLE IF NOT EXISTS metrics_monthly (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		bucket_timestamp INTEGER NOT NULL,
		cluster TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		metric_type_id INTEGER NOT NULL,
		min_value REAL NOT NULL,
		max_value REAL NOT NULL,
		avg_value REAL NOT NULL,
		sample_count INTEGER NOT NULL,
		UNIQUE(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id)
	);

	-- Indexes for raw metrics
	CREATE INDEX IF NOT EXISTS idx_raw_timestamp ON metrics_raw(timestamp);
	CREATE INDEX IF NOT EXISTS idx_raw_resource ON metrics_raw(cluster, resource_type, resource_id, metric_type_id, timestamp);

	-- Indexes for rollup tables
	CREATE INDEX IF NOT EXISTS idx_hourly_resource ON metrics_hourly(cluster, resource_type, resource_id, metric_type_id, bucket_timestamp);
	CREATE INDEX IF NOT EXISTS idx_daily_resource ON metrics_daily(cluster, resource_type, resource_id, metric_type_id, bucket_timestamp);
	CREATE INDEX IF NOT EXISTS idx_weekly_resource ON metrics_weekly(cluster, resource_type, resource_id, metric_type_id, bucket_timestamp);
	CREATE INDEX IF NOT EXISTS idx_monthly_resource ON metrics_monthly(cluster, resource_type, resource_id, metric_type_id, bucket_timestamp);
	`

	if _, err := db.conn.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// Seed metric types
	for _, mt := range MetricTypes {
		_, err := db.conn.Exec(
			"INSERT OR IGNORE INTO metric_types (id, name, unit) VALUES (?, ?, ?)",
			mt.ID, mt.Name, mt.Unit,
		)
		if err != nil {
			return fmt.Errorf("seed metric type %s: %w", mt.Name, err)
		}
	}

	return nil
}

// prepareStatements prepares frequently used statements
func (db *DB) prepareStatements() error {
	var err error

	db.insertRawStmt, err = db.conn.Prepare(`
		INSERT INTO metrics_raw (timestamp, cluster, resource_type, resource_id, metric_type_id, value)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert raw: %w", err)
	}

	db.insertHourlyStmt, err = db.conn.Prepare(`
		INSERT OR REPLACE INTO metrics_hourly
		(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id, min_value, max_value, avg_value, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert hourly: %w", err)
	}

	db.insertDailyStmt, err = db.conn.Prepare(`
		INSERT OR REPLACE INTO metrics_daily
		(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id, min_value, max_value, avg_value, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert daily: %w", err)
	}

	db.insertWeeklyStmt, err = db.conn.Prepare(`
		INSERT OR REPLACE INTO metrics_weekly
		(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id, min_value, max_value, avg_value, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert weekly: %w", err)
	}

	db.insertMonthlyStmt, err = db.conn.Prepare(`
		INSERT OR REPLACE INTO metrics_monthly
		(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id, min_value, max_value, avg_value, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert monthly: %w", err)
	}

	return nil
}

// Conn returns the underlying database connection for advanced queries
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// BeginTx starts a new transaction
func (db *DB) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return db.conn.BeginTx(ctx, nil)
}

// InsertRawMetric inserts a single raw metric
func (db *DB) InsertRawMetric(m RawMetric) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.insertRawStmt.Exec(m.Timestamp, m.Cluster, m.ResourceType, m.ResourceID, m.MetricTypeID, m.Value)
	return err
}

// InsertRawMetricsBatch inserts multiple raw metrics in a transaction
func (db *DB) InsertRawMetricsBatch(ctx context.Context, metrics []RawMetric) error {
	if len(metrics) == 0 {
		return nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt := tx.StmtContext(ctx, db.insertRawStmt)
	for _, m := range metrics {
		_, err := stmt.ExecContext(ctx, m.Timestamp, m.Cluster, m.ResourceType, m.ResourceID, m.MetricTypeID, m.Value)
		if err != nil {
			return fmt.Errorf("insert metric: %w", err)
		}
	}

	return tx.Commit()
}

// InsertRollup inserts a rollup metric into the appropriate table
func (db *DB) InsertRollup(table string, m RollupMetric) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var stmt *sql.Stmt
	switch table {
	case "hourly":
		stmt = db.insertHourlyStmt
	case "daily":
		stmt = db.insertDailyStmt
	case "weekly":
		stmt = db.insertWeeklyStmt
	case "monthly":
		stmt = db.insertMonthlyStmt
	default:
		return fmt.Errorf("unknown rollup table: %s", table)
	}

	_, err := stmt.Exec(m.BucketTimestamp, m.Cluster, m.ResourceType, m.ResourceID, m.MetricTypeID,
		m.MinValue, m.MaxValue, m.AvgValue, m.SampleCount)
	return err
}

// Vacuum runs VACUUM to reclaim space after deletions
func (db *DB) Vacuum() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.conn.Exec("VACUUM")
	return err
}

// GetStats returns database statistics
func (db *DB) GetStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	tables := []string{"metrics_raw", "metrics_hourly", "metrics_daily", "metrics_weekly", "metrics_monthly"}
	for _, table := range tables {
		var count int64
		err := db.conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		if err != nil {
			return nil, err
		}
		stats[table] = count
	}

	return stats, nil
}
