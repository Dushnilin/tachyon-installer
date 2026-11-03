package tui

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"tachyon-installer/internal/fleet"
)

// ShowFleetModal displays the interactive Multi-Router Fleet Manager.
func ShowFleetModal(ctx *AppContext, returnPage string) {
	panel := buildStepPanel("Управление роутерами в сети (Fleet)")

	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	statusView.SetBackgroundColor(ColorBgSpace)
	statusView.SetText(fmt.Sprintf("  %sПоиск поддерживаемых OpenWrt роутеров в локальной сети...%s", TagCyanBold, TagReset))

	hintView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	hintView.SetBackgroundColor(ColorBgSpace)
	hintView.SetText("  " + FormatHotkey("Space", "Выбрать") + "  " + FormatHotkey("A", "Все") + "  " + FormatHotkey("U", "Устаревшие") + "  " + FormatHotkey("C", "Чистые") + "  " + FormatHotkey("S", "Перескан") + "  " + FormatVioletHotkey("Enter", "Запуск"))

	table := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false)
	table.SetBackgroundColor(ColorBgSpace)
	table.SetSelectedStyle(tcell.StyleDefault.
		Background(ColorBgInputFocus).
		Foreground(ColorTextPure).
		Bold(true))

	form := tview.NewForm()
	StyleForm(form)
	form.SetBorderPadding(0, 0, 0, 0)
	form.SetItemPadding(1)

	closeModal := func() {
		ctx.Pages.RemovePage("fleet_modal")
		if returnPage != "" {
			ctx.Pages.SwitchToPage(returnPage)
			page := ctx.Pages.GetPage(returnPage)
			if page != nil {
				ctx.App.SetFocus(page)
			}
		}
	}

	form.AddButton("Назад (Esc)", closeModal)
	form.SetCancelFunc(closeModal)
	EnableFormArrowNavigation(form)

	panel.AddItem(statusView, 2, 0, false)
	panel.AddItem(hintView, 1, 0, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(table, 0, 1, true)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 5, 0, false)

	modal := CreateWizardModalCustom(panel, 94, 28)
	ctx.Pages.AddPage("fleet_modal", modal, true, true)
	ctx.App.SetFocus(table)

	var (
		discoveredNodes []*fleet.FleetNode
		isDeploying     bool
		targetVer       = "latest"
	)

	if ctx.Config != nil && ctx.Config.TachyonVersion != "" {
		targetVer = ctx.Config.TachyonVersion
	}

	updateHeaders := func() {
		headers := []string{"[✓]", "IP-адрес", "Модель роутера", "Архитектура", "ОЗУ (Своб)", "Статус Tachyon", "Ядро"}
		widths := []int{5, 16, 22, 12, 12, 22, 12}
		for col, h := range headers {
			table.SetCell(0, col, tview.NewTableCell(" "+h).
				SetTextColor(ColorCyanElectric).
				SetSelectable(false).
				SetMaxWidth(widths[col]))
		}
	}

	renderTable := func() {
		table.Clear()
		updateHeaders()

		upToDateCnt := 0
		outdatedCnt := 0
		cleanCnt := 0
		selectedCnt := 0

		for idx, n := range discoveredNodes {
			row := idx + 1
			switch n.Status {
			case fleet.StatusUpToDate:
				upToDateCnt++
			case fleet.StatusOutdated:
				outdatedCnt++
			case fleet.StatusClean:
				cleanCnt++
			}
			if n.Selected {
				selectedCnt++
			}

			chkText := " [ ]"
			chkColor := ColorTextMuted
			if n.Selected {
				chkText = fmt.Sprintf(" %s[✓]%s", TagGreenBold, TagReset)
				chkColor = ColorStatusSuccess
			}

			statusColColor := ColorTextSecondary
			switch n.Status {
			case fleet.StatusUpToDate:
				statusColColor = ColorStatusSuccess
			case fleet.StatusOutdated:
				statusColColor = ColorStatusWarning
			case fleet.StatusClean:
				statusColColor = ColorCyanElectric
			case fleet.StatusLowResources, fleet.StatusAuthFailed:
				statusColColor = ColorStatusError
			}

			statusText := n.StatusText
			if n.DeployStage != "" {
				statusText = fmt.Sprintf("[%d%%] %s", int(n.DeployProgress*100), n.DeployStage)
			}

			ramStr := fmt.Sprintf("%.0f МБ", n.RAMFreeMB)
			if n.RAMFreeMB == 0 {
				ramStr = "-"
			}

			archStr := n.Arch
			if archStr == "" {
				archStr = "-"
			}

			modelStr := n.Model
			if modelStr == "" {
				modelStr = "OpenWrt Router"
			}

			table.SetCell(row, 0, tview.NewTableCell(chkText).SetTextColor(chkColor))
			table.SetCell(row, 1, tview.NewTableCell(" "+n.IP).SetTextColor(ColorTextPure))
			table.SetCell(row, 2, tview.NewTableCell(" "+modelStr).SetTextColor(ColorTextPrimary))
			table.SetCell(row, 3, tview.NewTableCell(" "+archStr).SetTextColor(ColorSkyCyber))
			table.SetCell(row, 4, tview.NewTableCell(" "+ramStr).SetTextColor(ColorTextSecondary))
			table.SetCell(row, 5, tview.NewTableCell(" "+statusText).SetTextColor(statusColColor))
			table.SetCell(row, 6, tview.NewTableCell(" "+n.RecommendedEngine).SetTextColor(ColorVioletHolo))
		}

		statusView.SetText(fmt.Sprintf(
			"  %sНайдено:%s %s%d%s  •  %sОбновить:%s %s%d%s  •  %sЧистые:%s %s%d%s  •  %sАктуальны:%s %s%d%s  •  %sВыбрано: %d%s",
			TagCyanBold, TagReset, TagTextBold, len(discoveredNodes), TagReset,
			TagAmberBold, TagReset, TagTextBold, outdatedCnt, TagReset,
			TagCyanBold, TagReset, TagTextBold, cleanCnt, TagReset,
			TagGreenBold, TagReset, TagTextBold, upToDateCnt, TagReset,
			TagWhiteBold, selectedCnt, TagReset,
		))
	}

	var runScan func()
	var runDeploy func()

	runDeploy = func() {
		if isDeploying {
			return
		}
		var selectedCount int
		for _, n := range discoveredNodes {
			if n.Selected {
				selectedCount++
			}
		}
		if selectedCount == 0 {
			statusView.SetText("  [#e5484d:b]✗ Не выбрано ни одного роутера для установки. Нажмите [Space] или [A].[-]")
			return
		}

		isDeploying = true
		statusView.SetText(fmt.Sprintf("  [#38bdf8:b]Параллельная установка на выбранные роутеры (%d)...[-]", selectedCount))
		hintView.SetText("  [#eab308]Выполняется развертывание...[-]")

		form.Clear(true)
		StyleForm(form)
		form.SetBorderPadding(0, 0, 0, 0)
		form.AddButton("Выполняется развертывание...", nil)

		go func() {
			mirror := "auto"
			if ctx.Config != nil && ctx.Config.SelectedMirror != "" {
				mirror = ctx.Config.SelectedMirror
			}
			installI18n := true
			if ctx.Config != nil {
				installI18n = ctx.Config.InstallI18n
			}

			deployCfg := fleet.DeployConfig{
				TargetVersion:  targetVer,
				SelectedMirror: mirror,
				InstallI18n:    installI18n,
				Concurrency:    3,
			}

			_ = fleet.DeployFleet(context.Background(), discoveredNodes, deployCfg, func(node *fleet.FleetNode) {
				ctx.App.QueueUpdateDraw(func() {
					renderTable()
				})
			})

			ctx.App.QueueUpdateDraw(func() {
				isDeploying = false
				var successCnt, errCnt int
				for _, n := range discoveredNodes {
					if n.Selected {
						if n.DeploySuccess {
							successCnt++
						} else {
							errCnt++
						}
					}
				}

				statusView.SetText(fmt.Sprintf(
					"  [#4cc38a:b]✓ Развертывание завершено:[-] успешно: [#4cc38a]%d[-], ошибок: [#e5484d]%d[-]",
					successCnt, errCnt,
				))
				hintView.SetText("  [#a1a1a6]Выбранные роутеры обновлены.[-]")

				form.Clear(true)
				StyleForm(form)
				form.SetBorderPadding(0, 0, 0, 0)
				form.AddButton("Завершить (Enter)", closeModal)
				form.AddButton("Сканировать снова", runScan)
				form.SetCancelFunc(closeModal)
				EnableFormArrowNavigation(form)
				ctx.App.SetFocus(form)
			})
		}()
	}

	runScan = func() {
		statusView.SetText("  [#38bdf8:b]Поиск и опрос роутеров в локальной сети...[-]")
		hintView.SetText("  [#71717a]Сканирование портов SSH и авторизация...[-]")
		table.Clear()
		updateHeaders()

		userPass := ""
		if ctx.Config != nil {
			userPass = ctx.Config.Password
		}
		scanCfg := fleet.DefaultScanConfig(userPass, targetVer)
		if ctx.Config != nil {
			scanCfg.KeyPath = ctx.Config.KeyPath
			scanCfg.Gateway = ctx.Config.RouterIP
		}

		go func() {
			nodes, err := fleet.ScanFleet(context.Background(), scanCfg, func(done, total int, curIP string) {
				ctx.App.QueueUpdateDraw(func() {
					statusView.SetText(fmt.Sprintf(
						"  [#38bdf8:b]Сканирование сети:[-] [%d/%d] Проверка хоста [#38bdf8]%s[-]...",
						done, total, curIP,
					))
				})
			})

			ctx.App.QueueUpdateDraw(func() {
				if err != nil {
					statusView.SetText(fmt.Sprintf("  [#e5484d:b]✗ Ошибка сканирования:[-] %v", err))
					return
				}
				discoveredNodes = nodes
				renderTable()

				form.Clear(true)
				StyleForm(form)
				form.SetBorderPadding(0, 0, 0, 0)
				form.AddButton("Установить / обновить на выбранные (Enter)", runDeploy)
				form.AddButton("Повторить сканирование (S)", runScan)
				form.AddButton("Закрыть (Esc)", closeModal)
				form.SetCancelFunc(closeModal)
				EnableFormArrowNavigation(form)
				ctx.App.SetFocus(table)
			})
		}()
	}

	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if isDeploying {
			return event
		}
		row, _ := table.GetSelection()
		nodeIdx := row - 1

		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case ' ':
				if nodeIdx >= 0 && nodeIdx < len(discoveredNodes) {
					discoveredNodes[nodeIdx].Selected = !discoveredNodes[nodeIdx].Selected
					renderTable()
					table.Select(row, 0)
					return nil
				}
			case 'a', 'A':
				// Toggle all
				allSelected := true
				for _, n := range discoveredNodes {
					if !n.Selected {
						allSelected = false
						break
					}
				}
				for _, n := range discoveredNodes {
					n.Selected = !allSelected
				}
				renderTable()
				return nil
			case 'u', 'U':
				// Select only outdated
				for _, n := range discoveredNodes {
					n.Selected = (n.Status == fleet.StatusOutdated)
				}
				renderTable()
				return nil
			case 'c', 'C':
				// Select only clean
				for _, n := range discoveredNodes {
					n.Selected = (n.Status == fleet.StatusClean)
				}
				renderTable()
				return nil
			case 's', 'S':
				runScan()
				return nil
			}
		case tcell.KeyEnter:
			runDeploy()
			return nil
		case tcell.KeyEscape:
			closeModal()
			return nil
		case tcell.KeyTab:
			ctx.App.SetFocus(form)
			return nil
		}
		return event
	})

	runScan()
}
