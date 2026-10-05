package router

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// DNSResolver represents a public secure/fast DNS upstream candidate.
type DNSResolver struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PrimaryIP   string `json:"primary_ip"`
	SecondaryIP string `json:"secondary_ip"`
	DoHURL      string `json:"doh_url"`
	Description string `json:"description"`
	LatencyMs   int64  `json:"latency_ms"`
	Working     bool   `json:"working"`
}

// DNSTestReport represents the outcome of DNS poisoning and benchmark checks on the router.
type DNSTestReport struct {
	UDPPoisoned   bool          `json:"udp_poisoned"`
	PoisonDetail  string        `json:"poison_detail"`
	Fastest       *DNSResolver  `json:"fastest"`
	Resolvers     []DNSResolver `json:"resolvers"`
	CurrentServer string        `json:"current_server"`
}

// DefaultDNSResolvers returns the curated set of high-performance uncensored DNS providers.
func DefaultDNSResolvers() []DNSResolver {
	return []DNSResolver{
		{
			ID:          "cloudflare",
			Name:        "Cloudflare DNS",
			PrimaryIP:   "1.1.1.1",
			SecondaryIP: "1.0.0.1",
			DoHURL:      "https://cloudflare-dns.com/dns-query",
			Description: "Ультра-быстрый мировой Anycast DNS, защита приватности",
		},
		{
			ID:          "quad9",
			Name:        "Quad9 Security",
			PrimaryIP:   "9.9.9.9",
			SecondaryIP: "149.112.112.112",
			DoHURL:      "https://dns.quad9.net/dns-query",
			Description: "Швейцарский фонд, встроенная блокировка фишинга и вредоносов",
		},
		{
			ID:          "adguard",
			Name:        "AdGuard DNS",
			PrimaryIP:   "94.140.14.14",
			SecondaryIP: "94.140.15.15",
			DoHURL:      "https://dns.adguard-dns.com/dns-query",
			Description: "Блокировка баннеров, трекеров и вредоносных сайтов",
		},
		{
			ID:          "google",
			Name:        "Google Public DNS",
			PrimaryIP:   "8.8.8.8",
			SecondaryIP: "8.8.4.4",
			DoHURL:      "https://dns.google/dns-query",
			Description: "Глобальная инфраструктура Google Anycast, высокая надежность",
		},
		{
			ID:          "comss",
			Name:        "Comss.one DNS",
			PrimaryIP:   "92.223.65.71",
			SecondaryIP: "92.38.169.130",
			DoHURL:      "https://dns.comss.one/dns-query",
			Description: "Оптимизирован для РФ, предотвращает троттлинг зарубежных CDN",
		},
	}
}

// RunDNSTest benchmarks DNS resolvers and checks for ISP hijacking on the router.
func RunDNSTest(client *gossh.Client, execFn sshutil.ExecFunc) DNSTestReport {
	report := DNSTestReport{
		Resolvers: DefaultDNSResolvers(),
	}

	// 1. Check current router DNS config
	curCmd := `uci -q get dhcp.@dnsmasq[0].server || cat /etc/resolv.conf 2>/dev/null | grep nameserver | head -n 2 | awk '{print $2}'`
	if curOut, err := execFn(client, curCmd); err == nil {
		report.CurrentServer = strings.TrimSpace(curOut)
	}

	// 2. Test plain UDP 53 for poisoning/tampering
	// Querying a domain that ISPs frequently hijack or poison to 127.0.0.1 or block page
	poisonCmd := `
out=$(timeout 3 nslookup rutracker.org 1.1.1.1 2>/dev/null || echo "TIMEOUT")
echo "$out"
`
	poisonOut, _ := execFn(client, poisonCmd)
	if strings.Contains(poisonOut, "TIMEOUT") || strings.Contains(poisonOut, "connection timed out") {
		report.UDPPoisoned = true
		report.PoisonDetail = "Провайдер блокирует прямой UDP 53 к внешним серверам (таймаут)"
	} else if strings.Contains(poisonOut, "127.0.0.1") || strings.Contains(poisonOut, "0.0.0.0") || strings.Contains(poisonOut, "warning.rt.ru") {
		report.UDPPoisoned = true
		report.PoisonDetail = "Провайдер подменяет DNS-ответы (обнаружен спуфинг/заглушка РКН)"
	} else {
		report.UDPPoisoned = false
		report.PoisonDetail = "Прямой UDP 53 не перехватывается провайдером"
	}

	// 3. Benchmark each resolver sequentially or via a fast shell loop on the router
	benchScript := `
bench_dns() {
    id="$1"
    ip="$2"
    t_start=$(date +%s%N 2>/dev/null || echo "")
    if [ -n "$t_start" ]; then
        if timeout 2 nslookup google.com "$ip" >/dev/null 2>&1; then
            t_end=$(date +%s%N 2>/dev/null || echo "")
            if [ -n "$t_end" ]; then
                diff=$(( (t_end - t_start) / 1000000 ))
                echo "DNS_BENCH:$id:$diff:OK"
                return
            fi
        fi
    fi

    # Fallback to ping/nc latency if nslookup without nanoseconds
    if ping -c 1 -W 2 "$ip" >/dev/null 2>&1; then
        rtt=$(ping -c 1 -W 2 "$ip" 2>/dev/null | grep -o 'time=[0-9.]*' | cut -d= -f2 | cut -d. -f1)
        if [ -n "$rtt" ]; then
            echo "DNS_BENCH:$id:$rtt:OK"
            return
        fi
        echo "DNS_BENCH:$id:25:OK"
        return
    fi
    echo "DNS_BENCH:$id:999:FAIL"
}
`
	for _, r := range report.Resolvers {
		benchScript += fmt.Sprintf("bench_dns %s %s\n", r.ID, r.PrimaryIP)
	}

	benchOut, _ := execFn(client, benchScript)
	latencies := parseBenchOutput(benchOut)

	for i := range report.Resolvers {
		if lat, ok := latencies[report.Resolvers[i].ID]; ok {
			report.Resolvers[i].LatencyMs = lat.LatencyMs
			report.Resolvers[i].Working = lat.Success
		} else {
			report.Resolvers[i].LatencyMs = 999
			report.Resolvers[i].Working = false
		}
	}

	// Sort resolvers by latency
	sort.Slice(report.Resolvers, func(i, j int) bool {
		if report.Resolvers[i].Working && !report.Resolvers[j].Working {
			return true
		}
		if !report.Resolvers[i].Working && report.Resolvers[j].Working {
			return false
		}
		return report.Resolvers[i].LatencyMs < report.Resolvers[j].LatencyMs
	})

	if len(report.Resolvers) > 0 && report.Resolvers[0].Working {
		fastestCopy := report.Resolvers[0]
		report.Fastest = &fastestCopy
	}

	return report
}

type benchItem struct {
	LatencyMs int64
	Success   bool
}

func parseBenchOutput(out string) map[string]benchItem {
	res := make(map[string]benchItem)
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "DNS_BENCH:") {
			parts := strings.Split(strings.TrimPrefix(l, "DNS_BENCH:"), ":")
			if len(parts) >= 3 {
				id := parts[0]
				lat, _ := strconv.ParseInt(parts[1], 10, 64)
				success := parts[2] == "OK"
				res[id] = benchItem{
					LatencyMs: lat,
					Success:   success,
				}
			}
		}
	}
	return res
}

// GenerateApplyDNSCommand returns the shell script to configure dnsmasq and Tachyon on OpenWrt
// to use the chosen DNS resolver with anti-leak protection.
func GenerateApplyDNSCommand(resolver DNSResolver) string {
	script := fmt.Sprintf(`
# 1. Configure Tachyon internal DNS settings if Tachyon is installed
if command -v uci >/dev/null 2>&1 && uci -q get tachyon.settings >/dev/null; then
    while uci -q delete tachyon.settings.dns_server 2>/dev/null; do :; done
    uci -q add_list tachyon.settings.dns_server='%s' 2>/dev/null || true
    if [ -n "%s" ]; then
        uci -q add_list tachyon.settings.dns_server='%s' 2>/dev/null || true
    fi
    while uci -q delete tachyon.settings.bootstrap_dns_server 2>/dev/null; do :; done
    uci -q add_list tachyon.settings.bootstrap_dns_server='%s' 2>/dev/null || true
    if [ -n "%s" ]; then
        uci -q add_list tachyon.settings.bootstrap_dns_server='%s' 2>/dev/null || true
    fi
    uci commit tachyon 2>/dev/null || true
fi

# 2. Configure dnsmasq upstream servers and anti-leak
if command -v uci >/dev/null 2>&1; then
    while uci -q delete dhcp.@dnsmasq[0].server 2>/dev/null; do :; done
    uci -q add_list dhcp.@dnsmasq[0].server='%s' 2>/dev/null || true
    if [ -n "%s" ]; then
        uci -q add_list dhcp.@dnsmasq[0].server='%s' 2>/dev/null || true
    fi
    uci -q set dhcp.@dnsmasq[0].noresolv='1' 2>/dev/null || true
    uci -q set dhcp.@dnsmasq[0].localuse='1' 2>/dev/null || true
    uci -q set dhcp.@dnsmasq[0].cachesize='4096' 2>/dev/null || true
    uci commit dhcp 2>/dev/null || true
    /etc/init.d/dnsmasq restart >/dev/null 2>&1 || true
fi
`, resolver.PrimaryIP, resolver.SecondaryIP, resolver.SecondaryIP,
		resolver.PrimaryIP, resolver.SecondaryIP, resolver.SecondaryIP,
		resolver.PrimaryIP, resolver.SecondaryIP, resolver.SecondaryIP)

	return strings.ReplaceAll(script, "\r\n", "\n")
}

// ApplyDNSConfig executes DNS configuration directly on the router.
func ApplyDNSConfig(client *gossh.Client, execFn sshutil.ExecFunc, resolver DNSResolver) error {
	cmd := GenerateApplyDNSCommand(resolver)
	_, err := execFn(client, cmd)
	if err != nil {
		return fmt.Errorf("ошибка настройки DNS на роутере: %w", err)
	}
	return nil
}
