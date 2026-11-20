package router

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// SubscriptionStatus holds the result of checking subscription configuration.
type SubscriptionStatus struct {
	HasEmptySubscription bool
	SectionName          string
	CurrentURL           string
	MissingKernelMods    []string // e.g. "kmod-nft-tproxy", "kmod-tun"
}

// SubscriptionAnalysis holds validation & metadata about a user-provided subscription.
type SubscriptionAnalysis struct {
	Valid     bool
	Type      string   // "https", "direct_links", "base64", "invalid"
	Summary   string   // e.g. "Обнаружено 8 узлов (VLESS Reality, Hysteria 2)"
	NodeCount int
	Protocols []string // e.g. ["VLESS", "Hysteria 2"]
	Nodes     []string // Individual node links if direct or base64 decoded
	ErrorMsg  string
}

var knownProtocols = map[string]string{
	"vless":       "VLESS",
	"vmess":       "VMess",
	"trojan":      "Trojan",
	"ss":          "Shadowsocks",
	"shadowsocks": "Shadowsocks",
	"ssr":         "ShadowsocksR",
	"hy2":         "Hysteria 2",
	"hysteria2":   "Hysteria 2",
	"hysteria":    "Hysteria",
	"tuic":        "TUIC",
	"anytls":      "AnyTLS",
	"wireguard":   "WireGuard",
	"wg":          "WireGuard",
	"awg":         "AmneziaWG",
	"amneziawg":   "AmneziaWG",
	"socks":       "SOCKS5",
	"socks5":      "SOCKS5",
	"http":        "HTTP Proxy",
	"https":       "HTTPS Proxy",
	"naive+https": "NaiveProxy",
	"naive":       "NaiveProxy",
	"mieru":       "Mieru",
	"mierus":      "Mieru",
	"csqtt":       "CSQTT",
	"wdtt":        "WDTT",
	"qwdtt":       "QWDTT",
	"openvpn":     "OpenVPN",
	"tailscale":   "Tailscale",
	"snell":       "Snell",
	"sudoku":      "Sudoku",
	"masque":      "MASQUE",
	"tewfai":      "Tewfai",
	"trusttunnel": "TrustTunnel",
	"openconnect": "OpenConnect",
	"tg":          "MTProto",
	"mtproto":     "MTProto",
	"warp":        "WARP",
	"fptn":        "FPTN",
	"fptnb":       "FPTN",
	"happ":        "Happ",
}

// AnalyzeSubscription inspects a raw string to determine if it is a valid HTTP(S) link,
// Happ encrypted subscription, raw proxy URIs (vless://, hysteria2://, fptn:, etc.), or a base64 encoded bundle.
func AnalyzeSubscription(raw string) SubscriptionAnalysis {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SubscriptionAnalysis{
			Valid:    false,
			Type:     "invalid",
			ErrorMsg: "Строка подписки не может быть пустой",
		}
	}

	// 1. Direct HTTP/HTTPS subscription URL or Happ crypt4 subscription
	if !strings.ContainsAny(raw, " \t\r\n") && (strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")) {
		parsed, err := url.ParseRequestURI(raw)
		if err != nil || parsed.Host == "" {
			return SubscriptionAnalysis{
				Valid:    false,
				Type:     "invalid",
				ErrorMsg: "Некорректный формат HTTP/HTTPS URL",
			}
		}
		return SubscriptionAnalysis{
			Valid:     true,
			Type:      "https",
			Summary:   fmt.Sprintf("HTTP(S) ссылка на сервер подписки (%s)", parsed.Host),
			NodeCount: 1,
			Protocols: []string{"Авто-обновление"},
		}
	}
	if !strings.ContainsAny(raw, " \t\r\n") && strings.HasPrefix(strings.ToLower(raw), "happ://crypt4/") {
		return SubscriptionAnalysis{
			Valid:     true,
			Type:      "https",
			Summary:   "Зашифрованная ссылка подписки Happ (crypt4)",
			NodeCount: 1,
			Protocols: []string{"Happ crypt4"},
		}
	}

	// Helper to extract nodes and protocol counts (supports multiple share URIs per line)
	parseNodes := func(lines []string, srcType string) (SubscriptionAnalysis, bool) {
		var validNodes []string
		protoCounts := make(map[string]int)

		for _, rawLine := range lines {
			rawLine = strings.TrimSpace(rawLine)
			if rawLine == "" || strings.HasPrefix(rawLine, "#") {
				continue
			}
			tokens := []string{rawLine}
			if strings.Count(rawLine, "://") > 1 {
				tokens = strings.FieldsFunc(rawLine, func(r rune) bool {
					return r == ' ' || r == '\t' || r == ',' || r == ';'
				})
			}
			for _, token := range tokens {
				token = strings.TrimSpace(token)
				if token == "" {
					continue
				}
				lowerToken := strings.ToLower(token)
				if (strings.HasPrefix(lowerToken, "fptn:") || strings.HasPrefix(lowerToken, "fptnb:")) && !strings.Contains(lowerToken, "://") {
					validNodes = append(validNodes, token)
					protoCounts["FPTN"]++
					continue
				}
				parts := strings.SplitN(token, "://", 2)
				if len(parts) == 2 {
					scheme := strings.ToLower(parts[0])
					if name, ok := knownProtocols[scheme]; ok {
						validNodes = append(validNodes, token)
						protoCounts[name]++
					}
				}
			}
		}

		if len(validNodes) == 0 {
			return SubscriptionAnalysis{}, false
		}

		var protoNames []string
		for name := range protoCounts {
			protoNames = append(protoNames, name)
		}
		sort.Strings(protoNames)

		var protoSummary []string
		for _, name := range protoNames {
			protoSummary = append(protoSummary, fmt.Sprintf("%s: %d", name, protoCounts[name]))
		}

		summary := fmt.Sprintf("Обнаружено узлов: %d (%s)", len(validNodes), strings.Join(protoSummary, ", "))
		return SubscriptionAnalysis{
			Valid:     true,
			Type:      srcType,
			Summary:   summary,
			NodeCount: len(validNodes),
			Protocols: protoNames,
			Nodes:     validNodes,
		}, true
	}

	// 2. Direct URI links (one or multiple lines)
	if lines := strings.Split(raw, "\n"); len(lines) > 0 {
		if res, ok := parseNodes(lines, "direct_links"); ok {
			return res
		}
	}

	// 3. Base64 encoded payload
	// Try multiple encodings: Standard, RawStd, URL, RawURL
	tryBase64 := func(s string) ([]byte, error) {
		s = strings.TrimSpace(s)
		if data, err := base64.StdEncoding.DecodeString(s); err == nil {
			return data, nil
		}
		if data, err := base64.RawStdEncoding.DecodeString(s); err == nil {
			return data, nil
		}
		if data, err := base64.URLEncoding.DecodeString(s); err == nil {
			return data, nil
		}
		return base64.RawURLEncoding.DecodeString(s)
	}

	if decoded, err := tryBase64(raw); err == nil && len(decoded) > 0 {
		decodedLines := strings.Split(string(decoded), "\n")
		if res, ok := parseNodes(decodedLines, "base64"); ok {
			res.Summary = "[Base64] " + res.Summary
			return res
		}
	}

	return SubscriptionAnalysis{
		Valid:    false,
		Type:     "invalid",
		ErrorMsg: "Неизвестный формат: укажите https:// ссылку, конфигурационные ссылки (vless://...) или base64",
	}
}

// CheckSubscriptions inspects the UCI config and installed packages.
func CheckSubscriptions(client *gossh.Client, execFn sshutil.ExecFunc) SubscriptionStatus {
	status := SubscriptionStatus{SectionName: "main"}

	pkgOut, _ := execFn(client, "(opkg list-installed 2>/dev/null || apk info 2>/dev/null); [ -d /sys/module/nft_tproxy ] && echo kmod-nft-tproxy; [ -d /sys/module/tun ] && echo kmod-tun")
	if !strings.Contains(pkgOut, "kmod-nft-tproxy") {
		status.MissingKernelMods = append(status.MissingKernelMods, "kmod-nft-tproxy")
	}
	if !strings.Contains(pkgOut, "kmod-tun") {
		status.MissingKernelMods = append(status.MissingKernelMods, "kmod-tun")
	}

	uciOut, uciErr := execFn(client, "uci show tachyon 2>/dev/null")
	if uciErr != nil || strings.TrimSpace(uciOut) == "" {
		// Check Tachyon 2.0 JSON state store when UCI is absent
		jsonOut, jsonErr := execFn(client, "cat /etc/tachyon/tachyon.json 2>/dev/null")
		if jsonErr == nil && strings.TrimSpace(jsonOut) != "" {
			if (strings.Contains(jsonOut, `"subscription_url"`) || strings.Contains(jsonOut, `"outbounds"`) || strings.Contains(jsonOut, `"proxy_links"`)) &&
				(strings.Contains(jsonOut, "https://") || strings.Contains(jsonOut, "vless://") || strings.Contains(jsonOut, "trojan://") || strings.Contains(jsonOut, "hy2://") || strings.Contains(jsonOut, "fptn:") || strings.Contains(jsonOut, "happ://")) {
				return status
			}
		}
		status.HasEmptySubscription = true
		return status
	}

	// Check if any subscription_url, proxy_links, selector_proxy_links, or standalone desync/awg section is defined
	hasURL := false
	hasProxyLink := false
	for _, line := range strings.Split(uciOut, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ".url=") && !strings.Contains(line, "example.com") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				val := strings.Trim(parts[1], "'\" \t")
				if val != "" {
					hasURL = true
					status.CurrentURL = val
				}
			}
		}
		if (strings.Contains(line, "selector_proxy_links") || strings.Contains(line, ".proxy_links")) && !strings.Contains(line, "vless://example") {
			hasProxyLink = true
		}
		if strings.Contains(line, ".action='desync'") || strings.Contains(line, ".action='awg'") || strings.Contains(line, ".action='zapret'") || strings.Contains(line, ".action='zapret2'") {
			hasProxyLink = true
		}
	}

	if !hasURL && !hasProxyLink {
		status.HasEmptySubscription = true
	}

	return status
}

// SaveSubscription writes subscription configuration to Tachyon UCI.
// Supports HTTP(S) subscription URLs, base64 bundles, or direct node URIs.
func SaveSubscription(client *gossh.Client, execFn sshutil.ExecFunc, subInput string) error {
	analysis := AnalyzeSubscription(subInput)
	if !analysis.Valid {
		return fmt.Errorf("invalid subscription: %s", analysis.ErrorMsg)
	}

	var cmd strings.Builder
	cmd.WriteString(`
# 1. Initialize main section and community lists (RKN + YouTube + Discord + Meta + Twitter)
uci -q get tachyon.main >/dev/null || uci set tachyon.main=section
uci set tachyon.main.label='Main'
uci set tachyon.main.enabled='1'
uci set tachyon.main.action='connection'
uci -q del_list tachyon.main.community_lists 2>/dev/null || true
uci -q add_list tachyon.main.community_lists='russia_inside' 2>/dev/null || true
uci -q add_list tachyon.main.community_lists='youtube' 2>/dev/null || true
uci -q add_list tachyon.main.community_lists='discord' 2>/dev/null || true
uci -q add_list tachyon.main.community_lists='meta' 2>/dev/null || true
uci -q add_list tachyon.main.community_lists='twitter' 2>/dev/null || true
`)

	if analysis.Type == "https" {
		safeURL := strings.ReplaceAll(strings.TrimSpace(subInput), "'", "'\\''")
		cmd.WriteString(fmt.Sprintf(`
# 2. Configure subscription_url
uci -q delete tachyon.@subscription_url[0] 2>/dev/null || true
uci add tachyon subscription_url >/dev/null
uci set tachyon.@subscription_url[-1].section='main'
uci set tachyon.@subscription_url[-1].url='%s'
uci set tachyon.@subscription_url[-1].subscription_update_enabled='1'
uci set tachyon.@subscription_url[-1].subscription_update_interval='1h'
`, safeURL))
	} else if len(analysis.Nodes) > 0 {
		// Direct proxy links or decoded base64 links
		cmd.WriteString("\n# 2. Configure selector_proxy_links and proxy_links\nuci -q del_list tachyon.main.selector_proxy_links 2>/dev/null || true\nuci -q del_list tachyon.main.proxy_links 2>/dev/null || true\n")
		for _, node := range analysis.Nodes {
			safeNode := strings.ReplaceAll(node, "'", "'\\''")
			cmd.WriteString(fmt.Sprintf("uci add_list tachyon.main.selector_proxy_links='%s'\n", safeNode))
			cmd.WriteString(fmt.Sprintf("uci add_list tachyon.main.proxy_links='%s'\n", safeNode))
		}
	}

	cmd.WriteString(`
# 3. Ensure active proxy engine (tachyon-core) is enabled
uci -q set tachyon.settings.engine='tachyon-core' 2>/dev/null || true
uci -q set tachyon.settings.subscription_engine='core' 2>/dev/null || true
uci -q set tachyon.settings.enabled='1' 2>/dev/null || true
uci commit tachyon 2>/dev/null || true

# 4. Import UCI into Tachyon 2.0 JSON store if running 2.0, then update subscriptions
if [ -f /etc/tachyon/tachyon.json ] && [ -x /usr/bin/tachyon ]; then
    /usr/bin/tachyon import_legacy >/dev/null 2>&1 || true
fi
if [ -x /usr/bin/tachyon ]; then
    /usr/bin/tachyon subscription_update main >/dev/null 2>&1 || /usr/bin/tachyon subscription_update >/dev/null 2>&1 || true
fi

# 5. Populate community ruleset binary files (.trs / .srs)
if [ -x /usr/bin/tachyon ]; then
    /usr/bin/tachyon list_update_async >/dev/null 2>&1 || /usr/bin/tachyon list_update >/dev/null 2>&1 || true
fi

# 6. Enable and restart Tachyon service
/etc/init.d/tachyon enable >/dev/null 2>&1 || true
/etc/init.d/tachyon restart 2>&1 || /usr/bin/tachyon restart 2>&1 || true
`)

	_, err := execFn(client, cmd.String())
	return err
}
