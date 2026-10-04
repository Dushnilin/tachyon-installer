package tui_test

import (
	"strings"
	"testing"

	"tachyon-installer/internal/tui"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"red text", "\x1b[31mRedText\x1b[0m", "RedText"},
		{"bold yellow", "\x1b[1;33mBoldYellow\x1b[0m text", "BoldYellow text"},
		{"plain text", "Plain text", "Plain text"},
		{"mixed", "Hello \x1b[32mworld\x1b[0m!", "Hello world!"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tui.StripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestProfileData_RAMStatus(t *testing.T) {
	tests := []struct {
		name           string
		ramFree        float64
		ramTotal       float64
		wantCritical   bool
		wantWarning    bool
		wantSufficient bool
	}{
		{"plenty", 200.0, 512.0, false, false, true},
		{"low total", 100.0, 100.0, false, true, false},
		{"critical free", 20.0, 256.0, true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tui.ProfileData{RAMFree: tt.ramFree, RAMTotal: tt.ramTotal}
			s := p.RAMStatus()
			hasCritical := strings.Contains(s, "Критически мало")
			hasWarning := strings.Contains(s, "Рекомендуется 128")
			hasSufficient := strings.Contains(s, "Достаточно")
			if hasCritical != tt.wantCritical {
				t.Errorf("RAMStatus() critical = %v, want %v, got: %s", hasCritical, tt.wantCritical, s)
			}
			if hasWarning != tt.wantWarning {
				t.Errorf("RAMStatus() warning = %v, want %v, got: %s", hasWarning, tt.wantWarning, s)
			}
			if hasSufficient != tt.wantSufficient {
				t.Errorf("RAMStatus() sufficient = %v, want %v, got: %s", hasSufficient, tt.wantSufficient, s)
			}
		})
	}
}

func TestProfileData_FlashStatus(t *testing.T) {
	tests := []struct {
		name         string
		flashFree    float64
		wantCritical bool
		wantWarning  bool
		wantOK       bool
	}{
		{"plenty", 50.0, false, false, true},
		{"low", 10.0, false, true, false},
		{"critical", 5.0, true, false, false},
		{"zero flash", 0.0, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tui.ProfileData{FlashFree: tt.flashFree}
			s := p.FlashStatus()
			hasCritical := strings.Contains(s, "Недостаточно")
			hasWarning := strings.Contains(s, "Мало места")
			hasOK := strings.Contains(s, "Достаточно")
			if hasCritical != tt.wantCritical {
				t.Errorf("FlashStatus() critical = %v, want %v", hasCritical, tt.wantCritical)
			}
			if hasWarning != tt.wantWarning {
				t.Errorf("FlashStatus() warning = %v, want %v", hasWarning, tt.wantWarning)
			}
			if hasOK != tt.wantOK {
				t.Errorf("FlashStatus() ok = %v, want %v", hasOK, tt.wantOK)
			}
		})
	}
}

func TestProfileData_FirewallStatus(t *testing.T) {
	p4 := tui.ProfileData{Firewall: "fw4"}
	if !strings.Contains(p4.FirewallStatus(), "fw4") {
		t.Error("expected fw4 in FirewallStatus")
	}

	p3 := tui.ProfileData{Firewall: "fw3"}
	if !strings.Contains(p3.FirewallStatus(), "fw3") {
		t.Error("expected fw3 in FirewallStatus")
	}
}

func TestProfileData_ConflictStatus(t *testing.T) {
	noConflicts := tui.ProfileData{Conflicts: nil}
	if strings.Contains(noConflicts.ConflictStatus(), "Обнаружен") {
		t.Error("expected no conflict detection")
	}

	withConflicts := tui.ProfileData{Conflicts: []string{"openclash", "passwall"}}
	s := withConflicts.ConflictStatus()
	if !strings.Contains(s, "openclash") || !strings.Contains(s, "passwall") {
		t.Errorf("expected conflicts in output: %s", s)
	}
}

func TestProfileData_Warnings(t *testing.T) {
	noWarnings := tui.ProfileData{RAMTotal: 256, RAMFree: 200, FlashFree: 50, Firewall: "fw4"}
	if len(noWarnings.Warnings()) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(noWarnings.Warnings()), noWarnings.Warnings())
	}

	allWarnings := tui.ProfileData{RAMTotal: 64, RAMFree: 20, FlashFree: 5, Firewall: "fw3"}
	warnings := allWarnings.Warnings()
	if len(warnings) != 4 {
		t.Errorf("expected 4 warnings, got %d: %v", len(warnings), warnings)
	}
}
