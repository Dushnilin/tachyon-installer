// Package config handles loading, saving and detecting router connection settings.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Config holds the setup and connection parameters for the target router.
type Config struct {
	RouterIP       string `json:"router_ip"`
	SSHPort        int    `json:"ssh_port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	TachyonVersion string `json:"tachyon_version,omitempty"`
	SelectedEngine string `json:"selected_engine,omitempty"`
	SelectedMirror string `json:"selected_mirror,omitempty"`
	InstallI18n    bool   `json:"install_i18n"`
}

// DefaultConfig returns a Config with sensible defaults (auto-detected gateway IP).
func DefaultConfig() *Config {
	return &Config{
		RouterIP:       GetDefaultGateway(),
		SSHPort:        22,
		Username:       "root",
		Password:       "",
		TachyonVersion: "latest",
		SelectedEngine: "sing-box-extended",
		SelectedMirror: "auto",
		InstallI18n:    true,
	}
}

// Load reads connection config from the given JSON file path.
// If the file is missing or invalid, returns DefaultConfig.
func Load(path string) *Config {
	data, err := readLocalFile(path)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err == nil && cfg.RouterIP != "" {
			if cfg.SelectedEngine == "" {
				cfg.SelectedEngine = "sing-box-extended"
			}
			if cfg.SelectedMirror == "" {
				cfg.SelectedMirror = "auto"
			}
			if cfg.TachyonVersion == "" {
				cfg.TachyonVersion = "latest"
			}
			return &cfg
		}
	}
	return DefaultConfig()
}

// Save persists the config to the given JSON file path with indentation.
func (c *Config) Save(path string) error {
	newData, err := json.MarshalIndent(*c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return writeLocalFile(path, newData)
}

// readLocalFile is a thin wrapper for testing. Overridden in tests.
var readLocalFile = os.ReadFile

// writeLocalFile is a thin wrapper for testing. Overridden in tests.
var writeLocalFile = func(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// GetDefaultGateway auto-detects the default gateway IP address
// based on the current operating system.
func GetDefaultGateway() string {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-NoProfile",
			"-Command", "Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Select-Object -ExpandProperty NextHop -First 1")
		setHideWindow(cmd)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			ip := strings.TrimSpace(stdout.String())
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	} else if runtime.GOOS == "darwin" {
		cmd := exec.Command("sh", "-c", "route -n get default 2>/dev/null | awk '/gateway:/ {print $2}'")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			ip := strings.TrimSpace(stdout.String())
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	} else {
		cmd := exec.Command("sh", "-c", "ip route show 2>/dev/null | grep default | awk '{print $3}'")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			ip := strings.TrimSpace(stdout.String())
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	return "192.168.1.1"
}
