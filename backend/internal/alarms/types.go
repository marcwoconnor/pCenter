package alarms

// Severity levels for alarms
type Severity string

const (
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// AlarmState represents the current state of an alarm instance
type AlarmState string

const (
	StateNormal   AlarmState = "normal"
	StateWarning  AlarmState = "warning"
	StateCritical AlarmState = "critical"
)

// Condition for threshold comparison
type Condition string

const (
	CondAbove Condition = "above"
	CondBelow Condition = "below"
)

// AlarmDefinition is a rule that defines when to fire an alarm
type AlarmDefinition struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Enabled           bool      `json:"enabled"`
	MetricType        string    `json:"metric_type"`        // cpu, mem_percent, disk_percent
	ResourceType      string    `json:"resource_type"`      // node, vm, ct, storage
	Scope             string    `json:"scope"`              // global, cluster, resource
	ScopeTarget       string    `json:"scope_target"`       // cluster name or resourceType:resourceID
	Condition         Condition `json:"condition"`          // above, below
	WarningThreshold  float64   `json:"warning_threshold"`  // e.g. 90.0 (percent)
	CriticalThreshold float64   `json:"critical_threshold"` // e.g. 95.0
	ClearThreshold    float64   `json:"clear_threshold"`    // e.g. 80.0 (hysteresis)
	DurationSamples   int       `json:"duration_samples"`   // consecutive samples needed (3 = 90s)
	NotifyChannels    []string  `json:"notify_channels"`    // channel IDs
	CreatedAt         int64     `json:"created_at"`
}

// AlarmInstance is the current state of an alarm for a specific resource
type AlarmInstance struct {
	ID               string     `json:"id"`
	DefinitionID     string     `json:"definition_id"`
	DefinitionName   string     `json:"definition_name"`
	Cluster          string     `json:"cluster"`
	ResourceType     string     `json:"resource_type"`
	ResourceID       string     `json:"resource_id"`
	ResourceName     string     `json:"resource_name"`
	State            AlarmState `json:"state"`
	PreviousState    AlarmState `json:"previous_state,omitempty"`
	CurrentValue     float64    `json:"current_value"`
	Threshold        float64    `json:"threshold"`
	TriggeredAt      int64      `json:"triggered_at,omitempty"`
	LastEvaluatedAt  int64      `json:"last_evaluated_at"`
	AcknowledgedBy   string     `json:"acknowledged_by,omitempty"`
	AcknowledgedAt   int64      `json:"acknowledged_at,omitempty"`
	ConsecutiveCount int        `json:"consecutive_count"` // samples in violation
}

// NotificationChannel is a destination for alarm notifications
type NotificationChannel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`    // webhook
	Config  string `json:"config"`  // JSON: {"url":"...","headers":{}}
	Enabled bool   `json:"enabled"`
}

// WebhookConfig is the parsed config for a webhook channel
type WebhookConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// CreateDefinitionRequest for POST /api/alarms/definitions
type CreateDefinitionRequest struct {
	Name              string   `json:"name"`
	MetricType        string   `json:"metric_type"`
	ResourceType      string   `json:"resource_type"`
	Scope             string   `json:"scope"`
	ScopeTarget       string   `json:"scope_target,omitempty"`
	Condition         string   `json:"condition"`
	WarningThreshold  float64  `json:"warning_threshold"`
	CriticalThreshold float64  `json:"critical_threshold"`
	ClearThreshold    float64  `json:"clear_threshold"`
	DurationSamples   int      `json:"duration_samples"`
	NotifyChannels    []string `json:"notify_channels,omitempty"`
}

// CreateChannelRequest for POST /api/alarms/channels
type CreateChannelRequest struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Config string `json:"config"` // JSON string
}

// AcknowledgeRequest for POST /api/alarms/{id}/acknowledge
type AcknowledgeRequest struct {
	User string `json:"user"`
}

// Valid metric types for alarms
var ValidMetricTypes = map[string]bool{
	"cpu":             true,
	"mem_percent":     true,
	"disk_percent":    true,
	"netin":           true,
	"netout":          true,
	"cert_days_left":  true, // Min days-until-expiry across node certs (polled every 5 min)
}

// Valid resource types for alarms
var ValidResourceTypes = map[string]bool{
	"node":    true,
	"vm":      true,
	"ct":      true,
	"storage": true,
}
