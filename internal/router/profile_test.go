package router_test

import (
	"testing"

	"tachyon-installer/internal/router"
)

func TestParsePipePair(t *testing.T) {
	tests := []struct {
		name  string
		input string
		wantA float64
		wantB float64
	}{
		{"normal", "1024|512", 1024, 512},
		{"large", "131072|65536", 131072, 65536},
		{"no pipe", "nothing", 0, 0},
		{"empty", "", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := router.ParsePipePair(tt.input)
			if a != tt.wantA || b != tt.wantB {
				t.Errorf("parsePipePair(%q) = (%v, %v), want (%v, %v)", tt.input, a, b, tt.wantA, tt.wantB)
			}
		})
	}
}

func TestParseConflicts(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"none", "none", nil},
		{"empty", "", nil},
		{"single", "passwall -1.0-1 ", []string{"passwall"}},
		{"multiple", "openclash -1.0 passwall-2.0", []string{"openclash", "passwall"}},
		{"duplicates", "passwall passwall", []string{"passwall"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.ParseConflicts(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseConflicts(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestClassifyArch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"aarch64", "aarch64", "arm64"},
		{"arm64 uname", "arm64", "arm64"},
		{"x86_64", "x86_64", "amd64"},
		{"amd64", "amd64", "amd64"},
		{"mipsel", "mipsel", "mipsle"},
		{"mipsle", "mipsle", "mipsle"},
		{"mips", "mips", "mips"},
		{"armv7", "armv7l", "arm"},
		{"unknown", "sparc64", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.ClassifyArch(tt.input)
			if result != tt.expected {
				t.Errorf("classifyArch(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseProfile(t *testing.T) {
	input := `=== TACHYON-PROFILE ===
Generic Router
OpenWrt 24.10 (noble)
aarch64
aarch64_cortex-a53
524288|262144
16384|8192
none
opkg
fw4
`

	profile, err := router.ParseProfile(input)
	if err != nil {
		t.Fatalf("ParseProfile failed: %v", err)
	}
	if profile.Model != "Generic Router" {
		t.Errorf("expected Model 'Generic Router', got %q", profile.Model)
	}
	if profile.Firewall != "fw4" {
		t.Errorf("expected Firewall fw4, got %q", profile.Firewall)
	}
	if profile.IsAPK {
		t.Error("expected IsAPK false")
	}
	if len(profile.Conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d", len(profile.Conflicts))
	}
}

func TestParseProfile_WithConflicts(t *testing.T) {
	input := `=== TACHYON-PROFILE ===
Device X
OpenWrt 23.05
mips_24kc
mips_24kc
131072|65536
8192|4096
openclash -1.0 passwall -2.0
opkg
fw3
`

	profile, err := router.ParseProfile(input)
	if err != nil {
		t.Fatalf("ParseProfile failed: %v", err)
	}
	if profile.Firewall != "fw3" {
		t.Errorf("expected Firewall fw3, got %q", profile.Firewall)
	}
	if len(profile.Conflicts) != 2 {
		t.Errorf("expected 2 conflicts, got %d: %v", len(profile.Conflicts), profile.Conflicts)
	}
}

func TestParseProfile_Invalid(t *testing.T) {
	_, err := router.ParseProfile("no marker here")
	if err == nil {
		t.Error("expected error for invalid input without marker")
	}
}

func TestParseProfile_Short(t *testing.T) {
	_, err := router.ParseProfile("=== TACHYON-PROFILE ===\nline1\n")
	if err == nil {
		t.Error("expected error for too-short input")
	}
}
