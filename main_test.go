package main_test

import (
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
		{"plain text", "Plain text", "Plain text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := tui.StripANSI(tt.input)
			if res != tt.expected {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, res, tt.expected)
			}
		})
	}
}
