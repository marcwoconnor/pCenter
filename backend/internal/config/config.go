package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration
type Config struct {
	Clusters []ClusterConfig `yaml:"clusters"`
	Poller   PollerConfig    `yaml:"poller"`
	DRS      DRSConfig       `yaml:"drs"`
	Server   ServerConfig    `yaml:"server"`
	Metrics  MetricsConfig   `yaml:"metrics"`
	Folders  FoldersConfig   `yaml:"folders"`
	Activity ActivityConfig  `yaml:"activity"`

	// ClusterSecrets maps cluster name to API token secret
	// Secrets stay in config/env, cluster definitions move to inventory DB
	ClusterSecrets map[string]string `yaml:"cluster_secrets"`

	// Inventory settings for datacenter/cluster management
	Inventory InventoryConfig `yaml:"inventory"`

	// Legacy: flat nodes array (auto-converted to single cluster)
	Nodes []NodeConfig `yaml:"nodes,omitempty"`
}

// ActivityConfig holds activity logging settings
type ActivityConfig struct {
	DatabasePath  string `yaml:"database_path"`
	RetentionDays int    `yaml:"retention_days"`
}

// PollerConfig holds poller settings
type PollerConfig struct {
	Enabled bool `yaml:"enabled"` // Default: true
}

// FoldersConfig holds folder organization settings
type FoldersConfig struct {
	DatabasePath string `yaml:"database_path"`
}

// InventoryConfig holds datacenter/cluster inventory settings
type InventoryConfig struct {
	DatabasePath string `yaml:"database_path"`
}

// MetricsConfig holds metrics collection settings
type MetricsConfig struct {
	Enabled            bool            `yaml:"enabled"`
	DatabasePath       string          `yaml:"database_path"`
	CollectionInterval int             `yaml:"collection_interval"` // seconds
	Retention          RetentionConfig `yaml:"retention"`
}

// RetentionConfig defines how long metrics are kept at each resolution
type RetentionConfig struct {
	RawHours     int `yaml:"raw_hours"`     // Default: 24
	HourlyDays   int `yaml:"hourly_days"`   // Default: 7
	DailyDays    int `yaml:"daily_days"`    // Default: 30
	WeeklyMonths int `yaml:"weekly_months"` // Default: 12
}

// ClusterConfig defines a Proxmox cluster to manage
type ClusterConfig struct {
	Name          string `yaml:"name"`
	DiscoveryNode string `yaml:"discovery_node"` // host:port of any node in cluster
	TokenID       string `yaml:"token_id"`
	TokenSecret   string `yaml:"token_secret"`
	Insecure      bool   `yaml:"insecure"`
}

// NodeConfig is legacy per-node config (for backward compatibility)
type NodeConfig struct {
	Name        string `yaml:"name"`
	Host        string `yaml:"host"`
	TokenID     string `yaml:"token_id"`
	TokenSecret string `yaml:"token_secret"`
	Insecure    bool   `yaml:"insecure"`
}

// DRSConfig holds DRS (Distributed Resource Scheduler) settings
type DRSConfig struct {
	Enabled       bool    `yaml:"enabled"`
	Mode          string  `yaml:"mode"` // manual, semi-automatic, fully-automatic
	CheckInterval int     `yaml:"check_interval"`
	CPUThreshold  float64 `yaml:"cpu_threshold"`
	MemThreshold  float64 `yaml:"mem_threshold"`
	MigrationRate int     `yaml:"migration_rate"` // max concurrent migrations per cluster
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Port        int      `yaml:"port"`
	CORSOrigins []string `yaml:"cors_origins"`
}

// Load reads config from a YAML file, expanding env vars
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Expand environment variables (${VAR} syntax)
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Convert legacy nodes format to clusters
	if len(cfg.Nodes) > 0 && len(cfg.Clusters) == 0 {
		slog.Warn("using legacy config format - please migrate to 'clusters' array")
		// Group all nodes into a single "default" cluster using first node for discovery
		cfg.Clusters = []ClusterConfig{{
			Name:          "default",
			DiscoveryNode: cfg.Nodes[0].Host,
			TokenID:       cfg.Nodes[0].TokenID,
			TokenSecret:   cfg.Nodes[0].TokenSecret,
			Insecure:      cfg.Nodes[0].Insecure || !strings.HasPrefix(cfg.Nodes[0].Host, "http"),
		}}
	}

	// Defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}

	// DRS defaults
	if cfg.DRS.CheckInterval == 0 {
		cfg.DRS.CheckInterval = 300 // 5 minutes
	}
	if cfg.DRS.CPUThreshold == 0 {
		cfg.DRS.CPUThreshold = 0.8
	}
	if cfg.DRS.MemThreshold == 0 {
		cfg.DRS.MemThreshold = 0.85
	}
	if cfg.DRS.MigrationRate == 0 {
		cfg.DRS.MigrationRate = 2
	}
	if cfg.DRS.Mode == "" {
		cfg.DRS.Mode = "manual"
	}

	// Folders defaults
	if cfg.Folders.DatabasePath == "" {
		cfg.Folders.DatabasePath = "data/folders.db"
	}

	// Inventory defaults
	if cfg.Inventory.DatabasePath == "" {
		cfg.Inventory.DatabasePath = "data/inventory.db"
	}

	// Activity defaults
	if cfg.Activity.DatabasePath == "" {
		cfg.Activity.DatabasePath = "data/activity.db"
	}
	if cfg.Activity.RetentionDays == 0 {
		cfg.Activity.RetentionDays = 30
	}

	// Metrics defaults
	if cfg.Metrics.DatabasePath == "" {
		cfg.Metrics.DatabasePath = "data/metrics.db"
	}
	if cfg.Metrics.CollectionInterval == 0 {
		cfg.Metrics.CollectionInterval = 30
	}
	if cfg.Metrics.Retention.RawHours == 0 {
		cfg.Metrics.Retention.RawHours = 24
	}
	if cfg.Metrics.Retention.HourlyDays == 0 {
		cfg.Metrics.Retention.HourlyDays = 7
	}
	if cfg.Metrics.Retention.DailyDays == 0 {
		cfg.Metrics.Retention.DailyDays = 30
	}
	if cfg.Metrics.Retention.WeeklyMonths == 0 {
		cfg.Metrics.Retention.WeeklyMonths = 12
	}

	// Poller defaults to enabled unless explicitly set to false
	// We use a pointer check approach - if not in config, default to true
	// For now, if clusters are configured, poller is enabled by default

	// Initialize ClusterSecrets map if nil
	if cfg.ClusterSecrets == nil {
		cfg.ClusterSecrets = make(map[string]string)
	}

	// Validate legacy clusters array and populate ClusterSecrets for backward compatibility
	// Clusters can also be defined in inventory DB, so empty clusters array is OK
	for i, cluster := range cfg.Clusters {
		if cluster.Name == "" {
			return nil, fmt.Errorf("cluster %d: name is required", i)
		}
		if cluster.DiscoveryNode == "" {
			return nil, fmt.Errorf("cluster %s: discovery_node is required", cluster.Name)
		}
		if cluster.TokenID == "" {
			return nil, fmt.Errorf("cluster %s: token_id is required", cluster.Name)
		}
		if cluster.TokenSecret == "" {
			return nil, fmt.Errorf("cluster %s: token_secret is required", cluster.Name)
		}
		// Default to insecure for self-signed certs
		if !strings.HasPrefix(cluster.DiscoveryNode, "http") {
			cfg.Clusters[i].Insecure = true
		}
		// Auto-populate ClusterSecrets from legacy clusters for backward compat
		if _, exists := cfg.ClusterSecrets[cluster.Name]; !exists {
			cfg.ClusterSecrets[cluster.Name] = cluster.TokenSecret
		}
	}

	return &cfg, nil
}

// MustLoad loads config or panics
func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	return cfg
}
