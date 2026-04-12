package rbac

import "time"

// --- Permission constants ---

const (
	PermView    = "view"
	PermAll     = "*"

	// VM permissions
	PermVMPower    = "vm.power"
	PermVMConsole  = "vm.console"
	PermVMConfig   = "vm.config"
	PermVMSnapshot = "vm.snapshot"
	PermVMCreate   = "vm.create"
	PermVMDelete   = "vm.delete"
	PermVMMigrate  = "vm.migrate"
	PermVMClone    = "vm.clone"

	// Container permissions
	PermCTPower    = "ct.power"
	PermCTConsole  = "ct.console"
	PermCTConfig   = "ct.config"
	PermCTSnapshot = "ct.snapshot"
	PermCTCreate   = "ct.create"
	PermCTDelete   = "ct.delete"
	PermCTMigrate  = "ct.migrate"
	PermCTClone    = "ct.clone"

	// Host permissions
	PermHostConfig      = "host.config"
	PermHostMaintenance = "host.maintenance"

	// Storage permissions
	PermStorageView   = "storage.view"
	PermStorageUpload = "storage.upload"

	// Cluster/infrastructure permissions
	PermClusterConfig  = "cluster.config"
	PermInventoryManage = "inventory.manage"

	// Management permissions
	PermAlarmsManage  = "alarms.manage"
	PermTagsManage    = "tags.manage"
	PermFoldersManage = "folders.manage"
	PermUsersManage   = "users.manage"
)

// AllPermissions lists every defined permission (for documentation/UI)
var AllPermissions = []string{
	PermView,
	PermVMPower, PermVMConsole, PermVMConfig, PermVMSnapshot,
	PermVMCreate, PermVMDelete, PermVMMigrate, PermVMClone,
	PermCTPower, PermCTConsole, PermCTConfig, PermCTSnapshot,
	PermCTCreate, PermCTDelete, PermCTMigrate, PermCTClone,
	PermHostConfig, PermHostMaintenance,
	PermStorageView, PermStorageUpload,
	PermClusterConfig, PermInventoryManage,
	PermAlarmsManage, PermTagsManage, PermFoldersManage, PermUsersManage,
}

// --- Object types for scoping ---

const (
	ObjectRoot       = "root"
	ObjectDatacenter = "datacenter"
	ObjectCluster    = "cluster"
	ObjectNode       = "node"
	ObjectVM         = "vm"
	ObjectCT         = "ct"
	ObjectStorage    = "storage"
	ObjectFolder     = "folder"
)

// --- Built-in role IDs ---

const (
	RoleIDAdmin    = "builtin:admin"
	RoleIDOperator = "builtin:operator"
	RoleIDVMAdmin  = "builtin:vm-admin"
	RoleIDReadOnly = "builtin:read-only"
)

// --- Types ---

// Role defines a named set of permissions
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	BuiltIn     bool     `json:"builtin"`
	Permissions []string `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RoleAssignment binds a user+role to a specific object in the hierarchy
type RoleAssignment struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Username   string    `json:"username,omitempty"` // populated on read
	RoleID     string    `json:"role_id"`
	RoleName   string    `json:"role_name,omitempty"` // populated on read
	ObjectType string    `json:"object_type"` // root, datacenter, cluster, node, vm, ct
	ObjectID   string    `json:"object_id"`   // "" for root
	Propagate  bool      `json:"propagate"`   // inherit to children
	CreatedAt  time.Time `json:"created_at"`
}

// ObjectRef identifies an object in the hierarchy
type ObjectRef struct {
	Type string // ObjectRoot, ObjectDatacenter, etc.
	ID   string // object identifier
}

// --- Request types ---

// CreateRoleRequest for creating custom roles
type CreateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// UpdateRoleRequest for modifying custom roles
type UpdateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// AssignRoleRequest for creating role assignments
type AssignRoleRequest struct {
	UserID     string `json:"user_id"`
	RoleID     string `json:"role_id"`
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	Propagate  bool   `json:"propagate"`
}
