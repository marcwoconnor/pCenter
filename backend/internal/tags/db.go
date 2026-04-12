package tags

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite connection for tag operations
type DB struct {
	conn *sql.DB
	mu   sync.Mutex

	stmtInsertTag       *sql.Stmt
	stmtUpdateTag       *sql.Stmt
	stmtDeleteTag       *sql.Stmt
	stmtListTags        *sql.Stmt
	stmtGetTag          *sql.Stmt
	stmtInsertAssign    *sql.Stmt
	stmtDeleteAssign    *sql.Stmt
	stmtGetObjectTags   *sql.Stmt
	stmtGetAllAssign    *sql.Stmt
	stmtDeleteTagAssign *sql.Stmt
}

// Open creates or opens the tags database
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
	if err := db.prepareStatements(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("prepare statements: %w", err)
	}

	slog.Info("tags database opened", "path", dbPath)
	return db, nil
}

// Close closes all prepared statements and connection
func (db *DB) Close() error {
	for _, s := range []*sql.Stmt{
		db.stmtInsertTag, db.stmtUpdateTag, db.stmtDeleteTag,
		db.stmtListTags, db.stmtGetTag,
		db.stmtInsertAssign, db.stmtDeleteAssign,
		db.stmtGetObjectTags, db.stmtGetAllAssign, db.stmtDeleteTagAssign,
	} {
		if s != nil {
			s.Close()
		}
	}
	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tags (
		id TEXT PRIMARY KEY,
		category TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		color TEXT NOT NULL DEFAULT '#3b82f6',
		created_at INTEGER NOT NULL,
		UNIQUE(category, name)
	);

	CREATE TABLE IF NOT EXISTS tag_assignments (
		tag_id TEXT NOT NULL,
		object_type TEXT NOT NULL,
		object_id TEXT NOT NULL,
		cluster TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (tag_id, object_type, object_id, cluster),
		FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tag_assignments_object
		ON tag_assignments(object_type, object_id, cluster);
	`
	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) prepareStatements() error {
	var err error

	db.stmtInsertTag, err = db.conn.Prepare(
		"INSERT INTO tags (id, category, name, color, created_at) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}

	db.stmtUpdateTag, err = db.conn.Prepare(
		"UPDATE tags SET category = COALESCE(NULLIF(?, ''), category), name = COALESCE(NULLIF(?, ''), name), color = COALESCE(NULLIF(?, ''), color) WHERE id = ?")
	if err != nil {
		return err
	}

	db.stmtDeleteTag, err = db.conn.Prepare("DELETE FROM tags WHERE id = ?")
	if err != nil {
		return err
	}

	db.stmtListTags, err = db.conn.Prepare(
		"SELECT id, category, name, color, created_at FROM tags ORDER BY category, name")
	if err != nil {
		return err
	}

	db.stmtGetTag, err = db.conn.Prepare(
		"SELECT id, category, name, color, created_at FROM tags WHERE id = ?")
	if err != nil {
		return err
	}

	db.stmtInsertAssign, err = db.conn.Prepare(
		"INSERT OR IGNORE INTO tag_assignments (tag_id, object_type, object_id, cluster) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}

	db.stmtDeleteAssign, err = db.conn.Prepare(
		"DELETE FROM tag_assignments WHERE tag_id = ? AND object_type = ? AND object_id = ? AND cluster = ?")
	if err != nil {
		return err
	}

	db.stmtGetObjectTags, err = db.conn.Prepare(`
		SELECT t.id, t.category, t.name, t.color, t.created_at
		FROM tags t
		JOIN tag_assignments ta ON t.id = ta.tag_id
		WHERE ta.object_type = ? AND ta.object_id = ? AND ta.cluster = ?
		ORDER BY t.category, t.name`)
	if err != nil {
		return err
	}

	db.stmtGetAllAssign, err = db.conn.Prepare(
		"SELECT tag_id, object_type, object_id, cluster FROM tag_assignments")
	if err != nil {
		return err
	}

	db.stmtDeleteTagAssign, err = db.conn.Prepare(
		"DELETE FROM tag_assignments WHERE tag_id = ?")
	if err != nil {
		return err
	}

	return nil
}

// ── Tag CRUD ────────────────────────────────────────────────────────────────

func (db *DB) CreateTag(ctx context.Context, category, name, color string) (*Tag, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	id := uuid.New().String()
	now := time.Now()
	_, err := db.stmtInsertTag.ExecContext(ctx, id, category, name, color, now.Unix())
	if err != nil {
		return nil, fmt.Errorf("insert tag: %w", err)
	}
	return &Tag{ID: id, Category: category, Name: name, Color: color, CreatedAt: now}, nil
}

func (db *DB) UpdateTag(ctx context.Context, id, category, name, color string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	result, err := db.stmtUpdateTag.ExecContext(ctx, category, name, color, id)
	if err != nil {
		return fmt.Errorf("update tag: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tag not found")
	}
	return nil
}

func (db *DB) DeleteTag(ctx context.Context, id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// CASCADE deletes assignments
	_, err := db.stmtDeleteTag.ExecContext(ctx, id)
	return err
}

func (db *DB) GetTag(ctx context.Context, id string) (*Tag, error) {
	row := db.stmtGetTag.QueryRowContext(ctx, id)
	return scanTag(row)
}

func (db *DB) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := db.stmtListTags.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTags(rows)
}

// ── Assignments ─────────────────────────────────────────────────────────────

func (db *DB) AssignTag(ctx context.Context, tagID, objectType, objectID, cluster string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.stmtInsertAssign.ExecContext(ctx, tagID, objectType, objectID, cluster)
	return err
}

func (db *DB) UnassignTag(ctx context.Context, tagID, objectType, objectID, cluster string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.stmtDeleteAssign.ExecContext(ctx, tagID, objectType, objectID, cluster)
	return err
}

func (db *DB) GetObjectTags(ctx context.Context, objectType, objectID, cluster string) ([]Tag, error) {
	rows, err := db.stmtGetObjectTags.QueryContext(ctx, objectType, objectID, cluster)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTags(rows)
}

func (db *DB) GetAllAssignments(ctx context.Context) ([]TagAssignment, error) {
	rows, err := db.stmtGetAllAssign.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []TagAssignment
	for rows.Next() {
		var a TagAssignment
		if err := rows.Scan(&a.TagID, &a.ObjectType, &a.ObjectID, &a.Cluster); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

func (db *DB) BulkAssign(ctx context.Context, tagIDs []string, objects []ObjectRef) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO tag_assignments (tag_id, object_type, object_id, cluster) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, tagID := range tagIDs {
		for _, obj := range objects {
			if _, err := stmt.ExecContext(ctx, tagID, obj.ObjectType, obj.ObjectID, obj.Cluster); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func scanTag(row *sql.Row) (*Tag, error) {
	var t Tag
	var createdAt int64
	if err := row.Scan(&t.ID, &t.Category, &t.Name, &t.Color, &createdAt); err != nil {
		return nil, err
	}
	t.CreatedAt = time.Unix(createdAt, 0)
	return &t, nil
}

func scanTags(rows *sql.Rows) ([]Tag, error) {
	var tags []Tag
	for rows.Next() {
		var t Tag
		var createdAt int64
		if err := rows.Scan(&t.ID, &t.Category, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(createdAt, 0)
		tags = append(tags, t)
	}
	return tags, nil
}
