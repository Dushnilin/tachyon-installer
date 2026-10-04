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
# 1. Stop proxy services
if [ -x /etc/init.d/tachyon ]; then
    /etc/init.d/tachyon stop >/dev/null 2>&1 || true
fi
killall -9 sing-box steer 2>/dev/null || true

# 2. Flush and delete nftables rules (fw4)
if command -v nft >/dev/null 2>&1; then
    nft delete table inet tachyon >/dev/null 2>&1 || true
    nft delete table ip tachyon >/dev/null 2>&1 || true
    nft delete table ip6 tachyon >/dev/null 2>&1 || true
fi

# 3. Flush legacy iptables rules (fw3)
if command -v iptables >/dev/null 2>&1; then
    iptables -t nat -F TACHYON >/dev/null 2>&1 || true
    iptables -t mangle -F TACHYON >/dev/null 2>&1 || true
    iptables -t nat -D PREROUTING -j TACHYON >/dev/null 2>&1 || true
    iptables -t mangle -D PREROUTING -j TACHYON >/dev/null 2>&1 || true
fi

# 4. Clean TProxy policy routing
ip rule del fwmark 0x1/0x1 table 100 >/dev/null 2>&1 || true
ip route flush table 100 >/dev/null 2>&1 || true
ip -6 rule del fwmark 0x1/0x1 table 100 >/dev/null 2>&1 || true
ip -6 route flush table 100 >/dev/null 2>&1 || true

# 5. Restart standard firewall & dnsmasq
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
		"Остановлена служба Tachyon и процессы sing-box/steer",
		"Сброшены таблицы nftables (inet tachyon)",
		"Очищены правила iptables TACHYON",
		"Удалены маршруты TProxy (table 100)",
		"Перезапущены стандартные dnsmasq и firewall",
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
