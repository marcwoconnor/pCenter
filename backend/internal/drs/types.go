package drs

// RuleType defines the kind of affinity rule
type RuleType string

const (
	RuleAffinity     RuleType = "affinity"      // Keep VMs together on same host
	RuleAntiAffinity RuleType = "anti-affinity"  // Keep VMs apart on different hosts
	RuleHostPin      RuleType = "host-pin"       // Pin VM to specific host
)

// Rule defines an affinity/anti-affinity constraint
type Rule struct {
	ID       string   `json:"id"`
	Cluster  string   `json:"cluster"`
	Name     string   `json:"name"`
	Type     RuleType `json:"type"`
	Enabled  bool     `json:"enabled"`
	Members  []int    `json:"members"`    // VMIDs (for affinity/anti-affinity)
	HostNode string   `json:"host_node"`  // target host (for host-pin only)
}

// RuleViolation describes a current rule violation
type RuleViolation struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	RuleType string `json:"rule_type"`
	Cluster  string `json:"cluster"`
	Message  string `json:"message"`
}

// CreateRuleRequest for POST /api/clusters/{cluster}/drs/rules
type CreateRuleRequest struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Members  []int    `json:"members"`
	HostNode string   `json:"host_node,omitempty"`
}

// Valid rule types
var ValidRuleTypes = map[string]bool{
	"affinity":      true,
	"anti-affinity": true,
	"host-pin":      true,
}
