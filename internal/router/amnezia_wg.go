package router

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/curve25519"
	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// AWGConfig represents an AmneziaWG configuration structure.
type AWGConfig struct {
	Version                string `json:"version"` // "2.0", "3.0", "3.1"
	Address                string `json:"address"` // e.g. "172.16.0.2/32" or "10.77.0.2/32, fd00::2/128"
	PrivateKey             string `json:"private_key"`
	PeerPublicKey          string `json:"peer_public_key"`
	ServerAddress          string `json:"server_address"`
	ServerPort             int    `json:"server_port"`
	PresharedKey           string `json:"preshared_key,omitempty"`
	MTU                    int    `json:"mtu,omitempty"`
	Keepalive              string `json:"keepalive,omitempty"`
	DNS                    string `json:"dns,omitempty"`

	// Obfuscation parameters (AWG 2.0 / 3.x)
	Jc   int    `json:"jc"`   // Junk packet count (e.g. 4)
	Jmin int    `json:"jmin"` // Min junk packet size (e.g. 40)
	Jmax int    `json:"jmax"` // Max junk packet size (e.g. 70)
	S1   int    `json:"s1"`   // Init packet junk size (e.g. 20)
	S2   int    `json:"s2"`   // Response packet junk size (e.g. 20)
	S3   int    `json:"s3"`   // Cookie packet junk size
	S4   int    `json:"s4"`   // Data packet junk size
	H1   string `json:"h1"`   // Handshake init packet header magic (number or range)
	H2   string `json:"h2"`   // Handshake resp packet header magic
	H3   string `json:"h3"`   // Cookie packet header magic
	H4   string `json:"h4"`   // Transport packet header magic
	I1   string `json:"i1,omitempty"` // Decoy payload 1
	I2   string `json:"i2,omitempty"` // Decoy payload 2
	I3   string `json:"i3,omitempty"` // Decoy payload 3
	I4   string `json:"i4,omitempty"` // Decoy payload 4
	I5   string `json:"i5,omitempty"` // Decoy payload 5

	// AWG 3.1 parameters
	HeaderProtectionKey    string `json:"header_protection_key,omitempty"`
	ContentPaddingAddition string `json:"content_padding_addition,omitempty"`
}

// AWGTestReport holds the outcome of testing the AmneziaWG section.
type AWGTestReport struct {
	SectionName    string
	ServiceRunning bool
	EngineName     string
	FakeIPActive   bool
	ResolvedIP     string
	EgressIP       string
	EgressLocation string
	EgressISP      string
	IsWARP         bool
	LatencyMs      int64
	Success        bool
	Details        string
	Probes         []TargetProbe
}

// FormatSummary formats a clean human-readable summary of the test report.
func (r AWGTestReport) FormatSummary() string {
	var sb strings.Builder

	if r.ServiceRunning {
		sb.WriteString(fmt.Sprintf("  • Ядро прокси:   [#22c55e]%s (работает) ✓[-]\n", r.EngineName))
	} else {
		sb.WriteString(fmt.Sprintf("  • Ядро прокси:   [#ef5350]%s (не запущен) ✗[-]\n", r.EngineName))
	}

	if r.FakeIPActive {
		sb.WriteString(fmt.Sprintf("  • DNS Fake-IP:   [#22c55e]перехвачен (%s)[-]\n", r.ResolvedIP))
	} else if r.ResolvedIP != "" {
		sb.WriteString(fmt.Sprintf("  • DNS Запросы:   [#38bdf8]прямой ответ (%s)[-]\n", r.ResolvedIP))
	} else {
		sb.WriteString("  • DNS Запросы:   [#ef5350]нет ответа[-]\n")
	}

	if r.EgressIP != "" {
		warpBadge := ""
		if r.IsWARP {
			warpBadge = " [#a855f7:b][WARP Active][-] "
		}
		loc := r.EgressLocation
		if r.EgressISP != "" {
			loc += fmt.Sprintf(" (%s)", r.EgressISP)
		}
		sb.WriteString(fmt.Sprintf("  • Выходной IP:   [#38bdf8:b]%s[-]%s[#94a3b8]%s[-]\n", r.EgressIP, warpBadge, loc))
	}

	for _, p := range r.Probes {
		if p.Success {
			sb.WriteString(fmt.Sprintf("  • %-14s [#22c55e]HTTP %d (%d мс) ✓[-]\n", p.Name+":", p.HTTPStatus, p.LatencyMs))
		} else {
			sb.WriteString(fmt.Sprintf("  • %-14s [#eab308]HTTP %d (таймаут/недоступен)[-]\n", p.Name+":", p.HTTPStatus))
		}
	}

	return sb.String()
}

// GenerateCurve25519Keypair generates a valid Curve25519 keypair for WireGuard/AmneziaWG.
func GenerateCurve25519Keypair() (privBase64, pubBase64 string, err error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Apply WireGuard / RFC 7748 clamping
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)

	return base64.StdEncoding.EncodeToString(priv[:]), base64.StdEncoding.EncodeToString(pub[:]), nil
}

func randomIntBetween(minVal, maxVal int) int {
	if minVal >= maxVal {
		return minVal
	}
	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(maxVal-minVal+1)))
	if err != nil {
		return minVal
	}
	return minVal + int(nBig.Int64())
}

// GenerateRandomAWGParameters populates recommended obfuscation headers.
func GenerateRandomAWGParameters(cfg *AWGConfig) {
	if cfg.Jc == 0 {
		cfg.Jc = randomIntBetween(3, 7)
	}
	if cfg.Jmin == 0 {
		cfg.Jmin = randomIntBetween(40, 50)
	}
	if cfg.Jmax == 0 {
		cfg.Jmax = randomIntBetween(70, 100)
	}
	if cfg.S1 == 0 {
		cfg.S1 = randomIntBetween(15, 35)
	}
	if cfg.S2 == 0 {
		cfg.S2 = randomIntBetween(15, 35)
	}
	if cfg.H1 == "" {
		cfg.H1 = strconv.Itoa(randomIntBetween(100000000, 999999999))
	}
	if cfg.H2 == "" {
		cfg.H2 = strconv.Itoa(randomIntBetween(100000000, 999999999))
	}
	if cfg.H3 == "" {
		cfg.H3 = strconv.Itoa(randomIntBetween(100000000, 999999999))
	}
	if cfg.H4 == "" {
		cfg.H4 = strconv.Itoa(randomIntBetween(100000000, 999999999))
	}
	if cfg.Version == "" {
		cfg.Version = "2.0"
	}
	if cfg.MTU == 0 {
		cfg.MTU = 1280
	}
	if cfg.Keepalive == "" {
		cfg.Keepalive = "25"
	}
}

// GenerateCustomAWGConfig creates an AmneziaWG configuration with generated keypair and random obfuscation.
func GenerateCustomAWGConfig(serverAddress string, serverPort int, peerPublicKey string) (*AWGConfig, error) {
	priv, _, err := GenerateCurve25519Keypair()
	if err != nil {
		return nil, err
	}

	cfg := &AWGConfig{
		Version:       "2.0",
		Address:       "10.77.0.2/32",
		PrivateKey:    priv,
		PeerPublicKey: peerPublicKey,
		ServerAddress: serverAddress,
		ServerPort:    serverPort,
		MTU:           1280,
		Keepalive:     "25",
	}

	GenerateRandomAWGParameters(cfg)
	return cfg, nil
}

// GenerateWarpAWG generates a Cloudflare WARP profile with AmneziaWG obfuscation.
// It first attempts running Tachyon's built-in generator on the router;
// if unavailable or failing, it falls back to direct Cloudflare client API registration.
func GenerateWarpAWG(client *gossh.Client, execFn sshutil.ExecFunc) (*AWGConfig, error) {
	// 1. Try router-native generator if available
	if client != nil && execFn != nil {
		out, err := execFn(client, "if [ -x /usr/bin/tachyon ]; then /usr/bin/tachyon generate_warp '' '2.0' 2>/dev/null || true ; fi")
		if err == nil {
			out = strings.TrimSpace(out)
			if strings.HasPrefix(out, "{") && strings.Contains(out, `"success"`) {
				var r struct {
					Success       bool   `json:"success"`
					LocalAddress  string `json:"local_address"`
					PrivateKey    string `json:"private_key"`
					PeerPublicKey string `json:"peer_public_key"`
					ServerAddress string `json:"server_address"`
					ServerPort    any    `json:"server_port"`
					AWGVersion    string `json:"awg_version"`
					AWGJc         any    `json:"awg_jc"`
					AWGJmin       any    `json:"awg_jmin"`
					AWGJmax       any    `json:"awg_jmax"`
					AWGS1         any    `json:"awg_s1"`
					AWGS2         any    `json:"awg_s2"`
					AWGS3         any    `json:"awg_s3"`
					AWGS4         any    `json:"awg_s4"`
					AWGH1         any    `json:"awg_h1"`
					AWGH2         any    `json:"awg_h2"`
					AWGH3         any    `json:"awg_h3"`
					AWGH4         any    `json:"awg_h4"`
					AWGI1         string `json:"awg_i1"`
					AWGI2         string `json:"awg_i2"`
					AWGI3         string `json:"awg_i3"`
					AWGI4         string `json:"awg_i4"`
					AWGI5         string `json:"awg_i5"`
				}

				if jsonErr := json.Unmarshal([]byte(out), &r); jsonErr == nil && r.Success {
					toInt := func(val any) int {
						switch v := val.(type) {
						case float64:
							return int(v)
						case string:
							i, _ := strconv.Atoi(v)
							return i
						}
						return 0
					}
					toStr := func(val any) string {
						if val == nil {
							return ""
						}
						return fmt.Sprintf("%v", val)
					}

					cfg := &AWGConfig{
						Version:       "2.0",
						Address:       r.LocalAddress,
						PrivateKey:    r.PrivateKey,
						PeerPublicKey: r.PeerPublicKey,
						ServerAddress: r.ServerAddress,
						ServerPort:    toInt(r.ServerPort),
						Jc:            toInt(r.AWGJc),
						Jmin:          toInt(r.AWGJmin),
						Jmax:          toInt(r.AWGJmax),
						S1:            toInt(r.AWGS1),
						S2:            toInt(r.AWGS2),
						S3:            toInt(r.AWGS3),
						S4:            toInt(r.AWGS4),
						H1:            toStr(r.AWGH1),
						H2:            toStr(r.AWGH2),
						H3:            toStr(r.AWGH3),
						H4:            toStr(r.AWGH4),
						I1:            r.AWGI1,
						I2:            r.AWGI2,
						I3:            r.AWGI3,
						I4:            r.AWGI4,
						I5:            r.AWGI5,
						MTU:           1280,
						Keepalive:     "25",
					}
					if cfg.ServerPort == 0 {
						cfg.ServerPort = 2408
					}
					return cfg, nil
				}
			}
		}
	}

	// 2. Direct Go fallback via Cloudflare WARP registration API
	privKey, pubKey, err := GenerateCurve25519Keypair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate Curve25519 keypair: %w", err)
	}

	reqBody := map[string]any{
		"key":           pubKey,
		"install_id":    "",
		"fcm_token":     "",
		"tos":           time.Now().UTC().Format(time.RFC3339Nano),
		"model":         "PC",
		"serial_number": "",
		"locale":        "en_US",
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://api.cloudflareclient.com/v0a2158/reg", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("User-Agent", "okhttp/3.12.1")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cloudflare WARP API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var warpRes struct {
		ID     string `json:"id"`
		Config struct {
			Interface struct {
				Addresses struct {
					V4 string `json:"v4"`
					V6 string `json:"v6"`
				} `json:"addresses"`
			} `json:"interface"`
			Peers []struct {
				PublicKey string `json:"public_key"`
				Endpoint  struct {
					V4   string `json:"v4"`
					V6   string `json:"v6"`
					Host string `json:"host"`
				} `json:"endpoint"`
			} `json:"peers"`
		} `json:"config"`
	}

	if err := json.Unmarshal(respBytes, &warpRes); err != nil {
		return nil, fmt.Errorf("failed to parse Cloudflare WARP registration response: %w", err)
	}

	if warpRes.Config.Interface.Addresses.V4 == "" {
		return nil, fmt.Errorf("Cloudflare WARP API did not return an assigned IP (response status: %d)", resp.StatusCode)
	}

	serverIP := "162.159.192.1"
	if len(warpRes.Config.Peers) > 0 && warpRes.Config.Peers[0].Endpoint.V4 != "" {
		host, _, splitErr := net.SplitHostPort(warpRes.Config.Peers[0].Endpoint.V4)
		if splitErr == nil && host != "" {
			serverIP = host
		}
	}

	peerPub := "bmXIKPyTXGG1hbmSfgTLi0h644qThu+Bcg0OK+jWZTU="
	if len(warpRes.Config.Peers) > 0 && warpRes.Config.Peers[0].PublicKey != "" {
		peerPub = warpRes.Config.Peers[0].PublicKey
	}

	addr := warpRes.Config.Interface.Addresses.V4 + "/32"
	if warpRes.Config.Interface.Addresses.V6 != "" {
		addr += " " + warpRes.Config.Interface.Addresses.V6 + "/128"
	}

	cfg := &AWGConfig{
		Version:       "2.0",
		Address:       addr,
		PrivateKey:    privKey,
		PeerPublicKey: peerPub,
		ServerAddress: serverIP,
		ServerPort:    2408,
		MTU:           1280,
		Keepalive:     "25",
	}

	GenerateRandomAWGParameters(cfg)
	return cfg, nil
}

// ParseAWGConfig parses standard INI .conf, JSON, or vpn:// Amnezia containers.
func ParseAWGConfig(raw string) (*AWGConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("configuration string cannot be empty")
	}

	// 1. Check for vpn:// Amnezia format
	if strings.HasPrefix(raw, "vpn://") {
		payload := strings.TrimPrefix(raw, "vpn://")
		if idx := strings.Index(payload, "#"); idx >= 0 {
			payload = payload[:idx]
		}
		// Base64 decode URL/standard
		decoded, err := base64.RawURLEncoding.DecodeString(payload)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(payload)
		}
		if err == nil && len(decoded) > 0 {
			raw = string(decoded)
		}
	}

	// 2. Check for JSON format
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		var amneziaJSON struct {
			Containers []struct {
				Container string `json:"container"`
				AWG       struct {
					LastConfig string `json:"last_config"`
				} `json:"awg"`
			} `json:"containers"`
		}
		if jsonErr := json.Unmarshal([]byte(raw), &amneziaJSON); jsonErr == nil {
			for _, c := range amneziaJSON.Containers {
				if (c.Container == "amnezia-awg" || c.Container == "wireguard") && c.AWG.LastConfig != "" {
					var inner struct {
						Config string `json:"config"`
					}
					if err := json.Unmarshal([]byte(c.AWG.LastConfig), &inner); err == nil && inner.Config != "" {
						raw = inner.Config
						break
					}
				}
			}
		}
	}

	// 3. Parse INI [Interface] and [Peer]
	lines := strings.Split(raw, "\n")
	cfg := &AWGConfig{
		Version:   "2.0",
		MTU:       1280,
		Keepalive: "25",
	}

	curBlock := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.EqualFold(line, "[Interface]") {
			curBlock = "interface"
			continue
		}
		if strings.EqualFold(line, "[Peer]") {
			curBlock = "peer"
			continue
		}
		if strings.HasPrefix(line, "[") {
			curBlock = ""
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || curBlock == "" {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		// Strip inline comments if not a payload tag like <b ...>
		if !strings.HasPrefix(val, "<") {
			if commentIdx := strings.IndexAny(val, "#;"); commentIdx >= 0 {
				val = strings.TrimSpace(val[:commentIdx])
			}
		}

		switch key {
		case "privatekey", "private_key":
			cfg.PrivateKey = val
		case "publickey", "public_key", "peer_public_key":
			cfg.PeerPublicKey = val
		case "presharedkey", "preshared_key", "psk":
			cfg.PresharedKey = val
		case "address", "local_address", "ip":
			// Normalize commas to spaces
			cfg.Address = strings.Join(strings.Fields(strings.ReplaceAll(val, ",", " ")), " ")
		case "dns":
			cfg.DNS = val
		case "mtu":
			cfg.MTU, _ = strconv.Atoi(val)
		case "persistentkeepalive", "persistent_keepalive", "keepalive":
			cfg.Keepalive = val
		case "endpoint":
			if host, port, err := net.SplitHostPort(val); err == nil {
				cfg.ServerAddress = host
				cfg.ServerPort, _ = strconv.Atoi(port)
			} else {
				// Fallback split
				idx := strings.LastIndex(val, ":")
				if idx > 0 {
					cfg.ServerAddress = val[:idx]
					cfg.ServerPort, _ = strconv.Atoi(val[idx+1:])
				}
			}
		case "jc":
			cfg.Jc, _ = strconv.Atoi(val)
		case "jmin":
			cfg.Jmin, _ = strconv.Atoi(val)
		case "jmax":
			cfg.Jmax, _ = strconv.Atoi(val)
		case "s1":
			cfg.S1, _ = strconv.Atoi(val)
		case "s2":
			cfg.S2, _ = strconv.Atoi(val)
		case "s3":
			cfg.S3, _ = strconv.Atoi(val)
		case "s4":
			cfg.S4, _ = strconv.Atoi(val)
		case "h1":
			cfg.H1 = val
		case "h2":
			cfg.H2 = val
		case "h3":
			cfg.H3 = val
		case "h4":
			cfg.H4 = val
		case "i1":
			cfg.I1 = val
		case "i2":
			cfg.I2 = val
		case "i3":
			cfg.I3 = val
		case "i4":
			cfg.I4 = val
		case "i5":
			cfg.I5 = val
		case "headerprotectionkey", "header_protection_key":
			cfg.HeaderProtectionKey = val
			cfg.Version = "3.1"
		case "contentpaddingaddition", "content_padding_addition":
			cfg.ContentPaddingAddition = val
			cfg.Version = "3.1"
		}
	}

	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("отсутствует PrivateKey в конфигурации")
	}
	if cfg.PeerPublicKey == "" {
		return nil, fmt.Errorf("отсутствует PublicKey (Peer) в конфигурации")
	}
	if cfg.ServerAddress == "" || cfg.ServerPort == 0 {
		return nil, fmt.Errorf("отсутствует корректный Endpoint (server:port) в конфигурации")
	}
	if cfg.Address == "" {
		cfg.Address = "10.77.0.2/32"
	}

	GenerateRandomAWGParameters(cfg)
	return cfg, nil
}

// ApplyAWGSection saves the AmneziaWG configuration to Tachyon UCI and restarts the service.
func ApplyAWGSection(client *gossh.Client, execFn sshutil.ExecFunc, cfg *AWGConfig, sectionName string, enableCommunityLists bool) error {
	if cfg == nil {
		return fmt.Errorf("nil AWG configuration")
	}
	if sectionName == "" {
		sectionName = "awg"
	}

	sec := strings.TrimSpace(sectionName)
	var cmd strings.Builder

	cmd.WriteString(fmt.Sprintf(`
# 1. Initialize section in Tachyon UCI
uci -q get tachyon.%s >/dev/null || uci set tachyon.%s=section
uci set tachyon.%s.label='AmneziaWG'
uci set tachyon.%s.enabled='1'
uci set tachyon.%s.action='awg'
uci set tachyon.%s.awg_version='%s'
uci set tachyon.%s.awg_local_address='%s'
uci set tachyon.%s.awg_private_key='%s'
uci set tachyon.%s.awg_peer_public_key='%s'
uci set tachyon.%s.awg_server_address='%s'
uci set tachyon.%s.awg_server_port='%d'
uci set tachyon.%s.awg_jc='%d'
uci set tachyon.%s.awg_jmin='%d'
uci set tachyon.%s.awg_jmax='%d'
uci set tachyon.%s.awg_s1='%d'
uci set tachyon.%s.awg_s2='%d'
uci set tachyon.%s.awg_h1='%s'
uci set tachyon.%s.awg_h2='%s'
uci set tachyon.%s.awg_h3='%s'
uci set tachyon.%s.awg_h4='%s'
`,
		sec, sec,
		sec, sec, sec,
		sec, cfg.Version,
		sec, cfg.Address,
		sec, cfg.PrivateKey,
		sec, cfg.PeerPublicKey,
		sec, cfg.ServerAddress,
		sec, cfg.ServerPort,
		sec, cfg.Jc,
		sec, cfg.Jmin,
		sec, cfg.Jmax,
		sec, cfg.S1,
		sec, cfg.S2,
		sec, cfg.H1,
		sec, cfg.H2,
		sec, cfg.H3,
		sec, cfg.H4,
	))

	if cfg.S3 > 0 {
		cmd.WriteString(fmt.Sprintf("uci set tachyon.%s.awg_s3='%d'\n", sec, cfg.S3))
	}
	if cfg.S4 > 0 {
		cmd.WriteString(fmt.Sprintf("uci set tachyon.%s.awg_s4='%d'\n", sec, cfg.S4))
	}
	if cfg.PresharedKey != "" {
		cmd.WriteString(fmt.Sprintf("uci set tachyon.%s.awg_preshared_key='%s'\n", sec, cfg.PresharedKey))
	}
	if cfg.MTU > 0 {
		cmd.WriteString(fmt.Sprintf("uci set tachyon.%s.awg_mtu='%d'\n", sec, cfg.MTU))
	}
	if cfg.Keepalive != "" {
		cmd.WriteString(fmt.Sprintf("uci set tachyon.%s.awg_keepalive='%s'\n", sec, cfg.Keepalive))
	}
	if cfg.I1 != "" {
		cmd.WriteString(fmt.Sprintf("uci set tachyon.%s.awg_i1='%s'\n", sec, cfg.I1))
	}
	if cfg.I2 != "" {
		cmd.WriteString(fmt.Sprintf("uci set tachyon.%s.awg_i2='%s'\n", sec, cfg.I2))
	}
	if cfg.HeaderProtectionKey != "" {
		cmd.WriteString(fmt.Sprintf("uci set tachyon.%s.awg_header_protection_key='%s'\n", sec, cfg.HeaderProtectionKey))
	}
	if cfg.ContentPaddingAddition != "" {
		cmd.WriteString(fmt.Sprintf("uci set tachyon.%s.awg_content_padding_addition='%s'\n", sec, cfg.ContentPaddingAddition))
	}

	if enableCommunityLists {
		cmd.WriteString(fmt.Sprintf(`
# 2. Attach essential community lists for RU routing
uci -q del_list tachyon.%s.community_lists 2>/dev/null || true
uci -q add_list tachyon.%s.community_lists='russia_inside' 2>/dev/null || true
uci -q add_list tachyon.%s.community_lists='youtube' 2>/dev/null || true
uci -q add_list tachyon.%s.community_lists='discord' 2>/dev/null || true
uci -q add_list tachyon.%s.community_lists='meta' 2>/dev/null || true
uci -q add_list tachyon.%s.community_lists='twitter' 2>/dev/null || true
`, sec, sec, sec, sec, sec, sec))
	}

	cmd.WriteString(`
# 3. Ensure sing-box engine is active and enabled
uci -q set tachyon.settings.engine='sing-box' 2>/dev/null || true
uci -q set tachyon.settings.enabled='1' 2>/dev/null || true
uci commit tachyon 2>/dev/null || true

# 4. Synchronously fetch rulesets (.srs)
if [ -x /usr/bin/tachyon ]; then
    /usr/bin/tachyon list_update_async >/dev/null 2>&1 || /usr/bin/tachyon list_update >/dev/null 2>&1 || true
fi

# 5. Enable and restart Tachyon service
/etc/init.d/tachyon enable >/dev/null 2>&1 || true
/etc/init.d/tachyon restart 2>&1 || /usr/bin/tachyon restart 2>&1 || true
`)

	_, err := execFn(client, cmd.String())
	return err
}

// TestAWGSection verifies that the newly configured AmneziaWG section routes traffic and bypasses censorship.
func TestAWGSection(client *gossh.Client, execFn sshutil.ExecFunc, sectionName string) (*AWGTestReport, error) {
	if sectionName == "" {
		sectionName = "awg"
	}

	report := &AWGTestReport{
		SectionName: sectionName,
		EngineName:  "sing-box",
		Probes:      []TargetProbe{},
	}

	// 1. Process check
	procCheck := `
if pgrep -x sing-box >/dev/null || pgrep -f "sing-box" >/dev/null; then
    echo "PROCESS:OK"
else
    echo "PROCESS:FAILED"
fi
`
	procOut, _ := execFn(client, procCheck)
	if strings.Contains(procOut, "PROCESS:OK") {
		report.ServiceRunning = true
	}

	// 2. DNS Fake-IP check
	dnsCmd := `
out=$(nslookup youtube.com 127.0.0.1 2>/dev/null || nslookup youtube.com 2>/dev/null || echo "")
echo "$out" | grep -iE 'Address|Адрес' | tail -n 1 | awk '{print $NF}'
`
	dnsOut, _ := execFn(client, dnsCmd)
	resolvedIP := strings.TrimSpace(dnsOut)
	report.ResolvedIP = resolvedIP
	if strings.HasPrefix(resolvedIP, "198.18.") || strings.HasPrefix(resolvedIP, "198.19.") {
		report.FakeIPActive = true
	}

	// 3. Probes and Egress trace
	testScript := `
probe_site() {
    name="$1"
    url="$2"
    if command -v curl >/dev/null 2>&1; then
        res=$(curl -m 7 -s -L -o /dev/null -w "%{http_code} %{time_total}" "$url" 2>/dev/null || echo "000 0")
    else
        if wget -q --spider --timeout=7 "$url" 2>/dev/null; then
            res="200 0.15"
        else
            res="000 0"
        fi
    fi
    echo "PROBE:$name $res"
}

probe_site "YouTube"   "https://www.youtube.com" &
probe_site "Discord"   "https://discord.com" &
probe_site "Rutracker" "https://rutracker.org" &
wait

# Egress trace via Cloudflare trace & GeoIP
if command -v curl >/dev/null 2>&1; then
    trace=$(curl -m 5 -s "https://cloudflare.com/cdn-cgi/trace" 2>/dev/null || echo "")
    if [ -n "$trace" ]; then
        echo "TRACE_DATA:"
        echo "$trace"
    fi
    geo=$(curl -m 4 -s "http://ip-api.com/line?fields=country,city,isp,query" 2>/dev/null || echo "")
    if [ -n "$geo" ]; then
        echo "GEOIP_DATA:"
        echo "$geo"
    fi
fi
`
	testOut, err := execFn(client, testScript)
	if err != nil && testOut == "" {
		return report, fmt.Errorf("ssh execution error: %w", err)
	}

	lines := strings.Split(testOut, "\n")
	inTrace := false
	inGeo := false
	var geoLines []string

	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "TRACE_DATA:") {
			inTrace = true
			inGeo = false
			continue
		}
		if strings.HasPrefix(l, "GEOIP_DATA:") {
			inGeo = true
			inTrace = false
			continue
		}

		if inTrace {
			if strings.HasPrefix(l, "ip=") {
				report.EgressIP = strings.TrimPrefix(l, "ip=")
			} else if strings.HasPrefix(l, "warp=") {
				warpVal := strings.TrimPrefix(l, "warp=")
				if warpVal == "on" || warpVal == "plus" {
					report.IsWARP = true
				}
			} else if strings.HasPrefix(l, "loc=") {
				report.EgressLocation = strings.TrimPrefix(l, "loc=")
			}
			continue
		}

		if inGeo {
			if l != "" {
				geoLines = append(geoLines, l)
			}
			continue
		}

		if strings.HasPrefix(l, "PROBE:") {
			raw := strings.TrimPrefix(l, "PROBE:")
			parts := strings.Fields(raw)
			if len(parts) >= 3 {
				name := parts[0]
				statusCode, _ := strconv.Atoi(parts[1])
				var latMs int64
				if durSec, err := strconv.ParseFloat(parts[2], 64); err == nil {
					latMs = int64(durSec * 1000.0)
				}
				// 200/204 or non-302 2xx/3xx (reject Russian ISP 302 warning blockpages)
				success := (statusCode == 200 || statusCode == 204 || (statusCode >= 200 && statusCode < 400 && statusCode != 302))
				probe := TargetProbe{
					Name:       name,
					HTTPStatus: statusCode,
					LatencyMs:  latMs,
					Success:    success,
				}
				report.Probes = append(report.Probes, probe)
				if name == "YouTube" && success {
					report.Success = true
					report.LatencyMs = latMs
				}
				if name == "Discord" && success {
					report.Success = true
				}
			}
		}
	}

	if len(geoLines) >= 4 {
		if report.EgressLocation == "" {
			report.EgressLocation = geoLines[0]
		}
		report.EgressISP = geoLines[2]
		if report.EgressIP == "" {
			report.EgressIP = geoLines[3]
		}
	}

	if report.Success {
		report.Details = "Обход успешно работает через AmneziaWG"
	} else if report.ServiceRunning {
		report.Details = "Ядро запущено, но тестовые сайты не ответили (проверьте блокировку UDP или параметры сервера)"
	} else {
		report.Details = "Служба прокси не запущена"
	}

	return report, nil
}
