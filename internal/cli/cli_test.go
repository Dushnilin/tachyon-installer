package cli

import (
	"testing"

	appconfig "tachyon-installer/internal/config"
)

func TestParseFlags_Defaults(t *testing.T) {
	cfg := appconfig.DefaultConfig()
	opts, err := ParseFlags([]string{}, cfg, "v1.3.0")
	if err != nil {
		t.Fatalf("ParseFlags failed: %v", err)
	}

	if opts.ShouldRunHeadless() {
		t.Errorf("expected ShouldRunHeadless to be false by default")
	}

	if opts.SSHPort != 22 {
		t.Errorf("expected default SSH port 22, got %d", opts.SSHPort)
	}
}

func TestParseFlags_HeadlessTriggers(t *testing.T) {
	cfg := appconfig.DefaultConfig()

	// 1. -yes
	opts, err := ParseFlags([]string{"-yes", "-ip", "192.168.1.50", "-engine", "steer"}, cfg, "v1.3.0")
	if err != nil || !opts.ShouldRunHeadless() {
		t.Errorf("expected -yes to trigger ShouldRunHeadless")
	}
	if opts.RouterIP != "192.168.1.50" || opts.Engine != "steer" {
		t.Errorf("parsed options mismatch: ip=%s, engine=%s", opts.RouterIP, opts.Engine)
	}

	// 2. -diag
	optsDiag, err := ParseFlags([]string{"-diag", "-export-diag", "test.txt"}, cfg, "v1.3.0")
	if err != nil || !optsDiag.ShouldRunHeadless() || !optsDiag.RunDiag {
		t.Errorf("expected -diag to trigger headless mode")
	}

	// 3. -uninstall
	optsUn, err := ParseFlags([]string{"-uninstall"}, cfg, "v1.3.0")
	if err != nil || !optsUn.ShouldRunHeadless() || !optsUn.RunUninstall {
		t.Errorf("expected -uninstall to trigger headless mode")
	}

	// 4. -restore
	optsRes, err := ParseFlags([]string{"-restore", "latest"}, cfg, "v1.3.0")
	if err != nil || !optsRes.ShouldRunHeadless() || optsRes.RunRestore != "latest" {
		t.Errorf("expected -restore to trigger headless mode")
	}

	// 5. -list-backups
	optsList, err := ParseFlags([]string{"-list-backups"}, cfg, "v1.3.0")
	if err != nil || !optsList.ShouldRunHeadless() || !optsList.ListBackups {
		t.Errorf("expected -list-backups to trigger headless mode")
	}

	// 6. -check-update
	optsUpd, err := ParseFlags([]string{"-check-update"}, cfg, "v1.3.0")
	if err != nil || !optsUpd.ShouldRunHeadless() || !optsUpd.CheckUpdate {
		t.Errorf("expected -check-update to trigger headless mode")
	}
}
