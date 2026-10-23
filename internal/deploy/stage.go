package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// StagingArea manages a temporary local directory for storing packages before upload.
type StagingArea struct {
	Dir string
}

// NewStagingArea creates a new temporary staging directory on the host.
func NewStagingArea() (*StagingArea, error) {
	dir, err := os.MkdirTemp("", "tachyon_staging_*")
	if err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	return &StagingArea{Dir: dir}, nil
}

// Cleanup removes the staging directory.
func (s *StagingArea) Cleanup() {
	if s != nil && s.Dir != "" {
		_ = os.RemoveAll(s.Dir)
	}
}

// VariantMarkerForEngine maps an installer engine selection to the Tachyon
// /etc/tachyon/tachyon-core-variant marker value (used by Tachyon 1.5.20+ for
// AmneziaWG schema and TLS feature detection).
func VariantMarkerForEngine(selectedEngine string) string {
	switch selectedEngine {
	case "tachyon-core":
		return "tachyon-core"
	case "sing-box-extended":
		return "extended"
	case "sing-box-extended-compressed":
		return "extended-compressed"
	case "sing-box-lx":
		return "lx"
	case "sing-box-tiny":
		return "tiny"
	case "sing-box":
		return "stable"
	default:
		return ""
	}
}

// GenerateRunnerScript generates the shell script to be executed on the OpenWrt router.
func GenerateRunnerScript(selectedEngine string, isAPK bool, installZRAM ...bool) string {
	withZRAM := false
	if len(installZRAM) > 0 && installZRAM[0] {
		withZRAM = true
	}

	zramSection := ""
	if withZRAM {
		zramSection = `
echo "==> Настройка zRAM-swap (защита от OOM)..."
if [ "$PKG_MGR" = "apk" ]; then
    apk add zram-swap 2>&1 || true
else
    opkg update >/dev/null 2>&1 || true
    opkg install zram-swap 2>&1 || true
fi
if [ -x /etc/init.d/zram ]; then
    /etc/init.d/zram enable >/dev/null 2>&1 || true
    /etc/init.d/zram restart >/dev/null 2>&1 || true
fi
`
	}

	variantMarker := VariantMarkerForEngine(selectedEngine)

	script := `#!/bin/sh
# Tachyon Host-Assisted Express Installer
set -e

echo "==> [1/5] Подготовка системы и остановка служб..."
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
if [ -x /usr/bin/tachyon ]; then
    /usr/bin/tachyon stop >/dev/null 2>&1 || true
fi
killall -9 tachyon-core sing-box steer 2>/dev/null || true

# Determine package manager
if command -v apk >/dev/null 2>&1; then
    PKG_MGR="apk"
else
    PKG_MGR="opkg"
fi
` + zramSection + `
ENGINE="` + selectedEngine + `"
VARIANT_MARKER="` + variantMarker + `"

# Pre-clean conflicting engine packages/symlinks before installing packages
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

echo "==> [2/5] Установка пакетов Tachyon..."
if [ "$PKG_MGR" = "apk" ]; then
    # Install local APK packages
    apk add --allow-untrusted --force-overwrite /tmp/tachyon-install/*.apk 2>&1 || {
        echo "Внимание: обновление репозиториев для доустановки зависимостей..."
        apk update >/dev/null 2>&1 || true
        apk add --allow-untrusted --force-overwrite /tmp/tachyon-install/*.apk
    }
else
    # Install local IPK packages
    opkg install --force-reinstall --force-overwrite /tmp/tachyon-install/*.ipk 2>&1 || {
        echo "Внимание: обновление opkg для доустановки зависимостей..."
        opkg update >/dev/null 2>&1 || true
        opkg install --force-reinstall --force-overwrite /tmp/tachyon-install/*.ipk
    }
fi

# If standalone tachyon-core or sing-box binary was uploaded (or compressed archive)
if [ -f /tmp/tachyon-install/tachyon-core ]; then
    echo "==> Установка бинарного файла ядра tachyon-core..."
    rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
    mv -f /tmp/tachyon-install/tachyon-core /usr/bin/tachyon-core
    chmod 0755 /usr/bin/tachyon-core
    ln -sf /usr/bin/tachyon-core /usr/bin/sing-box
elif [ -f /tmp/tachyon-install/sing-box ]; then
    echo "==> Установка бинарного файла ядра ($ENGINE)..."
    rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
    mv -f /tmp/tachyon-install/sing-box /usr/bin/tachyon-core
    chmod 0755 /usr/bin/tachyon-core
    ln -sf /usr/bin/tachyon-core /usr/bin/sing-box
elif ls /tmp/tachyon-install/tachyon-core*.tar.gz >/dev/null 2>&1; then
    echo "==> Распаковка архива ядра tachyon-core..."
    for archive in /tmp/tachyon-install/tachyon-core*.tar.gz; do
        tar -xzf "$archive" -C /tmp/tachyon-install/ 2>/dev/null || tar -xf "$archive" -C /tmp/tachyon-install/ 2>/dev/null || true
        rm -f "$archive"
    done
    FOUND_BIN=$(find /tmp/tachyon-install/ -type f \( -name "tachyon-core*" -o -name "sing-box" \) ! -name "*.sha256" ! -name "*.txt" ! -name "*.md" ! -name "*.sh" 2>/dev/null | head -1)
    if [ -n "$FOUND_BIN" ]; then
        rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
        mv -f "$FOUND_BIN" /usr/bin/tachyon-core
        chmod 0755 /usr/bin/tachyon-core
        ln -sf /usr/bin/tachyon-core /usr/bin/sing-box
    fi
elif ls /tmp/tachyon-install/sing-box*.tar.gz >/dev/null 2>&1; then
    echo "==> Распаковка архива ядра sing-box..."
    for archive in /tmp/tachyon-install/sing-box*.tar.gz; do
        tar -xzf "$archive" -C /tmp/tachyon-install/ 2>/dev/null || true
        rm -f "$archive"
    done
    FOUND_BIN=$(find /tmp/tachyon-install/ -type f -name "sing-box*" ! -name "*.tar.gz" ! -name "*.ipk" ! -name "*.apk" ! -name "*.sha256" 2>/dev/null | head -1)
    if [ -n "$FOUND_BIN" ]; then
        rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
        mv -f "$FOUND_BIN" /usr/bin/tachyon-core
        chmod 0755 /usr/bin/tachyon-core
        ln -sf /usr/bin/tachyon-core /usr/bin/sing-box
    fi
elif [ "` + selectedEngine + `" = "sing-box-tiny" ]; then
    echo "==> Установка sing-box-tiny через пакетный менеджер роутера..."
    if [ -x /usr/bin/tachyon ]; then
        /usr/bin/tachyon component_action sing_box install_tiny 2>&1 || true
    elif [ "$PKG_MGR" = "apk" ]; then
        apk update >/dev/null 2>&1 || true
        apk add sing-box-tiny 2>&1 || true
    else
        opkg update >/dev/null 2>&1 || true
        opkg install sing-box-tiny 2>&1 || true
    fi
elif [ "` + selectedEngine + `" = "sing-box" ]; then
    echo "==> Установка sing-box (stable) через пакетный менеджер роутера..."
    if [ -x /usr/bin/tachyon ]; then
        /usr/bin/tachyon component_action sing_box install_stable 2>&1 || true
    elif [ "$PKG_MGR" = "apk" ]; then
        apk update >/dev/null 2>&1 || true
        apk add sing-box 2>&1 || true
    else
        opkg update >/dev/null 2>&1 || true
        opkg install sing-box 2>&1 || true
    fi
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
    printf '%s\n' "$VARIANT_MARKER" > /etc/tachyon/tachyon-core-variant 2>/dev/null || true
    rm -f /tmp/tachyon-core/cache.db* /usr/share/tachyon-core/cache.db* /usr/share/sing-box/cache.db* /etc/tachyon-core/cache.db* /var/run/sing-box/cache.db* 2>/dev/null || true
    if [ -x /etc/init.d/sing-box ]; then
        /etc/init.d/sing-box stop >/dev/null 2>&1 || true
        /etc/init.d/sing-box disable >/dev/null 2>&1 || true
    fi
    if [ ! -x /etc/init.d/tachyon-core ] && command -v ucode >/dev/null 2>&1 && [ -f /usr/lib/tachyon/core/process.uc ]; then
        ucode -L /usr/lib/tachyon -e 'let fs = require("fs"); let cp = require("core.process"); fs.writefile("/etc/init.d/tachyon-core", cp.managed_core_init_script("", true)); fs.chmod("/etc/init.d/tachyon-core", 0755);' 2>/dev/null || true
    fi
fi

# Ensure /usr/bin/steer symlink exists if steer is in /usr/sbin/steer (Steer 2.0+)
if [ -x /usr/sbin/steer ] && [ ! -e /usr/bin/steer ]; then
    ln -sf /usr/sbin/steer /usr/bin/steer 2>/dev/null || true
fi

if [ -n "$ENGINE" ] && [ "$ENGINE" != "skip" ]; then
    echo "==> [3/5] Настройка активного ядра в UCI ($ENGINE)..."
    case "$ENGINE" in
        tachyon-core|sing-box*|extended*|tiny|lx)
            uci -q set tachyon.settings.engine="sing-box"
            if [ -x /etc/init.d/steer ]; then
                /etc/init.d/steer stop >/dev/null 2>&1 || true
                /etc/init.d/steer disable >/dev/null 2>&1 || true
            fi
            ;;
        steer*)
            uci -q set tachyon.settings.engine="$ENGINE"
            ;;
        *)
            uci -q set tachyon.settings.engine="$ENGINE"
            ;;
    esac
    uci commit tachyon 2>/dev/null || true
fi

echo "==> [4/5] Очистка кэша веб-интерфейса LuCI..."
rm -rf /tmp/luci-modulecache/ /tmp/luci-indexcache* 2>/dev/null || true

echo "==> [5/5] Активация и запуск службы Tachyon..."
/etc/init.d/tachyon enable >/dev/null 2>&1 || true
/etc/init.d/tachyon restart 2>&1 || /usr/bin/tachyon start 2>&1 || true

echo "==> Экспресс-установка на роутере успешно завершена!"
`

	// Ensure UNIX line endings (LF)
	script = strings.ReplaceAll(script, "\r\n", "\n")
	return script
}

// WriteRunnerScript writes the runner script to the staging directory.
func (s *StagingArea) WriteRunnerScript(selectedEngine string, isAPK bool, installZRAM ...bool) (string, error) {
	content := GenerateRunnerScript(selectedEngine, isAPK, installZRAM...)
	path := filepath.Join(s.Dir, "install_on_router.sh")
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return "", err
	}
	return path, nil
}

// GenerateUninstallScript creates a clean uninstaller shell script.
func GenerateUninstallScript(isAPK bool) string {
	script := `#!/bin/sh
# Tachyon Clean Complete Uninstaller
set -e

echo "==> [1/4] Остановка служб Tachyon..."
if [ -x /etc/init.d/tachyon ]; then
    /etc/init.d/tachyon stop >/dev/null 2>&1 || true
    /etc/init.d/tachyon disable >/dev/null 2>&1 || true
fi
if [ -x /etc/init.d/tachyon-core ]; then
    /etc/init.d/tachyon-core stop >/dev/null 2>&1 || true
    /etc/init.d/tachyon-core disable >/dev/null 2>&1 || true
fi
killall -9 tachyon tachyon-core sing-box steer 2>/dev/null || true
nft delete table inet TachyonTable 2>/dev/null || true
nft delete table inet tachyon 2>/dev/null || true
nft delete table inet steer 2>/dev/null || true

echo "==> [2/4] Удаление пакетов Tachyon..."
if command -v apk >/dev/null 2>&1; then
    apk del luci-app-tachyon luci-i18n-tachyon-ru tachyon 2>&1 || true
else
    opkg remove --autoremove luci-app-tachyon luci-i18n-tachyon-ru tachyon 2>&1 || true
fi

echo "==> [3/4] Очистка конфигурации и временных файлов..."
rm -rf /etc/config/tachyon /etc/tachyon /etc/tachyon-core /etc/init.d/tachyon-core /usr/bin/tachyon /usr/bin/tachyon-core /usr/sbin/steer /tmp/tachyon-install /tmp/tachyon-core /tmp/luci-modulecache/ /tmp/luci-indexcache* 2>/dev/null || true

echo "==> [4/4] Сброс сетевых правил и перезапуск firewall..."
if [ -x /etc/init.d/firewall ]; then
    /etc/init.d/firewall restart >/dev/null 2>&1 || true
fi

echo "==> Tachyon успешно удален с роутера."
`
	return strings.ReplaceAll(script, "\r\n", "\n")
}

// UninstallTachyon executes clean removal of Tachyon on the remote router.
func UninstallTachyon(client *gossh.Client, isAPK bool) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session: %w", err)
	}
	defer sess.Close()

	script := GenerateUninstallScript(isAPK)
	cmd := fmt.Sprintf("sh -c '%s'", strings.ReplaceAll(script, "'", "'\\''"))
	out, err := sess.CombinedOutput(cmd)
	if err != nil {
		return string(out), fmt.Errorf("uninstall failed: %w", err)
	}
	return string(out), nil
}
