package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"
	gossh "golang.org/x/crypto/ssh"

	backuppkg "tachyon-installer/internal/backup"
	"tachyon-installer/internal/browser"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
	"tachyon-installer/internal/updater"
)

// ShowMonitorModal displays a real-time live performance monitor for the connected router.
func ShowMonitorModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel("Мониторинг роутера")

	statsView := tview.NewTextView().
		SetDynamicColors(true)
	statsView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	statsView.SetText("\n  [#38bdf8]Сбор метрик с роутера...[-]")

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

	form.AddButton("Закрыть (Esc)", closeModal)
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
					statsView.SetText("\n  [#e5484d]✗ Нет активного SSH-подключения к роутеру.[-]")
				})
				return
			}
			stats, err := routerpkg.CollectLiveStats(ctx.SSHClient, execFn)
			if err != nil {
				ctx.App.QueueUpdateDraw(func() {
					statsView.SetText(fmt.Sprintf("\n  [#e5484d]! Ошибка опроса роутера: %v[-]", err))
				})
				return
			}

			ramPct := float64(0)
			if stats.RAMTotalMB > 0 {
				ramPct = (stats.RAMUsedMB / stats.RAMTotalMB) * 100.0
			}
			ramColor := "#4cc38a"
			if ramPct > 75.0 {
				ramColor = "#eab308"
			}
			if ramPct > 90.0 {
				ramColor = "#e5484d"
			}

			engStatus := "[#71717a]не запущен[-]"
			if stats.EngineName != "" {
				engStatus = fmt.Sprintf("[#4cc38a:b]%s[-] [#a1a1a6](PID: %s, Память: %s)[-]", stats.EngineName, stats.EnginePID, stats.EngineVSZ)
			}

			text := fmt.Sprintf("\n"+
				"  [#38bdf8]CPU Loadavg:[-]        [#ededed:b]%.2f[-] (1м)  ·  [#a1a1a6]%.2f[-] (5м)  ·  [#71717a]%.2f[-] (15м)\n"+
				"  [#38bdf8]Оперативная память:[-] [%s]%s[-] [#ededed]%.1f / %.1f МБ[-] ([%s]%.0f%%[-])\n"+
				"  [#38bdf8]Время работы:[-]       [#ededed]%s[-]\n"+
				"  [#38bdf8]Служба ядра прокси:[-] %s\n"+
				"  [#38bdf8]Сетевой трафик LAN:[-] [#38bdf8]↓ %s[-]  ·  [#a1a1a6]↑ %s[-]\n"+
				"  [#38bdf8]Активных сессий:[-]    [#ededed]%d conntrack[-]\n",
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

	modal := CreateWizardModalCustom(panel, 88, 20)
	ctx.Pages.AddPage("monitor_modal", modal, true, true)
	ctx.App.SetFocus(form)
}

// ShowRescueModal displays a confirmation dialog and executes Emergency Rescue.
func ShowRescueModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel("Аварийное восстановление сети")

	infoView := tview.NewTextView().
		SetDynamicColors(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	initialText := "\n" +
		"  [#e5484d:b]Экстренное восстановление прямого интернета:[-]\n\n" +
		"  [#a1a1a6]1. Остановит службы Tachyon, tachyon-core и sing-box[-]\n" +
		"  [#a1a1a6]2. Очистит таблицы nftables (inet tachyon, tachyon_tailscale) и iptables[-]\n" +
		"  [#a1a1a6]3. Сбросит маршруты TProxy (таблицы tachyon/105, 100, 4247/4248)[-]\n" +
		"  [#a1a1a6]4. Восстановит настройки dnsmasq и перезапустит firewall[-]\n" +
		"  [#a1a1a6]5. Проверит доступность шлюза провайдера и интернета (8.8.8.8)[-]\n"

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
		infoView.SetText("\n  [#38bdf8]Выполняется сброс сетевых правил...[-]\n")
		form.Clear(true)
		form.AddButton("Выполняется...", nil)

		go func() {
			rep := routerpkg.EmergencyRescue(ctx.SSHClient, execFn)

			ctx.App.QueueUpdateDraw(func() {
				var sb strings.Builder
				sb.WriteString("\n")
				if rep.Success {
					sb.WriteString("  [#4cc38a:b]✓ Сброс сетевых правил выполнен[-]\n\n")
					for _, a := range rep.ActionsTaken {
						sb.WriteString(fmt.Sprintf("  [#71717a]•[-] [#a1a1a6]%s[-]\n", a))
					}
					sb.WriteString("\n")
					if rep.GatewayPing {
						sb.WriteString("  [#4cc38a]✓ Пинг до шлюза провайдера: доступен[-]\n")
					} else {
						sb.WriteString("  [#e5484d]✗ Пинг до шлюза: нет ответа[-]\n")
					}
					if rep.InternetPing {
						sb.WriteString("  [#4cc38a]✓ Прямой интернет (8.8.8.8): доступен[-]\n")
					} else {
						sb.WriteString("  [#eab308]! Прямой интернет (8.8.8.8): не отвечает (проверьте WAN)[-]\n")
					}
				} else {
					sb.WriteString(fmt.Sprintf("  [#e5484d:b]✗ Сбой выполнения:[-] %s\n", rep.Detail))
				}

				infoView.SetText(sb.String())

				form.Clear(true)
				form.AddButton("Закрыть", closeModal)
				ctx.App.SetFocus(form)
			})
		}()
	}

	form.AddButton("Выполнить сброс", runRescue)
	form.AddButton("Отмена", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 12, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 88, 22)
	ctx.Pages.AddPage("rescue_modal", modal, true, true)
	ctx.App.SetFocus(form)
}

// ShowConflictFixModal scans and disables conflicting proxy services.
func ShowConflictFixModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel("Устранение сетевых конфликтов")

	infoView := tview.NewTextView().
		SetDynamicColors(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	initialText := "\n" +
		"  [#eab308:b]Отключение конфликтующих прокси-служб:[-]\n\n" +
		"  [#a1a1a6]Службы Passwall, OpenClash, ShadowsocksR, Zapret и Xray[-]\n" +
		"  [#a1a1a6]перехватывают DNS (порт 53) и правила TProxy, мешая работе Tachyon.[-]\n\n" +
		"  [#71717a]Нажмите кнопку ниже для остановки конфликтующих служб и отключения их автозапуска.[-]\n"

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
		infoView.SetText("\n  [#38bdf8]Поиск и отключение конфликтующих служб...[-]\n")
		form.Clear(true)
		form.AddButton("Выполняется...", nil)

		go func() {
			rep := routerpkg.DisableConflicts(ctx.SSHClient, execFn, nil)

			ctx.App.QueueUpdateDraw(func() {
				var sb strings.Builder
				sb.WriteString("\n")
				if rep.Success {
					if rep.DisabledCount > 0 {
						sb.WriteString("  [#4cc38a:b]✓ Конфликтующие службы остановлены и отключены:[-]\n\n")
						for _, s := range rep.DisabledList {
							sb.WriteString(fmt.Sprintf("    [#e5484d]✗[-] [#ededed]%s[-] (остановлена, автозапуск отключен)\n", s))
						}
					} else {
						sb.WriteString("  [#4cc38a:b]✓ Конфликтов не обнаружено:[-] сторонние прокси-пакеты не активны.\n")
					}
				} else {
					sb.WriteString(fmt.Sprintf("  [#e5484d:b]✗ Ошибка:[-] %s\n", rep.Details))
				}

				infoView.SetText(sb.String())

				form.Clear(true)
				form.AddButton("Закрыть", closeModal)
				ctx.App.SetFocus(form)
			})
		}()
	}

	form.AddButton("Отключить конфликты", runFix)
	form.AddButton("Отмена", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 11, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 88, 20)
	ctx.Pages.AddPage("conflicts_modal", modal, true, true)
	ctx.App.SetFocus(form)
}

// ShowSnapshotModal creates a full configuration archive (network, dhcp, firewall, tachyon).
func ShowSnapshotModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel("Снимок конфигурации (Snapshot)")

	infoView := tview.NewTextView().
		SetDynamicColors(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	initialText := "\n" +
		"  [#38bdf8:b]Создание архива настроек OpenWrt и Tachyon на компьютере:[-]\n\n" +
		"  [#a1a1a6]В архив войдут:[-] /etc/config/tachyon, /etc/tachyon, /etc/tachyon-core,\n" +
		"  /etc/config/network, /etc/config/dhcp, /etc/config/firewall, /etc/nftables.d/.\n\n" +
		"  [#71717a]Архив сохраняется в каталог backups/ в рабочей папке.[-]\n"

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
		infoView.SetText("\n  [#38bdf8]Создание архива и загрузка с роутера...[-]\n")
		form.Clear(true)
		form.AddButton("Выполняется...", nil)

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
					sb.WriteString("  [#4cc38a:b]✓ Снимок конфигурации создан[-]\n\n")
					sb.WriteString(fmt.Sprintf("  [#ededed]%s[-]\n\n", msg))
					sb.WriteString("  [#71717a]Восстановить конфигурацию можно ключом -restore или через меню.[-]\n")
				} else {
					sb.WriteString(fmt.Sprintf("  [#e5484d:b]✗ Ошибка создания снимка:[-] %v\n", err))
				}

				infoView.SetText(sb.String())

				form.Clear(true)
				form.AddButton("Закрыть", closeModal)
				ctx.App.SetFocus(form)
			})
		}()
	}

	form.AddButton("Создать снимок", runSnapshot)
	form.AddButton("Отмена", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 11, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 88, 20)
	ctx.Pages.AddPage("snapshot_modal", modal, true, true)
	ctx.App.SetFocus(form)
}

// ShowNetworkTuneModal optimizes router TCP parameters, buffer sizes, and flow offloading.
func ShowNetworkTuneModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel("Оптимизация сетевого стека (sysctl и BBR)")

	infoView := tview.NewTextView().
		SetDynamicColors(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	initialText := "\n" +
		"  [#38bdf8:b]Настройка сетевого стека ядра OpenWrt:[-]\n\n" +
		"  [#a1a1a6]• Контроль перегрузки:[-] TCP BBR + fq_codel (снижение bufferbloat)\n" +
		"  [#a1a1a6]• Буферы сокетов:[-] rmem/wmem пропорционально объёму ОЗУ роутера\n" +
		"  [#a1a1a6]• TCP Fast Open (TFO):[-] ускорение повторных рукопожатий\n" +
		"  [#a1a1a6]• Таблица conntrack:[-] увеличение лимита сессий и сокращение таймаутов\n" +
		"  [#a1a1a6]• Flow Offloading:[-] отключение ускорения в fw4 для корректной работы TProxy/nftables\n\n" +
		"  [#71717a]Настройки сохраняются в /etc/sysctl.d/99-tachyon-tune.conf[-]\n"

	infoView.SetText(initialText)

	form := tview.NewForm()
	StyleForm(form)

	closeModal := func() {
		ctx.Pages.RemovePage("tune_modal")
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

	runTune := func() {
		infoView.SetText("\n  [#38bdf8]Применение параметров ядра и оптимизация сети...[-]\n")
		form.Clear(true)
		form.AddButton("Выполняется...", nil)

		go func() {
			rep := routerpkg.TuneNetwork(ctx.SSHClient, execFn)

			ctx.App.QueueUpdateDraw(func() {
				var sb strings.Builder
				sb.WriteString("\n")
				if rep.Success {
					sb.WriteString("  [#4cc38a:b]✓ Оптимизация сети применена[-]\n\n")
					for _, r := range rep.AppliedRules {
						sb.WriteString(fmt.Sprintf("    [#4cc38a]✓[-] [#ededed]%s[-]\n", r))
					}
					sb.WriteString(fmt.Sprintf("\n  [#71717a]Конфигурация сохранена в:[-] [#38bdf8]%s[-]\n", rep.PersistPath))
				} else {
					sb.WriteString(fmt.Sprintf("  [#e5484d:b]✗ Ошибка оптимизации:[-] %s\n", rep.Details))
				}

				infoView.SetText(sb.String())

				form.Clear(true)
				form.AddButton("Закрыть", closeModal)
				ctx.App.SetFocus(form)
			})
		}()
	}

	form.AddButton("Применить оптимизацию", runTune)
	form.AddButton("Отмена", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(infoView, 12, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 88, 22)
	ctx.Pages.AddPage("tune_modal", modal, true, true)
	ctx.App.SetFocus(form)
}

// ShowSelfUpdateModal checks GitHub for a newer installer release and shows
// a notification with a link. Binaries are never replaced in-place.
func ShowSelfUpdateModal(ctx *AppContext, returnPage, currentVersion string) {
	panel := buildStepPanel("Обновление установщика")

	infoView := tview.NewTextView().
		SetDynamicColors(true)
	infoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	infoView.SetText(fmt.Sprintf("\n  [#38bdf8]Проверка наличия новых версий на GitHub...[-]\n\n  [#a1a1a6]Текущая версия:[-] [#ededed]%s[-]\n", currentVersion))

	form := tview.NewForm()
	StyleForm(form)

	closeModal := func() {
		ctx.Pages.RemovePage("self_update_modal")
		if returnPage != "" {
			ctx.Pages.SwitchToPage(returnPage)
			page := ctx.Pages.GetPage(returnPage)
			if page != nil {
				ctx.App.SetFocus(page)
			}
		}
	}

	var releaseURL string

	form.AddButton("Открыть страницу релиза", func() {
		if releaseURL != "" {
			_ = browser.OpenURL(releaseURL)
		}
	})
	form.AddButton("Закрыть", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	go func() {
		hasUpdate, latestVer, relURL, err := updater.CheckForUpdate(context.Background(), currentVersion)

		ctx.App.QueueUpdateDraw(func() {
			releaseURL = relURL
			var sb strings.Builder
			sb.WriteString("\n")
			switch {
			case err != nil:
				sb.WriteString(fmt.Sprintf("  [#e5484d]✗ Не удалось проверить обновления: %v[-]\n", err))
			case hasUpdate:
				sb.WriteString(fmt.Sprintf("  [#eab308:b]! Доступна новая версия: %s (текущая: %s)[-]\n", latestVer, currentVersion))
				sb.WriteString(fmt.Sprintf("  [#a1a1a6]Страница релиза:[-] [#38bdf8]%s[-]\n", relURL))
				sb.WriteString("\n  [#a1a1a6]Обновление: повторно запустите скрипт установки (install.sh / install.ps1)[-]\n")
			default:
				sb.WriteString(fmt.Sprintf("  [#4cc38a:b]✓ Установлена актуальная версия (%s)[-]\n", currentVersion))
			}
			infoView.SetText(sb.String())
			ctx.App.SetFocus(form)
		})
	}()

	panel.AddItem(infoView, 11, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 4, 1, true)

	modal := CreateWizardModalCustom(panel, 88, 20)
	ctx.Pages.AddPage("self_update_modal", modal, true, true)
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
