package webhooks

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var ErrNotFound = errors.New("webhook endpoint not found")

// DB handles webhook endpoint persistence.
type DB struct {
	conn *sql.DB
	mu   sync.Mutex
}

// Open opens or creates the webhooks database.
func Open(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	slog.Info("webhooks database opened", "path", dbPath)
	return db, nil
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS webhook_endpoints (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		events TEXT NOT NULL,           -- JSON array; empty array = all events
		secret_encrypted TEXT NOT NULL, -- AES-GCM ciphertext (or plaintext if encryption disabled)
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		last_fired_at INTEGER,
		last_status TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_enabled ON webhook_endpoints(enabled);
	`
	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) Close() error { return db.conn.Close() }

// endpointRow reads a row into Endpoint + secret ciphertext.
type endpointRow struct {
	Endpoint
	SecretEncrypted string
}

func scanRow(scanner interface {
	Scan(dest ...any) error
}) (*endpointRow, error) {
	var r endpointRow
	var eventsJSON string
	var createdAt, updatedAt int64
	var lastFired sql.NullInt64
	var lastStatus sql.NullString
	var enabled int
	if err := scanner.Scan(
		&r.ID, &r.Name, &r.URL, &eventsJSON, &r.SecretEncrypted,
		&enabled, &createdAt, &updatedAt, &lastFired, &lastStatus,
	); err != nil {
		return nil, err
	}
	if eventsJSON != "" {
		if err := json.Unmarshal([]byte(eventsJSON), &r.Events); err != nil {
			return nil, fmt.Errorf("decode events: %w", err)
		}
	}
	r.Enabled = enabled != 0
	r.CreatedAt = time.Unix(createdAt, 0)
	r.UpdatedAt = time.Unix(updatedAt, 0)
	if lastFired.Valid {
		r.LastFiredAt = time.Unix(lastFired.Int64, 0)
	}
	if lastStatus.Valid {
		r.LastStatus = lastStatus.String
	}
	return &r, nil
}

const selectCols = `id, name, url, events, secret_encrypted, enabled, created_at, updated_at, last_fired_at, last_status`

// List returns all endpoints.
func (db *DB) List() ([]*endpointRow, error) {
	rows, err := db.conn.Query(`SELECT ` + selectCols + ` FROM webhook_endpoints ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*endpointRow
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListEnabled returns only enabled endpoints (hot path for dispatch).
func (db *DB) ListEnabled() ([]*endpointRow, error) {
	rows, err := db.conn.Query(`SELECT ` + selectCols + ` FROM webhook_endpoints WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*endpointRow
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get returns a single endpoint.
func (db *DB) Get(id string) (*endpointRow, error) {
	row := db.conn.QueryRow(`SELECT `+selectCols+` FROM webhook_endpoints WHERE id = ?`, id)
	r, err := scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

// Insert creates a new endpoint row.
func (db *DB) Insert(r *endpointRow) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	events := r.Events
	if events == nil {
		events = []string{}
	}
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return err
	}
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	_, err = db.conn.Exec(
		`INSERT INTO webhook_endpoints (`+selectCols+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Name, r.URL, string(eventsJSON), r.SecretEncrypted,
		enabled, r.CreatedAt.Unix(), r.UpdatedAt.Unix(), nil, nil,
	)
	return err
}

// Update modifies name/url/events/enabled (not secret).
func (db *DB) Update(id string, req UpdateRequest) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	events := req.Events
	if events == nil {
		events = []string{}
	}
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return err
	}
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	res, err := db.conn.Exec(
		`UPDATE webhook_endpoints SET name=?, url=?, events=?, enabled=?, updated_at=? WHERE id=?`,
		req.Name, req.URL, string(eventsJSON), enabled, time.Now().Unix(), id,
	)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes an endpoint.
func (db *DB) Delete(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	res, err := db.conn.Exec(`DELETE FROM webhook_endpoints WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordDelivery updates last_fired_at + last_status after a dispatch attempt.
func (db *DB) RecordDelivery(id string, success bool, when time.Time) {
	db.mu.Lock()
	defer db.mu.Unlock()

	status := "failure"
	if success {
		status = "success"
	}
	if _, err := db.conn.Exec(
		`UPDATE webhook_endpoints SET last_fired_at=?, last_status=? WHERE id=?`,
		when.Unix(), status, id,
	); err != nil {
		slog.Warn("webhook: failed to record delivery", "id", id, "err", err)
	}
}

// sanitizeName returns a non-empty display name.
func sanitizeName(n string) string {
	n = strings.TrimSpace(n)
	if n == "" {
		return "(unnamed)"
	}
	return n
}
