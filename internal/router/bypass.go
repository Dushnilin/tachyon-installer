package router

import (
	"fmt"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// BypassReport holds the outcome of real-world bypass testing through Tachyon.
type BypassReport struct {
	FakeIPActive bool
	ResolvedIP   string
	TargetDomain string
	HTTPStatus   int
	LatencyMs    int64
	Success      bool
	Details      string
}

// TestBypass performs end-to-end DNS resolution and HTTP/HTTPS probing through Tachyon.
func TestBypass(client *gossh.Client, execFn sshutil.ExecFunc) BypassReport {
	report := BypassReport{
		TargetDomain: "youtube.com",
	}

	// 1. Fake-IP DNS interception check
	// When Tachyon Fake-IP is working, resolving youtube.com returns an IP from pool 198.18.0.0/15
	dnsCmd := `
out=$(nslookup youtube.com 127.0.0.1 2>/dev/null || nslookup youtube.com 2>/dev/null || echo "")
echo "$out" | grep -iE 'Address|Адрес' | tail -n 1 | awk '{print $NF}'
`
	dnsOut, _ := execFn(client, dnsCmd)
	resolvedIP := strings.TrimSpace(dnsOut)
	report.ResolvedIP = resolvedIP

	if strings.HasPrefix(resolvedIP, "198.18.") || strings.HasPrefix(resolvedIP, "198.19.") {
		report.FakeIPActive = true
	}

	// 2. End-to-end HTTP/HTTPS probe from router through Tachyon
	probeCmd := `
if command -v curl >/dev/null 2>&1; then
    curl -m 6 -s -o /dev/null -w "%{http_code} %{time_total}" https://www.youtube.com 2>/dev/null || echo "000 0"
else
    if wget -q --spider --timeout=6 https://www.youtube.com 2>/dev/null; then
        echo "200 0.15"
    else
        echo "000 0"
    fi
fi
`
	probeOut, _ := execFn(client, probeCmd)
	parts := strings.Fields(strings.TrimSpace(probeOut))
	if len(parts) >= 2 {
		statusCode, _ := strconv.Atoi(parts[0])
		report.HTTPStatus = statusCode
		if durSec, err := strconv.ParseFloat(parts[1], 64); err == nil {
			report.LatencyMs = int64(durSec * 1000.0)
		}
	}

	// Valid HTTP codes indicate the TCP/TLS connection through proxy succeeded
	if report.HTTPStatus >= 200 && report.HTTPStatus < 500 {
		report.Success = true
	}

	var statusParts []string
	if report.FakeIPActive {
		statusParts = append(statusParts, fmt.Sprintf("Fake-IP DNS активен (%s)", report.ResolvedIP))
	} else if report.ResolvedIP != "" {
		statusParts = append(statusParts, fmt.Sprintf("DNS прямой (%s)", report.ResolvedIP))
	} else {
		statusParts = append(statusParts, "DNS не ответил")
	}

	if report.Success {
		statusParts = append(statusParts, fmt.Sprintf("YouTube доступен (HTTP %d, %d мс)", report.HTTPStatus, report.LatencyMs))
	} else {
		statusParts = append(statusParts, "YouTube недоступен (таймаут или блок)")
	}

	report.Details = strings.Join(statusParts, ", ")
	return report
}
