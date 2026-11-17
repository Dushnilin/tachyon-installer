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

	normTarget := string(dlpkg.NormalizeEngine(targetEngine))
	logFn(fmt.Sprintf("→ Обновление ядра на [%s]...\n", normTarget))

	staging, err := NewStagingArea()
	if err != nil {
		return fmt.Errorf("create staging area: %w", err)
	}
	defer staging.Cleanup()

	// 1. Resolve and download tachyon-core package/binary unless "skip"
	if normTarget != "skip" {
		mirrorMgr := dlpkg.NewMirrorManager(mirror)
		dlClient := dlpkg.NewClient(mirrorMgr)

		logFn(fmt.Sprintf("→ Поиск пакетов ядра %s для %s (%s)...\n", normTarget, detectedArch, distribArch))
		assets, err := dlpkg.ResolveEngineAssets(ctx, dlClient, dlpkg.EngineType(normTarget), distribArch, detectedArch, isAPK)
		if err != nil {
			return fmt.Errorf("resolve engine assets: %w", err)
		}
		if len(assets) == 0 {
			return fmt.Errorf("no engine assets found for %s on %s", normTarget, detectedArch)
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
	uciEngine := "tachyon-core"
	variantMarker := VariantMarkerForEngine(normTarget)

	script := fmt.Sprintf(`#!/bin/sh
set -e
echo "==> [1/3] Остановка службы Tachyon..."
rm -rf /var/run/tachyon/starting /var/run/tachyon/reloading /var/run/tachyon*.lock 2>/dev/null || true
if [ -x /etc/init.d/tachyon ]; then
    /etc/init.d/tachyon stop >/dev/null 2>&1 || true
fi
if [ -x /etc/init.d/tachyon-core ]; then
    /etc/init.d/tachyon-core stop >/dev/null 2>&1 || true
fi
if [ -x /etc/init.d/sing-box ]; then
    /etc/init.d/sing-box stop >/dev/null 2>&1 || true
    /etc/init.d/sing-box disable >/dev/null 2>&1 || true
fi
killall -9 tachyon-core sing-box 2>/dev/null || true

ENGINE="%s"
VARIANT_MARKER="%s"
if command -v apk >/dev/null 2>&1; then
    PKG_MGR="apk"
else
    PKG_MGR="opkg"
fi

# Clean legacy sing-box packages when deploying tachyon-core (Tachyon 1.8.0+)
if [ "$ENGINE" = "tachyon-core" ]; then
    if [ "$PKG_MGR" = "apk" ]; then
        apk del --no-network --repositories-file /dev/null --force-broken-world sing-box-extended sing-box-tiny sing-box >/dev/null 2>&1 || true
    else
        opkg remove --force-depends sing-box-extended sing-box-tiny sing-box >/dev/null 2>&1 || true
    fi
fi

echo "==> [2/3] Обновление файлов ядра (%s)..."

# If standalone tachyon-core binary or tar.gz archive was uploaded
if [ -f /tmp/tachyon-install/tachyon-core ]; then
    rm -f /usr/bin/tachyon-core 2>/dev/null || true
    mv -f /tmp/tachyon-install/tachyon-core /usr/bin/tachyon-core
    chmod 0755 /usr/bin/tachyon-core
elif ls /tmp/tachyon-install/tachyon-core*.tar.gz >/dev/null 2>&1; then
    for a in /tmp/tachyon-install/tachyon-core*.tar.gz; do
        tar -xzf "$a" -C /tmp/tachyon-install/ 2>/dev/null || true
        rm -f "$a"
    done
    FOUND=$(find /tmp/tachyon-install/ -type f -name "tachyon-core*" ! -name "*.tar.gz" ! -name "*.ipk" ! -name "*.apk" ! -name "*.sha256" ! -name "*.txt" ! -name "*.md" ! -name "*.sh" 2>/dev/null | head -1)
    if [ -n "$FOUND" ]; then
        rm -f /usr/bin/tachyon-core 2>/dev/null || true
        mv -f "$FOUND" /usr/bin/tachyon-core
        chmod 0755 /usr/bin/tachyon-core
    fi
fi

# If IPK or APK packages exist (tachyon-core-*.apk/.ipk)
if [ "$PKG_MGR" = "apk" ]; then
    ls /tmp/tachyon-install/*.apk >/dev/null 2>&1 && (apk add --allow-untrusted --force-overwrite --no-network --repositories-file /dev/null /tmp/tachyon-install/*.apk 2>&1 || apk add --allow-untrusted --force-overwrite /tmp/tachyon-install/*.apk 2>&1) || true
else
    ls /tmp/tachyon-install/*.ipk >/dev/null 2>&1 && opkg install --force-reinstall --force-overwrite /tmp/tachyon-install/*.ipk 2>&1 || true
fi

# Write variant marker & clear stale cache.db
if [ -n "$VARIANT_MARKER" ]; then
    mkdir -p /etc/tachyon /etc/tachyon-core /tmp/tachyon-core /usr/share/tachyon-core 2>/dev/null || true
    printf '%%s\n' "$VARIANT_MARKER" > /etc/tachyon/tachyon-core-variant 2>/dev/null || true
    if [ -x /usr/bin/tachyon-core ]; then
        _tc_ver=$(/usr/bin/tachyon-core --version 2>/dev/null | head -n 1 | awk '{print $NF}')
        [ -n "$_tc_ver" ] && printf '%%s\n' "$_tc_ver" > /etc/tachyon/tachyon-core-version 2>/dev/null || true
    fi
    rm -f /tmp/tachyon-core/cache.db* /usr/share/tachyon-core/cache.db* /etc/tachyon-core/cache.db* /var/run/sing-box/cache.db* 2>/dev/null || true
    if [ ! -x /etc/init.d/tachyon-core ] && command -v ucode >/dev/null 2>&1 && [ -f /usr/lib/tachyon/core/process.uc ]; then
        ucode -L /usr/lib/tachyon -e 'let fs = require("fs"); let cp = require("core.process"); fs.writefile("/etc/init.d/tachyon-core", cp.managed_core_init_script("", true)); fs.chmod("/etc/init.d/tachyon-core", 0755);' 2>/dev/null || true
    fi
fi

echo "==> [3/3] Настройка UCI (engine=%s) и перезапуск службы..."
uci -q set tachyon.settings.engine="%s"
uci -q set tachyon.settings.subscription_engine="core"
uci commit tachyon 2>/dev/null || true
/etc/init.d/tachyon enable >/dev/null 2>&1 || true
/etc/init.d/tachyon restart 2>&1 || /usr/bin/tachyon start 2>&1 || true
rm -rf /tmp/tachyon-install 2>/dev/null || true
echo "==> Обновление ядра завершено."
`, normTarget, variantMarker, normTarget, uciEngine, uciEngine)

	scriptPath := filepath.Join(staging.Dir, "install_on_router.sh")
	if err := os.WriteFile(scriptPath, []byte(strings.ReplaceAll(script, "\r\n", "\n")), 0755); err != nil {
		return fmt.Errorf("write runner script: %w", err)
	}

	// 3. Upload & execute
	logFn("→ Загрузка и активация на роутере...\n")
	if err := UploadStagingAndExecute(client, staging.Dir, os.Stdout, logFn); err != nil {
		return fmt.Errorf("swap execute failed: %w", err)
	}

	time.Sleep(3 * time.Second)

	// Verify status
	sess, err := client.NewSession()
	if err == nil {
		defer sess.Close()
		checkCmd := `(pgrep -x tachyon-core >/dev/null 2>&1 || pgrep -f "tachyon-core" >/dev/null 2>&1) && echo RUNNING || echo STOPPED`
		out, _ := sess.CombinedOutput(checkCmd)
		if strings.Contains(string(out), "RUNNING") {
			logFn(fmt.Sprintf("[#4cc38a]✓ Ядро [%s] запущено и активно[-]\n", normTarget))
		} else {
			logFn(fmt.Sprintf("[#eab308]! Ядро обновлено [%s], статус: %s[-]\n", normTarget, strings.TrimSpace(string(out))))
		}
	}

	return nil
}
