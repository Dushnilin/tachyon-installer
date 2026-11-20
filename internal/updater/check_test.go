package updater

import (
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		remote string
		local  string
		want   bool
	}{
		{"v1.3.1", "v1.3.0", true},
		{"v1.4.0", "v1.3.9", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.3.0", "v1.3.0", false},
		{"v1.2.9", "v1.3.0", false},
		{"v1.3.0-rc1", "v1.3.0", false},
		{"v1.3.1-rc1", "v1.3.0", true},
		{"v2.0.0-beta.7", "v2.0.0-beta.6", true},
		{"v2.0.0-beta.7", "v1.8.2", true},
		{"v2.0.0", "v2.0.0-beta.7", true},
		{"v2.0.0-beta.7", "v2.0.0", false},
	}

	for _, tt := range tests {
		got := isNewerVersion(tt.remote, tt.local)
		if got != tt.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.remote, tt.local, got, tt.want)
		}
	}
}
