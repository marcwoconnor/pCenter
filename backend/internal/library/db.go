package library

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite connection for content library operations
type DB struct {
	conn *sql.DB
	mu   sync.Mutex
}

// Open creates or opens the content library database
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

	slog.Info("content library database opened", "path", dbPath)
	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS library_items (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		type TEXT NOT NULL,
		category TEXT DEFAULT '',
		version TEXT DEFAULT '',
		tags TEXT DEFAULT '',

		cluster TEXT NOT NULL,
		node TEXT DEFAULT '',
		storage TEXT NOT NULL,
		volume TEXT NOT NULL,

		size INTEGER DEFAULT 0,
		format TEXT DEFAULT '',

		vmid INTEGER DEFAULT 0,
		os_type TEXT DEFAULT '',
		cores INTEGER DEFAULT 0,
		memory INTEGER DEFAULT 0,

		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		created_by TEXT DEFAULT ''
	);

	CREATE INDEX IF NOT EXISTS idx_library_type ON library_items(type);
	CREATE INDEX IF NOT EXISTS idx_library_cluster ON library_items(cluster);
	CREATE INDEX IF NOT EXISTS idx_library_category ON library_items(category);
	`

	_, err := db.conn.Exec(schema)
	return err
}

// Create adds a new item to the content library
func (db *DB) Create(ctx context.Context, item *Item) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	now := time.Now()
	item.CreatedAt = now
	item.UpdatedAt = now

	tags := strings.Join(item.Tags, ",")

	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO library_items (id, name, description, type, category, version, tags,
			cluster, node, storage, volume, size, format,
			vmid, os_type, cores, memory, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.Name, item.Description, string(item.Type), item.Category, item.Version, tags,
		item.Cluster, item.Node, item.Storage, item.Volume, item.Size, item.Format,
		item.VMID, item.OSType, item.Cores, item.Memory,
		item.CreatedAt.Unix(), item.UpdatedAt.Unix(), item.CreatedBy)

	return err
}

// Get retrieves a single library item by ID
func (db *DB) Get(ctx context.Context, id string) (*Item, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT id, name, description, type, category, version, tags,
			cluster, node, storage, volume, size, format,
			vmid, os_type, cores, memory, created_at, updated_at, created_by
		FROM library_items WHERE id = ?
	`, id)
	return scanItem(row)
}

// List returns library items matching the given filter
func (db *DB) List(ctx context.Context, filter ListFilter) ([]*Item, error) {
	query := `SELECT id, name, description, type, category, version, tags,
		cluster, node, storage, volume, size, format,
		vmid, os_type, cores, memory, created_at, updated_at, created_by
		FROM library_items WHERE 1=1`

	var args []interface{}

	if filter.Type != "" {
		query += " AND type = ?"
		args = append(args, string(filter.Type))
	}
	if filter.Category != "" {
		query += " AND category = ?"
		args = append(args, filter.Category)
	}
	if filter.Cluster != "" {
		query += " AND cluster = ?"
		args = append(args, filter.Cluster)
	}
	if filter.Search != "" {
		query += " AND (name LIKE ? OR description LIKE ?)"
		search := "%" + filter.Search + "%"
		args = append(args, search, search)
	}
	if filter.Tag != "" {
		query += " AND (',' || tags || ',') LIKE ?"
		args = append(args, "%,"+filter.Tag+",%")
	}

	query += " ORDER BY name ASC"

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query library items: %w", err)
	}
	defer rows.Close()

	var items []*Item
	for rows.Next() {
		item, err := scanItemRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []*Item{}
	}
	return items, rows.Err()
}

// Update modifies an existing library item
func (db *DB) Update(ctx context.Context, id string, req UpdateItemRequest) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	sets := []string{"updated_at = ?"}
	args := []interface{}{time.Now().Unix()}

	if req.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}
	if req.Category != nil {
		sets = append(sets, "category = ?")
		args = append(args, *req.Category)
	}
	if req.Version != nil {
		sets = append(sets, "version = ?")
		args = append(args, *req.Version)
	}
	if req.Tags != nil {
		sets = append(sets, "tags = ?")
		args = append(args, strings.Join(req.Tags, ","))
	}

	args = append(args, id)

	result, err := db.conn.ExecContext(ctx, fmt.Sprintf(
		"UPDATE library_items SET %s WHERE id = ?", strings.Join(sets, ", ")), args...)
	if err != nil {
		return err
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("item not found")
	}
	return nil
}

// Delete removes a library item by ID (metadata only, does not delete the actual file)
func (db *DB) Delete(ctx context.Context, id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.conn.ExecContext(ctx, "DELETE FROM library_items WHERE id = ?", id)
	if err != nil {
		return err
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("item not found")
	}
	return nil
}

// GetCategories returns all distinct categories
func (db *DB) GetCategories(ctx context.Context) ([]string, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT DISTINCT category FROM library_items WHERE category != '' ORDER BY category
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	if cats == nil {
		cats = []string{}
	}
	return cats, rows.Err()
}

// scanner is an interface for sql.Row and sql.Rows
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanItem(s scanner) (*Item, error) {
	var item Item
	var tags string
	var createdAt, updatedAt int64

	err := s.Scan(
		&item.ID, &item.Name, &item.Description, &item.Type, &item.Category,
		&item.Version, &tags, &item.Cluster, &item.Node, &item.Storage,
		&item.Volume, &item.Size, &item.Format, &item.VMID, &item.OSType,
		&item.Cores, &item.Memory, &createdAt, &updatedAt, &item.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	item.CreatedAt = time.Unix(createdAt, 0)
	item.UpdatedAt = time.Unix(updatedAt, 0)
	if tags != "" {
		item.Tags = strings.Split(tags, ",")
	} else {
		item.Tags = []string{}
	}

	return &item, nil
}

func scanItemRows(rows *sql.Rows) (*Item, error) {
	return scanItem(rows)
}
