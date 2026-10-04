package router

import (
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestTestBypass_Success(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "nslookup") {
			return "198.18.0.42\n", nil
		}
		if strings.Contains(cmd, "probe_site") {
			return `PROBE:YouTube 200 0.054
PROBE:Discord 200 0.065
PROBE:Telegram 200 0.045
PROBE:GitHub 200 0.080
GEOIP_DATA:
Germany
Frankfurt
Hetzner Online GmbH
159.69.1.1
`, nil
		}
		return "", nil
	}

	report := TestBypass(nil, mockExec)
	if !report.FakeIPActive {
		t.Errorf("expected FakeIPActive to be true for 198.18.0.42, got %v", report.FakeIPActive)
	}
	if !report.Success {
		t.Errorf("expected Success to be true for HTTP 200, got %v", report.Success)
	}
	if report.HTTPStatus != 200 {
		t.Errorf("expected HTTPStatus 200, got %d", report.HTTPStatus)
	}
	if report.LatencyMs != 54 {
		t.Errorf("expected LatencyMs 54, got %d", report.LatencyMs)
	}
	if len(report.Probes) != 4 {
		t.Errorf("expected 4 probes, got %d", len(report.Probes))
	}
	if report.GeoIP.Country != "Germany" || report.GeoIP.City != "Frankfurt" {
		t.Errorf("unexpected GeoIP: %+v", report.GeoIP)
	}
	if !strings.Contains(report.Details, "Fake-IP активен") {
		t.Errorf("expected details to contain Fake-IP status, got %s", report.Details)
	}
	summary := report.FormatSummary()
	if !strings.Contains(summary, "Discord") || !strings.Contains(summary, "Germany") {
		t.Errorf("unexpected summary output:\n%s", summary)
	}
}

func TestTestBypass_Failure(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "nslookup") {
			return "142.250.186.206\n", nil // Direct IP (no fake-ip)
		}
		if strings.Contains(cmd, "probe_site") {
			return "PROBE:YouTube 000 0\n", nil // Dropped by DPI
		}
		return "", nil
	}

	report := TestBypass(nil, mockExec)
	if report.FakeIPActive {
		t.Errorf("expected FakeIPActive to be false for direct IP")
	}
	if report.Success {
		t.Errorf("expected Success to be false for HTTP 000")
	}
	if !strings.Contains(report.Details, "YouTube недоступен") {
		t.Errorf("expected details to report failure, got %s", report.Details)
	}
}
