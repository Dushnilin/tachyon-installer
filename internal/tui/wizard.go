package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	appconfig "tachyon-installer/internal/config"
	"tachyon-installer/internal/discover"
	dlpkg "tachyon-installer/internal/downloader"
	sshpkg "tachyon-installer/internal/ssh"
)

// ShowWelcomeWizard initializes and displays the 4-step installation wizard.
func ShowWelcomeWizard(ctx *AppContext) {
	var step2Form, step3Form *tview.Form
	var menuList *tview.List

	defaultIP := "192.168.1.1"
	defaultPort := "22"
	defaultUser := "root"
	defaultPass := ""
	if ctx.Config != nil {
		if ctx.Config.RouterIP != "" {
			defaultIP = ctx.Config.RouterIP
		}
		if ctx.Config.SSHPort != 0 {
			defaultPort = strconv.Itoa(ctx.Config.SSHPort)
		}
		if ctx.Config.Username != "" {
			defaultUser = ctx.Config.Username
		}
		defaultPass = ctx.Config.Password
	}

	ipInput := tview.NewInputField().
		SetLabel("IP-адрес роутера: ").
		SetText(defaultIP).
		SetFieldWidth(22)

	portInput := tview.NewInputField().
		SetLabel("SSH-порт:         ").
		SetText(defaultPort).
		SetFieldWidth(10)

	userInput := tview.NewInputField().
		SetLabel("Имя пользователя: ").
		SetText(defaultUser).
		SetFieldWidth(22)

	passInput := tview.NewInputField().
		SetLabel("Пароль от роутера:").
		SetText(defaultPass).
		SetMaskCharacter('*').
		SetFieldWidth(22)

	// ==========================================
	// --- ШАГ 1: TACHYON CONTROL CENTER & MENU ---
	// ==========================================
	step1Panel := buildStepPanel(" ⚡ TACHYON CONTROL CENTER & INSTALLER ⚡ ")

	welcomeInfoView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	welcomeInfoView.SetBackgroundColor(ColorBgSpace)
	welcomeInfoView.SetText(fmt.Sprintf("  %s🛰️  Tachyon Express Hub%s  •  %sВыберите действие стрелками [↑/↓] или нажав цифру/клавишу%s\n",
		TagCyanBold, TagReset, TagSubText, TagReset))

	menuList = tview.NewList()
	StyleList(menuList)
	menuList.ShowSecondaryText(true)

	startAction := func(action string) {
		ctx.PendingAction = action
		ctx.DiagOnly = (action == ActionDiag)
		ctx.RescueOnly = (action == ActionRescue)
		ctx.GuidedOnly = (action == ActionGuided)
		ctx.SpeedDoctorOnly = (action == ActionSpeed)

		switch action {
		case ActionFleet:
			ShowFleetModal(ctx, "welcome_wizard")
			return
		case ActionSelfUpdate:
			ShowSelfUpdateModal(ctx, "welcome_wizard", ctx.AppVersion)
			return
		}

		if ctx.SSHClient != nil {
			switch action {
			case ActionInstall:
				go ctx.OnProfileReady(nil)
			case ActionGuided:
				ShowGuidedSetupModal(ctx, "welcome_wizard")
			case ActionSpeed:
				ShowSpeedDoctorModal(ctx, "welcome_wizard")
			case ActionDiag:
				ShowDiagnosticsWizard(ctx, "welcome_wizard")
			case ActionMonitor:
				ShowMonitorModal(ctx, "welcome_wizard")
			case ActionTune:
				ShowNetworkTuneModal(ctx, "welcome_wizard")
			case ActionRescue:
				ShowRescueModal(ctx, "welcome_wizard")
			case ActionConflicts:
				ShowConflictFixModal(ctx, "welcome_wizard")
			case ActionSnapshot:
				ShowSnapshotModal(ctx, "welcome_wizard")
			case ActionAWG:
				ShowAWGModal(ctx, "welcome_wizard")
			}
			return
		}

		ctx.Pages.SwitchToPage("ssh_wizard")
		ctx.App.SetFocus(step2Form)
	}

	menuList.AddItem("🚀 [1] Экспресс-установка Tachyon", "Выбор ядра (sing-box / steer), версии, зеркал и параметров установки", '1', func() {
		startAction(ActionInstall)
	})
	menuList.AddItem("✨ [2] Авто-мастер (Настройка под ключ)", "Полный автопилот: аудит роутера, установка и конфигурация в 1 клик", '2', func() {
		startAction(ActionGuided)
	})
	menuList.AddItem("🛡️ [W] AmneziaWG (Генератор WARP & Конфигов)", "Встроенный генератор AmneziaWG/WARP, импорт .conf/vpn:// и проверка обхода", 'w', func() {
		startAction(ActionAWG)
	})
	menuList.AddItem("🌐 [3] Флот роутеров (Multi-Router Fleet)", "Массовый поиск роутеров в сети, аудит версий и параллельная установка", '3', func() {
		startAction(ActionFleet)
	})
	menuList.AddItem("⚡ [4] Скорость & Bufferbloat Doctor", "Замер задержки в покое/нагрузке, Bufferbloat Grade и троттлинг CPU", '4', func() {
		startAction(ActionSpeed)
	})
	menuList.AddItem("🔍 [5] Расширенная диагностика", "Анализ служб, firewall (fw4/fw3), DNS перехвата и маршрутизации", '5', func() {
		startAction(ActionDiag)
	})
	menuList.AddItem("📊 [6] Мониторинг в реальном времени", "Живой монитор нагрузки CPU, памяти, сетевых соединений и ячеек", '6', func() {
		startAction(ActionMonitor)
	})
	menuList.AddItem("🚀 [7] Оптимизация сети (BBR & fq_codel)", "Тюнинг сетевого стека, очередей fq_codel, TCP BBR и сокетов", '7', func() {
		startAction(ActionTune)
	})
	menuList.AddItem("🚑 [8] Аварийный Rescue (Сброс сети)", "Экстренное восстановление прямого интернета и сброс правил перехвата", '8', func() {
		startAction(ActionRescue)
	})
	menuList.AddItem("🛡️ [9] Устранение конфликтов", "Поиск и отключение Passwall, OpenClash, Zapret и дублирующих DNS", '9', func() {
		startAction(ActionConflicts)
	})
	menuList.AddItem("💾 [B] Полный бэкап и снимки (Snapshot)", "Резервная копия настроек сети, dhcp, firewall и конфигурации Tachyon", 'b', func() {
		startAction(ActionSnapshot)
	})
	menuList.AddItem("🆙 [U] Обновление установщика", "Проверка новых версий на GitHub и самообновление программы", 'u', func() {
		startAction(ActionSelfUpdate)
	})
	menuList.AddItem("🚪 [Q] Выход из программы", "Завершить работу Tachyon Express Installer", 'q', func() {
		ctx.App.Stop()
	})

	menuList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			ctx.App.Stop()
			return nil
		}
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'q', 'Q', 'й', 'Й':
				ctx.App.Stop()
				return nil
			case 'b', 'B', 'и', 'И':
				startAction(ActionSnapshot)
				return nil
			case 'u', 'U', 'г', 'Г':
				startAction(ActionSelfUpdate)
				return nil
			}
		}
		return event
	})

	footerView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	footerView.SetBackgroundColor(ColorBgSpace)
	footerView.SetText("  " + FormatHotkey("↑/↓", "Навигация") + " · " + FormatHotkey("Enter/Цифра", "Запуск") + " · " + FormatExitHotkey("Esc/Q", "Выход"))

	step1Panel.AddItem(welcomeInfoView, 2, 0, false)
	step1Panel.AddItem(menuList, 0, 1, true)
	step1Panel.AddItem(nil, 1, 0, false)
	step1Panel.AddItem(footerView, 1, 0, false)

	step1Modal := CreateWizardModalCustom(step1Panel, 96, 28)

	// ==========================================
	// --- ШАГ 2: НАСТРОЙКА SSH ПОДКЛЮЧЕНИЯ ---
	// ==========================================
	step2Panel := buildStepPanel(" ШАГ 2/4 ")

	step2Text := "\n  [#38bdf8]🔑  Параметры подключения к роутеру[-]\n\n" +
		"  [#cbd5e1]Укажите IP-адрес роутера (обычно 192.168.1.1), порт SSH[-]\n" +
		"  [#cbd5e1]и имя учетной записи (по умолчанию root).[-]"

	step2TextView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(step2Text)
	step2TextView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	step2Err := tview.NewTextView().SetDynamicColors(true)
	step2Err.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	step2Form = tview.NewForm()
	StyleForm(step2Form)
	step2Form.AddFormItem(ipInput).
		AddFormItem(portInput).
		AddFormItem(userInput)

	var step3ShowTime time.Time

	goStep3 := func() {
		field, msg := validateSSHInput(ipInput.GetText(), portInput.GetText(), userInput.GetText())
		if msg != "" {
			step2Err.SetText("  [#ef5350]✗ " + msg + "[-]")
			switch field {
			case 0:
				ctx.App.SetFocus(step2Form)
				step2Form.SetFocus(0)
			case 1:
				ctx.App.SetFocus(step2Form)
				step2Form.SetFocus(1)
			default:
				ctx.App.SetFocus(step2Form)
				step2Form.SetFocus(2)
			}
			return
		}
		step3ShowTime = time.Now()
		step2Err.SetText("")
		ctx.Pages.SwitchToPage("password_wizard")
		ctx.App.SetFocus(step3Form)
	}

	step2Form.AddButton("Продолжить →", goStep3)
	step2Form.AddButton("🔎 Найти роутер", func() {
		step2Err.SetText("  [#38bdf8]Поиск роутеров в локальной сети…[-]")
		port, _ := strconv.Atoi(strings.TrimSpace(portInput.GetText()))
		if port < 1 || port > 65535 {
			port = 22
		}
		go func() {
			ctxScan, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			hosts := discover.Scan(ctxScan, port, appconfig.GetDefaultGateway())
			ctx.App.QueueUpdateDraw(func() {
				if len(hosts) == 0 {
					step2Err.SetText("  [#eab308]Роутеры с открытым SSH не найдены. Введите адрес вручную.[-]")
					return
				}
				best := hosts[0]
				ipInput.SetText(best.IP)
				kind := "SSH"
				if best.OpenWrt {
					kind = "OpenWrt (Dropbear)"
				}
				msg := fmt.Sprintf("  [#22c55e]✓ Найден %s: %s[-]", kind, best.IP)
				if len(hosts) > 1 {
					var others []string
					for _, h := range hosts[1:min(len(hosts), 4)] {
						others = append(others, h.IP)
					}
					msg += fmt.Sprintf("  [#94a3b8]ещё: %s[-]", strings.Join(others, ", "))
				}
				step2Err.SetText(msg)
			})
		}()
	})
	step2Form.AddButton("← Назад", func() {
		ctx.Pages.SwitchToPage("welcome_wizard")
		ctx.App.SetFocus(menuList)
	})
	EnableFormArrowNavigation(step2Form)

	step2Panel.AddItem(textViewLayout(step2TextView), 6, 1, false)
	step2Panel.AddItem(step2Err, 1, 0, false)
	step2Panel.AddItem(formLayout(step2Form), 10, 1, true)

	step2Modal := CreateWizardModal(step2Panel)

	// ==========================================
	// --- ШАГ 3: ВВОД SSH ПАРОЛЯ ---
	// ==========================================
	step3Panel := buildStepPanel(" ШАГ 3/4 ")

	step3Text := "\n  [#38bdf8]🔒  Аутентификация администратора[-]\n\n" +
		"  [#cbd5e1]Введите системный пароль администратора (root) роутера.[-]\n" +
		"  [#94a3b8]Пароль используется только для SSH-сессии и не сохраняется на диск.[-]\n" +
		"  [#94a3b8]Вход по ключу: укажите путь к ключу (пароль станет его парольной фразой).[-]"

	step3TextView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(step3Text)
	step3TextView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	step3Form = tview.NewForm()
	StyleForm(step3Form)
	keyInput := tview.NewInputField().
		SetLabel("SSH-ключ (путь):").
		SetFieldWidth(36)
	if ctx.Config != nil {
		keyInput.SetText(ctx.Config.KeyPath)
	}
	step3Err := tview.NewTextView().SetDynamicColors(true)
	step3Err.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	step3Form.AddFormItem(passInput)
	step3Form.AddFormItem(keyInput)

	submitCredentials := func() {
		// Prevent accidental double-Enter carry-over from step 2
		if !step3ShowTime.IsZero() && time.Since(step3ShowTime) < 200*time.Millisecond {
			return
		}

		port, err := strconv.Atoi(strings.TrimSpace(portInput.GetText()))
		if err != nil || port <= 0 {
			port = 22
		}

		ctx.Config.RouterIP = strings.TrimSpace(ipInput.GetText())
		ctx.Config.SSHPort = port
		ctx.Config.Username = strings.TrimSpace(userInput.GetText())
		keyPath := strings.TrimSpace(keyInput.GetText())
		if keyPath != "" {
			if _, err := sshpkg.LoadSigner(keyPath, passInput.GetText()); err != nil {
				step3Err.SetText("  [#ef5350]✗ " + tview.Escape(err.Error()) + "[-]")
				return
			}
		}
		step3Err.SetText("")
		ctx.Config.Password = passInput.GetText()
		ctx.Config.KeyPath = keyPath
		sshpkg.PrivateKeyPath = keyPath

		ShowLoadingWizard(ctx)

		if ctx.OnProfileReady != nil {
			go ctx.OnProfileReady(nil)
		}
	}

	step3Form.AddButton("Продолжить →", submitCredentials)
	step3Form.AddButton("← Назад", func() {
		ctx.Pages.SwitchToPage("ssh_wizard")
		ctx.App.SetFocus(step2Form)
	})
	EnableFormArrowNavigation(step3Form)

	step3Panel.AddItem(textViewLayout(step3TextView), 7, 1, false)
	step3Panel.AddItem(step3Err, 1, 0, false)
	step3Panel.AddItem(formLayout(step3Form), 8, 1, true)

	step3Modal := CreateWizardModal(step3Panel)

	// Enter handlers
	ipInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			ctx.App.SetFocus(portInput)
		}
	})
	portInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			ctx.App.SetFocus(userInput)
		}
	})
	userInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			goStep3()
		}
	})
	passInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			submitCredentials()
		}
	})
	keyInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			submitCredentials()
		}
	})

	ctx.Pages.AddPage("welcome_wizard", step1Modal, true, true)
	ctx.Pages.AddPage("ssh_wizard", step2Modal, true, false)
	ctx.Pages.AddPage("password_wizard", step3Modal, true, false)

	ctx.Pages.SwitchToPage("welcome_wizard")
}

// ProfileData is a transport struct for router profile information.
type ProfileData struct {
	Model               string
	Version             string
	Arch                string
	DistribArch         string
	RAMTotal            float64
	RAMFree             float64
	FlashFree           float64
	Conflicts           []string
	Firewall            string
	IsAPK               bool
	ActiveEngine        string
	InstalledTachyonVer string
	InstalledEngineVer  string
}

// ShowOptionsWizard displays step 4/4 with all router specs and visible installation parameters.
func ShowOptionsWizard(ctx *AppContext, profile ProfileData) {
	panel := buildStepPanel(" ШАГ 4/4: ПАРАМЕТРЫ УСТАНОВКИ ")

	optSelector := NewOptionsSelector(profile)

	if ctx.Config != nil {
		optSelector.Preselect(ctx.Config.SelectedEngine, ctx.Config.SelectedMirror, ctx.Config.TachyonVersion, ctx.Config.InstallI18n)
	}

	startInstall := func(opts InstallOptions) {
		ctx.Pages.RemovePage("confirm_wizard")
		ctx.Pages.SwitchToPage("progress")
		ctx.App.SetFocus(ctx.ConsoleView)

		if ctx.OnStartInstall != nil {
			ctx.OnStartInstall(opts)
		}
	}

	optSelector.OnSubmit = func(opts InstallOptions) {
		showConfirmModal(ctx, profile, opts,
			func() { startInstall(opts) },
			func() {
				ctx.Pages.RemovePage("confirm_wizard")
				ctx.Pages.SwitchToPage("options_wizard")
				ctx.App.SetFocus(optSelector)
			})
	}

	optSelector.OnDiagnostics = func() {
		ShowDiagnosticsWizard(ctx, "options_wizard")
	}

	optSelector.OnMonitor = func() {
		ShowMonitorModal(ctx, "options_wizard")
	}

	optSelector.OnRescue = func() {
		ShowRescueModal(ctx, "options_wizard")
	}

	optSelector.OnFixConflicts = func() {
		ShowConflictFixModal(ctx, "options_wizard")
	}

	optSelector.OnSnapshot = func() {
		ShowSnapshotModal(ctx, "options_wizard")
	}

	optSelector.OnTuneNetwork = func() {
		ShowNetworkTuneModal(ctx, "options_wizard")
	}

	optSelector.OnSelfUpdate = func() {
		ShowSelfUpdateModal(ctx, "options_wizard", ctx.AppVersion)
	}

	optSelector.OnGuidedSetup = func() {
		ShowGuidedSetupModal(ctx, "options_wizard")
	}

	optSelector.OnFleetManager = func() {
		ShowFleetModal(ctx, "options_wizard")
	}

	optSelector.OnSpeedDoctor = func() {
		ShowSpeedDoctorModal(ctx, "options_wizard")
	}

	optSelector.OnAWGManager = func() {
		ShowAWGModal(ctx, "options_wizard")
	}

	optSelector.OnHotSwap = func(engineKey, mirrorKey string) {
		if ctx.OnHotSwap != nil {
			ctx.Pages.SwitchToPage("progress")
			ctx.App.SetFocus(ctx.ConsoleView)
			ctx.OnHotSwap(engineKey, mirrorKey)
		}
	}

	optSelector.OnBack = func() {
		ctx.Pages.SwitchToPage("password_wizard")
		ctx.App.SetFocus(ctx.Pages.GetPage("password_wizard"))
	}

	optSelector.OnRequestManualVersion = func(current string, callback func(newVer string)) {
		showManualVersionModal(ctx, current, callback, func() {
			ctx.Pages.SwitchToPage("options_wizard")
			ctx.App.SetFocus(optSelector)
		})
	}

	panel.AddItem(optSelector, 0, 1, true)

	// Load real release tags in the background; the list updates when they arrive.
	go func() {
		mirror := "auto"
		if ctx.Config != nil && ctx.Config.SelectedMirror != "" {
			mirror = ctx.Config.SelectedMirror
		}
		cctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rels, err := dlpkg.FetchTachyonReleases(cctx, dlpkg.NewClient(dlpkg.NewMirrorManager(mirror)))
		if err != nil {
			return
		}
		var tags []string
		for _, r := range rels {
			if !r.Draft && !r.Prerelease && r.TagName != "" && len(tags) < 5 {
				tags = append(tags, r.TagName)
			}
		}
		if len(tags) == 0 {
			return
		}
		ctx.App.QueueUpdateDraw(func() { optSelector.SetReleases(tags) })
	}()

	modal := CreateWizardModalCustom(panel, 94, 30)
	ctx.Pages.AddPage("options_wizard", modal, true, true)
	ctx.Pages.SwitchToPage("options_wizard")
	ctx.App.SetFocus(optSelector)
}

func (p ProfileData) RAMStatus() string {
	if p.RAMFree < 40.0 {
		return fmt.Sprintf("[#ef5350]%.1f МБ свободного ОЗУ (Критически мало!)[-]", p.RAMFree)
	}
	if p.RAMTotal < 120.0 {
		return fmt.Sprintf("[#eab308]%.1f МБ (Рекомендуется 128 МБ+)[-]", p.RAMTotal)
	}
	return fmt.Sprintf("[#22c55e]%.1f МБ свободного ОЗУ (Достаточно)[-]", p.RAMFree)
}

func (p ProfileData) RAMDot() string {
	if p.RAMFree < 40.0 {
		return "[#ef5350]●[-]"
	}
	if p.RAMTotal < 120.0 {
		return "[#eab308]●[-]"
	}
	return "[#22c55e]●[-]"
}

func (p ProfileData) FlashStatus() string {
	if p.FlashFree > 0 && p.FlashFree < 8.0 {
		return fmt.Sprintf("[#ef5350]%.1f МБ (Недостаточно места!)[-]", p.FlashFree)
	}
	if p.FlashFree > 0 && p.FlashFree < 12.0 {
		return fmt.Sprintf("[#eab308]%.1f МБ (Мало места)[-]", p.FlashFree)
	}
	return fmt.Sprintf("[#22c55e]%.1f МБ (Достаточно)[-]", p.FlashFree)
}

func (p ProfileData) FlashDot() string {
	if p.FlashFree > 0 && p.FlashFree < 8.0 {
		return "[#ef5350]●[-]"
	}
	if p.FlashFree > 0 && p.FlashFree < 12.0 {
		return "[#eab308]●[-]"
	}
	return "[#22c55e]●[-]"
}

func (p ProfileData) FirewallStatus() string {
	if p.Firewall == "fw3" {
		return "[#eab308]fw3 (iptables - Устаревшая)[-]"
	}
	return "[#22c55e]fw4 (nftables - Рекомендуется)[-]"
}

func (p ProfileData) FirewallDot() string {
	if p.Firewall == "fw3" {
		return "[#eab308]●[-]"
	}
	return "[#22c55e]●[-]"
}

func (p ProfileData) ConflictStatus() string {
	if len(p.Conflicts) > 0 {
		return fmt.Sprintf("[#ef5350]Обнаружен %s (рекомендуется удалить)[-]", strings.Join(p.Conflicts, ", "))
	}
	return "[#22c55e]Не обнаружено[-]"
}

func (p ProfileData) ConflictDot() string {
	if len(p.Conflicts) > 0 {
		return "[#ef5350]●[-]"
	}
	return "[#22c55e]●[-]"
}

func (p ProfileData) Warnings() []string {
	var warnings []string
	if p.RAMTotal < 128.0 {
		warnings = append(warnings, "[#eab308]⚠️  ВНИМАНИЕ: ОЗУ менее 128 МБ. Возможны зависания при пиковых нагрузках.[-]")
	}
	if p.RAMFree < 35.0 {
		warnings = append(warnings, "[#ef5350]❌  КРИТИЧЕСКИ: Свободного ОЗУ менее 35 МБ! sing-box может аварийно завершиться.[-]")
	}
	if p.FlashFree > 0 && p.FlashFree < 8.0 {
		warnings = append(warnings, "[#ef5350]❌  КРИТИЧЕСКИ: Мало места на диске (< 8 МБ). Освободите память или подключите USB ExtRoot.[-]")
	}
	if p.Firewall == "fw3" {
		warnings = append(warnings, "[#eab308]⚠️  ПРЕДУПРЕЖДЕНИЕ: Обнаружен старый брандмауэр fw3. Рекомендуется обновить OpenWrt.[-]")
	}
	if len(p.Conflicts) > 0 {
		warnings = append(warnings, fmt.Sprintf("[#eab308]⚠️  Конфликтующие пакеты: %s[-]", strings.Join(p.Conflicts, ", ")))
	}
	return warnings
}

func buildStepPanel(title string) *SolidFlex {
	panel := NewSolidFlex()
	panel.SetDirection(tview.FlexRow)
	panel.SetBackgroundColor(ColorBgSpace)
	panel.SetBorder(true).
		SetTitle(FormatStepTitle(title)).
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(ColorCyanElectric).
		SetBorderColor(ColorBorderFocused)
	return panel
}

func EnableFormArrowNavigation(form *tview.Form) {
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if idx, _ := form.GetFocusedItemIndex(); idx >= 0 {
			item := form.GetFormItem(idx)
			if dd, ok := item.(*tview.DropDown); ok && dd.IsOpen() {
				return event
			}
		}
		switch event.Key() {
		case tcell.KeyDown:
			return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
		case tcell.KeyUp:
			return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
		}
		return event
	})
}


// showManualVersionModal displays a prompt modal to enter a custom Tachyon git tag or version.
func showManualVersionModal(ctx *AppContext, current string, callback func(newVer string), onClose func()) {
	panel := buildStepPanel(" ВВОД ВЕРСИИ TACHYON ")

	closeModal := func() {
		ctx.Pages.RemovePage("manual_version_modal")
		if onClose != nil {
			onClose()
		}
	}

	input := tview.NewInputField().
		SetLabel("Версия / Git tag: ").
		SetText(current).
		SetFieldWidth(24)

	form := tview.NewForm()
	StyleForm(form)
	form.AddFormItem(input)

	apply := func() {
		val := strings.TrimSpace(input.GetText())
		if val != "" {
			callback(val)
		}
		closeModal()
	}

	form.AddButton("✅ Применить", apply)
	form.AddButton("Отмена", func() {
		closeModal()
	})

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			apply()
		} else if key == tcell.KeyEscape {
			closeModal()
		}
	})

	panel.AddItem(formLayout(form), 7, 1, true)
	modal := CreateWizardModalCustom(panel, 60, 9)

	ctx.Pages.AddPage("manual_version_modal", modal, true, true)
	ctx.App.SetFocus(input)
}

// validateSSHInput checks the SSH connection fields. It returns the index of the
// offending field (0 host, 1 port, 2 user) and a message, or an empty message if valid.
func validateSSHInput(host, port, user string) (int, string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return 0, "Укажите IP-адрес или имя роутера"
	}
	if strings.ContainsAny(host, " \t/\\") {
		return 0, "Адрес не должен содержать пробелов и слэшей"
	}
	p, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil || p < 1 || p > 65535 {
		return 1, "Порт SSH должен быть числом от 1 до 65535"
	}
	if strings.TrimSpace(user) == "" {
		return 2, "Укажите имя пользователя (обычно root)"
	}
	return 0, ""
}
