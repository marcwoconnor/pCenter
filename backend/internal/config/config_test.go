package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_MinimalConfig verifies that a minimal valid config loads with
// correct defaults applied.
func TestLoad_MinimalConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  port: 9090
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}

	// Check defaults
	if cfg.DRS.CPUThreshold != 0.8 {
		t.Errorf("expected default CPU threshold 0.8, got %f", cfg.DRS.CPUThreshold)
	}
	if cfg.DRS.Mode != "manual" {
		t.Errorf("expected default DRS mode 'manual', got %q", cfg.DRS.Mode)
	}
	if cfg.Metrics.CollectionInterval != 30 {
		t.Errorf("expected default collection interval 30, got %d", cfg.Metrics.CollectionInterval)
	}
	if cfg.Auth.Session.DurationHours != 24 {
		t.Errorf("expected default session duration 24h, got %d", cfg.Auth.Session.DurationHours)
	}
	if cfg.Auth.Lockout.MaxAttempts != 5 {
		t.Errorf("expected default max attempts 5, got %d", cfg.Auth.Lockout.MaxAttempts)
	}
}

// TestLoad_DefaultPort verifies that an empty server config gets port 8080.
func TestLoad_DefaultPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	os.WriteFile(cfgPath, []byte("{}"), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

// TestLoad_EnvVarExpansion verifies that ${VAR} syntax is expanded from
// environment variables.
func TestLoad_EnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	t.Setenv("TEST_PCENTER_SECRET", "my-secret-token")

	yaml := `
clusters:
  - name: test
    discovery_node: "10.0.0.1:8006"
    token_id: "root@pam!test"
    token_secret: "${TEST_PCENTER_SECRET}"
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Clusters[0].TokenSecret != "my-secret-token" {
		t.Errorf("expected expanded secret, got %q", cfg.Clusters[0].TokenSecret)
	}
}

// TestLoad_ClusterValidation_MissingName verifies that clusters without a
// name are rejected.
func TestLoad_ClusterValidation_MissingName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
clusters:
  - discovery_node: "10.0.0.1:8006"
    token_id: "root@pam!test"
    token_secret: "secret"
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Error("cluster without name should fail validation")
	}
}

// TestLoad_ClusterValidation_MissingTokenSecret verifies that clusters
// without token_secret are rejected.
func TestLoad_ClusterValidation_MissingTokenSecret(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
clusters:
  - name: test
    discovery_node: "10.0.0.1:8006"
    token_id: "root@pam!test"
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Error("cluster without token_secret should fail validation")
	}
}

// TestLoad_MissingFile verifies that a missing config file returns an error.
func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("missing file should return error")
	}
}

// TestLoad_InvalidYAML verifies that malformed YAML is rejected.
func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	os.WriteFile(cfgPath, []byte("{{invalid yaml"), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Error("invalid YAML should return error")
	}
}

// TestLoad_CORSOrigins verifies that CORS origins are loaded correctly.
func TestLoad_CORSOrigins(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
server:
  port: 8080
  cors_origins:
    - "http://localhost:5173"
    - "https://pcenter.local"
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.Server.CORSOrigins) != 2 {
		t.Fatalf("expected 2 CORS origins, got %d", len(cfg.Server.CORSOrigins))
	}
	if cfg.Server.CORSOrigins[0] != "http://localhost:5173" {
		t.Errorf("expected first origin 'http://localhost:5173', got %q", cfg.Server.CORSOrigins[0])
	}
}

// TestLoad_AgentAuthToken verifies that the new agent auth_token field
// is loaded correctly.
func TestLoad_AgentAuthToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
agent:
  auth_token: "super-secret-agent-token"
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Agent.AuthToken != "super-secret-agent-token" {
		t.Errorf("expected agent auth token, got %q", cfg.Agent.AuthToken)
	}
}

// TestLoad_LegacyNodesFormat verifies backward compatibility with the old
// per-node config format.
func TestLoad_LegacyNodesFormat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yaml := `
nodes:
  - name: pve01
    host: "10.0.0.1:8006"
    token_id: "root@pam!test"
    token_secret: "secret"
`
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.Clusters) != 1 {
		t.Fatalf("legacy nodes should create 1 cluster, got %d", len(cfg.Clusters))
	}
	if cfg.Clusters[0].Name != "default" {
		t.Errorf("legacy cluster should be named 'default', got %q", cfg.Clusters[0].Name)
	}
}
