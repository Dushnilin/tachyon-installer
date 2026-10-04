package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	// A legacy file may still contain a plaintext password; it must be ignored.
	os.WriteFile(path, []byte(`{"router_ip":"10.0.0.1","ssh_port":2222,"username":"admin","password":"secret","key_path":"/k/id"}`), 0644)

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
	if cfg.Password != "" {
		t.Errorf("password must never be loaded from disk, got %q", cfg.Password)
	}
	if cfg.KeyPath != "/k/id" {
		t.Errorf("expected KeyPath /k/id, got %q", cfg.KeyPath)
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
	if strings.Contains(string(data), "pass123") {
		t.Errorf("password leaked into the config file:\n%s", data)
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
