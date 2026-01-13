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
	DRS      DRSConfig       `yaml:"drs"`
	Server   ServerConfig    `yaml:"server"`

	// Legacy: flat nodes array (auto-converted to single cluster)
	Nodes []NodeConfig `yaml:"nodes,omitempty"`
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

	// Validate clusters
	if len(cfg.Clusters) == 0 {
		return nil, fmt.Errorf("at least one cluster must be configured")
	}

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
