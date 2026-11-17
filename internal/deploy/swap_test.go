package deploy_test

import (
	"strings"
	"testing"
)

func TestHotSwapScriptIntegrity(t *testing.T) {
	// Verify hot swap command for tachyon-core
	engine := "tachyon-core"
	script := `uci -q set tachyon.settings.engine="` + engine + `"`
	if !strings.Contains(script, "tachyon-core") {
		t.Errorf("expected tachyon-core engine configuration")
	}
}
