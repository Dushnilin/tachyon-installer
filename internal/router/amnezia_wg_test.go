package router

import (
	"encoding/base64"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestGenerateCurve25519Keypair(t *testing.T) {
	priv, pub, err := GenerateCurve25519Keypair()
	if err != nil {
		t.Fatalf("GenerateCurve25519Keypair failed: %v", err)
	}

	privBytes, err := base64.StdEncoding.DecodeString(priv)
	if err != nil || len(privBytes) != 32 {
		t.Fatalf("invalid private key length: %d, err: %v", len(privBytes), err)
	}

	pubBytes, err := base64.StdEncoding.DecodeString(pub)
	if err != nil || len(pubBytes) != 32 {
		t.Fatalf("invalid public key length: %d, err: %v", len(pubBytes), err)
	}

	// Verify WireGuard clamping
	if privBytes[0]&7 != 0 {
		t.Errorf("expected priv[0] to be clamped: %d", privBytes[0])
	}
	if privBytes[31]&128 != 0 || privBytes[31]&64 == 0 {
		t.Errorf("expected priv[31] to be clamped: %d", privBytes[31])
	}
}

func TestParseAWGConfig_INI(t *testing.T) {
	conf := `
[Interface]
PrivateKey = aW5ib3VuZF9wcml2YXRlX2tleQ==
Address = 10.77.0.2/32, fd00::2/128
DNS = 1.1.1.1
MTU = 1360
Jc = 4
Jmin = 40
Jmax = 70
S1 = 20
S2 = 25
H1 = 123456
H2 = 654321
H3 = 111222
H4 = 333444

[Peer]
PublicKey = cGVlcl9wdWJsaWNfa2V5
Endpoint = 198.51.100.1:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`

	cfg, err := ParseAWGConfig(conf)
	if err != nil {
		t.Fatalf("ParseAWGConfig failed: %v", err)
	}

	if cfg.PrivateKey != "aW5ib3VuZF9wcml2YXRlX2tleQ==" {
		t.Errorf("unexpected private key: %s", cfg.PrivateKey)
	}
	if cfg.PeerPublicKey != "cGVlcl9wdWJsaWNfa2V5" {
		t.Errorf("unexpected peer public key: %s", cfg.PeerPublicKey)
	}
	if cfg.ServerAddress != "198.51.100.1" || cfg.ServerPort != 51820 {
		t.Errorf("unexpected endpoint: %s:%d", cfg.ServerAddress, cfg.ServerPort)
	}
	if cfg.Jc != 4 || cfg.Jmin != 40 || cfg.Jmax != 70 || cfg.S1 != 20 || cfg.S2 != 25 {
		t.Errorf("unexpected junk params: Jc=%d Jmin=%d Jmax=%d S1=%d S2=%d", cfg.Jc, cfg.Jmin, cfg.Jmax, cfg.S1, cfg.S2)
	}
	if cfg.H1 != "123456" || cfg.H2 != "654321" {
		t.Errorf("unexpected H1/H2: %s / %s", cfg.H1, cfg.H2)
	}
}

func TestParseAWGConfig_VPNUri(t *testing.T) {
	rawConf := `[Interface]
PrivateKey = aW5ib3VuZF9wcml2YXRlX2tleQ==
Address = 10.77.0.2/32
Jc = 5
Jmin = 40
Jmax = 80
S1 = 15
S2 = 25
H1 = 100
H2 = 200
H3 = 300
H4 = 400
HeaderProtectionKey = aW5ib3VuZF9wcml2YXRlX2tleQ==

[Peer]
PublicKey = cGVlcl9wdWJsaWNfa2V5
Endpoint = [2001:db8::1]:51820
AllowedIPs = 0.0.0.0/0
`
	vpnURI := "vpn://" + base64.RawURLEncoding.EncodeToString([]byte(rawConf))

	cfg, err := ParseAWGConfig(vpnURI)
	if err != nil {
		t.Fatalf("ParseAWGConfig vpn:// failed: %v", err)
	}

	if cfg.Version != "3.1" {
		t.Errorf("expected version 3.1 for HeaderProtectionKey, got: %s", cfg.Version)
	}
	if cfg.ServerAddress != "2001:db8::1" || cfg.ServerPort != 51820 {
		t.Errorf("unexpected ipv6 endpoint: %s:%d", cfg.ServerAddress, cfg.ServerPort)
	}
}

func TestApplyAWGSection(t *testing.T) {
	cfg := &AWGConfig{
		Version:       "2.0",
		Address:       "172.16.0.2/32",
		PrivateKey:    "privkey123",
		PeerPublicKey: "pubkey456",
		ServerAddress: "162.159.192.1",
		ServerPort:    2408,
		Jc:            5,
		Jmin:          40,
		Jmax:          70,
		S1:            20,
		S2:            20,
		H1:            "1",
		H2:            "2",
		H3:            "3",
		H4:            "4",
	}

	var executedCmd string
	mockExec := func(client *gossh.Client, cmd string) (string, error) {
		executedCmd = cmd
		return "", nil
	}

	err := ApplyAWGSection(nil, mockExec, cfg, "awg_test", true)
	if err != nil {
		t.Fatalf("ApplyAWGSection failed: %v", err)
	}

	if !strings.Contains(executedCmd, "uci set tachyon.awg_test.action='awg'") {
		t.Errorf("expected action='awg' in command")
	}
	if !strings.Contains(executedCmd, "uci set tachyon.awg_test.awg_server_address='162.159.192.1'") {
		t.Errorf("expected server_address in command")
	}
	if !strings.Contains(executedCmd, "community_lists='youtube'") {
		t.Errorf("expected youtube in community_lists")
	}
	if !strings.Contains(executedCmd, "uci -q set tachyon.settings.engine='sing-box'") {
		t.Errorf("expected engine='sing-box' in command")
	}
}

func TestTestAWGSection_Mock(t *testing.T) {
	mockExec := func(client *gossh.Client, cmd string) (string, error) {
		if strings.Contains(cmd, "sing-box") && strings.Contains(cmd, "pgrep") {
			return "PROCESS:OK", nil
		}
		if strings.Contains(cmd, "nslookup") {
			return "198.18.0.5", nil
		}
		if strings.Contains(cmd, "probe_site") {
			return "PROBE:YouTube 200 0.12\nPROBE:Discord 200 0.09\nTRACE_DATA:\nip=185.220.101.5\nwarp=on\nloc=DE\nGEOIP_DATA:\nGermany\nFrankfurt\nCloudflare Inc.\n185.220.101.5\n", nil
		}
		return "", nil
	}

	report, err := TestAWGSection(nil, mockExec, "awg")
	if err != nil {
		t.Fatalf("TestAWGSection failed: %v", err)
	}

	if !report.ServiceRunning {
		t.Errorf("expected ServiceRunning = true")
	}
	if !report.FakeIPActive {
		t.Errorf("expected FakeIPActive = true")
	}
	if !report.IsWARP {
		t.Errorf("expected IsWARP = true")
	}
	if report.EgressIP != "185.220.101.5" {
		t.Errorf("unexpected egress IP: %s", report.EgressIP)
	}
	if !report.Success {
		t.Errorf("expected Success = true")
	}
}
