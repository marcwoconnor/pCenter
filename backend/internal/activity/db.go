package activity

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB handles activity log persistence
type DB struct {
	conn       *sql.DB
	mu         sync.Mutex
	insertStmt *sql.Stmt
}

// OpenDB opens or creates the activity database
func OpenDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=1000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	db := &DB{conn: conn}

	if err := db.createSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	if err := db.prepareStatements(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("prepare statements: %w", err)
	}

	slog.Info("activity database opened", "path", dbPath)
	return db, nil
}

func (db *DB) createSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS activity (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		action TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		resource_name TEXT,
		cluster TEXT NOT NULL,
		details TEXT,
		status TEXT DEFAULT 'success'
	);
	CREATE INDEX IF NOT EXISTS idx_activity_timestamp ON activity(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_activity_resource ON activity(resource_type, resource_id);
	CREATE INDEX IF NOT EXISTS idx_activity_cluster ON activity(cluster);
	`
	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) prepareStatements() error {
	var err error
	db.insertStmt, err = db.conn.Prepare(`
		INSERT INTO activity (timestamp, action, resource_type, resource_id, resource_name, cluster, details, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	return err
}

// Insert adds a new activity entry
func (db *DB) Insert(e Entry) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	ts := e.Timestamp.Unix()
	if e.Timestamp.IsZero() {
		ts = time.Now().Unix()
	}

	result, err := db.insertStmt.Exec(
		ts,
		e.Action,
		e.ResourceType,
		e.ResourceID,
		e.ResourceName,
		e.Cluster,
		e.Details,
		e.Status,
	)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// Query retrieves activity entries with optional filters
func (db *DB) Query(params QueryParams) ([]Entry, error) {
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 500 {
		params.Limit = 500
	}

	query := `SELECT id, timestamp, action, resource_type, resource_id,
	          COALESCE(resource_name, ''), cluster, COALESCE(details, ''), status
	          FROM activity WHERE 1=1`
	args := []interface{}{}

	if params.ResourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, params.ResourceType)
	}
	if params.ResourceID != "" {
		query += " AND resource_id = ?"
		args = append(args, params.ResourceID)
	}
	if params.Cluster != "" {
		query += " AND cluster = ?"
		args = append(args, params.Cluster)
	}
	if params.Action != "" {
		query += " AND action = ?"
		args = append(args, params.Action)
	}

	query += " ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, params.Limit, params.Offset)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var ts int64
		err := rows.Scan(&e.ID, &ts, &e.Action, &e.ResourceType, &e.ResourceID,
			&e.ResourceName, &e.Cluster, &e.Details, &e.Status)
		if err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(ts, 0)
		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// Cleanup removes entries older than retention period
func (db *DB) Cleanup(retentionDays int) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	result, err := db.conn.Exec("DELETE FROM activity WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("cleaned activity log", "deleted", rows, "retention_days", retentionDays)
	}
	return rows, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.insertStmt != nil {
		db.insertStmt.Close()
	}
	return db.conn.Close()
}
