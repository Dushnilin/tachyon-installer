package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// GenerateRunnerScript generates the shell script to be executed on the OpenWrt router.
func GenerateRunnerScript(selectedEngine string, isAPK bool) string {
	script := `#!/bin/sh
# Tachyon Host-Assisted Express Installer
set -e

echo "==> [1/5] Подготовка системы и остановка служб..."
if [ -x /etc/init.d/tachyon ]; then
    /etc/init.d/tachyon stop >/dev/null 2>&1 || true
fi
if [ -x /usr/bin/tachyon ]; then
    /usr/bin/tachyon stop >/dev/null 2>&1 || true
fi

# Determine package manager
if command -v apk >/dev/null 2>&1; then
    PKG_MGR="apk"
else
    PKG_MGR="opkg"
fi

echo "==> [2/5] Установка пакетов Tachyon..."
if [ "$PKG_MGR" = "apk" ]; then
    # Install local APK packages
    apk add --allow-untrusted /tmp/tachyon-install/*.apk 2>&1 || {
        echo "Внимание: обновление репозиториев для доустановки зависимостей..."
        apk update >/dev/null 2>&1 || true
        apk add --allow-untrusted /tmp/tachyon-install/*.apk
    }
else
    # Install local IPK packages
    opkg install --force-reinstall /tmp/tachyon-install/*.ipk 2>&1 || {
        echo "Внимание: обновление opkg для доустановки зависимостей..."
        opkg update >/dev/null 2>&1 || true
        opkg install --force-reinstall /tmp/tachyon-install/*.ipk
    }
fi

# If standalone sing-box binary was uploaded
if [ -f /tmp/tachyon-install/sing-box ]; then
    echo "==> Установка бинарного файла ядра sing-box..."
    mv -f /tmp/tachyon-install/sing-box /usr/bin/sing-box
    chmod 0755 /usr/bin/sing-box
fi

ENGINE="` + selectedEngine + `"
if [ -n "$ENGINE" ] && [ "$ENGINE" != "skip" ]; then
    echo "==> [3/5] Настройка активного ядра в UCI ($ENGINE)..."
    uci -q set tachyon.settings.engine="$ENGINE"
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
func (s *StagingArea) WriteRunnerScript(selectedEngine string, isAPK bool) (string, error) {
	content := GenerateRunnerScript(selectedEngine, isAPK)
	path := filepath.Join(s.Dir, "install_on_router.sh")
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return "", err
	}
	return path, nil
}
