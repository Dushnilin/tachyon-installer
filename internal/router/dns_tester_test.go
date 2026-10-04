package router

import (
	"errors"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestDefaultDNSResolvers(t *testing.T) {
	resolvers := DefaultDNSResolvers()
	if len(resolvers) < 4 {
		t.Fatalf("expected at least 4 default resolvers, got %d", len(resolvers))
	}

	foundCloudflare := false
	for _, r := range resolvers {
		if r.ID == "cloudflare" && r.PrimaryIP == "1.1.1.1" {
			foundCloudflare = true
		}
	}
	if !foundCloudflare {
		t.Errorf("expected Cloudflare DNS resolver in default list")
	}
}

func TestParseBenchOutput(t *testing.T) {
	output := `
DNS_BENCH:cloudflare:14:OK
DNS_BENCH:quad9:8:OK
DNS_BENCH:adguard:22:OK
DNS_BENCH:google:999:FAIL
`
	parsed := parseBenchOutput(output)
	if len(parsed) != 4 {
		t.Fatalf("expected 4 parsed items, got %d", len(parsed))
	}
	if parsed["quad9"].LatencyMs != 8 || !parsed["quad9"].Success {
		t.Errorf("unexpected quad9 item: %+v", parsed["quad9"])
	}
	if parsed["google"].Success {
		t.Errorf("expected google to be marked as failed")
	}
}

func TestGenerateApplyDNSCommand(t *testing.T) {
	res := DNSResolver{
		ID:          "quad9",
		Name:        "Quad9",
		PrimaryIP:   "9.9.9.9",
		SecondaryIP: "149.112.112.112",
	}

	cmd := GenerateApplyDNSCommand(res)
	if !strings.Contains(cmd, "9.9.9.9") {
		t.Errorf("command missing primary IP: %s", cmd)
	}
	if !strings.Contains(cmd, "149.112.112.112") {
		t.Errorf("command missing secondary IP: %s", cmd)
	}
	if !strings.Contains(cmd, "noresolv='1'") {
		t.Errorf("command missing noresolv anti-leak setting")
	}
}

func TestRunDNSTest_Poisoned(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "rutracker.org") {
			return "Address 1: 127.0.0.1\n", nil
		}
		if strings.Contains(cmd, "bench_dns") {
			return "DNS_BENCH:cloudflare:12:OK\nDNS_BENCH:quad9:18:OK\n", nil
		}
		return "192.168.1.1", nil
	}

	report := RunDNSTest(nil, mockExec)
	if !report.UDPPoisoned {
		t.Errorf("expected UDPPoisoned to be true for 127.0.0.1 redirect")
	}
	if report.Fastest == nil || report.Fastest.ID != "cloudflare" {
		t.Errorf("expected fastest to be cloudflare, got %+v", report.Fastest)
	}
}

func TestApplyDNSConfig_Error(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "", errors.New("uci failed")
	}

	res := DNSResolver{PrimaryIP: "1.1.1.1", SecondaryIP: "1.0.0.1"}
	err := ApplyDNSConfig(nil, mockExec, res)
	if err == nil {
		t.Fatalf("expected error from ApplyDNSConfig, got nil")
	}
}
