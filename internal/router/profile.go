// Package router handles remote router profiling, architecture detection,
// conflict checking, and service verification logic.
package router

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// RouterProfile contains hardware and software characteristics of the target router.
type RouterProfile struct {
	Model               string
	Version             string
	Arch                string // normalized arch: arm64, amd64, mipsle, etc.
	DistribArch         string // OpenWrt DISTRIB_ARCH e.g. aarch64_cortex-a53, x86_64
	UnameM              string
	RAMTotal            float64 // MB
	RAMFree             float64 // MB
	FlashTotal          float64 // MB
	FlashFree           float64 // MB
	Conflicts           []string
	IsAPK               bool
	Firewall            string // fw4, fw3
	ActiveEngine        string // sing-box, steer, steer-extended, none
	InstalledTachyonVer string
	InstalledAppVer     string
	InstalledEngineVer  string
}

// RunPreConnectionCheck connects to the router, collects hardware profile data,
// and returns a populated RouterProfile.
func RunPreConnectionCheck(client *gossh.Client) (*RouterProfile, error) {
	cmd := `echo "=== TACHYON-PROFILE ==="
cat /tmp/sysinfo/model 2>/dev/null || awk -F: '/model name/ {print $2; exit}' /proc/cpuinfo 2>/dev/null || echo "Generic OpenWrt Device"
[ -f /etc/openwrt_release ] && . /etc/openwrt_release && { [ -n "$DISTRIB_CODENAME" ] && echo "$DISTRIB_RELEASE ($DISTRIB_CODENAME)" || echo "$DISTRIB_RELEASE"; } || echo "OpenWrt"
uname -m
( [ -f /etc/apk/arch ] && cat /etc/apk/arch 2>/dev/null ) || ( command -v apk >/dev/null 2>&1 && apk --print-arch 2>/dev/null ) || ( [ -f /etc/os-release ] && . /etc/os-release && [ -n "$OPENWRT_ARCH" ] && echo "$OPENWRT_ARCH" ) || ( [ -f /etc/openwrt_release ] && . /etc/openwrt_release && [ -n "$DISTRIB_ARCH" ] && echo "$DISTRIB_ARCH" ) || ( command -v opkg >/dev/null 2>&1 && opkg print-architecture 2>/dev/null | awk '$2 != "all" && $2 != "noarch" {a=$2} END{print a}' ) || uname -m
free -k | awk '/Mem:/ {print $2"|"$4}'
df -k /overlay 2>/dev/null | awk 'END{print $2"|"$4}' || df -k / 2>/dev/null | awk 'END{print $2"|"$4}'
(opkg list-installed 2>/dev/null | grep -E "nextdns|https-dns-proxy|passwall|bypass|shadowsocksr|openclash|forkop|podkop|netshift" || apk info 2>/dev/null | grep -E "nextdns|https-dns-proxy|passwall|bypass|shadowsocksr|openclash|forkop|podkop|netshift" || echo "none") | tr '\n' ' ' && echo ""
command -v apk >/dev/null 2>&1 && echo "apk" || echo "opkg"
[ -x /sbin/fw4 ] && echo "fw4" || echo "fw3"
uci -q get tachyon.settings.engine || echo "none"
opkg status tachyon 2>/dev/null | awk -F': ' '/Version:/{print $2; exit}' || apk info -e tachyon 2>/dev/null || echo ""
opkg status sing-box-extended 2>/dev/null | awk -F': ' '/Version:/{print $2; exit}' || opkg status sing-box 2>/dev/null | awk -F': ' '/Version:/{print $2; exit}' || apk info -e sing-box-extended 2>/dev/null || apk info -e sing-box 2>/dev/null || echo ""`

	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	outBytes, err := session.CombinedOutput(cmd)
	if err != nil {
		return nil, err
	}

	return ParseProfile(string(outBytes))
}

// ParseProfile parses the raw SSH output into a RouterProfile struct.
func ParseProfile(out string) (*RouterProfile, error) {
	if !strings.Contains(out, "=== TACHYON-PROFILE ===") {
		return nil, fmt.Errorf("invalid response from router")
	}

	lines := strings.Split(out, "\n")
	profileStart := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "=== TACHYON-PROFILE ===" {
			profileStart = i
			break
		}
	}
	if profileStart == -1 || len(lines) < profileStart+11 {
		return nil, fmt.Errorf("failed to parse profile response (need 11 lines after marker)")
	}

	model := strings.TrimSpace(lines[profileStart+1])
	version := strings.TrimSpace(lines[profileStart+2])
	unameM := strings.TrimSpace(lines[profileStart+3])
	distArch := strings.TrimSpace(lines[profileStart+4])
	ramStr := strings.TrimSpace(lines[profileStart+5])
	flashStr := strings.TrimSpace(lines[profileStart+6])
	conflictStr := strings.TrimSpace(lines[profileStart+7])
	pkgSys := strings.TrimSpace(lines[profileStart+8])
	firewall := strings.TrimSpace(lines[profileStart+9])
	activeEngine := strings.TrimSpace(lines[profileStart+10])

	var tachyonVer, engineVer string
	if len(lines) > profileStart+11 {
		tachyonVer = strings.TrimSpace(lines[profileStart+11])
	}
	if len(lines) > profileStart+12 {
		engineVer = strings.TrimSpace(lines[profileStart+12])
	}

	ramTotal, ramFree := ParsePipePair(ramStr)
	flashTotal, flashFree := ParsePipePair(flashStr)

	ramTotal /= 1024.0
	ramFree /= 1024.0
	flashTotal /= 1024.0
	flashFree /= 1024.0

	conflicts := ParseConflicts(conflictStr)

	normArch := ClassifyArch(distArch)
	if normArch == "" {
		normArch = ClassifyArch(unameM)
	}

	return &RouterProfile{
		Model:               model,
		Version:             version,
		Arch:                normArch,
		DistribArch:         distArch,
		UnameM:              unameM,
		RAMTotal:            ramTotal,
		RAMFree:             ramFree,
		FlashTotal:          flashTotal,
		FlashFree:           flashFree,
		Conflicts:           conflicts,
		IsAPK:               pkgSys == "apk",
		Firewall:            firewall,
		ActiveEngine:        activeEngine,
		InstalledTachyonVer: tachyonVer,
		InstalledEngineVer:  engineVer,
	}, nil
}

// ParsePipePair splits "1024|512" into (1024, 512).
func ParsePipePair(s string) (float64, float64) {
	var a, b float64
	if strings.Contains(s, "|") {
		parts := strings.Split(s, "|")
		a, _ = strconv.ParseFloat(parts[0], 64)
		b, _ = strconv.ParseFloat(parts[1], 64)
	}
	return a, b
}

// ParseConflicts splits a space-separated conflict list into unique entries.
func ParseConflicts(s string) []string {
	var conflicts []string
	if s == "none" || s == "" {
		return conflicts
	}
	for _, w := range strings.Fields(s) {
		w = strings.TrimSpace(w)
		if idx := strings.Index(w, " "); idx != -1 {
			w = strings.TrimSpace(w[:idx])
		}
		if idx := strings.Index(w, "-"); idx != -1 && !strings.HasPrefix(w, "https-") && !strings.HasPrefix(w, "sing-") {
			w = strings.TrimSpace(w[:idx])
		}
		if w != "" && w != "none" && !slices.Contains(conflicts, w) {
			conflicts = append(conflicts, w)
		}
	}
	return conflicts
}

// DetectArch queries the router for its normalized architecture.
func DetectArch(client *gossh.Client) string {
	return ClassifyArch(DetectRawArch(client))
}

// DetectRawArch queries the router for its package architecture (DISTRIB_ARCH/apk arch).
func DetectRawArch(client *gossh.Client) string {
	sess, err := client.NewSession()
	if err != nil {
		return ""
	}
	defer sess.Close()

	cmd := `( [ -f /etc/apk/arch ] && cat /etc/apk/arch 2>/dev/null ) || ( command -v apk >/dev/null 2>&1 && apk --print-arch 2>/dev/null ) || ( [ -f /etc/os-release ] && . /etc/os-release && [ -n "$OPENWRT_ARCH" ] && echo "$OPENWRT_ARCH" ) || ( [ -f /etc/openwrt_release ] && . /etc/openwrt_release && [ -n "$DISTRIB_ARCH" ] && echo "$DISTRIB_ARCH" ) || ( command -v opkg >/dev/null 2>&1 && opkg print-architecture 2>/dev/null | awk '$2 != "all" && $2 != "noarch" {a=$2} END{print a}' ) || uname -m`
	out, err := sess.Output(cmd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ClassifyArch maps raw uname/arch strings to normalized architecture names.
func ClassifyArch(archStr string) string {
	a := strings.ToLower(strings.TrimSpace(archStr))
	if strings.Contains(a, "aarch64") || strings.Contains(a, "arm64") {
		return "arm64"
	}
	if strings.Contains(a, "x86_64") || strings.Contains(a, "amd64") {
		return "amd64"
	}
	if strings.Contains(a, "mipsel") || strings.Contains(a, "mipsle") {
		return "mipsle"
	}
	if strings.Contains(a, "mips") {
		return "mips"
	}
	if strings.Contains(a, "arm") {
		return "arm"
	}
	if strings.Contains(a, "i386") || strings.Contains(a, "i686") || strings.Contains(a, "x86") {
		return "386"
	}
	return ""
}

// IsAPKPackage checks if the router uses the APK package manager.
func IsAPKPackage(client *gossh.Client) bool {
	sess, err := client.NewSession()
	if err != nil {
		return false
	}
	defer sess.Close()

	out, err := sess.Output("command -v apk >/dev/null 2>&1 && echo yes || echo no")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "yes"
}
