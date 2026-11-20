package router

import (
	"fmt"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// RescueReport summarizes the outcome of emergency network rescue.
type RescueReport struct {
	Success        bool
	ActionsTaken   []string
	NetworkRestored bool
	GatewayPing    bool
	InternetPing   bool
	Detail         string
}

// EmergencyRescue cleans all proxy routing, nftables/iptables interception rules,
// restores dnsmasq and restarts the firewall to restore clean native internet access.
func EmergencyRescue(client *gossh.Client, execFn sshutil.ExecFunc) RescueReport {
	report := RescueReport{
		ActionsTaken: []string{},
	}

	rescueScript := `
# 1. Release stale locks and stop proxy services
rm -rf /var/run/tachyon/starting /var/run/tachyon/reloading /var/run/tachyon*.lock 2>/dev/null || true
for _p in $(pgrep -f "99-tachyon-wan" 2>/dev/null || true); do kill -9 "$_p" 2>/dev/null || true; done
for _svc in tachyon tachyon-core sing-box; do
    if [ -x "/etc/init.d/$_svc" ]; then
        "/etc/init.d/$_svc" stop >/dev/null 2>&1 || true
    fi
done
killall -9 tachyon-core sing-box nfqws nfqws2 2>/dev/null || true

# 2. Flush and delete nftables rules (fw4)
if command -v nft >/dev/null 2>&1; then
    nft delete table inet TachyonTable >/dev/null 2>&1 || true
    nft delete table inet TachyonKillSwitch >/dev/null 2>&1 || true
    nft delete table inet tachyon_table >/dev/null 2>&1 || true
    nft delete table inet tachyon >/dev/null 2>&1 || true
    nft delete table inet tachyon_tailscale >/dev/null 2>&1 || true
    nft delete table ip tachyon >/dev/null 2>&1 || true
    nft delete table ip6 tachyon >/dev/null 2>&1 || true
fi
rm -f /usr/share/nftables.d/ruleset-post/90-tachyon-kill-switch.nft /usr/share/nftables.d/chain-pre/input/10-tachyon.nft 2>/dev/null || true

# 3. Flush legacy iptables rules (fw3)
if command -v iptables >/dev/null 2>&1; then
    iptables -t nat -F TACHYON >/dev/null 2>&1 || true
    iptables -t mangle -F TACHYON >/dev/null 2>&1 || true
    iptables -t nat -D PREROUTING -j TACHYON >/dev/null 2>&1 || true
    iptables -t mangle -D PREROUTING -j TACHYON >/dev/null 2>&1 || true
fi

# 4. Clean TProxy and policy routing (tables tachyon/105, 100, Tailscale 4247/4248, 2.0 TUN kill-switch pref 9499-9501)
for _ipt in "ip" "ip -6"; do
    for _p in 9499 9500 9501; do
        while $_ipt rule del pref "$_p" 2>/dev/null; do :; done
    done
    while $_ipt rule del fwmark 0x04000000/0x04000000 lookup tachyon 2>/dev/null; do :; done
    while $_ipt rule del fwmark 0x04000000/0x04000000 lookup 105 2>/dev/null; do :; done
    while $_ipt rule del fwmark 0x10000000/0x10000000 lookup 100 2>/dev/null; do :; done
    while $_ipt rule del fwmark 0x1/0x1 table 100 2>/dev/null; do :; done
    while $_ipt rule del lookup tachyon 2>/dev/null; do :; done
    while $_ipt rule del lookup 105 2>/dev/null; do :; done
    while $_ipt rule del lookup 100 2>/dev/null; do :; done
    while $_ipt rule del lookup 4247 2>/dev/null; do :; done
    while $_ipt rule del lookup 4248 2>/dev/null; do :; done
    for _tbl in tachyon 105 100 4247 4248; do
        $_ipt route flush table "$_tbl" 2>/dev/null || true
    done
done
ip netns del fkpsc 2>/dev/null || true
ip link del fkpsc0 2>/dev/null || true

# 5. Restore dnsmasq backups if present & restart standard firewall + dnsmasq
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
fi
/etc/init.d/dnsmasq restart >/dev/null 2>&1 || true
/etc/init.d/firewall restart >/dev/null 2>&1 || true

# 6. Verify native connectivity
GW_OK="0"
NET_OK="0"
GW=$(ip route show default 2>/dev/null | awk '/default/ {print $3}' | head -n 1)
if [ -n "$GW" ]; then
    ping -c 1 -W 2 "$GW" >/dev/null 2>&1 && GW_OK="1" || true
fi
ping -c 1 -W 3 8.8.8.8 >/dev/null 2>&1 && NET_OK="1" || true

echo "RESCUE_DONE:$GW_OK:$NET_OK"
`
	out, err := execFn(client, rescueScript)
	if err != nil {
		report.Success = false
		report.Detail = fmt.Sprintf("Ошибка выполнения rescue скрипта: %v", err)
		return report
	}

	report.ActionsTaken = append(report.ActionsTaken,
		"Остановлены службы Tachyon и tachyon-core",
		"Сброшены таблицы nftables (TachyonTable / TachyonKillSwitch / tachyon_tailscale)",
		"Очищены маршруты TProxy (таблицы tachyon/105, 100, 4247/4248)",
		"Восстановлены настройки dnsmasq и перезапущен DNS-резолвер",
		"Перезапущен системный межсетевой экран (firewall)",
	)

	report.Success = true

	// Parse ping results
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "RESCUE_DONE:") {
			parts := strings.Split(strings.TrimPrefix(l, "RESCUE_DONE:"), ":")
			if len(parts) >= 2 {
				report.GatewayPing = (parts[0] == "1")
				report.InternetPing = (parts[1] == "1")
			}
		}
	}

	if report.InternetPing {
		report.NetworkRestored = true
		report.Detail = "Сетевые правила сброшены. Прямой доступ в интернет (8.8.8.8) успешно восстановлен!"
	} else if report.GatewayPing {
		report.NetworkRestored = true
		report.Detail = "Правила перехвата сброшены. Шлюз роутера отвечает, ожидается поднятие WAN соединения."
	} else {
		report.Detail = "Правила перехвата и служба Tachyon сброшены. Проверьте физический кабель провайдера (WAN)."
	}

	return report
}
