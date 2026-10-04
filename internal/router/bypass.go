package router

import (
	"fmt"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// TargetProbe represents the outcome of probing a single destination.
type TargetProbe struct {
	Name       string
	URL        string
	HTTPStatus int
	LatencyMs  int64
	Success    bool
}

// GeoIPReport holds egress IP geolocation and DNS leak detection details.
type GeoIPReport struct {
	IP      string
	Country string
	City    string
	ISP     string
	DNSLeak bool
}

// BypassReport holds the outcome of real-world bypass testing through Tachyon.
type BypassReport struct {
	FakeIPActive bool
	ResolvedIP   string
	TargetDomain string
	HTTPStatus   int
	LatencyMs    int64
	Success      bool
	Details      string

	Probes []TargetProbe
	GeoIP  GeoIPReport
}

// FormatSummary returns a human-readable multi-line summary of probes and geo details.
func (r BypassReport) FormatSummary() string {
	var sb strings.Builder

	if r.FakeIPActive {
		sb.WriteString(fmt.Sprintf("  • DNS Fake-IP:  [#22c55e]перехвачен (%s)[-]\n", r.ResolvedIP))
	} else if r.ResolvedIP != "" {
		sb.WriteString(fmt.Sprintf("  • DNS Fake-IP:  [#eab308]прямой ответ (%s)[-]\n", r.ResolvedIP))
	} else {
		sb.WriteString("  • DNS Fake-IP:  [#ef5350]нет ответа[-]\n")
	}

	for _, p := range r.Probes {
		if p.Success {
			sb.WriteString(fmt.Sprintf("  • %-12s: [#22c55e]HTTP %d (%d мс) ✓[-]\n", p.Name, p.HTTPStatus, p.LatencyMs))
		} else {
			sb.WriteString(fmt.Sprintf("  • %-12s: [#eab308]HTTP %d (таймаут/блок)[-]\n", p.Name, p.HTTPStatus))
		}
	}

	if r.GeoIP.IP != "" {
		location := r.GeoIP.Country
		if r.GeoIP.City != "" {
			location = fmt.Sprintf("%s, %s", r.GeoIP.City, r.GeoIP.Country)
		}
		sb.WriteString(fmt.Sprintf("  • Выходной IP:  [#38bdf8]%s[-] ([#94a3b8]%s, %s[-])\n", r.GeoIP.IP, location, r.GeoIP.ISP))
	}

	return sb.String()
}

// TestBypass performs end-to-end DNS resolution, multi-service probing, and egress IP check through Tachyon.
func TestBypass(client *gossh.Client, execFn sshutil.ExecFunc) BypassReport {
	report := BypassReport{
		TargetDomain: "youtube.com",
		Probes:       []TargetProbe{},
	}

	// 1. Fake-IP DNS interception check
	dnsCmd := `
out=$(nslookup youtube.com 127.0.0.1 2>/dev/null || nslookup youtube.com 2>/dev/null || echo "")
echo "$out" | grep -iE 'Address|Адрес' | tail -n 1 | awk '{print $NF}'
`
	dnsOut, _ := execFn(client, dnsCmd)
	resolvedIP := strings.TrimSpace(dnsOut)
	report.ResolvedIP = resolvedIP

	if strings.HasPrefix(resolvedIP, "198.18.") || strings.HasPrefix(resolvedIP, "198.19.") {
		report.FakeIPActive = true
	} else {
		report.GeoIP.DNSLeak = true
	}

	// 2. Parallel multi-service HTTP/HTTPS probes and GeoIP query
	probeScript := `
probe_site() {
    name="$1"
    url="$2"
    if command -v curl >/dev/null 2>&1; then
        res=$(curl -m 5 -s -o /dev/null -w "%{http_code} %{time_total}" "$url" 2>/dev/null || echo "000 0")
    else
        if wget -q --spider --timeout=5 "$url" 2>/dev/null; then
            res="200 0.15"
        else
            res="000 0"
        fi
    fi
    echo "PROBE:$name $res"
}

probe_site "YouTube"  "https://www.youtube.com" &
probe_site "Discord"  "https://discord.com" &
probe_site "Telegram" "https://api.telegram.org" &
probe_site "GitHub"   "https://raw.githubusercontent.com" &
wait

# GeoIP resolution
if command -v curl >/dev/null 2>&1; then
    geo=$(curl -m 4 -s "http://ip-api.com/line?fields=country,city,isp,query" 2>/dev/null || echo "")
    if [ -n "$geo" ]; then
        echo "GEOIP_DATA:"
        echo "$geo"
    fi
fi
`
	probeOut, _ := execFn(client, probeScript)

	// Parse probes and GeoIP
	lines := strings.Split(probeOut, "\n")
	var geoLines []string
	inGeo := false

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "GEOIP_DATA:") {
			inGeo = true
			continue
		}
		if inGeo {
			if l != "" {
				geoLines = append(geoLines, l)
			}
			continue
		}

		if strings.HasPrefix(l, "PROBE:") {
			raw := strings.TrimPrefix(l, "PROBE:")
			parts := strings.Fields(raw)
			if len(parts) >= 3 {
				name := parts[0]
				statusCode, _ := strconv.Atoi(parts[1])
				var latMs int64
				if durSec, err := strconv.ParseFloat(parts[2], 64); err == nil {
					latMs = int64(durSec * 1000.0)
				}
				success := (statusCode >= 200 && statusCode < 500)
				probe := TargetProbe{
					Name:       name,
					HTTPStatus: statusCode,
					LatencyMs:  latMs,
					Success:    success,
				}
				report.Probes = append(report.Probes, probe)

				if name == "YouTube" {
					report.HTTPStatus = statusCode
					report.LatencyMs = latMs
					report.Success = success
				}
			}
		}
	}

	if len(geoLines) >= 4 {
		report.GeoIP.Country = geoLines[0]
		report.GeoIP.City = geoLines[1]
		report.GeoIP.ISP = geoLines[2]
		report.GeoIP.IP = geoLines[3]
	}

	// Backwards compatibility Details string
	var statusParts []string
	if report.FakeIPActive {
		statusParts = append(statusParts, fmt.Sprintf("Fake-IP активен (%s)", report.ResolvedIP))
	} else if report.ResolvedIP != "" {
		statusParts = append(statusParts, fmt.Sprintf("DNS прямой (%s)", report.ResolvedIP))
	} else {
		statusParts = append(statusParts, "DNS не ответил")
	}

	if report.Success {
		statusParts = append(statusParts, fmt.Sprintf("YouTube: %d мс", report.LatencyMs))
	} else {
		statusParts = append(statusParts, "YouTube недоступен")
	}

	for _, p := range report.Probes {
		if p.Name != "YouTube" && p.Success {
			statusParts = append(statusParts, fmt.Sprintf("%s: %d мс", p.Name, p.LatencyMs))
		}
	}

	if report.GeoIP.Country != "" {
		statusParts = append(statusParts, fmt.Sprintf("Выход: %s", report.GeoIP.Country))
	}

	report.Details = strings.Join(statusParts, ", ")
	return report
}
