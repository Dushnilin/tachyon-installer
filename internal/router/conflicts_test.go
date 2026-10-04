package router

import (
	"fmt"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestDisableConflicts_Success(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "DISABLED_SERVICES:passwall zapret\n", nil
	}

	report := DisableConflicts(nil, mockExec, []string{"passwall", "zapret"})
	if !report.Success {
		t.Fatalf("expected Success=true")
	}
	if report.DisabledCount != 2 {
		t.Errorf("expected DisabledCount=2, got %d", report.DisabledCount)
	}
	if report.DisabledList[0] != "passwall" || report.DisabledList[1] != "zapret" {
		t.Errorf("unexpected DisabledList: %v", report.DisabledList)
	}
}

func TestDisableConflicts_None(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "DISABLED_SERVICES:\n", nil
	}

	report := DisableConflicts(nil, mockExec, nil)
	if !report.Success {
		t.Fatalf("expected Success=true")
	}
	if report.DisabledCount != 0 {
		t.Errorf("expected DisabledCount=0, got %d", report.DisabledCount)
	}
}

func TestDisableConflicts_Error(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "", fmt.Errorf("command timeout")
	}

	report := DisableConflicts(nil, mockExec, nil)
	if report.Success {
		t.Errorf("expected Success=false on execution error")
	}
}
