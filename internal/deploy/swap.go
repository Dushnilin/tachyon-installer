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

	// 1. Resolve and download engine packages/binaries if not "skip" or repo-managed
	if targetEngine != "skip" && targetEngine != "sing-box-tiny" && targetEngine != "sing-box" {
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
	case "tachyon-core", "sing-box-extended", "sing-box-extended-compressed", "sing-box-tiny", "sing-box-lx", "sing-box":
		uciEngine = "sing-box"
	case "steer", "steer-extended":
		uciEngine = targetEngine
	}

	variantMarker := VariantMarkerForEngine(targetEngine)

	repoInstallCmd := ""
	switch targetEngine {
	case "sing-box-tiny":
		if isAPK {
			repoInstallCmd = "if [ -x /usr/bin/tachyon ]; then /usr/bin/tachyon component_action sing_box install_tiny 2>&1 || true; else apk update >/dev/null 2>&1 || true; apk add sing-box-tiny 2>&1 || true; fi"
		} else {
			repoInstallCmd = "if [ -x /usr/bin/tachyon ]; then /usr/bin/tachyon component_action sing_box install_tiny 2>&1 || true; else opkg update >/dev/null 2>&1 && opkg install sing-box-tiny 2>&1 || true; fi"
		}
	case "sing-box":
		if isAPK {
			repoInstallCmd = "if [ -x /usr/bin/tachyon ]; then /usr/bin/tachyon component_action sing_box install_stable 2>&1 || true; else apk update >/dev/null 2>&1 || true; apk add sing-box 2>&1 || true; fi"
		} else {
			repoInstallCmd = "if [ -x /usr/bin/tachyon ]; then /usr/bin/tachyon component_action sing_box install_stable 2>&1 || true; else opkg update >/dev/null 2>&1 && opkg install sing-box 2>&1 || true; fi"
		}
	}

	script := fmt.Sprintf(`#!/bin/sh
set -e
echo "==> [1/3] Остановка службы Tachyon..."
if [ -x /etc/init.d/tachyon ]; then
    /etc/init.d/tachyon stop >/dev/null 2>&1 || true
fi
if [ -x /etc/init.d/tachyon-core ]; then
    /etc/init.d/tachyon-core stop >/dev/null 2>&1 || true
fi
if [ -x /etc/init.d/sing-box ]; then
    /etc/init.d/sing-box stop >/dev/null 2>&1 || true
fi
if [ -x /etc/init.d/steer ]; then
    /etc/init.d/steer stop >/dev/null 2>&1 || true
fi
killall -9 tachyon-core sing-box steer 2>/dev/null || true

ENGINE="%s"
VARIANT_MARKER="%s"
if command -v apk >/dev/null 2>&1; then
    PKG_MGR="apk"
else
    PKG_MGR="opkg"
fi

# Pre-clean conflicting engine packages/symlinks before installing
case "$ENGINE" in
    tachyon-core|sing-box-extended-compressed|sing-box-lx)
        if [ "$PKG_MGR" = "apk" ]; then
            apk del sing-box-extended sing-box-tiny sing-box >/dev/null 2>&1 || true
        else
            opkg remove --force-depends sing-box-extended sing-box-tiny sing-box >/dev/null 2>&1 || true
        fi
        ;;
    sing-box-extended|sing-box-tiny|sing-box)
        rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
        if [ "$ENGINE" != "sing-box-extended" ]; then
            if [ "$PKG_MGR" = "apk" ]; then apk del sing-box-extended >/dev/null 2>&1 || true; else opkg remove --force-depends sing-box-extended >/dev/null 2>&1 || true; fi
        fi
        if [ "$ENGINE" != "sing-box-tiny" ]; then
            if [ "$PKG_MGR" = "apk" ]; then apk del sing-box-tiny >/dev/null 2>&1 || true; else opkg remove --force-depends sing-box-tiny >/dev/null 2>&1 || true; fi
        fi
        if [ "$ENGINE" != "sing-box" ]; then
            if [ "$PKG_MGR" = "apk" ]; then apk del sing-box >/dev/null 2>&1 || true; else opkg remove --force-depends sing-box >/dev/null 2>&1 || true; fi
        fi
        ;;
    steer)
        if [ "$PKG_MGR" = "apk" ]; then
            apk del steer-extended >/dev/null 2>&1 || true
        else
            opkg remove --force-depends steer-extended >/dev/null 2>&1 || true
        fi
        ;;
esac

echo "==> [2/3] Обновление файлов ядра (%s)..."
%s

# If tachyon-core or sing-box binary or tar.gz was uploaded
if [ -f /tmp/tachyon-install/tachyon-core ]; then
    rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
    mv -f /tmp/tachyon-install/tachyon-core /usr/bin/tachyon-core
    chmod 0755 /usr/bin/tachyon-core
    ln -sf /usr/bin/tachyon-core /usr/bin/sing-box
elif [ -f /tmp/tachyon-install/sing-box ]; then
    rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
    mv -f /tmp/tachyon-install/sing-box /usr/bin/tachyon-core
    chmod 0755 /usr/bin/tachyon-core
    ln -sf /usr/bin/tachyon-core /usr/bin/sing-box
elif ls /tmp/tachyon-install/tachyon-core*.tar.gz >/dev/null 2>&1; then
    for a in /tmp/tachyon-install/tachyon-core*.tar.gz; do
        tar -xzf "$a" -C /tmp/tachyon-install/ 2>/dev/null || true
        rm -f "$a"
    done
    FOUND=$(find /tmp/tachyon-install/ -type f \( -name "tachyon-core*" -o -name "sing-box" \) ! -name "*.sha256" ! -name "*.txt" ! -name "*.md" ! -name "*.sh" 2>/dev/null | head -1)
    if [ -n "$FOUND" ]; then
        rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
        mv -f "$FOUND" /usr/bin/tachyon-core
        chmod 0755 /usr/bin/tachyon-core
        ln -sf /usr/bin/tachyon-core /usr/bin/sing-box
    fi
elif ls /tmp/tachyon-install/sing-box*.tar.gz >/dev/null 2>&1; then
    for a in /tmp/tachyon-install/sing-box*.tar.gz; do
        tar -xzf "$a" -C /tmp/tachyon-install/ 2>/dev/null || true
        rm -f "$a"
    done
    FOUND=$(find /tmp/tachyon-install/ -type f -name "sing-box*" ! -name "*.tar.gz" ! -name "*.ipk" ! -name "*.apk" ! -name "*.sha256" 2>/dev/null | head -1)
    if [ -n "$FOUND" ]; then
        rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
        mv -f "$FOUND" /usr/bin/tachyon-core
        chmod 0755 /usr/bin/tachyon-core
        ln -sf /usr/bin/tachyon-core /usr/bin/sing-box
    fi
fi

# If IPK or APK packages exist (steer, sing-box-extended, etc.)
if [ "$PKG_MGR" = "apk" ]; then
    ls /tmp/tachyon-install/*.apk >/dev/null 2>&1 && apk add --allow-untrusted --force-overwrite /tmp/tachyon-install/*.apk 2>&1 || true
else
    ls /tmp/tachyon-install/*.ipk >/dev/null 2>&1 && opkg install --force-reinstall --force-overwrite /tmp/tachyon-install/*.ipk 2>&1 || true
fi

# Ensure /usr/bin/tachyon-core and /usr/bin/sing-box symlinks are consistent for package installs
if [ -f /usr/bin/sing-box ] && [ ! -L /usr/bin/sing-box ] && [ ! -e /usr/bin/tachyon-core ]; then
    ln -sf /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
elif [ -f /usr/bin/tachyon-core ] && [ ! -L /usr/bin/tachyon-core ] && [ ! -e /usr/bin/sing-box ]; then
    ln -sf /usr/bin/tachyon-core /usr/bin/sing-box 2>/dev/null || true
fi

# Write variant marker & clear stale cache.db if installing a tachyon-core / sing-box family engine
if [ -n "$VARIANT_MARKER" ]; then
    mkdir -p /etc/tachyon /etc/tachyon-core /tmp/tachyon-core /usr/share/tachyon-core /usr/share/sing-box 2>/dev/null || true
    printf '%%s\n' "$VARIANT_MARKER" > /etc/tachyon/tachyon-core-variant 2>/dev/null || true
    rm -f /tmp/tachyon-core/cache.db* /usr/share/tachyon-core/cache.db* /usr/share/sing-box/cache.db* /etc/tachyon-core/cache.db* /var/run/sing-box/cache.db* 2>/dev/null || true
    if [ -x /etc/init.d/sing-box ]; then
        /etc/init.d/sing-box stop >/dev/null 2>&1 || true
        /etc/init.d/sing-box disable >/dev/null 2>&1 || true
    fi
    if [ ! -x /etc/init.d/tachyon-core ] && command -v ucode >/dev/null 2>&1 && [ -f /usr/lib/tachyon/core/process.uc ]; then
        ucode -L /usr/lib/tachyon -e 'let fs = require("fs"); let cp = require("core.process"); fs.writefile("/etc/init.d/tachyon-core", cp.managed_core_init_script("", true)); fs.chmod("/etc/init.d/tachyon-core", 0755);' 2>/dev/null || true
    fi
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
`, targetEngine, variantMarker, targetEngine, repoInstallCmd, uciEngine, uciEngine)

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
		checkCmd := fmt.Sprintf(`(pgrep -x %s >/dev/null 2>&1 || pgrep -f "%s" >/dev/null 2>&1) && echo RUNNING || echo STOPPED`, uciEngine, uciEngine)
		if uciEngine == "sing-box" || uciEngine == "tachyon-core" {
			checkCmd = `(pgrep -x tachyon-core >/dev/null 2>&1 || pgrep -x sing-box >/dev/null 2>&1 || pgrep -f "tachyon-core" >/dev/null 2>&1 || pgrep -f "sing-box" >/dev/null 2>&1) && echo RUNNING || echo STOPPED`
		}
		out, _ := sess.CombinedOutput(checkCmd)
		if strings.Contains(string(out), "RUNNING") {
			logFn(fmt.Sprintf("[#22c55e]✓ Новое ядро [%s] успешно запущено и активно![-]\n", targetEngine))
		} else {
			logFn(fmt.Sprintf("[#eab308]⚠️ Ядро переключено на [%s], статус: %s[-]\n", targetEngine, strings.TrimSpace(string(out))))
		}
	}

	return nil
}
