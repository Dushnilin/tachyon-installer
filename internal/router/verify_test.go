package router_test

import (
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"tachyon-installer/internal/router"
)

func TestCheckSubscriptions_Empty(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "tachyon.main.subscription_url") {
			return "", nil
		}
		return "", nil
	}

	status := router.CheckSubscriptions(nil, mockExec)
	if !status.HasEmptySubscription {
		t.Errorf("expected empty subscription, got %v", status)
	}
}

func TestCheckSubscriptions_Present(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "uci show tachyon") {
			return "tachyon.sub0.url='https://myserver.org/api/sub'\n", nil
		}
		return "", nil
	}

	status := router.CheckSubscriptions(nil, mockExec)
	if status.HasEmptySubscription {
		t.Errorf("expected non-empty subscription, got empty")
	}
	if status.CurrentURL != "https://myserver.org/api/sub" {
		t.Errorf("expected CurrentURL='https://myserver.org/api/sub', got %s", status.CurrentURL)
	}
}

func TestVerifyAndFallback_EmptySubscriptionSkipsWaiting(t *testing.T) {
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		// Mock engine
		if strings.Contains(cmd, "engine") {
			return "sing-box-extended\n", nil
		}
		// Subscription is empty
		return "", nil
	}

	res := router.VerifyAndFallback(nil, mockExec, nil)
	if !res.OK || res.Reason != "need_subscription_update" {
		t.Errorf("expected OK=true Reason=need_subscription_update, got OK=%v Reason=%s", res.OK, res.Reason)
	}
}

