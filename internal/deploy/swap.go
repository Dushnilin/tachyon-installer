package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	dlpkg "tachyon-installer/internal/downloader"
)

// HotSwapEngine downloads only the chosen engine and updates the router's active engine in UCI
// without reinstalling LuCI or clearing subscription configurations.
func HotSwapEngine(
	ctx context.Context,
	client *gossh.Client,
	targetEngine string,
	mirror string,
	isAPK bool,
	distribArch string,
	detectedArch string,
	logFn func(string),
) error {
	if logFn == nil {
		logFn = func(string) {}
	}

	logFn(fmt.Sprintf("⚡ Горячая смена ядра на [%s]...\n", targetEngine))

	staging, err := NewStagingArea()
	if err != nil {
		return fmt.Errorf("create staging area: %w", err)
	}
	defer staging.Cleanup()

	// 1. Resolve and download engine packages/binaries if not "skip"
	if targetEngine != "skip" && targetEngine != "sing-box-tiny" {
		mirrorMgr := dlpkg.NewMirrorManager(mirror)
		dlClient := dlpkg.NewClient(mirrorMgr)

		logFn(fmt.Sprintf("⚡ Поиск пакетов ядра %s для %s (%s)...\n", targetEngine, detectedArch, distribArch))
		assets, err := dlpkg.ResolveEngineAssets(ctx, dlClient, dlpkg.EngineType(targetEngine), distribArch, detectedArch, isAPK)
		if err != nil {
			return fmt.Errorf("resolve engine assets: %w", err)
		}
		if len(assets) == 0 {
			return fmt.Errorf("no engine assets found for %s on %s", targetEngine, detectedArch)
		}

		for _, ea := range assets {
			if ea == nil || ea.URL == "" {
				continue
			}
			logFn(fmt.Sprintf("  -> Скачивание %s...\n", ea.Filename))
			_, errDl := dlpkg.DownloadEnginePackage(ctx, dlClient, ea, staging.Dir, logFn, nil)
			if errDl != nil {
				return fmt.Errorf("download engine package %s: %w", ea.Filename, errDl)
			}
		}
	}

	// 2. Generate hot-swap runner script
	uciEngine := targetEngine
	switch targetEngine {
	case "sing-box-extended", "sing-box-extended-compressed", "sing-box-tiny", "sing-box-lx":
		uciEngine = "sing-box"
	case "steer", "steer-extended":
		uciEngine = targetEngine
	}

	tinyInstallCmd := ""
	if targetEngine == "sing-box-tiny" {
		if isAPK {
			tinyInstallCmd = "apk add sing-box-tiny 2>&1 || true"
		} else {
			tinyInstallCmd = "opkg update >/dev/null 2>&1 && opkg install sing-box-tiny 2>&1 || true"
		}
	}

	script := fmt.Sprintf(`#!/bin/sh
set -e
echo "==> [1/3] Остановка службы Tachyon..."
if [ -x /etc/init.d/tachyon ]; then
    /etc/init.d/tachyon stop >/dev/null 2>&1 || true
fi
killall -9 sing-box steer 2>/dev/null || true

echo "==> [2/3] Обновление файлов ядра (%s)..."
%s

# If sing-box binary or tar.gz was uploaded
if [ -f /tmp/tachyon-install/sing-box ]; then
    mv -f /tmp/tachyon-install/sing-box /usr/bin/sing-box
    chmod 0755 /usr/bin/sing-box
elif ls /tmp/tachyon-install/sing-box*.tar.gz >/dev/null 2>&1; then
    for a in /tmp/tachyon-install/sing-box*.tar.gz; do
        tar -xzf "$a" -C /tmp/tachyon-install/ 2>/dev/null || true
        rm -f "$a"
    done
    FOUND=$(find /tmp/tachyon-install/ -type f -name sing-box 2>/dev/null | head -1)
    if [ -n "$FOUND" ]; then
        mv -f "$FOUND" /usr/bin/sing-box
        chmod 0755 /usr/bin/sing-box
    fi
fi

# If IPK or APK packages exist (steer, etc.)
if command -v apk >/dev/null 2>&1; then
    ls /tmp/tachyon-install/*.apk >/dev/null 2>&1 && apk add --allow-untrusted /tmp/tachyon-install/*.apk 2>&1 || true
else
    ls /tmp/tachyon-install/*.ipk >/dev/null 2>&1 && opkg install --force-reinstall /tmp/tachyon-install/*.ipk 2>&1 || true
fi

# Ensure /usr/bin/steer symlink
if [ -x /usr/sbin/steer ] && [ ! -e /usr/bin/steer ]; then
    ln -sf /usr/sbin/steer /usr/bin/steer 2>/dev/null || true
fi

echo "==> [3/3] Переключение UCI (engine=%s) и перезапуск службы..."
uci -q set tachyon.settings.engine="%s"
uci commit tachyon 2>/dev/null || true
/etc/init.d/tachyon enable >/dev/null 2>&1 || true
/etc/init.d/tachyon restart 2>&1 || /usr/bin/tachyon start 2>&1 || true
rm -rf /tmp/tachyon-install 2>/dev/null || true
echo "==> Горячая смена ядра завершена успешно!"
`, targetEngine, tinyInstallCmd, uciEngine, uciEngine)

	scriptPath := filepath.Join(staging.Dir, "install_on_router.sh")
	if err := os.WriteFile(scriptPath, []byte(strings.ReplaceAll(script, "\r\n", "\n")), 0755); err != nil {
		return fmt.Errorf("write runner script: %w", err)
	}

	// 3. Upload & execute
	logFn("⚡ Загрузка и активация на роутере...\n")
	if err := UploadStagingAndExecute(client, staging.Dir, os.Stdout, logFn); err != nil {
		return fmt.Errorf("swap execute failed: %w", err)
	}

	time.Sleep(3 * time.Second)

	// Verify status
	sess, err := client.NewSession()
	if err == nil {
		defer sess.Close()
		out, _ := sess.CombinedOutput(fmt.Sprintf(`(pgrep -x %s >/dev/null 2>&1 || pgrep -f "%s" >/dev/null 2>&1) && echo RUNNING || echo STOPPED`, uciEngine, uciEngine))
		if strings.Contains(string(out), "RUNNING") {
			logFn(fmt.Sprintf("[#22c55e]✓ Новое ядро [%s] успешно запущено и активно![-]\n", targetEngine))
		} else {
			logFn(fmt.Sprintf("[#eab308]⚠️ Ядро переключено на [%s], статус: %s[-]\n", targetEngine, strings.TrimSpace(string(out))))
		}
	}

	return nil
}
