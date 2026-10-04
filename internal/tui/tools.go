package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"
	gossh "golang.org/x/crypto/ssh"

	backuppkg "tachyon-installer/internal/backup"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
)

// ShowMonitorModal displays a real-time live performance monitor for the connected router.
func ShowMonitorModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel(" 📊 ЖИВОЙ МОНИТОРИНГ РОУТЕРА ")

	statsView := tview.NewTextView().
		SetDynamicColors(true)
	statsView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	statsView.SetText("\n  [#38bdf8]⚡ Сбор метрик с роутера...[-]")

	form := tview.NewForm()
	StyleForm(form)

	closeCtx, cancel := context.WithCancel(context.Background())

	closeModal := func() {
		cancel()
		ctx.Pages.RemovePage("monitor_modal")
		if returnPage != "" {
			ctx.Pages.SwitchToPage(returnPage)
			page := ctx.Pages.GetPage(returnPage)
			if page != nil {
				ctx.App.SetFocus(page)
			}
		}
	}

	form.AddButton("🚪 Закрыть (Esc)", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	reconnectFn := func() (*gossh.Client, error) {
		if ctx.Config == nil {
			return nil, fmt.Errorf("no router config")
		}
		return sshpkg.Connect(ctx.Config.RouterIP, ctx.Config.SSHPort, ctx.Config.Username, ctx.Config.Password)
	}

	execFn := func(_ *gossh.Client, cmd string) (string, error) {
		return sshpkg.Exec(ctx.SSHClient, cmd, reconnectFn)
	}

	// Real-time update loop
	go func() {
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()

		// Initial immediate fetch
		updateStats := func() {
			if ctx.SSHClient == nil {
				ctx.App.QueueUpdateDraw(func() {
					statsView.SetText("\n  [#ef5350]❌ Нет активного SSH-подключения к роутеру.[-]")
				})
				return
			}
			stats, err := routerpkg.CollectLiveStats(ctx.SSHClient, execFn)
			if err != nil {
				ctx.App.QueueUpdateDraw(func() {
					statsView.SetText(fmt.Sprintf("\n  [#ef5350]⚠️ Ошибка опроса роутера: %v[-]", err))
				})
				return
			}

			ramPct := float64(0)
			if stats.RAMTotalMB > 0 {
				ramPct = (stats.RAMUsedMB / stats.RAMTotalMB) * 100.0
			}
			ramColor := "#22c55e"
			if ramPct > 75.0 {
				ramColor = "#eab308"
			}
			if ramPct > 90.0 {
				ramColor = "#ef5350"
			}

			engStatus := "[#64748b]не запущен[-]"
			if stats.EngineName != "" {
				engStatus = fmt.Sprintf("[#22c55e:b]%s[-] [#94a3b8](PID: %s, Память: %s)[-]", stats.EngineName, stats.EnginePID, stats.EngineVSZ)
			}

			text := fmt.Sprintf("\n"+
				"  [#38bdf8]CPU Loadavg:[-]      [#f1f5f9:b]%.2f[-] (1м)  ·  [#cbd5e1]%.2f[-] (5м)  ·  [#94a3b8]%.2f[-] (15м)\n"+
				"  [#38bdf8]Оперативная память:[-] [%s]%s[-] [#f1f5f9]%.1f / %.1f МБ[-] ([%s]%.0f%%[-])\n"+
				"  [#38bdf8]Время работы:[-]     [#f1f5f9]%s[-]\n"+
				"  [#38bdf8]Служба ядра прокси:[-] %s\n"+
				"  [#38bdf8]Сетевой трафик LAN:[-] [#38bdf8]↓ %s[-]  ·  [#a855f7]↑ %s[-]\n"+
				"  [#38bdf8]Активных сессий:[-]  [#f1f5f9]%d conntrack[-]\n",
				stats.Load1, stats.Load5, stats.Load15,
				ramColor, renderBar(ramPct, 16), stats.RAMUsedMB, stats.RAMTotalMB, ramColor, ramPct,
				stats.UptimeString(),
				engStatus,
				formatBytes(stats.RXBytes), formatBytes(stats.TXBytes),
				stats.Connections,
			)

			ctx.App.QueueUpdateDraw(func() {
				statsView.SetText(text)
			})
		}

		updateStats()

		for {
			select {
			case <-closeCtx.Done():
				return
			case <-ticker.C:
				updateStats()
			}
		}
	}()

	panel.AddItem(statsView, 11, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 80, 18)
	ctx.Pages.AddPage("monitor_modal", modal, true, true)
	ctx.App.SetFocus(form)
}

// ShowRescueModal displays a confirmation dialog and executes Emergency Rescue.
func ShowRescueModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel(" 🚑 АВАРИЙНЫЙ СБРОС И ВОССТАНОВЛЕНИЕ СЕТИ ")

	infoView := tview.NewTextView().
		SetDynamicColors(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	initialText := "\n" +
		"  [#ef5350:b]Внимание! Экстренное восстановление прямого интернета:[-]\n\n" +
		"  [#cbd5e1]1. Принудительно остановит службы Tachyon, sing-box и steer[-]\n" +
		"  [#cbd5e1]2. Очистит и удалит таблицы правил nftables и iptables[-]\n" +
		"  [#cbd5e1]3. Сбросит маршруты TProxy (fwmark / table 100)[-]\n" +
		"  [#cbd5e1]4. Перезапустит стандартные службы dnsmasq и firewall[-]\n" +
		"  [#cbd5e1]5. Проверит пинг до шлюза провайдера и в интернет (8.8.8.8)[-]\n"

	infoView.SetText(initialText)

	form := tview.NewForm()
	StyleForm(form)

	closeModal := func() {
		ctx.Pages.RemovePage("rescue_modal")
		if returnPage != "" {
			ctx.Pages.SwitchToPage(returnPage)
			page := ctx.Pages.GetPage(returnPage)
			if page != nil {
				ctx.App.SetFocus(page)
			}
		}
	}

	reconnectFn := func() (*gossh.Client, error) {
		if ctx.Config == nil {
			return nil, fmt.Errorf("no router config")
		}
		return sshpkg.Connect(ctx.Config.RouterIP, ctx.Config.SSHPort, ctx.Config.Username, ctx.Config.Password)
	}

	execFn := func(_ *gossh.Client, cmd string) (string, error) {
		return sshpkg.Exec(ctx.SSHClient, cmd, reconnectFn)
	}

	runRescue := func() {
		infoView.SetText("\n  [#38bdf8]⚡ Выполняется экстренный сброс сетевых правил... Подождите[-]\n")
		form.Clear(true)
		form.AddButton("⏳ Выполняется...", nil)

		go func() {
			rep := routerpkg.EmergencyRescue(ctx.SSHClient, execFn)

			ctx.App.QueueUpdateDraw(func() {
				var sb strings.Builder
				sb.WriteString("\n")
				if rep.Success {
					sb.WriteString("  [#22c55e:b]✓ Аварийный сброс успешно выполнен![-]\n\n")
					for _, a := range rep.ActionsTaken {
						sb.WriteString(fmt.Sprintf("  [#94a3b8]•[-] [#cbd5e1]%s[-]\n", a))
					}
					sb.WriteString("\n")
					if rep.GatewayPing {
						sb.WriteString("  [#22c55e]✓ Пинг до шлюза провайдера: УСПЕШНО[-]\n")
					} else {
						sb.WriteString("  [#ef5350]✗ Пинг до шлюза: НЕТ ОТВЕТА[-]\n")
					}
					if rep.InternetPing {
						sb.WriteString("  [#22c55e]✓ Прямой интернет (8.8.8.8): РАБОТАЕТ[-]\n")
					} else {
						sb.WriteString("  [#eab308]⚠️ Прямой интернет (8.8.8.8): НЕ ОТВЕЧАЕТ (проверьте WAN/кабель)[-]\n")
					}
				} else {
					sb.WriteString(fmt.Sprintf("  [#ef5350:b]❌ Сбой выполнения:[-] %s\n", rep.Detail))
				}

				infoView.SetText(sb.String())

				form.Clear(true)
				form.AddButton("✅ Закрыть", closeModal)
				ctx.App.SetFocus(form)
			})
		}()
	}

	form.AddButton("🚑 Выполнить сброс", runRescue)
	form.AddButton("Отмена", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 12, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 82, 19)
	ctx.Pages.AddPage("rescue_modal", modal, true, true)
	ctx.App.SetFocus(form)
}

// ShowConflictFixModal scans and disables conflicting proxy services.
func ShowConflictFixModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel(" 🛡️ УСТРАНЕНИЕ СЕТЕВЫХ КОНФЛИКТОВ ")

	infoView := tview.NewTextView().
		SetDynamicColors(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	initialText := "\n" +
		"  [#eab308:b]Отключение конфликтующих служб обхода блокировок:[-]\n\n" +
		"  [#cbd5e1]Службы вроде Passwall, OpenClash, ShadowsocksR, Zapret и Xray[-]\n" +
		"  [#cbd5e1]перехватывают DNS (порт 53) и правила TProxy, вызывая сбои в работе Tachyon.[-]\n\n" +
		"  [#94a3b8]Нажмите кнопку ниже для безопасной остановки и отключения автозапуска.[-]\n"

	infoView.SetText(initialText)

	form := tview.NewForm()
	StyleForm(form)

	closeModal := func() {
		ctx.Pages.RemovePage("conflicts_modal")
		if returnPage != "" {
			ctx.Pages.SwitchToPage(returnPage)
			page := ctx.Pages.GetPage(returnPage)
			if page != nil {
				ctx.App.SetFocus(page)
			}
		}
	}

	reconnectFn := func() (*gossh.Client, error) {
		if ctx.Config == nil {
			return nil, fmt.Errorf("no router config")
		}
		return sshpkg.Connect(ctx.Config.RouterIP, ctx.Config.SSHPort, ctx.Config.Username, ctx.Config.Password)
	}

	execFn := func(_ *gossh.Client, cmd string) (string, error) {
		return sshpkg.Exec(ctx.SSHClient, cmd, reconnectFn)
	}

	runFix := func() {
		infoView.SetText("\n  [#38bdf8]⚡ Поиск и отключение конфликтующих служб...[-]\n")
		form.Clear(true)
		form.AddButton("⏳ Выполняется...", nil)

		go func() {
			rep := routerpkg.DisableConflicts(ctx.SSHClient, execFn, nil)

			ctx.App.QueueUpdateDraw(func() {
				var sb strings.Builder
				sb.WriteString("\n")
				if rep.Success {
					if rep.DisabledCount > 0 {
						sb.WriteString("  [#22c55e:b]✓ Конфликтующие службы успешно остановлены и отключены:[-]\n\n")
						for _, s := range rep.DisabledList {
							sb.WriteString(fmt.Sprintf("    [#ef5350]✗[-] [#f1f5f9]%s[-] (служба остановлена, автозапуск выключен)\n", s))
						}
					} else {
						sb.WriteString("  [#22c55e:b]✓ Конфликтов не обнаружено:[-] сторонние прокси-пакеты не активны.\n")
					}
				} else {
					sb.WriteString(fmt.Sprintf("  [#ef5350:b]❌ Ошибка:[-] %s\n", rep.Details))
				}

				infoView.SetText(sb.String())

				form.Clear(true)
				form.AddButton("✅ Закрыть", closeModal)
				ctx.App.SetFocus(form)
			})
		}()
	}

	form.AddButton("🛡️ Отключить конфликты", runFix)
	form.AddButton("Отмена", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 11, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 82, 18)
	ctx.Pages.AddPage("conflicts_modal", modal, true, true)
	ctx.App.SetFocus(form)
}

// ShowSnapshotModal creates a full configuration archive (network, dhcp, firewall, tachyon).
func ShowSnapshotModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel(" 💾 ПОЛНЫЙ СНИМОК СИСТЕМЫ (SNAPSHOT) ")

	infoView := tview.NewTextView().
		SetDynamicColors(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	initialText := "\n" +
		"  [#38bdf8:b]Создание полного снимка настроек OpenWrt на ваш ПК:[-]\n\n" +
		"  [#cbd5e1]В архив войдут:[-] /etc/config/network, /etc/config/dhcp,\n" +
		"  /etc/config/firewall, /etc/nftables.d/ и /etc/config/tachyon.\n\n" +
		"  [#94a3b8]Архив сохраняется в папку backups/ на вашем компьютере.[-]\n"

	infoView.SetText(initialText)

	form := tview.NewForm()
	StyleForm(form)

	closeModal := func() {
		ctx.Pages.RemovePage("snapshot_modal")
		if returnPage != "" {
			ctx.Pages.SwitchToPage(returnPage)
			page := ctx.Pages.GetPage(returnPage)
			if page != nil {
				ctx.App.SetFocus(page)
			}
		}
	}

	runSnapshot := func() {
		infoView.SetText("\n  [#38bdf8]⚡ Создание архива и загрузка с роутера на ПК...[-]\n")
		form.Clear(true)
		form.AddButton("⏳ Выполняется...", nil)

		go func() {
			routerIP := "router"
			if ctx.Config != nil && ctx.Config.RouterIP != "" {
				routerIP = ctx.Config.RouterIP
			}
			msg, err := backuppkg.CreateFullSnapshot(ctx.SSHClient, routerIP)

			ctx.App.QueueUpdateDraw(func() {
				var sb strings.Builder
				sb.WriteString("\n")
				if err == nil {
					sb.WriteString("  [#22c55e:b]✓ Полный снимок успешно создан![-]\n\n")
					sb.WriteString(fmt.Sprintf("  [#f1f5f9]%s[-]\n\n", msg))
					sb.WriteString("  [#94a3b8]Вы всегда можете восстановить конфигурацию через -restore или меню.[-]\n")
				} else {
					sb.WriteString(fmt.Sprintf("  [#ef5350:b]❌ Ошибка создания снимка:[-] %v\n", err))
				}

				infoView.SetText(sb.String())

				form.Clear(true)
				form.AddButton("✅ Закрыть", closeModal)
				ctx.App.SetFocus(form)
			})
		}()
	}

	form.AddButton("💾 Создать снимок", runSnapshot)
	form.AddButton("Отмена", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 11, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 82, 18)
	ctx.Pages.AddPage("snapshot_modal", modal, true, true)
	ctx.App.SetFocus(form)
}

func renderBar(percent float64, width int) string {
	if width <= 0 {
		width = 16
	}
	filled := int(percent / 100.0 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	empty := width - filled
	return strings.Repeat("█", filled) + strings.Repeat("░", empty)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
