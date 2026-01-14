package activity

import "time"

// Entry represents a single activity log entry
type Entry struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	ResourceName string    `json:"resource_name,omitempty"`
	Cluster      string    `json:"cluster"`
	Details      string    `json:"details,omitempty"`
	Status       string    `json:"status"`
}

// Action constants
const (
	ActionConfigUpdate = "config_update"
	ActionVMStart      = "vm_start"
	ActionVMStop       = "vm_stop"
	ActionVMShutdown   = "vm_shutdown"
	ActionCTStart      = "ct_start"
	ActionCTStop       = "ct_stop"
	ActionCTShutdown   = "ct_shutdown"
	ActionMigrate      = "migrate"
	ActionHAEnable     = "ha_enable"
	ActionHADisable    = "ha_disable"
	ActionDRSApply     = "drs_apply"
	ActionDRSDismiss   = "drs_dismiss"
	ActionFolderCreate = "folder_create"
	ActionFolderRename = "folder_rename"
	ActionFolderDelete = "folder_delete"
	ActionFolderMove   = "folder_move"
	ActionResourceMove = "resource_move"
)

// Status constants
const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// QueryParams for filtering activity queries
type QueryParams struct {
	Limit        int
	Offset       int
	ResourceType string
	ResourceID   string
	Cluster      string
	Action       string
}
