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
# Release stale procd/tachyon locks
rm -rf /var/run/tachyon/starting /var/run/tachyon/reloading /var/run/tachyon*.lock 2>/dev/null || true
for _p in $(pgrep -f "99-tachyon-wan" 2>/dev/null || true); do kill -9 "$_p" 2>/dev/null || true; done

if [ -x /etc/init.d/tachyon ]; then
    /etc/init.d/tachyon stop >/dev/null 2>&1 || true
fi
if [ -x /etc/init.d/tachyon-core ]; then
    /etc/init.d/tachyon-core stop >/dev/null 2>&1 || true
fi
if [ -x /etc/init.d/sing-box ]; then
    /etc/init.d/sing-box stop >/dev/null 2>&1 || true
fi
if [ -x /usr/bin/tachyon ]; then
    /usr/bin/tachyon stop >/dev/null 2>&1 || true
fi
killall -9 tachyon-core sing-box nfqws nfqws2 2>/dev/null || true

# Determine package manager
if command -v apk >/dev/null 2>&1; then
    PKG_MGR="apk"
else
    PKG_MGR="opkg"
fi

# OpenWrt 25.12+: release ip-tiny conflict so ip-full can install cleanly
if [ "$PKG_MGR" = "apk" ]; then
    if [ -f /etc/apk/world ]; then
        sed -i -E "/^ip-tiny([ \t<>=~].*)?\$/d" /etc/apk/world 2>/dev/null || true
    fi
    apk del --no-network --repositories-file /dev/null --force-broken-world ip-tiny >/dev/null 2>&1 || true
else
    opkg remove --force-depends ip-tiny >/dev/null 2>&1 || true
fi
` + zramSection + `
ENGINE="` + selectedEngine + `"
VARIANT_MARKER="` + variantMarker + `"

# Pre-clean conflicting engine packages/symlinks before installing packages
case "$ENGINE" in
    tachyon-core|sing-box-extended-compressed|sing-box-lx)
        if [ "$PKG_MGR" = "apk" ]; then
            apk del --no-network --repositories-file /dev/null --force-broken-world sing-box-extended sing-box-tiny sing-box >/dev/null 2>&1 || true
        else
            opkg remove --force-depends sing-box-extended sing-box-tiny sing-box >/dev/null 2>&1 || true
        fi
        ;;
    sing-box-extended|sing-box-tiny|sing-box)
        rm -f /usr/bin/sing-box /usr/bin/tachyon-core 2>/dev/null || true
        if [ "$ENGINE" != "sing-box-extended" ]; then
            if [ "$PKG_MGR" = "apk" ]; then apk del --no-network --repositories-file /dev/null --force-broken-world sing-box-extended >/dev/null 2>&1 || true; else opkg remove --force-depends sing-box-extended >/dev/null 2>&1 || true; fi
        fi
        if [ "$ENGINE" != "sing-box-tiny" ]; then
            if [ "$PKG_MGR" = "apk" ]; then apk del --no-network --repositories-file /dev/null --force-broken-world sing-box-tiny >/dev/null 2>&1 || true; else opkg remove --force-depends sing-box-tiny >/dev/null 2>&1 || true; fi
        fi
        if [ "$ENGINE" != "sing-box" ]; then
            if [ "$PKG_MGR" = "apk" ]; then apk del --no-network --repositories-file /dev/null --force-broken-world sing-box >/dev/null 2>&1 || true; else opkg remove --force-depends sing-box >/dev/null 2>&1 || true; fi
        fi
        ;;
esac

echo "==> [2/5] Установка пакетов Tachyon..."
if [ "$PKG_MGR" = "apk" ]; then
    # Scrub legacy packages and pinned tachyon hash constraints from /etc/apk/world
    if [ -f /etc/apk/world ]; then
        for _lp in forkop luci-app-forkop podkop luci-app-podkop forkop_plus luci-app-forkop_plus podkop_plus luci-app-podkop_plus netshift luci-app-netshift; do
            sed -i -E "/^${_lp}([ \t<>=~].*)?\$/d" /etc/apk/world 2>/dev/null || true
        done
        sed -i -E "/^tachyon([ \t<>=~].*)?\$/d; /^luci-app-tachyon([ \t<>=~].*)?\$/d; /^luci-i18n-tachyon-ru([ \t<>=~].*)?\$/d" /etc/apk/world 2>/dev/null || true
    fi
    # Ensure runtime dependencies & kmods (skipping built-in kernel modules)
    for _kmod in kmod-nft-tproxy:nft_tproxy kmod-tun:tun kmod-inet-diag:inet_diag kmod-netlink-diag:netlink_diag kmod-nft-nat:nft_nat; do
        _pkg="${_kmod%:*}"
        _mod="${_kmod#*:}"
        if [ ! -d "/sys/module/$_mod" ] && ! modprobe "$_mod" >/dev/null 2>&1; then
            apk info -e "$_pkg" >/dev/null 2>&1 || apk add --force-missing-repositories "$_pkg" >/dev/null 2>&1 || true
        fi
    done
    # Try offline local APK install first, fallback to repo-assisted or virtual dep healing
    apk add --allow-untrusted --force-overwrite --no-network --repositories-file /dev/null /tmp/tachyon-install/*.apk 2>&1 || \
    apk add --allow-untrusted --force-overwrite /tmp/tachyon-install/*.apk 2>&1 || {
        echo "Обновление репозиториев apk для доустановки зависимостей..."
        apk update --force-missing-repositories >/dev/null 2>&1 || true
        for _dep in ca-bundle curl coreutils-base64 coreutils-timeout bind-dig nftables ip-full luci-base rpcd-mod-luci; do
            apk info -e "$_dep" >/dev/null 2>&1 || apk add --force-missing-repositories "$_dep" >/dev/null 2>&1 || \
            apk add --no-network --repositories-file /dev/null --force-non-repository --virtual "$_dep" >/dev/null 2>&1 || true
        done
        apk add --allow-untrusted --force-overwrite --no-network --repositories-file /dev/null /tmp/tachyon-install/*.apk 2>&1 || \
        apk add --allow-untrusted --force-overwrite --force-broken-world /tmp/tachyon-install/*.apk
    }
else
    # Ensure runtime kmods if not built-in
    for _kmod in kmod-nft-tproxy:nft_tproxy kmod-tun:tun kmod-inet-diag:inet_diag kmod-netlink-diag:netlink_diag kmod-nft-nat:nft_nat; do
        _pkg="${_kmod%:*}"
        _mod="${_kmod#*:}"
        if [ ! -d "/sys/module/$_mod" ] && ! modprobe "$_mod" >/dev/null 2>&1; then
            opkg status "$_pkg" 2>/dev/null | grep -q "installed" || opkg install "$_pkg" >/dev/null 2>&1 || true
        fi
    done
    # Install local IPK packages
    opkg install --force-reinstall --force-overwrite /tmp/tachyon-install/*.ipk 2>&1 || {
        echo "Обновление opkg для доустановки зависимостей..."
        opkg update >/dev/null 2>&1 || true
        opkg install --force-reinstall --force-overwrite /tmp/tachyon-install/*.ipk || \
        opkg install --force-reinstall --force-overwrite --force-depends /tmp/tachyon-install/*.ipk
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
    if [ "$VARIANT_MARKER" = "tachyon-core" ] && [ -x /usr/bin/tachyon-core ]; then
        _tc_ver=$(/usr/bin/tachyon-core --version 2>/dev/null | head -n 1 | awk '{print $NF}')
        [ -n "$_tc_ver" ] && printf '%s\n' "$_tc_ver" > /etc/tachyon/tachyon-core-version 2>/dev/null || true
    fi
    rm -f /tmp/tachyon-core/cache.db* /usr/share/tachyon-core/cache.db* /usr/share/sing-box/cache.db* /etc/tachyon-core/cache.db* /var/run/sing-box/cache.db* 2>/dev/null || true
    if [ -x /etc/init.d/sing-box ]; then
        /etc/init.d/sing-box stop >/dev/null 2>&1 || true
        /etc/init.d/sing-box disable >/dev/null 2>&1 || true
    fi
    if [ ! -x /etc/init.d/tachyon-core ] && command -v ucode >/dev/null 2>&1 && [ -f /usr/lib/tachyon/core/process.uc ]; then
        ucode -L /usr/lib/tachyon -e 'let fs = require("fs"); let cp = require("core.process"); fs.writefile("/etc/init.d/tachyon-core", cp.managed_core_init_script("", true)); fs.chmod("/etc/init.d/tachyon-core", 0755);' 2>/dev/null || true
    fi
fi

if [ -n "$ENGINE" ] && [ "$ENGINE" != "skip" ]; then
    echo "==> [3/5] Настройка активного ядра в UCI ($ENGINE)..."
    case "$ENGINE" in
        tachyon-core)
            uci -q set tachyon.settings.engine="tachyon-core"
            uci -q set tachyon.settings.subscription_engine="core"
            ;;
        sing-box*|extended*|tiny|lx)
            uci -q set tachyon.settings.engine="sing-box"
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

echo "==> Установка на роутере завершена."
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

// GenerateUninstallScript creates a clean uninstaller shell script synced with Tachyon uninstall.sh v1.3.0.
func GenerateUninstallScript(isAPK bool) string {
	script := `#!/bin/sh
# Tachyon Clean Complete Uninstaller (v1.3.0 sync)
set -e

echo "==> [1/4] Остановка служб Tachyon и освобождение блокировок..."
rm -rf /var/run/tachyon/starting /var/run/tachyon/reloading /var/run/tachyon*.lock 2>/dev/null || true
for _p in $(pgrep -f "99-tachyon-wan" 2>/dev/null || true); do kill -9 "$_p" 2>/dev/null || true; done

for _svc in tachyon tachyon-core sing-box; do
    if [ -x "/etc/init.d/$_svc" ]; then
        "/etc/init.d/$_svc" stop >/dev/null 2>&1 || true
        "/etc/init.d/$_svc" disable >/dev/null 2>&1 || true
    fi
done
killall -9 tachyon tachyon-core sing-box nfqws nfqws2 2>/dev/null || true

# Clean nftables tables
nft delete table inet TachyonTable 2>/dev/null || true
nft delete table inet tachyon 2>/dev/null || true
nft delete table inet tachyon_tailscale 2>/dev/null || true

# Clean policy routing rules and tables (105 / tachyon, 100, Tailscale 4247/4248)
for _ipt in "ip" "ip -6"; do
    while $_ipt rule del fwmark 0x04000000/0x04000000 lookup tachyon 2>/dev/null; do :; done
    while $_ipt rule del fwmark 0x04000000/0x04000000 lookup 105 2>/dev/null; do :; done
    while $_ipt rule del fwmark 0x10000000/0x10000000 lookup 100 2>/dev/null; do :; done
    while $_ipt rule del lookup tachyon 2>/dev/null; do :; done
    while $_ipt rule del lookup 105 2>/dev/null; do :; done
    while $_ipt rule del lookup 100 2>/dev/null; do :; done
    while $_ipt rule del lookup 4247 2>/dev/null; do :; done
    while $_ipt rule del lookup 4248 2>/dev/null; do :; done
    for _tbl in tachyon 105 100 4247 4248; do
        $_ipt route flush table "$_tbl" 2>/dev/null || true
    done
done
if [ -f /etc/iproute2/rt_tables ]; then
    sed -i '/^[[:space:]]*105[[:space:]]\+tachyon$/d' /etc/iproute2/rt_tables 2>/dev/null || true
fi
ip netns del fkpsc 2>/dev/null || true
ip link del fkpsc0 2>/dev/null || true

# Restore dnsmasq backups created by Tachyon
if command -v uci >/dev/null 2>&1; then
    if uci -q get dhcp.@dnsmasq[0].tachyon_server >/dev/null 2>&1; then
        uci -q delete dhcp.@dnsmasq[0].server 2>/dev/null || true
        for _srv in $(uci -q get dhcp.@dnsmasq[0].tachyon_server); do
            uci -q add_list dhcp.@dnsmasq[0].server="$_srv" 2>/dev/null || true
        done
        uci -q delete dhcp.@dnsmasq[0].tachyon_server 2>/dev/null || true
    fi
    for _opt in noresolv cachesize rebind_protection localuse; do
        _bak=$(uci -q get "dhcp.@dnsmasq[0].tachyon_${_opt}" 2>/dev/null || true)
        if [ -n "$_bak" ]; then
            if [ "$_bak" = "__unset__" ]; then
                uci -q delete "dhcp.@dnsmasq[0].${_opt}" 2>/dev/null || true
            else
                uci -q set "dhcp.@dnsmasq[0].${_opt}=${_bak}" 2>/dev/null || true
            fi
            uci -q delete "dhcp.@dnsmasq[0].tachyon_${_opt}" 2>/dev/null || true
        fi
    done
    uci -q del_list dhcp.@dnsmasq[0].addn_hosts='/tmp/tachyon/hosts' 2>/dev/null || true
    uci -q commit dhcp 2>/dev/null || true
    /etc/init.d/dnsmasq restart >/dev/null 2>&1 || true
fi

# Clean crontabs
if [ -f /etc/crontabs/root ]; then
    sed -i -E '/tachyon|parental_quota_tick/d' /etc/crontabs/root 2>/dev/null || true
    /etc/init.d/cron reload >/dev/null 2>&1 || true
fi

echo "==> [2/4] Удаление пакетов Tachyon..."
if command -v apk >/dev/null 2>&1; then
    if [ -f /etc/apk/world ]; then
        sed -i -E "/^(luci-app-tachyon|luci-i18n-tachyon-ru|tachyon)([ \t<>=~].*)?\$/d" /etc/apk/world 2>/dev/null || true
    fi
    apk del --no-network --repositories-file /dev/null --force-broken-world luci-app-tachyon luci-i18n-tachyon-ru tachyon 2>&1 || true
else
    opkg remove --force-removal-of-dependent-packages luci-app-tachyon luci-i18n-tachyon-ru tachyon 2>&1 || true
fi

echo "==> [3/4] Очистка конфигурации и временных файлов..."
rm -rf /etc/config/tachyon /etc/tachyon /etc/tachyon-core /etc/.tachyon /etc/init.d/tachyon /etc/init.d/tachyon-core /etc/hotplug.d/iface/99-tachyon-wan /usr/bin/tachyon /usr/bin/tachyon-core /usr/lib/tachyon /usr/share/tachyon-core /tmp/tachyon* /var/run/tachyon /tmp/luci-modulecache/ /tmp/luci-indexcache* 2>/dev/null || true

echo "==> [4/4] Сброс сетевых правил и перезапуск firewall..."
if [ -x /etc/init.d/firewall ]; then
    /etc/init.d/firewall restart >/dev/null 2>&1 || true
fi

echo "==> Tachyon удален с роутера."
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
