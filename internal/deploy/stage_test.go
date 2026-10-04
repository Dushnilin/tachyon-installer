package deploy_test

import (
	"os"
	"strings"
	"testing"

	"tachyon-installer/internal/deploy"
)

func TestStagingArea_Lifecycle(t *testing.T) {
	stage, err := deploy.NewStagingArea()
	if err != nil {
		t.Fatalf("NewStagingArea failed: %v", err)
	}
	defer stage.Cleanup()

	if _, err := os.Stat(stage.Dir); os.IsNotExist(err) {
		t.Errorf("staging directory does not exist: %s", stage.Dir)
	}

	path, err := stage.WriteRunnerScript("sing-box-extended", false)
	if err != nil {
		t.Fatalf("WriteRunnerScript failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runner script failed: %v", err)
	}

	strContent := string(content)
	if !strings.Contains(strContent, "opkg install --force-reinstall") {
		t.Errorf("expected opkg command in runner script for isAPK=false")
	}
	if !strings.Contains(strContent, "sing-box-extended") {
		t.Errorf("expected selected engine in runner script")
	}

	// Verify cleanup
	dirPath := stage.Dir
	stage.Cleanup()
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Errorf("staging directory was not removed after cleanup: %s", dirPath)
	}
}

func TestGenerateRunnerScript_APK(t *testing.T) {
	script := deploy.GenerateRunnerScript("steer", true, true)
	if !strings.Contains(script, "apk add --allow-untrusted") {
		t.Errorf("expected apk command in runner script for isAPK=true")
	}
	if !strings.Contains(script, "steer") {
		t.Errorf("expected steer engine configured")
	}
	if !strings.Contains(script, "zram-swap") {
		t.Errorf("expected zram-swap configuration in runner script when installZRAM=true")
	}
	if strings.Contains(script, "\r\n") {
		t.Errorf("expected UNIX LF line endings only")
	}
}

func TestGenerateUninstallScript(t *testing.T) {
	apkScript := deploy.GenerateUninstallScript(true)
	if !strings.Contains(apkScript, "apk del") {
		t.Errorf("expected apk del in APK uninstall script")
	}

	opkgScript := deploy.GenerateUninstallScript(false)
	if !strings.Contains(opkgScript, "opkg remove") {
		t.Errorf("expected opkg remove in opkg uninstall script")
	}
	if !strings.Contains(opkgScript, "rm -rf /etc/config/tachyon") {
		t.Errorf("expected tachyon config cleanup in uninstall script")
	}
}

