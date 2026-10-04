package router

import (
	"errors"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestTuneNetwork_SuccessBBR(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "99-tachyon-tune.conf") {
			return "TUNE_DONE:1:256:65536:1\n", nil
		}
		return "", nil
	}

	rep := TuneNetwork(nil, mockExec)
	if !rep.Success {
		t.Fatalf("expected success, got failure: %s", rep.Details)
	}

	if !strings.Contains(rep.BBRStatus, "BBR") {
		t.Errorf("expected BBR status to contain BBR, got: %s", rep.BBRStatus)
	}
	if !strings.Contains(rep.Conntrack, "65536") {
		t.Errorf("expected conntrack 65536, got: %s", rep.Conntrack)
	}
	if !strings.Contains(rep.Offloading, "включен") {
		t.Errorf("expected offloading enabled, got: %s", rep.Offloading)
	}
}

func TestTuneNetwork_FallbackNoBBR(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "TUNE_DONE:0:64:32768:0\n", nil
	}

	rep := TuneNetwork(nil, mockExec)
	if !rep.Success {
		t.Fatalf("expected success")
	}

	if strings.Contains(rep.BBRStatus, "не поддерживает") == false {
		t.Errorf("expected BBR unsupported message, got: %s", rep.BBRStatus)
	}
	if !strings.Contains(rep.Conntrack, "32768") {
		t.Errorf("expected conntrack 32768, got: %s", rep.Conntrack)
	}
}

func TestTuneNetwork_ExecError(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		return "", errors.New("ssh connection dropped")
	}

	rep := TuneNetwork(nil, mockExec)
	if rep.Success {
		t.Errorf("expected failure on exec error")
	}
}
