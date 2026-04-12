package rbac

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// DB handles RBAC persistence
type DB struct {
	db *sql.DB
}

// OpenDB opens or creates the RBAC database
func OpenDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open rbac db: %w", err)
	}

	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("init rbac schema: %w", err)
	}

	d := &DB{db: db}
	if err := d.seedBuiltInRoles(); err != nil {
		return nil, fmt.Errorf("seed built-in roles: %w", err)
	}

	return d, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS roles (
		id TEXT PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		description TEXT DEFAULT '',
		builtin INTEGER DEFAULT 0,
		permissions TEXT NOT NULL DEFAULT '[]',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS role_assignments (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		role_id TEXT NOT NULL,
		object_type TEXT NOT NULL,
		object_id TEXT NOT NULL DEFAULT '',
		propagate INTEGER DEFAULT 1,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
		UNIQUE(user_id, role_id, object_type, object_id)
	);

	CREATE INDEX IF NOT EXISTS idx_assignments_user ON role_assignments(user_id);
	CREATE INDEX IF NOT EXISTS idx_assignments_object ON role_assignments(object_type, object_id);
	`
	_, err := db.Exec(schema)
	return err
}

func (d *DB) seedBuiltInRoles() error {
	builtins := []Role{
		{
			ID:          RoleIDAdmin,
			Name:        "Administrator",
			Description: "Full access to all resources and settings",
			BuiltIn:     true,
			Permissions: []string{PermAll},
		},
		{
			ID:          RoleIDOperator,
			Name:        "Operator",
			Description: "Manage VMs, containers, hosts, and storage. No user management or inventory changes",
			BuiltIn:     true,
			Permissions: []string{
				PermView,
				"vm.*", "ct.*",
				PermHostConfig, PermHostMaintenance,
				PermStorageView, PermStorageUpload,
				PermClusterConfig,
				PermAlarmsManage, PermTagsManage, PermFoldersManage,
			},
		},
		{
			ID:          RoleIDVMAdmin,
			Name:        "VM Administrator",
			Description: "Full VM and container lifecycle. Read-only for hosts and storage",
			BuiltIn:     true,
			Permissions: []string{
				PermView,
				"vm.*", "ct.*",
				PermStorageView,
			},
		},
		{
			ID:          RoleIDReadOnly,
			Name:        "Read Only",
			Description: "View all resources. No modifications",
			BuiltIn:     true,
			Permissions: []string{PermView, PermStorageView},
		},
	}

	for _, role := range builtins {
		permsJSON, _ := json.Marshal(role.Permissions)
		now := time.Now().Unix()
		_, err := d.db.Exec(`
			INSERT INTO roles (id, name, description, builtin, permissions, created_at, updated_at)
			VALUES (?, ?, ?, 1, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				permissions = excluded.permissions,
				updated_at = excluded.updated_at
		`, role.ID, role.Name, role.Description, string(permsJSON), now, now)
		if err != nil {
			return fmt.Errorf("seed role %s: %w", role.ID, err)
		}
	}
	return nil
}

// --- Role CRUD ---

// ListRoles returns all roles
func (d *DB) ListRoles() ([]Role, error) {
	rows, err := d.db.Query(`SELECT id, name, description, builtin, permissions, created_at, updated_at FROM roles ORDER BY builtin DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var r Role
		var permsJSON string
		var builtin int
		var createdAt, updatedAt int64
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &builtin, &permsJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		r.BuiltIn = builtin == 1
		json.Unmarshal([]byte(permsJSON), &r.Permissions)
		r.CreatedAt = time.Unix(createdAt, 0)
		r.UpdatedAt = time.Unix(updatedAt, 0)
		roles = append(roles, r)
	}
	return roles, nil
}

// GetRole returns a single role by ID
func (d *DB) GetRole(id string) (*Role, error) {
	var r Role
	var permsJSON string
	var builtin int
	var createdAt, updatedAt int64
	err := d.db.QueryRow(`SELECT id, name, description, builtin, permissions, created_at, updated_at FROM roles WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.Description, &builtin, &permsJSON, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.BuiltIn = builtin == 1
	json.Unmarshal([]byte(permsJSON), &r.Permissions)
	r.CreatedAt = time.Unix(createdAt, 0)
	r.UpdatedAt = time.Unix(updatedAt, 0)
	return &r, nil
}

// CreateRole creates a custom role
func (d *DB) CreateRole(req CreateRoleRequest) (*Role, error) {
	permsJSON, _ := json.Marshal(req.Permissions)
	now := time.Now()
	id := "custom:" + uuid.New().String()[:8]

	_, err := d.db.Exec(`INSERT INTO roles (id, name, description, builtin, permissions, created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, ?, ?)`,
		id, req.Name, req.Description, string(permsJSON), now.Unix(), now.Unix())
	if err != nil {
		return nil, err
	}

	return &Role{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// UpdateRole updates a custom role (built-in roles cannot be modified)
func (d *DB) UpdateRole(id string, req UpdateRoleRequest) error {
	// Check not built-in
	var builtin int
	err := d.db.QueryRow(`SELECT builtin FROM roles WHERE id = ?`, id).Scan(&builtin)
	if err != nil {
		return fmt.Errorf("role not found")
	}
	if builtin == 1 {
		return fmt.Errorf("cannot modify built-in role")
	}

	permsJSON, _ := json.Marshal(req.Permissions)
	_, err = d.db.Exec(`UPDATE roles SET name = ?, description = ?, permissions = ?, updated_at = ? WHERE id = ?`,
		req.Name, req.Description, string(permsJSON), time.Now().Unix(), id)
	return err
}

// DeleteRole deletes a custom role and all its assignments
func (d *DB) DeleteRole(id string) error {
	var builtin int
	err := d.db.QueryRow(`SELECT builtin FROM roles WHERE id = ?`, id).Scan(&builtin)
	if err != nil {
		return fmt.Errorf("role not found")
	}
	if builtin == 1 {
		return fmt.Errorf("cannot delete built-in role")
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec(`DELETE FROM role_assignments WHERE role_id = ?`, id)
	tx.Exec(`DELETE FROM roles WHERE id = ?`, id)
	return tx.Commit()
}

// --- Role Assignment CRUD ---

// ListAssignments returns all role assignments, optionally filtered
func (d *DB) ListAssignments(userID, objectType, objectID string) ([]RoleAssignment, error) {
	query := `SELECT ra.id, ra.user_id, ra.role_id, ra.object_type, ra.object_id, ra.propagate, ra.created_at,
		r.name as role_name
		FROM role_assignments ra
		JOIN roles r ON r.id = ra.role_id
		WHERE 1=1`
	var args []interface{}

	if userID != "" {
		query += ` AND ra.user_id = ?`
		args = append(args, userID)
	}
	if objectType != "" {
		query += ` AND ra.object_type = ?`
		args = append(args, objectType)
	}
	if objectID != "" {
		query += ` AND ra.object_id = ?`
		args = append(args, objectID)
	}

	query += ` ORDER BY ra.created_at`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []RoleAssignment
	for rows.Next() {
		var a RoleAssignment
		var propagate int
		var createdAt int64
		if err := rows.Scan(&a.ID, &a.UserID, &a.RoleID, &a.ObjectType, &a.ObjectID, &propagate, &createdAt, &a.RoleName); err != nil {
			return nil, err
		}
		a.Propagate = propagate == 1
		a.CreatedAt = time.Unix(createdAt, 0)
		assignments = append(assignments, a)
	}
	return assignments, nil
}

// CreateAssignment creates a role assignment
func (d *DB) CreateAssignment(req AssignRoleRequest) (*RoleAssignment, error) {
	// Verify role exists
	var exists int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM roles WHERE id = ?`, req.RoleID).Scan(&exists); err != nil || exists == 0 {
		return nil, fmt.Errorf("role not found")
	}

	id := uuid.New().String()[:12]
	now := time.Now()
	propagate := 0
	if req.Propagate {
		propagate = 1
	}

	_, err := d.db.Exec(`INSERT INTO role_assignments (id, user_id, role_id, object_type, object_id, propagate, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, req.UserID, req.RoleID, req.ObjectType, req.ObjectID, propagate, now.Unix())
	if err != nil {
		return nil, err
	}

	return &RoleAssignment{
		ID:         id,
		UserID:     req.UserID,
		RoleID:     req.RoleID,
		ObjectType: req.ObjectType,
		ObjectID:   req.ObjectID,
		Propagate:  req.Propagate,
		CreatedAt:  now,
	}, nil
}

// DeleteAssignment removes a role assignment
func (d *DB) DeleteAssignment(id string) error {
	result, err := d.db.Exec(`DELETE FROM role_assignments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("assignment not found")
	}
	return nil
}

// GetUserAssignments returns all role assignments for a user with full role data
func (d *DB) GetUserAssignments(userID string) ([]RoleAssignment, []Role, error) {
	assignments, err := d.ListAssignments(userID, "", "")
	if err != nil {
		return nil, nil, err
	}

	// Collect unique role IDs and fetch roles
	roleMap := make(map[string]bool)
	for _, a := range assignments {
		roleMap[a.RoleID] = true
	}

	var roles []Role
	for roleID := range roleMap {
		role, err := d.GetRole(roleID)
		if err != nil || role == nil {
			continue
		}
		roles = append(roles, *role)
	}

	return assignments, roles, nil
}

// Close closes the database
func (d *DB) Close() error {
	return d.db.Close()
}
