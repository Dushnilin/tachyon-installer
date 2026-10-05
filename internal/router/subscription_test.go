package router

import (
	"encoding/base64"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestAnalyzeSubscription(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedValid bool
		expectedType  string
		expectedNodes int
	}{
		{
			name:          "Empty string",
			input:         "   ",
			expectedValid: false,
			expectedType:  "invalid",
		},
		{
			name:          "Valid HTTPS URL",
			input:         "https://my-vpn-sub.example.com/api/v1/sub?token=secret123",
			expectedValid: true,
			expectedType:  "https",
			expectedNodes: 1,
		},
		{
			name:          "Single direct VLESS link",
			input:         "vless://96b1b4b2-297c-4860-9833-c7820ad10cb0@198.51.100.1:443?security=reality&sni=yahoo.com#NL-Server",
			expectedValid: true,
			expectedType:  "direct_links",
			expectedNodes: 1,
		},
		{
			name: "Multiple mixed direct links",
			input: "vless://uuid@1.1.1.1:443?type=tcp#Node1\n" +
				"hysteria2://pass@2.2.2.2:443?sni=example.com#Node2\n" +
				"trojan://pass@3.3.3.3:443#Node3",
			expectedValid: true,
			expectedType:  "direct_links",
			expectedNodes: 3,
		},
		{
			name: "Base64 encoded subscription bundle",
			input: base64.StdEncoding.EncodeToString([]byte(
				"vless://uuid@1.1.1.1:443#NodeA\n" +
					"ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ=@4.4.4.4:8388#NodeB\n" +
					"hy2://pass@5.5.5.5:443#NodeC\n",
			)),
			expectedValid: true,
			expectedType:  "base64",
			expectedNodes: 3,
		},
		{
			name:          "Random gibberish",
			input:         "some random invalid text that is not a url or node",
			expectedValid: false,
			expectedType:  "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := AnalyzeSubscription(tt.input)
			if res.Valid != tt.expectedValid {
				t.Errorf("AnalyzeSubscription(%q) Valid = %v, want %v (error: %s)", tt.input, res.Valid, tt.expectedValid, res.ErrorMsg)
			}
			if res.Type != tt.expectedType {
				t.Errorf("AnalyzeSubscription(%q) Type = %q, want %q", tt.input, res.Type, tt.expectedType)
			}
			if tt.expectedValid && res.NodeCount != tt.expectedNodes {
				t.Errorf("AnalyzeSubscription(%q) NodeCount = %d, want %d", tt.input, res.NodeCount, tt.expectedNodes)
			}
			if tt.expectedValid && !strings.Contains(res.Summary, "Обнаружено") && !strings.Contains(res.Summary, "HTTP(S)") {
				t.Errorf("AnalyzeSubscription(%q) unexpected Summary: %q", tt.input, res.Summary)
			}
		})
	}
}

func TestSaveSubscription_HTTPS(t *testing.T) {
	var executedCmd string
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		executedCmd = cmd
		return "", nil
	}

	err := SaveSubscription(nil, mockExec, "https://sub.example.com/api?token=abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(executedCmd, "community_lists='youtube'") {
		t.Errorf("expected community_lists='youtube' in executed command")
	}
	if !strings.Contains(executedCmd, "community_lists='discord'") {
		t.Errorf("expected community_lists='discord' in executed command")
	}
	if !strings.Contains(executedCmd, "subscription_update main") {
		t.Errorf("expected subscription_update main in executed command")
	}
	if !strings.Contains(executedCmd, "list_update_async") {
		t.Errorf("expected list_update_async in executed command")
	}
	if !strings.Contains(executedCmd, "engine='sing-box'") {
		t.Errorf("expected engine='sing-box' in executed command")
	}
}

func TestSaveSubscription_DirectLinks(t *testing.T) {
	var executedCmd string
	mockExec := func(_ *gossh.Client, cmd string) (string, error) {
		executedCmd = cmd
		return "", nil
	}

	err := SaveSubscription(nil, mockExec, "vless://user@1.2.3.4:443?type=tcp#Node1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(executedCmd, "selector_proxy_links='vless://user@1.2.3.4:443?type=tcp#Node1'") {
		t.Errorf("expected selector_proxy_links with vless link")
	}
	if !strings.Contains(executedCmd, "community_lists='youtube'") {
		t.Errorf("expected community_lists='youtube' in executed command")
	}
	if !strings.Contains(executedCmd, "list_update_async") {
		t.Errorf("expected list_update_async in executed command")
	}
}
