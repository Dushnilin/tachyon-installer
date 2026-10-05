package router

import (
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestRunGuidedSetup_Success(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "uci add_list dhcp.@dnsmasq[0].server") {
			return "", nil
		}
		if strings.Contains(cmd, "tachyon.settings.steer_tcp_args") {
			return "", nil
		}
		if strings.Contains(cmd, "99-tachyon-tune.conf") {
			return "BBR_OK", nil
		}
		if strings.Contains(cmd, "PROBE:") {
			return "PROBE:YouTube 200 0.03\nPROBE:Discord 200 0.04\n", nil
		}
		return "OK", nil
	}

	plan := GuidedSetupPlan{
		Mode: ModeStandaloneDPI,
		ChosenDNS: DNSResolver{
			ID:        "cloudflare",
			Name:      "Cloudflare DNS",
			PrimaryIP: "1.1.1.1",
			LatencyMs: 14,
		},
		ChosenStrategy: DPIStrategy{
			ID:      "fakesplit_diso",
			Name:    "FakeSplit + Disorder",
			TCPArgs: "--filter-tcp=443 --lua-desync=fakedsplit",
		},
		SelectedEngine: "steer",
		EnableTune:     true,
	}

	progressCalls := 0
	res, err := RunGuidedSetup(nil, mockExec, plan, func(title string, frac float64) {
		progressCalls++
	})

	if err != nil {
		t.Fatalf("unexpected error from RunGuidedSetup: %v", err)
	}

	if progressCalls < 4 {
		t.Errorf("expected at least 4 progress updates, got %d", progressCalls)
	}

	if !res.DNSApplied {
		t.Errorf("expected DNS to be applied")
	}

	if !res.StrategyApplied {
		t.Errorf("expected strategy to be applied")
	}

	if !res.NetworkTuned {
		t.Errorf("expected network to be tuned")
	}

	if !res.OverallSuccess {
		t.Errorf("expected overall success to be true")
	}

	if !strings.Contains(res.SummaryMessage, "Cloudflare DNS") {
		t.Errorf("summary missing DNS name: %s", res.SummaryMessage)
	}
}

func TestRunGuidedSetup_TunnelWithSubscription(t *testing.T) {
	var savedSub bool
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "subscription_update") || strings.Contains(cmd, "community_lists") {
			savedSub = true
		}
		if strings.Contains(cmd, "PROBE:") {
			return "PROBE:YouTube 200 0.05\nPROBE:Discord 200 0.06\n", nil
		}
		return "OK", nil
	}

	plan := GuidedSetupPlan{
		Mode: ModeTunnel,
		ChosenDNS: DNSResolver{
			ID:        "cloudflare",
			Name:      "Cloudflare DNS",
			PrimaryIP: "1.1.1.1",
			LatencyMs: 14,
		},
		SubscriptionURL: "vless://user@1.1.1.1:443?type=tcp#Node",
		SelectedEngine:  "sing-box",
		EnableTune:      true,
	}

	res, err := RunGuidedSetup(nil, mockExec, plan, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !savedSub {
		t.Errorf("expected subscription to be saved and updated")
	}
	if !res.OverallSuccess {
		t.Errorf("expected overall success to be true")
	}
	if !strings.Contains(res.SummaryMessage, "VLESS Туннель активен") {
		t.Errorf("expected summary to mention VLESS tunnel: %s", res.SummaryMessage)
	}
}
