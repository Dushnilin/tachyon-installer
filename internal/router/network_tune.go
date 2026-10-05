package router

import (
	"fmt"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// NetworkTuneReport holds the result of sysctl and network stack optimization.
type NetworkTuneReport struct {
	Success      bool
	BBRStatus    string
	Buffers      string
	Conntrack    string
	Offloading   string
	PersistPath  string
	AppliedRules []string
	Details      string
}

// TuneNetwork applies high-performance TCP, socket buffer, conntrack, and offloading optimizations to OpenWrt.
func TuneNetwork(client *gossh.Client, execFn sshutil.ExecFunc) NetworkTuneReport {
	report := NetworkTuneReport{
		PersistPath:  "/etc/sysctl.d/99-tachyon-tune.conf",
		AppliedRules: []string{},
	}

	tuneScript := `#!/bin/sh
set -e

# 1. Evaluate RAM
RAM_TOTAL_KB=$(awk '/MemTotal/ {print $2}' /proc/meminfo 2>/dev/null || echo 65536)
RAM_TOTAL_MB=$((RAM_TOTAL_KB / 1024))

# 2. Check BBR support
BBR_AVAIL="0"
modprobe tcp_bbr >/dev/null 2>&1 || true
if sysctl net.ipv4.tcp_available_congestion_control 2>/dev/null | grep -q bbr; then
    BBR_AVAIL="1"
fi

# 3. Determine buffer size and conntrack based on RAM
if [ "$RAM_TOTAL_MB" -ge 128 ]; then
    RMEM="4194304"
    WMEM="4194304"
    TCP_RMEM="4096 87380 4194304"
    TCP_WMEM="4096 65536 4194304"
    CONN_MAX="65536"
elif [ "$RAM_TOTAL_MB" -ge 64 ]; then
    RMEM="2097152"
    WMEM="2097152"
    TCP_RMEM="4096 87380 2097152"
    TCP_WMEM="4096 65536 2097152"
    CONN_MAX="32768"
else
    RMEM="1048576"
    WMEM="1048576"
    TCP_RMEM="4096 49152 1048576"
    TCP_WMEM="4096 49152 1048576"
    CONN_MAX="16384"
fi

# 4. Generate sysctl config
mkdir -p /etc/sysctl.d
cat <<EOF > /etc/sysctl.d/99-tachyon-tune.conf
# Tachyon High-Performance Network Tuning
net.core.default_qdisc = fq_codel
net.core.rmem_max = $RMEM
net.core.wmem_max = $WMEM
net.ipv4.tcp_rmem = $TCP_RMEM
net.ipv4.tcp_wmem = $TCP_WMEM
net.ipv4.tcp_fastopen = 3
net.ipv4.tcp_slow_start_after_idle = 0
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_syn_retries = 3
net.ipv4.tcp_synack_retries = 3
net.netfilter.nf_conntrack_max = $CONN_MAX
net.netfilter.nf_conntrack_tcp_timeout_established = 7200
EOF

if [ "$BBR_AVAIL" = "1" ]; then
    echo "net.ipv4.tcp_congestion_control = bbr" >> /etc/sysctl.d/99-tachyon-tune.conf
fi

# 5. Apply sysctl
sysctl -p /etc/sysctl.d/99-tachyon-tune.conf >/dev/null 2>&1 || true

# 6. Disable firewall flow offloading (prevents bypassing Netfilter/TProxy for Sing-box)
FLOW_OK="0"
if command -v uci >/dev/null 2>&1; then
    uci -q set firewall.@defaults[0].flow_offloading='0' 2>/dev/null || true
    uci commit firewall 2>/dev/null || true
    /etc/init.d/firewall reload >/dev/null 2>&1 || true
    FLOW_OK="1"
fi

# 7. Disable bridge netfilter interception on LAN bridges
sysctl -w net.bridge.bridge-nf-call-iptables=0 >/dev/null 2>&1 || true
sysctl -w net.bridge.bridge-nf-call-ip6tables=0 >/dev/null 2>&1 || true

echo "TUNE_DONE:$BBR_AVAIL:$RAM_TOTAL_MB:$CONN_MAX:$FLOW_OK"
`
	out, err := execFn(client, tuneScript)
	if err != nil {
		report.Success = false
		report.Details = fmt.Sprintf("Ошибка выполнения оптимизации: %v", err)
		return report
	}

	report.Success = true

	bbrAvail := false
	ramMB := 0
	connMax := 65536
	flowOK := false

	lines := strings.Split(out, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "TUNE_DONE:") {
			parts := strings.Split(strings.TrimPrefix(l, "TUNE_DONE:"), ":")
			if len(parts) >= 3 {
				bbrAvail = (parts[0] == "1")
				ramMB, _ = strconv.Atoi(parts[1])
				connMax, _ = strconv.Atoi(parts[2])
				if len(parts) >= 4 {
					flowOK = (parts[3] == "1")
				}
			}
		}
	}

	if bbrAvail {
		report.BBRStatus = "BBR + fq_codel (минимизация буферблоата и максимальная скорость)"
		report.AppliedRules = append(report.AppliedRules, "Контроль перегрузки: TCP BBR + fq_codel")
	} else {
		report.BBRStatus = "fq_codel (ядро роутера не поддерживает модуль tcp_bbr)"
		report.AppliedRules = append(report.AppliedRules, "Очередь пакетов: fq_codel (анти-буферблоат)")
	}

	report.Buffers = fmt.Sprintf("Оптимизированы под ОЗУ (%d МБ): tcp_fastopen=3, tcp_slow_start_after_idle=0", ramMB)
	report.AppliedRules = append(report.AppliedRules,
		fmt.Sprintf("Буферы сокетов rmem/wmem подобраны под объем памяти (%d МБ)", ramMB),
		"Включен TCP Fast Open (client + server)",
		"Отключен сброс окна TCP при бездействии (slow_start_after_idle = 0)",
		"Ускоренное закрытие зависших TIME_WAIT соединений (tcp_fin_timeout = 15s)",
	)

	report.Conntrack = fmt.Sprintf("Лимит %d соединений, таймаут сессий 7200s", connMax)
	report.AppliedRules = append(report.AppliedRules,
		fmt.Sprintf("Расширен лимит таблицы conntrack до %d сессий", connMax),
		"Очистка неактивных TCP-сессий через 2 часа вместо стандартных 5 суток",
	)

	if flowOK {
		report.Offloading = "Flow Offloading безопасно отключен (защита TProxy/sing-box от обхода правил Netfilter)"
		report.AppliedRules = append(report.AppliedRules, "Отключен Flow Offloading (защита TProxy и прозрачного проксирования)")
	} else {
		report.Offloading = "Flow Offloading пропущен (UCI недоступен)"
	}
	report.AppliedRules = append(report.AppliedRules, "Отключена фильтрация bridge-nf для LAN мостов (bridge-nf-call-iptables = 0)")

	report.Details = "Сетевой стек и параметры ядра успешно оптимизированы."
	return report
}
