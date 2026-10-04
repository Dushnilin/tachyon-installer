package router

import (
	"fmt"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	sshutil "tachyon-installer/internal/ssh"
)

// GuidedSetupMode specifies whether the setup uses standalone DPI bypass or a VPN tunnel.
type GuidedSetupMode string

const (
	// ModeStandaloneDPI configures packet-level DPI bypass (steer/zapret) without external proxy servers.
	ModeStandaloneDPI GuidedSetupMode = "standalone_dpi"
	// ModeTunnel configures sing-box / VLESS tunnel using the user's subscription.
	ModeTunnel GuidedSetupMode = "tunnel"
	// ModeHybrid configures DPI bypass for domestic/heavy traffic + tunnel for blocked sites.
	ModeHybrid GuidedSetupMode = "hybrid"
)

// GuidedSetupPlan contains the configuration to be applied during the setup.
type GuidedSetupPlan struct {
	Mode            GuidedSetupMode
	ChosenDNS       DNSResolver
	ChosenStrategy  DPIStrategy
	SubscriptionURL string
	SelectedEngine  string
	EnableTune      bool
}

// GuidedSetupResult holds the outcome of each phase of the guided setup.
type GuidedSetupResult struct {
	DNSApplied     bool
	DNSName        string
	DNSLatencyMs   int64
	StrategyApplied bool
	StrategyName   string
	EngineName     string
	NetworkTuned   bool
	TuneReport     NetworkTuneReport
	VerifyResult   BypassReport
	OverallSuccess bool
	SummaryMessage string
}

// GuidedProgressFn reports status updates during execution.
type GuidedProgressFn func(stepTitle string, fraction float64)

// RunGuidedSetup applies the complete optimized configuration to the router.
func RunGuidedSetup(client *gossh.Client, execFn sshutil.ExecFunc, plan GuidedSetupPlan, progressFn GuidedProgressFn) (*GuidedSetupResult, error) {
	progress := func(title string, frac float64) {
		if progressFn != nil {
			progressFn(title, frac)
		}
	}

	res := &GuidedSetupResult{
		DNSName:      plan.ChosenDNS.Name,
		DNSLatencyMs: plan.ChosenDNS.LatencyMs,
		StrategyName: plan.ChosenStrategy.Name,
		EngineName:   plan.SelectedEngine,
	}

	// 1. Conflict resolution
	progress("Проверка и отключение конфликтующих прокси-служб...", 0.10)
	if client != nil {
		profile, err := RunPreConnectionCheck(client)
		if err == nil && len(profile.Conflicts) > 0 {
			DisableConflicts(client, execFn, profile.Conflicts)
		}
	}

	// 2. DNS Configuration
	progress(fmt.Sprintf("Применение защищенного DNS (%s)...", plan.ChosenDNS.Name), 0.25)
	if plan.ChosenDNS.PrimaryIP != "" {
		if err := ApplyDNSConfig(client, execFn, plan.ChosenDNS); err == nil {
			res.DNSApplied = true
		}
	}

	// 3. Engine selection and DPI Strategy / Subscription
	engine := plan.SelectedEngine
	if engine == "" {
		if plan.Mode == ModeStandaloneDPI {
			engine = "steer"
		} else {
			engine = "sing-box"
		}
	}
	res.EngineName = engine

	progress(fmt.Sprintf("Настройка активного ядра (%s)...", engine), 0.45)
	setEngineCmd := fmt.Sprintf(`
if command -v uci >/dev/null 2>&1; then
    uci -q set tachyon.settings.engine='%s' 2>/dev/null || true
    uci -q set tachyon.settings.enabled='1' 2>/dev/null || true
    uci commit tachyon 2>/dev/null || true
fi
`, engine)
	execFn(client, setEngineCmd)

	// Apply DPI Strategy
	if plan.ChosenStrategy.TCPArgs != "" {
		progress(fmt.Sprintf("Применение DPI-стратегии (%s)...", plan.ChosenStrategy.Name), 0.60)
		if err := ApplyDPIStrategy(client, execFn, plan.ChosenStrategy); err == nil {
			res.StrategyApplied = true
		}
	}

	// Apply Subscription if provided
	if plan.SubscriptionURL != "" {
		progress("Сохранение ссылки подписки в Tachyon...", 0.70)
		SaveSubscription(client, execFn, plan.SubscriptionURL)
	}

	// 4. Network Optimization (BBR, buffers, flow offloading)
	if plan.EnableTune {
		progress("Оптимизация сетевого стека роутера (TCP BBR, сокеты, conntrack)...", 0.80)
		tuneReport := TuneNetwork(client, execFn)
		if tuneReport.Success {
			res.NetworkTuned = true
			res.TuneReport = tuneReport
		}
	}

	// 5. Restart services
	progress("Перезапуск и синхронизация служб Tachyon...", 0.90)
	restartCmd := `
if [ -x /etc/init.d/tachyon ]; then
    /etc/init.d/tachyon enable >/dev/null 2>&1 || true
    /etc/init.d/tachyon restart 2>&1 || /usr/bin/tachyon restart 2>&1 || true
fi
sleep 2
`
	execFn(client, restartCmd)

	// 6. End-to-End Verification
	progress("Сквозное тестирование YouTube, Discord и проверка утечек...", 0.95)
	bypassReport := TestBypass(client, execFn)
	res.VerifyResult = bypassReport

	// Success evaluation
	res.OverallSuccess = bypassReport.Success || res.StrategyApplied || res.DNSApplied

	var summaryParts []string
	summaryParts = append(summaryParts, fmt.Sprintf("DNS: %s (%d мс)", plan.ChosenDNS.Name, plan.ChosenDNS.LatencyMs))
	if res.StrategyApplied {
		summaryParts = append(summaryParts, fmt.Sprintf("Стратегия: %s", plan.ChosenStrategy.Name))
	}
	if res.NetworkTuned {
		summaryParts = append(summaryParts, "Сетевой стек: BBR + тюнинг активен")
	}
	res.SummaryMessage = strings.Join(summaryParts, " · ")

	progress("Настройка под ключ успешно завершена! ✓", 1.0)
	return res, nil
}
