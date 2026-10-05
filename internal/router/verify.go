package router

import (
	"fmt"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// VerifyResult holds the outcome of post-install verification.
type VerifyResult struct {
	OK     bool
	Reason string
}

// ProgressFn is a callback for real-time progress updates during verification.
type ProgressFn func(label string, fraction float64)

// VerifyAndFallback runs post-install verification for Tachyon
// (service/process check, DNS, internet connectivity).
func VerifyAndFallback(client *gossh.Client, execFn sshutil.ExecFunc, progressFn ProgressFn) VerifyResult {
	progress := func(label string, f float64) {
		if progressFn != nil {
			progressFn(label, f)
		}
	}

	progress("Определение активного ядра прокси...", 0.05)
	activeEngine := detectActiveEngine(client, execFn)

	// Early check: if configuration is empty (e.g. fresh install with no subscription), skip waiting for sing-box
	subStatus := CheckSubscriptions(client, execFn)
	if subStatus.HasEmptySubscription {
		progress("Конфигурация еще не заполнена (требуется подписка)...", 0.30)
		return VerifyResult{OK: true, Reason: "need_subscription_update"}
	}

	// 1. Process and Service Check
	progress(fmt.Sprintf("Ожидание запуска Tachyon и ядра %s...", activeEngine), 0.35)
	ok := verifyProcessAndService(client, execFn, activeEngine, func(attempt int) {
		progress(fmt.Sprintf("Ожидание %s (попытка %d/10)...", activeEngine, attempt+1), 0.35+float64(attempt)*0.02)
	})

	if !ok {
		progress("Автоустановка отсутствующих модулей ядра (kmod-nft-tproxy)...", 0.55)
		if autoFixMissingPackages(client, execFn) {
			progress(fmt.Sprintf("Повторная проверка %s...", activeEngine), 0.65)
			ok = verifyProcessAndService(client, execFn, activeEngine, nil)
		}
		if !ok {
			reason := fmt.Sprintf("%s_not_started", activeEngine)
			progress("Откат: сервис не запустился, восстановление прямого доступа...", 0.70)
			TriggerFallback(client, execFn)
			return VerifyResult{OK: false, Reason: reason}
		}
	}

	// 2. DNS Check
	progress("Проверка работы DNS...", 0.75)
	if !verifyDNS(client, execFn, func(attempt int) {
		progress(fmt.Sprintf("Тест DNS (попытка %d/6)...", attempt+1), 0.75+float64(attempt)*0.02)
	}) {
		progress("Откат: сбой DNS, восстановление прямого доступа...", 0.85)
		TriggerFallback(client, execFn)
		return VerifyResult{OK: false, Reason: "dns_broken"}
	}

	// 3. TCP Connectivity Check
	progress("Проверка интернет-соединения...", 0.90)
	connOk := verifyConnectivity(client, execFn, func(attempt int) {
		progress(fmt.Sprintf("Тест соединения (попытка %d/3)...", attempt+1), 0.90+float64(attempt)*0.03)
	})
	if !connOk {
		progress("⚠ Нет доступа к тестовому серверу через прокси", 0.98)
		return VerifyResult{OK: false, Reason: "internet_connectivity_failed"}
	}

	progress("Все проверки успешно пройдены ✓", 1.0)
	return VerifyResult{OK: true, Reason: ""}
}

func verifyProcessAndService(client *gossh.Client, execFn sshutil.ExecFunc, activeEngine string, onRetry func(attempt int)) bool {
	checkCmd := fmt.Sprintf(`
if [ -x /usr/bin/tachyon ]; then
    /usr/bin/tachyon get_status 2>/dev/null | grep -Eq '"running"[[:space:]]*:[[:space:]]*(1|true)' && echo "RUNNING" && exit 0
fi
if pgrep -x %s >/dev/null || pgrep -f "%s" >/dev/null; then
    echo "RUNNING"
    exit 0
fi
echo "NOT_RUNNING"
`, activeEngine, activeEngine)

	for i := 0; i < 10; i++ {
		if onRetry != nil {
			onRetry(i)
		}
		out, err := execFn(client, checkCmd)
		if err == nil && strings.Contains(out, "RUNNING") {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

func verifyDNS(client *gossh.Client, execFn sshutil.ExecFunc, onRetry func(attempt int)) bool {
	dnsCmd := `
if timeout 3 nslookup google.com 127.0.0.1 >/dev/null 2>&1; then echo "DNS_OK"; exit 0; fi
if timeout 3 nslookup google.com >/dev/null 2>&1; then echo "DNS_OK"; exit 0; fi
if ping -c 1 -W 2 1.1.1.1 >/dev/null 2>&1; then echo "DNS_OK"; exit 0; fi
echo "DNS_FAILED"`

	for i := 0; i < 6; i++ {
		if onRetry != nil {
			onRetry(i)
		}
		out, err := execFn(client, dnsCmd)
		if err == nil && strings.Contains(out, "DNS_OK") {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

func verifyConnectivity(client *gossh.Client, execFn sshutil.ExecFunc, onRetry func(attempt int)) bool {
	connCmd := `
if command -v curl >/dev/null 2>&1; then
    curl -fsI --connect-timeout 4 http://cp.cloudflare.com/generate_204 >/dev/null 2>&1 && echo "OK" || echo "FAILED"
else
    wget -qO /dev/null --timeout=4 http://cp.cloudflare.com/generate_204 >/dev/null 2>&1 && echo "OK" || echo "FAILED"
fi`
	for i := 0; i < 3; i++ {
		if onRetry != nil {
			onRetry(i)
		}
		out, err := execFn(client, connCmd)
		if err == nil && strings.Contains(out, "OK") {
			return true
		}
		time.Sleep(1 * time.Second)
	}
	return false
}

func autoFixMissingPackages(client *gossh.Client, execFn sshutil.ExecFunc) bool {
	pkgOut, err := execFn(client, "command -v apk >/dev/null 2>&1 && echo apk || echo opkg")
	if err != nil {
		return false
	}
	pkgManager := strings.TrimSpace(pkgOut)

	var cmd string
	if pkgManager == "apk" {
		cmd = "apk update && apk add kmod-nft-tproxy kmod-tun"
	} else {
		cmd = "opkg update && opkg install kmod-nft-tproxy kmod-tun"
	}

	_, installErr := execFn(client, cmd)
	if installErr != nil {
		return false
	}

	_, _ = execFn(client, "/etc/init.d/tachyon restart")
	time.Sleep(4 * time.Second)
	return true
}

// TriggerFallback stops Tachyon and restores clean direct networking.
func TriggerFallback(client *gossh.Client, execFn sshutil.ExecFunc) {
	fallbackCmds := []string{
		"/usr/bin/tachyon stop >/dev/null 2>&1 || true",
		"/etc/init.d/tachyon stop >/dev/null 2>&1 || true",
		"/etc/init.d/sing-box stop >/dev/null 2>&1 || true",
		"/etc/init.d/steer stop >/dev/null 2>&1 || true",
		"uci -q set tachyon.settings.enabled='0'",
		"uci commit tachyon",
		"nft delete table inet tachyon_table >/dev/null 2>&1 || true",
		"nft delete table inet tachyon >/dev/null 2>&1 || true",
		"/usr/bin/tachyon dnsmasq_restore >/dev/null 2>&1 || { uci -q delete dhcp.@dnsmasq[0].server ; uci -q add_list dhcp.@dnsmasq[0].server=1.1.1.1 ; uci -q add_list dhcp.@dnsmasq[0].server=8.8.8.8 ; uci -q set dhcp.@dnsmasq[0].noresolv=0 ; uci commit dhcp ; }",
		"/etc/init.d/dnsmasq restart >/dev/null 2>&1 || true",
	}
	_, _ = execFn(client, strings.Join(fallbackCmds, " ; "))
	time.Sleep(2 * time.Second)
}

func detectActiveEngine(client *gossh.Client, execFn sshutil.ExecFunc) string {
	out, err := execFn(client, "uci -q get tachyon.settings.engine 2>/dev/null || echo ''")
	if err == nil {
		v := strings.TrimSpace(out)
		if v != "" && v != "none" {
			return v
		}
	}
	return "sing-box"
}
