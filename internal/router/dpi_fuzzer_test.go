package router

import (
	"errors"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestDefaultDPIStrategies(t *testing.T) {
	strats := DefaultDPIStrategies()
	if len(strats) < 3 {
		t.Fatalf("expected at least 3 default strategies, got %d", len(strats))
	}

	foundFakesplit := false
	for _, s := range strats {
		if s.ID == "fakesplit_diso" {
			foundFakesplit = true
		}
	}
	if !foundFakesplit {
		t.Errorf("expected fakesplit_diso strategy in list")
	}
}

func TestParseFuzzLine(t *testing.T) {
	output := `
FUZZ_RES:yt:200 0.045
FUZZ_RES:dc:000 0
FUZZ_RES:ru:200 0.015
`
	ytOK, ytLat := parseFuzzLine(output, "yt")
	if !ytOK || ytLat != 45 {
		t.Errorf("expected ytOK=true lat=45, got ok=%v lat=%d", ytOK, ytLat)
	}

	dcOK, dcLat := parseFuzzLine(output, "dc")
	if dcOK || dcLat != 0 {
		t.Errorf("expected dcOK=false lat=0, got ok=%v lat=%d", dcOK, dcLat)
	}

	ruOK, ruLat := parseFuzzLine(output, "ru")
	if !ruOK || ruLat != 15 {
		t.Errorf("expected ruOK=true lat=15, got ok=%v lat=%d", ruOK, ruLat)
	}
}

func TestRunDPIFuzzer(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "probe_target") {
			return "FUZZ_RES:yt:200 0.035\nFUZZ_RES:dc:200 0.040\nFUZZ_RES:ru:200 0.010\n", nil
		}
		return "steer", nil
	}

	progressCalls := 0
	report := RunDPIFuzzer(nil, mockExec, func(strat DPIStrategy, cur, total int) {
		progressCalls++
	})

	if progressCalls != len(DefaultDPIStrategies()) {
		t.Errorf("expected %d progress calls, got %d", len(DefaultDPIStrategies()), progressCalls)
	}

	if report.WinningStrategy == nil {
		t.Fatalf("expected a winning strategy, got nil")
	}

	if report.WinningStrategy.SuccessRate == 0 {
		t.Errorf("expected winning strategy success rate > 0")
	}
}

func TestGenerateApplyDPIStrategyScript(t *testing.T) {
	strat := DPIStrategy{
		ID:      "test_strat",
		Name:    "Test Strategy",
		TCPArgs: "--filter-tcp=443 --lua-desync=split2",
		UDPArgs: "--filter-udp=443 --lua-desync=fake",
	}

	script := GenerateApplyDPIStrategyScript(strat)
	if !strings.Contains(script, "test_strat") {
		t.Errorf("script missing strategy ID")
	}
	if !strings.Contains(script, "steer_tcp_args='--filter-tcp=443 --lua-desync=split2'") {
		t.Errorf("script missing TCP args: %s", script)
	}
	if !strings.Contains(script, "steer_udp_args='--filter-udp=443 --lua-desync=fake'") {
		t.Errorf("script missing UDP args: %s", script)
	}
}

func TestApplyDPIStrategy_Error(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "", errors.New("ssh exec error")
	}

	strat := DPIStrategy{ID: "err"}
	err := ApplyDPIStrategy(nil, mockExec, strat)
	if err == nil {
		t.Fatalf("expected error from ApplyDPIStrategy, got nil")
	}
}
