package router

import (
	"fmt"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestEmergencyRescue_Success(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "RESCUE_DONE:1:1\n", nil
	}

	report := EmergencyRescue(nil, mockExec)
	if !report.Success {
		t.Fatalf("expected Success=true, got false")
	}
	if !report.NetworkRestored {
		t.Errorf("expected NetworkRestored=true")
	}
	if !report.InternetPing {
		t.Errorf("expected InternetPing=true")
	}
	if len(report.ActionsTaken) != 5 {
		t.Errorf("expected 5 actions taken, got %d", len(report.ActionsTaken))
	}
}

func TestEmergencyRescue_GatewayOnly(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "RESCUE_DONE:1:0\n", nil
	}

	report := EmergencyRescue(nil, mockExec)
	if !report.Success {
		t.Fatalf("expected Success=true")
	}
	if !report.GatewayPing || report.InternetPing {
		t.Errorf("expected GatewayPing=true and InternetPing=false")
	}
}

func TestEmergencyRescue_Error(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "", fmt.Errorf("ssh connection severed")
	}

	report := EmergencyRescue(nil, mockExec)
	if report.Success {
		t.Errorf("expected Success=false on ssh error")
	}
}
