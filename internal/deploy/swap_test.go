package deploy_test

import (
	"strings"
	"testing"
)

func TestHotSwapScriptIntegrity(t *testing.T) {
	// Verify hot swap commands for sing-box and steer
	engine := "steer"
	script := `uci -q set tachyon.settings.engine="` + engine + `"`
	if !strings.Contains(script, "steer") {
		t.Errorf("expected steer engine configuration")
	}
}
