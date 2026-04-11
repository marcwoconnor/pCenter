package folders

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

// DB wraps the SQLite connection for folder operations
type DB struct {
	conn *sql.DB
	mu   sync.Mutex

	// Prepared statements
	stmtInsertFolder       *sql.Stmt
	stmtUpdateFolder       *sql.Stmt
	stmtDeleteFolder       *sql.Stmt
	stmtGetFolder          *sql.Stmt
	stmtGetFoldersByTree   *sql.Stmt
	stmtInsertMember       *sql.Stmt
	stmtDeleteMember       *sql.Stmt
	stmtGetMembersByFolder *sql.Stmt
	stmtGetResourceFolder  *sql.Stmt
	stmtDeleteResourceMemberships *sql.Stmt
}

// Open creates or opens the folders database
func Open(dbPath string) (*DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=1000&_foreign_keys=ON", dbPath)
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Single writer for SQLite
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

	slog.Info("folders database opened", "path", dbPath)
	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Close prepared statements
	if db.stmtInsertFolder != nil {
		db.stmtInsertFolder.Close()
	}
	if db.stmtUpdateFolder != nil {
		db.stmtUpdateFolder.Close()
	}
	if db.stmtDeleteFolder != nil {
		db.stmtDeleteFolder.Close()
	}
	if db.stmtGetFolder != nil {
		db.stmtGetFolder.Close()
	}
	if db.stmtGetFoldersByTree != nil {
		db.stmtGetFoldersByTree.Close()
	}
	if db.stmtInsertMember != nil {
		db.stmtInsertMember.Close()
	}
	if db.stmtDeleteMember != nil {
		db.stmtDeleteMember.Close()
	}
	if db.stmtGetMembersByFolder != nil {
		db.stmtGetMembersByFolder.Close()
	}
	if db.stmtGetResourceFolder != nil {
		db.stmtGetResourceFolder.Close()
	}
	if db.stmtDeleteResourceMemberships != nil {
		db.stmtDeleteResourceMemberships.Close()
	}

	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS folders (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		parent_id TEXT,
		tree_view TEXT NOT NULL,
		cluster TEXT,
		sort_order INTEGER DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		FOREIGN KEY (parent_id) REFERENCES folders(id) ON DELETE CASCADE,
		UNIQUE(parent_id, name, tree_view)
	);

	CREATE TABLE IF NOT EXISTS folder_members (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		folder_id TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		cluster TEXT NOT NULL,
		added_at INTEGER NOT NULL,
		FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE,
		UNIQUE(folder_id, resource_type, resource_id, cluster)
	);

	CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_id);
	CREATE INDEX IF NOT EXISTS idx_folders_tree ON folders(tree_view);
	CREATE INDEX IF NOT EXISTS idx_members_folder ON folder_members(folder_id);
	CREATE INDEX IF NOT EXISTS idx_members_resource ON folder_members(resource_type, resource_id, cluster);
	`

	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) prepareStatements() error {
	var err error

	db.stmtInsertFolder, err = db.conn.Prepare(`
		INSERT INTO folders (id, name, parent_id, tree_view, cluster, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert folder: %w", err)
	}

	db.stmtUpdateFolder, err = db.conn.Prepare(`
		UPDATE folders SET name = ?, parent_id = ?, sort_order = ?, updated_at = ? WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare update folder: %w", err)
	}

	db.stmtDeleteFolder, err = db.conn.Prepare(`DELETE FROM folders WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare delete folder: %w", err)
	}

	db.stmtGetFolder, err = db.conn.Prepare(`
		SELECT id, name, parent_id, tree_view, cluster, sort_order, created_at, updated_at
		FROM folders WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get folder: %w", err)
	}

	db.stmtGetFoldersByTree, err = db.conn.Prepare(`
		SELECT id, name, parent_id, tree_view, cluster, sort_order, created_at, updated_at
		FROM folders WHERE tree_view = ? ORDER BY sort_order, name
	`)
	if err != nil {
		return fmt.Errorf("prepare get folders by tree: %w", err)
	}

	db.stmtInsertMember, err = db.conn.Prepare(`
		INSERT OR REPLACE INTO folder_members (folder_id, resource_type, resource_id, cluster, added_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert member: %w", err)
	}

	db.stmtDeleteMember, err = db.conn.Prepare(`
		DELETE FROM folder_members WHERE folder_id = ? AND resource_type = ? AND resource_id = ? AND cluster = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare delete member: %w", err)
	}

	db.stmtGetMembersByFolder, err = db.conn.Prepare(`
		SELECT folder_id, resource_type, resource_id, cluster, added_at
		FROM folder_members WHERE folder_id = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get members by folder: %w", err)
	}

	db.stmtGetResourceFolder, err = db.conn.Prepare(`
		SELECT fm.folder_id FROM folder_members fm
		JOIN folders f ON fm.folder_id = f.id
		WHERE fm.resource_type = ? AND fm.resource_id = ? AND fm.cluster = ? AND f.tree_view = ?
	`)
	if err != nil {
		return fmt.Errorf("prepare get resource folder: %w", err)
	}

	db.stmtDeleteResourceMemberships, err = db.conn.Prepare(`
		DELETE FROM folder_members WHERE resource_type = ? AND resource_id = ? AND cluster = ?
		AND folder_id IN (SELECT id FROM folders WHERE tree_view = ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare delete resource memberships: %w", err)
	}

	return nil
}

// CreateFolder creates a new folder
func (db *DB) CreateFolder(ctx context.Context, req CreateFolderRequest) (*Folder, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now()
	folder := &Folder{
		ID:        uuid.New().String(),
		Name:      req.Name,
		ParentID:  req.ParentID,
		TreeView:  req.TreeView,
		Cluster:   req.Cluster,
		SortOrder: 0,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := db.stmtInsertFolder.ExecContext(ctx,
		folder.ID, folder.Name, folder.ParentID, folder.TreeView,
		folder.Cluster, folder.SortOrder,
		folder.CreatedAt.Unix(), folder.UpdatedAt.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert folder: %w", err)
	}

	return folder, nil
}

// GetFolder retrieves a folder by ID
func (db *DB) GetFolder(ctx context.Context, id string) (*Folder, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var f Folder
	var parentID, cluster sql.NullString
	var createdAt, updatedAt int64

	err := db.stmtGetFolder.QueryRowContext(ctx, id).Scan(
		&f.ID, &f.Name, &parentID, &f.TreeView, &cluster,
		&f.SortOrder, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get folder: %w", err)
	}

	if parentID.Valid {
		f.ParentID = &parentID.String
	}
	if cluster.Valid {
		f.Cluster = &cluster.String
	}
	f.CreatedAt = time.Unix(createdAt, 0)
	f.UpdatedAt = time.Unix(updatedAt, 0)

	return &f, nil
}

// GetFolderTree retrieves all folders for a tree view
func (db *DB) GetFolderTree(ctx context.Context, treeView TreeView) ([]Folder, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	rows, err := db.stmtGetFoldersByTree.QueryContext(ctx, treeView)
	if err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		var f Folder
		var parentID, cluster sql.NullString
		var createdAt, updatedAt int64

		if err := rows.Scan(
			&f.ID, &f.Name, &parentID, &f.TreeView, &cluster,
			&f.SortOrder, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}

		if parentID.Valid {
			f.ParentID = &parentID.String
		}
		if cluster.Valid {
			f.Cluster = &cluster.String
		}
		f.CreatedAt = time.Unix(createdAt, 0)
		f.UpdatedAt = time.Unix(updatedAt, 0)

		folders = append(folders, f)
	}

	return folders, rows.Err()
}

// RenameFolder renames a folder
func (db *DB) RenameFolder(ctx context.Context, id, newName string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Get current folder to preserve other fields
	var parentID sql.NullString
	var sortOrder int
	err := db.conn.QueryRowContext(ctx, "SELECT parent_id, sort_order FROM folders WHERE id = ?", id).
		Scan(&parentID, &sortOrder)
	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}

	_, err = db.stmtUpdateFolder.ExecContext(ctx, newName, parentID, sortOrder, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("update folder: %w", err)
	}

	return nil
}

// MoveFolder moves a folder to a new parent
func (db *DB) MoveFolder(ctx context.Context, id string, newParentID *string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Get current folder
	var name string
	var sortOrder int
	err := db.conn.QueryRowContext(ctx, "SELECT name, sort_order FROM folders WHERE id = ?", id).
		Scan(&name, &sortOrder)
	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}

	_, err = db.stmtUpdateFolder.ExecContext(ctx, name, newParentID, sortOrder, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("update folder: %w", err)
	}

	return nil
}

// DeleteFolder deletes a folder (CASCADE will remove children and memberships)
func (db *DB) DeleteFolder(ctx context.Context, id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.stmtDeleteFolder.ExecContext(ctx, id)
	if err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}

	return nil
}

// AddMember adds a resource to a folder
func (db *DB) AddMember(ctx context.Context, folderID string, member AddMemberRequest) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.stmtInsertMember.ExecContext(ctx,
		folderID, member.ResourceType, member.ResourceID, member.Cluster,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert member: %w", err)
	}

	return nil
}

// RemoveMember removes a resource from a folder
func (db *DB) RemoveMember(ctx context.Context, folderID string, req RemoveMemberRequest) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.stmtDeleteMember.ExecContext(ctx,
		folderID, req.ResourceType, req.ResourceID, req.Cluster,
	)
	if err != nil {
		return fmt.Errorf("delete member: %w", err)
	}

	return nil
}

// GetMembersByFolder gets all members of a folder
func (db *DB) GetMembersByFolder(ctx context.Context, folderID string) ([]FolderMember, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	rows, err := db.stmtGetMembersByFolder.QueryContext(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()

	var members []FolderMember
	for rows.Next() {
		var m FolderMember
		var addedAt int64

		if err := rows.Scan(&m.FolderID, &m.ResourceType, &m.ResourceID, &m.Cluster, &addedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}

		m.AddedAt = time.Unix(addedAt, 0)
		members = append(members, m)
	}

	return members, rows.Err()
}

// GetResourceFolder gets the folder a resource belongs to in a specific tree
func (db *DB) GetResourceFolder(ctx context.Context, treeView TreeView, resourceType, resourceID, cluster string) (*string, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var folderID string
	err := db.stmtGetResourceFolder.QueryRowContext(ctx, resourceType, resourceID, cluster, treeView).
		Scan(&folderID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get resource folder: %w", err)
	}

	return &folderID, nil
}

// MoveResource moves a resource to a folder (or to root if toFolderID is nil)
func (db *DB) MoveResource(ctx context.Context, treeView TreeView, resourceType, resourceID, cluster string, toFolderID *string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Remove from current folder(s) in this tree
	_, err := db.stmtDeleteResourceMemberships.ExecContext(ctx, resourceType, resourceID, cluster, treeView)
	if err != nil {
		return fmt.Errorf("delete existing memberships: %w", err)
	}

	// Add to new folder if specified
	if toFolderID != nil {
		_, err = db.stmtInsertMember.ExecContext(ctx,
			*toFolderID, resourceType, resourceID, cluster,
			time.Now().Unix(),
		)
		if err != nil {
			return fmt.Errorf("insert member: %w", err)
		}
	}

	return nil
}

// GetAllMembers gets all folder memberships for a tree view
func (db *DB) GetAllMembers(ctx context.Context, treeView TreeView) (map[string][]FolderMember, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	rows, err := db.conn.QueryContext(ctx, `
		SELECT fm.folder_id, fm.resource_type, fm.resource_id, fm.cluster, fm.added_at
		FROM folder_members fm
		JOIN folders f ON fm.folder_id = f.id
		WHERE f.tree_view = ?
	`, treeView)
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]FolderMember)
	for rows.Next() {
		var m FolderMember
		var addedAt int64

		if err := rows.Scan(&m.FolderID, &m.ResourceType, &m.ResourceID, &m.Cluster, &addedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}

		m.AddedAt = time.Unix(addedAt, 0)
		result[m.FolderID] = append(result[m.FolderID], m)
	}

	return result, rows.Err()
}
