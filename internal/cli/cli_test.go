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

	// 7. -switch-engine
	optsSwitch, err := ParseFlags([]string{"-switch-engine", "sing-box-tiny"}, cfg, "v1.5.0")
	if err != nil || !optsSwitch.ShouldRunHeadless() || optsSwitch.SwitchEngine != "sing-box-tiny" {
		t.Errorf("expected -switch-engine to trigger headless mode with engine sing-box-tiny")
	}

	// 8. -rescue
	optsRescue, err := ParseFlags([]string{"-rescue"}, cfg, "v1.6.0")
	if err != nil || !optsRescue.ShouldRunHeadless() || !optsRescue.RunRescue {
		t.Errorf("expected -rescue to trigger headless mode")
	}

	// 9. -fix-conflicts
	optsConflicts, err := ParseFlags([]string{"-fix-conflicts"}, cfg, "v1.6.0")
	if err != nil || !optsConflicts.ShouldRunHeadless() || !optsConflicts.FixConflicts {
		t.Errorf("expected -fix-conflicts to trigger headless mode")
	}

	// 10. -snapshot
	optsSnap, err := ParseFlags([]string{"-snapshot"}, cfg, "v1.6.0")
	if err != nil || !optsSnap.ShouldRunHeadless() || !optsSnap.FullSnapshot {
		t.Errorf("expected -snapshot to trigger headless mode")
	}

	// 11. -offline-bundle
	optsBundle, err := ParseFlags([]string{"-offline-bundle", "cache/"}, cfg, "v1.6.0")
	if err != nil || !optsBundle.ShouldRunHeadless() || optsBundle.OfflineBundle != "cache/" {
		t.Errorf("expected -offline-bundle to trigger headless mode")
	}

	// 12. -monitor
	optsMon, err := ParseFlags([]string{"-monitor"}, cfg, "v1.6.0")
	if err != nil || !optsMon.ShouldRunHeadless() || !optsMon.RunMonitor {
		t.Errorf("expected -monitor to trigger headless mode")
	}

	// 13. -test-sub
	optsSub, err := ParseFlags([]string{"-test-sub", "vless://test"}, cfg, "v1.6.0")
	if err != nil || !optsSub.ShouldRunHeadless() || optsSub.TestSub != "vless://test" {
		t.Errorf("expected -test-sub to trigger headless mode")
	}

	// 14. -self-update
	optsSelfUpd, err := ParseFlags([]string{"-self-update"}, cfg, "v1.6.0")
	if err != nil || !optsSelfUpd.ShouldRunHeadless() || !optsSelfUpd.SelfUpdate {
		t.Errorf("expected -self-update to trigger headless mode")
	}

	// 15. -tune-network
	optsTune, err := ParseFlags([]string{"-tune-network"}, cfg, "v1.6.0")
	if err != nil || !optsTune.ShouldRunHeadless() || !optsTune.TuneNetwork {
		t.Errorf("expected -tune-network to trigger headless mode")
	}

	// 16. -wizard / -guided / -autopilot
	optsWiz, err := ParseFlags([]string{"-wizard"}, cfg, "v1.6.2")
	if err != nil || !optsWiz.ShouldRunHeadless() || !optsWiz.GuidedSetup {
		t.Errorf("expected -wizard to trigger headless mode")
	}

	optsGuided, err := ParseFlags([]string{"-guided"}, cfg, "v1.6.2")
	if err != nil || !optsGuided.ShouldRunHeadless() || !optsGuided.GuidedSetup {
		t.Errorf("expected -guided to trigger headless mode")
	}

	// 17. -fleet-scan and -fleet-deploy
	optsFleetScan, err := ParseFlags([]string{"-fleet-scan", "-fleet-subnets", "192.168.1.0/24,192.168.2.0/24"}, cfg, "v1.6.3")
	if err != nil || !optsFleetScan.ShouldRunHeadless() || !optsFleetScan.FleetScan {
		t.Errorf("expected -fleet-scan to trigger headless mode")
	}
	if optsFleetScan.FleetSubnets != "192.168.1.0/24,192.168.2.0/24" {
		t.Errorf("parsed fleet subnets mismatch: %s", optsFleetScan.FleetSubnets)
	}

	optsFleetDeploy, err := ParseFlags([]string{"-fleet-deploy", "-fleet-only-outdated", "-fleet-concurrency", "5"}, cfg, "v1.6.3")
	if err != nil || !optsFleetDeploy.ShouldRunHeadless() || !optsFleetDeploy.FleetDeploy {
		t.Errorf("expected -fleet-deploy to trigger headless mode")
	}
	if !optsFleetDeploy.FleetOnlyOutdated || optsFleetDeploy.FleetConcurrency != 5 {
		t.Errorf("parsed fleet deploy options mismatch")
	}

	// 18. -speedtest and -speed-doctor
	optsSpeed, err := ParseFlags([]string{"-speedtest"}, cfg, "v1.7.0")
	if err != nil || !optsSpeed.ShouldRunHeadless() || !optsSpeed.SpeedDoctor {
		t.Errorf("expected -speedtest to trigger headless mode")
	}
	if optsSpeed.SpeedDuration != 5 {
		t.Errorf("expected default speed duration 5, got %d", optsSpeed.SpeedDuration)
	}

	optsSpeedDoc, err := ParseFlags([]string{"-speed-doctor", "-speed-duration", "10"}, cfg, "v1.7.0")
	if err != nil || !optsSpeedDoc.ShouldRunHeadless() || !optsSpeedDoc.SpeedDoctor {
		t.Errorf("expected -speed-doctor to trigger headless mode")
	}
	if optsSpeedDoc.SpeedDuration != 10 {
		t.Errorf("expected custom speed duration 10, got %d", optsSpeedDoc.SpeedDuration)
	}
}

