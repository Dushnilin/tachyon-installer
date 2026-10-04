package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"tachyon-installer/internal/config"
)

func TestLoad_NonExistent(t *testing.T) {
	cfg := config.Load("/nonexistent/path/config.json")
	// DefaultConfig uses GetDefaultGateway() which detects the real gateway.
	// So we only check non-IP fields.
	if cfg.SSHPort != 22 {
		t.Errorf("expected default port 22, got %d", cfg.SSHPort)
	}
	if cfg.Username != "root" {
		t.Errorf("expected default user root, got %s", cfg.Username)
	}
	if cfg.Password != "" {
		t.Errorf("expected empty password, got %q", cfg.Password)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	os.WriteFile(path, []byte("not json at all"), 0644)

	cfg := config.Load(path)
	// Should fall back to defaults
	if cfg.RouterIP == "" {
		t.Error("expected non-empty RouterIP from defaults")
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	data, _ := json.MarshalIndent(config.Config{
		RouterIP: "10.0.0.1",
		SSHPort:  2222,
		Username: "admin",
		Password: "secret",
	}, "", "  ")
	os.WriteFile(path, data, 0644)

	cfg := config.Load(path)
	if cfg.RouterIP != "10.0.0.1" {
		t.Errorf("expected RouterIP 10.0.0.1, got %s", cfg.RouterIP)
	}
	if cfg.SSHPort != 2222 {
		t.Errorf("expected SSHPort 2222, got %d", cfg.SSHPort)
	}
	if cfg.Username != "admin" {
		t.Errorf("expected Username admin, got %s", cfg.Username)
	}
	if cfg.Password != "secret" {
		t.Errorf("expected Password secret, got %s", cfg.Password)
	}
}

func TestLoad_EmptyRouterIP(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	os.WriteFile(path, []byte(`{"router_ip":"","ssh_port":0}`), 0644)

	cfg := config.Load(path)
	// Empty RouterIP should fall back to defaults
	if cfg.RouterIP == "" {
		t.Error("expected non-empty RouterIP from defaults when JSON has empty IP")
	}
}

func TestSave(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")

	cfg := &config.Config{
		RouterIP: "192.168.8.1",
		SSHPort:  22,
		Username: "root",
		Password: "pass123",
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	var loaded config.Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal saved JSON: %v", err)
	}
	if loaded.RouterIP != "192.168.8.1" {
		t.Errorf("expected RouterIP 192.168.8.1, got %s", loaded.RouterIP)
	}
	if loaded.Password != "pass123" {
		t.Errorf("expected Password pass123, got %s", loaded.Password)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.SSHPort != 22 {
		t.Errorf("expected default port 22, got %d", cfg.SSHPort)
	}
	if cfg.Username != "root" {
		t.Errorf("expected default user root, got %s", cfg.Username)
	}
}
