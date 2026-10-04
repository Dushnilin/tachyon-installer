package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	dlpkg "tachyon-installer/internal/downloader"
	"tachyon-installer/internal/tui/widgets"
)

// ShowWelcomeWizard initializes and displays the 4-step installation wizard.
func ShowWelcomeWizard(ctx *AppContext) {
	var step1Form, step2Form, step3Form *tview.Form

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
	// --- ШАГ 1: ПРИВЕТСТВИЕ И ОБЗОР ---
	// ==========================================
	step1Panel := buildStepPanel(" ШАГ 1/4 ")

	logoWidget := widgets.NewLogoText()
	logoWidget.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	welcomeText := "\n" +
		"  [#38bdf8]🛰️  Добро пожаловать в установщик Tachyon![-]\n\n" +
		"  [#cbd5e1]Этот мастер автоматически скачает дистрибутив Tachyon и ядро прокси[-]\n" +
		"  [#cbd5e1]на вашем ПК через быстрые зеркала GitHub и установит их на роутер OpenWrt.[-]\n\n" +
		"  [#94a3b8]⚠️  Перед началом убедитесь, что ваш ПК подключен к сети роутера[-]\n" +
		"  [#94a3b8](LAN-порт или домашний Wi-Fi).[-]"

	step1Form = tview.NewForm()
	StyleForm(step1Form)
	step1Form.AddButton("Продолжить →", func() {
		ctx.Pages.SwitchToPage("ssh_wizard")
		ctx.App.SetFocus(step2Form)
	})
	step1Form.AddButton("Выход", func() {
		ctx.App.Stop()
	})
	EnableFormArrowNavigation(step1Form)

	step1InfoView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	step1InfoView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	step1InfoView.SetText("")

	logoWidget.OnComplete = func() {
		ctx.App.QueueUpdateDraw(func() {
			step1InfoView.SetText(welcomeText)
			ctx.App.SetFocus(step1Form)
		})
	}
	logoWidget.StartAnimation(ctx.App)

	step1Panel.AddItem(logoWidget, 6, 1, false)
	step1Panel.AddItem(step1InfoView, 0, 1, false)
	step1Panel.AddItem(nil, 1, 0, false)
	step1Panel.AddItem(formLayout(step1Form), 5, 1, true)

	step1Modal := CreateWizardModal(step1Panel)

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
		step2Err.SetText("")
		ctx.Pages.SwitchToPage("password_wizard")
		ctx.App.SetFocus(step3Form)
	}
	step2Form.AddButton("Продолжить →", goStep3)
	step2Form.AddButton("← Назад", func() {
		ctx.Pages.SwitchToPage("welcome_wizard")
		ctx.App.SetFocus(step1Form)
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
		"  [#94a3b8]Пароль используется только для SSH-сессии установки.[-]"

	step3TextView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(step3Text)
	step3TextView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	step3Form = tview.NewForm()
	StyleForm(step3Form)
	step3Form.AddFormItem(passInput)

	submitCredentials := func() {
		port, err := strconv.Atoi(strings.TrimSpace(portInput.GetText()))
		if err != nil || port <= 0 {
			port = 22
		}

		ctx.Config.RouterIP = strings.TrimSpace(ipInput.GetText())
		ctx.Config.SSHPort = port
		ctx.Config.Username = strings.TrimSpace(userInput.GetText())
		ctx.Config.Password = passInput.GetText()

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

	step3Panel.AddItem(textViewLayout(step3TextView), 6, 1, false)
	step3Panel.AddItem(nil, 1, 0, false)
	step3Panel.AddItem(formLayout(step3Form), 6, 1, true)

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

	// Pre-select mirror if already configured
	if ctx.Config != nil && ctx.Config.SelectedMirror != "" {
		for idx, m := range optSelector.mirrors {
			if m.Key == ctx.Config.SelectedMirror {
				optSelector.selectedMirror = idx
				optSelector.mirrorCursor = idx
				break
			}
		}
	}

	optSelector.OnSubmit = func(opts InstallOptions) {
		ctx.Pages.SwitchToPage("progress")
		ctx.App.SetFocus(ctx.ConsoleView)

		if ctx.OnStartInstall != nil {
			ctx.OnStartInstall(opts)
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

	modal := CreateWizardModalCustom(panel, 92, 28)
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
	panel.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	panel.SetBorder(true).
		SetTitle(fmt.Sprintf(" 🛰️ TACHYON SETUP: %s ", title)).
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(tcell.NewRGBColor(56, 189, 248)).
		SetBorderColor(tcell.NewRGBColor(56, 189, 248))
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



// StyleForm applies custom dark Slate and Sky-600 styling to forms and buttons,
// eliminating glaring white inverted boxes on focus.
func StyleForm(form *tview.Form) {
	form.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	form.SetLabelColor(tview.Styles.SecondaryTextColor)
	form.SetFieldTextColor(tview.Styles.PrimaryTextColor)
	form.SetFieldBackgroundColor(tcell.NewRGBColor(30, 41, 59)) // Slate 800
	form.SetButtonBackgroundColor(tcell.NewRGBColor(30, 41, 59)) // Slate 800
	form.SetButtonTextColor(tcell.NewRGBColor(241, 245, 249))    // Slate 100
	form.SetButtonActivatedStyle(tcell.StyleDefault.
		Background(tcell.NewRGBColor(2, 132, 199)). // Sky 600
		Foreground(tcell.ColorWhite).
		Bold(true))
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
