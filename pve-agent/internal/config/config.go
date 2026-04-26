package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the agent configuration
type Config struct {
	PCenter    PCenterConfig    `yaml:"pcenter"`
	PVE        PVEConfig        `yaml:"pve"`
	Node       NodeConfig       `yaml:"node"`
	Collection CollectionConfig `yaml:"collection"`
}

// PVEConfig holds local Proxmox API settings
type PVEConfig struct {
	TokenID     string `yaml:"token_id"`     // e.g., root@pam!pve-agent
	TokenSecret string `yaml:"token_secret"` // the secret part
}

// PCenterConfig holds pCenter connection settings
type PCenterConfig struct {
	URL   string `yaml:"url"`   // wss://pcenter:8080/api/agent/ws
	Token string `yaml:"token"` // Agent authentication token
}

// NodeConfig holds node identification
type NodeConfig struct {
	Name    string `yaml:"name"`    // Node name (auto-detected if empty)
	Cluster string `yaml:"cluster"` // Cluster name
}

// CollectionConfig holds collection settings
type CollectionConfig struct {
	Interval      int  `yaml:"interval"`        // Status push interval in seconds
	IncludeSmart  bool `yaml:"include_smart"`   // Include SMART disk data
	IncludeCeph   bool `yaml:"include_ceph"`    // Include Ceph status
	SmartInterval int  `yaml:"smart_interval"`  // SMART scan interval in seconds (default 300)
}

// Load reads config from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		// Defaults
		Collection: CollectionConfig{
			Interval:      5,
			IncludeSmart:  false,
			IncludeCeph:   true,
			SmartInterval: 300,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// yaml.Unmarshal zeroes the field when absent — re-apply default after
	// load so an existing config without smart_interval still gets 300s.
	if cfg.Collection.SmartInterval == 0 {
		cfg.Collection.SmartInterval = 300
	}

	// Auto-detect node name if not set
	if cfg.Node.Name == "" {
		hostname, err := os.Hostname()
		if err == nil {
			cfg.Node.Name = hostname
		}
	}

	return cfg, nil
}
