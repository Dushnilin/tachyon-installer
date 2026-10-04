package fleet

import (
	"fmt"
	"strings"
)

// RouterStatus represents the installation and lifecycle state of Tachyon on a router.
type RouterStatus string

const (
	StatusUpToDate     RouterStatus = "up_to_date"     // Tachyon is installed and current
	StatusOutdated     RouterStatus = "outdated"       // Tachyon is installed but needs upgrade
	StatusClean        RouterStatus = "clean"          // Not installed, ready to deploy
	StatusLowResources RouterStatus = "low_resources"  // Insufficient RAM or Flash
	StatusAuthFailed   RouterStatus = "auth_failed"    // SSH Authentication failed
	StatusUnreachable  RouterStatus = "unreachable"    // Cannot connect to port 22
)

// FleetNode represents one discovered or configured router in the network.
type FleetNode struct {
	IP               string       `json:"ip"`
	Port             int          `json:"port"`
	Username         string       `json:"username"`
	Password         string       `json:"password"`
	KeyPath          string       `json:"key_path"`
	Model            string       `json:"model"`
	Version          string       `json:"version"` // OpenWrt version
	Arch             string       `json:"arch"`    // Normalized arch (arm64, mips, x86_64, etc.)
	DistribArch      string       `json:"distrib_arch"`
	RAMTotalMB       float64      `json:"ram_total_mb"`
	RAMFreeMB        float64      `json:"ram_free_mb"`
	FlashFreeMB      float64      `json:"flash_free_mb"`
	IsAPK            bool         `json:"is_apk"`
	Firewall         string       `json:"firewall"` // fw4 or fw3
	InstalledVersion string       `json:"installed_version"`
	ActiveEngine     string       `json:"active_engine"`
	Status           RouterStatus `json:"status"`
	StatusText       string       `json:"status_text"`
	Selected         bool         `json:"selected"`

	// Auto-calculated recommendations
	RecommendedEngine string `json:"recommended_engine"`
	RecommendedZRAM   bool   `json:"recommended_zram"`

	// Progress during mass rollout
	DeployProgress float64 `json:"deploy_progress"`
	DeployStage    string  `json:"deploy_stage"`
	DeployError    string  `json:"deploy_error"`
	DeploySuccess  bool    `json:"deploy_success"`
}

// ComputeRecommendations determines the best proxy engine and zRAM settings
// tailored to the router's architecture and memory limits.
func (n *FleetNode) ComputeRecommendations(targetVersion string) {
	// 1. Engine selection based on RAM & Flash
	switch {
	case n.RAMTotalMB < 64 || (n.FlashFreeMB > 0 && n.FlashFreeMB < 8):
		// Low-memory / low-flash routers: Steer C lightweight engine
		n.RecommendedEngine = "steer"
		n.RecommendedZRAM = true
	case n.RAMTotalMB >= 64 && n.RAMTotalMB < 128:
		// Mid-range routers: optimized sing-box-lx or steer-extended
		n.RecommendedEngine = "sing-box-lx"
		n.RecommendedZRAM = n.RAMFreeMB < 35
	default:
		// High-end routers (128MB+ RAM): full sing-box-lx
		n.RecommendedEngine = "sing-box-lx"
		n.RecommendedZRAM = false
	}

	// 2. Status determination
	cleanTarget := strings.TrimPrefix(targetVersion, "v")
	cleanInstalled := strings.TrimPrefix(n.InstalledVersion, "v")

	if n.InstalledVersion != "" && n.InstalledVersion != "none" {
		if cleanTarget != "" && (cleanInstalled == cleanTarget || strings.HasPrefix(cleanInstalled, cleanTarget)) {
			n.Status = StatusUpToDate
			n.StatusText = fmt.Sprintf("Актуален (v%s)", cleanInstalled)
		} else if cleanInstalled == "installed" {
			n.Status = StatusOutdated
			n.StatusText = "Установлен (Tachyon активен)"
		} else {
			n.Status = StatusOutdated
			if cleanTarget != "" {
				n.StatusText = fmt.Sprintf("Обновить (v%s → v%s)", cleanInstalled, cleanTarget)
			} else {
				n.StatusText = fmt.Sprintf("Установлен (v%s)", cleanInstalled)
			}
		}
	} else {
		if n.RAMFreeMB > 0 && n.RAMFreeMB < 15 {
			n.Status = StatusLowResources
			n.StatusText = "Критически мало ОЗУ"
		} else {
			n.Status = StatusClean
			n.StatusText = "Чистый (готов к установке)"
		}
	}
}

// StatusBadge returns a tview-formatted colored tag for table display.
func (n *FleetNode) StatusBadge() string {
	switch n.Status {
	case StatusUpToDate:
		return fmt.Sprintf("[#22c55e]%s[-]", n.StatusText)
	case StatusOutdated:
		return fmt.Sprintf("[#eab308:b]%s[-]", n.StatusText)
	case StatusClean:
		return fmt.Sprintf("[#38bdf8]%s[-]", n.StatusText)
	case StatusLowResources:
		return fmt.Sprintf("[#ef5350]%s[-]", n.StatusText)
	case StatusAuthFailed:
		return "[#94a3b8]Требуется пароль[-]"
	case StatusUnreachable:
		return "[#64748b]Недоступен[-]"
	default:
		return n.StatusText
	}
}
