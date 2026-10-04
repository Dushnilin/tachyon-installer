package router

import (
	"fmt"
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
func SaveSubscription(client *gossh.Client, execFn sshutil.ExecFunc, subURL string) error {
	subURL = strings.TrimSpace(subURL)
	if subURL == "" {
		return fmt.Errorf("subscription URL cannot be empty")
	}

	cmd := fmt.Sprintf(`
uci -q get tachyon.main >/dev/null || uci set tachyon.main=section
uci set tachyon.main.label='Main'
uci set tachyon.main.enabled='1'
uci set tachyon.main.action='connection'
uci -q add_list tachyon.main.community_lists='russia_inside' 2>/dev/null || true

# configure subscription_url
uci -q delete tachyon.@subscription_url[0] 2>/dev/null || true
uci add tachyon subscription_url >/dev/null
uci set tachyon.@subscription_url[-1].section='main'
uci set tachyon.@subscription_url[-1].url='%s'
uci set tachyon.@subscription_url[-1].subscription_update_enabled='1'

uci set tachyon.settings.enabled='1'
uci commit tachyon
/etc/init.d/tachyon restart >/dev/null 2>&1 || true
`, subURL)

	_, err := execFn(client, cmd)
	return err
}
