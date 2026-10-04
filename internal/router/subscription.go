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
	"vless":     "VLESS",
	"vmess":     "VMess",
	"trojan":    "Trojan",
	"ss":        "Shadowsocks",
	"ssr":       "ShadowsocksR",
	"hy2":       "Hysteria 2",
	"hysteria2": "Hysteria 2",
	"hysteria":  "Hysteria",
	"tuic":      "TUIC",
	"wireguard": "WireGuard",
	"wg":        "WireGuard",
}

// AnalyzeSubscription inspects a raw string to determine if it is a valid HTTP(S) link,
// raw proxy URIs (vless://, hysteria2://, etc.), or a base64 encoded bundle.
func AnalyzeSubscription(raw string) SubscriptionAnalysis {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SubscriptionAnalysis{
			Valid:    false,
			Type:     "invalid",
			ErrorMsg: "Строка подписки не может быть пустой",
		}
	}

	// 1. Direct HTTP/HTTPS subscription URL
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
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

	// Helper to extract nodes and protocol counts
	parseNodes := func(lines []string, srcType string) (SubscriptionAnalysis, bool) {
		var validNodes []string
		protoCounts := make(map[string]int)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "://", 2)
			if len(parts) == 2 {
				scheme := strings.ToLower(parts[0])
				if name, ok := knownProtocols[scheme]; ok {
					validNodes = append(validNodes, line)
					protoCounts[name]++
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

	pkgOut, _ := execFn(client, "opkg list-installed 2>/dev/null || apk info 2>/dev/null")
	if !strings.Contains(pkgOut, "kmod-nft-tproxy") {
		status.MissingKernelMods = append(status.MissingKernelMods, "kmod-nft-tproxy")
	}
	if !strings.Contains(pkgOut, "kmod-tun") {
		status.MissingKernelMods = append(status.MissingKernelMods, "kmod-tun")
	}

	uciOut, uciErr := execFn(client, "uci show tachyon 2>/dev/null")
	if uciErr != nil {
		status.HasEmptySubscription = true
		return status
	}

	// Check if any subscription_url or selector_proxy_links is defined
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
		if strings.Contains(line, "selector_proxy_links") && !strings.Contains(line, "vless://example") {
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
uci -q get tachyon.main >/dev/null || uci set tachyon.main=section
uci set tachyon.main.label='Main'
uci set tachyon.main.enabled='1'
uci set tachyon.main.action='connection'
uci -q add_list tachyon.main.community_lists='russia_inside' 2>/dev/null || true
`)

	if analysis.Type == "https" {
		safeURL := strings.ReplaceAll(strings.TrimSpace(subInput), "'", "'\\''")
		cmd.WriteString(fmt.Sprintf(`
# configure subscription_url
uci -q delete tachyon.@subscription_url[0] 2>/dev/null || true
uci add tachyon subscription_url >/dev/null
uci set tachyon.@subscription_url[-1].section='main'
uci set tachyon.@subscription_url[-1].url='%s'
uci set tachyon.@subscription_url[-1].subscription_update_enabled='1'
`, safeURL))
	} else if len(analysis.Nodes) > 0 {
		// Direct proxy links or decoded base64 links
		cmd.WriteString("\n# configure selector_proxy_links\nuci -q del_list tachyon.main.selector_proxy_links 2>/dev/null || true\n")
		for _, node := range analysis.Nodes {
			safeNode := strings.ReplaceAll(node, "'", "'\\''")
			cmd.WriteString(fmt.Sprintf("uci add_list tachyon.main.selector_proxy_links='%s'\n", safeNode))
		}
	}

	cmd.WriteString(`
uci set tachyon.settings.enabled='1'
uci commit tachyon
/etc/init.d/tachyon restart >/dev/null 2>&1 || true
`)

	_, err := execFn(client, cmd.String())
	return err
}
