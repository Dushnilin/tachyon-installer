package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"tachyon-installer/internal/browser"
	"tachyon-installer/internal/tui/widgets"
)

// ShowLoadingWizard displays the "diagnosing router" loading screen with animated spinner.
func ShowLoadingWizard(ctx *AppContext) {
	panel := NewSolidFlex()
	panel.SetDirection(tview.FlexRow)
	panel.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	panel.SetBorder(true).
		SetTitle(" ⏳ ДИАГНОСТИКА РОУТЕРА ").
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(tcell.NewRGBColor(234, 179, 8)).
		SetBorderColor(tcell.NewRGBColor(234, 179, 8))

	checklist := []widgets.CheckItem{
		{Text: "Проверка SSH-подключения", Checked: false, DelayMs: 0},
		{Text: "Сбор характеристик", Checked: false, DelayMs: 800},
		{Text: "Оценка RAM и Flash", Checked: false, DelayMs: 1500},
		{Text: "Поиск конфликтов", Checked: false, DelayMs: 2200},
	}

	loadingSpinner := widgets.NewLoadingSpinner("Подключение и диагностика роутера", checklist)
	loadingSpinner.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	loadingSpinner.StartAnimation(ctx.App)

	panel.AddItem(loadingSpinner, 12, 1, false)

	modal := CreateWizardModal(panel)
	ctx.Pages.AddPage("loading_wizard", modal, true, true)
	ctx.Pages.SwitchToPage("loading_wizard")
}

// ShowErrorWizard displays a connection error with contextual advice.
func ShowErrorWizard(ctx *AppContext, err error) {
	panel := NewSolidFlex()
	panel.SetDirection(tview.FlexRow)
	panel.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	panel.SetBorder(true).
		SetTitle(" ❌ ОШИБКА ПОДКЛЮЧЕНИЯ ").
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(tcell.NewRGBColor(239, 68, 68)).
		SetBorderColor(tcell.NewRGBColor(239, 68, 68))

	hint := "Убедитесь, что ваш компьютер подключен к LAN-порту роутера или по Wi-Fi."
	errStr := err.Error()
	if strings.Contains(errStr, "authenticate") || strings.Contains(errStr, "handshake") {
		hint = "Неверное имя пользователя или пароль. Пожалуйста, введите верные учетные данные роутера."
	} else if strings.Contains(errStr, "timeout") {
		hint = "Превышено время ожидания. Проверьте, что указан правильный IP-адрес роутера."
	} else if strings.Contains(errStr, "refused") {
		hint = "Подключение отклонено. Возможно, служба SSH отключена на роутере или порт указан неверно."
	}

	text := fmt.Sprintf(
		"\n  [#ef5350]❌ Не удалось подключиться к роутеру по SSH[-]\n\n"+
			"  [#cbd5e1]Техническая деталь:[-] [#94a3b8]%s[-]\n\n"+
			"  [#38bdf8]💡 Рекомендация:[-]\n  %s\n",
		errStr,
		hint,
	)

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(text)
	textView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	form := tview.NewForm()
	form.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	form.AddButton("← Назад к вводу пароля", func() {
		ctx.Pages.SwitchToPage("password_wizard")
		ctx.App.SetFocus(ctx.Pages.GetPage("password_wizard"))
	})
	form.AddButton("Выход", func() {
		ctx.App.Stop()
	})
	EnableFormArrowNavigation(form)

	textLayout := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 2, 0, false).
		AddItem(textView, 0, 1, false).
		AddItem(nil, 2, 0, false)

	formLayout := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 2, 0, false).
		AddItem(form, 0, 1, true).
		AddItem(nil, 2, 0, false)

	panel.AddItem(textLayout, 11, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout, 5, 1, true)

	modal := CreateWizardModal(panel)
	ctx.Pages.AddPage("error_wizard", modal, true, true)
	ctx.Pages.SwitchToPage("error_wizard")
	ctx.App.SetFocus(form)
}

// FinishAndExit signals the main routine to print final results natively and shows the completion dialog.
func FinishAndExit(ctx *AppContext, success bool, archiveVer string) {
	ctx.HasFinalResult = true
	ctx.FinalSuccess = success
	ctx.FinalArchiveVer = archiveVer
	ctx.Installing = false

	if ctx.ProgressView != nil {
		if success {
			ctx.ProgressView.MarkCompleted()
		}
		ctx.ProgressView.StopAnimation()
	}

	PrintFinalResultsToConsole(ctx, success, archiveVer)

	ip := "192.168.1.1"
	if ctx.Config != nil && ctx.Config.RouterIP != "" {
		ip = ctx.Config.RouterIP
	}
	luciURL := fmt.Sprintf("http://%s/cgi-bin/luci/admin/services/tachyon", ip)

	// Set input capture on console view to handle [O] to open browser and [Q/Esc/Enter] to exit
	ctx.ConsoleView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'o', 'O', 'щ', 'Щ':
			_ = browser.OpenURL(luciURL)
			ctx.ConsoleWritef("[#22c55e]✓ Открытие веб-интерфейса в браузере: %s[-]\n", luciURL)
			return nil
		case 'q', 'Q', 'й', 'Й':
			ctx.App.Stop()
			return nil
		}
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyEnter {
			ctx.App.Stop()
			return nil
		}
		return event
	})

	ctx.App.QueueUpdateDraw(func() {
		ShowFinishDialog(ctx, success, luciURL)
	})
}

// ShowFinishDialog displays a modal dialog with 1-click browser launch.
func ShowFinishDialog(ctx *AppContext, success bool, luciURL string) {
	panel := NewSolidFlex()
	panel.SetDirection(tview.FlexRow)
	panel.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	title := " 🎉 УСТАНОВКА TACHYON УСПЕШНО ЗАВЕРШЕНА "
	titleColor := tcell.NewRGBColor(34, 197, 94) // Green
	if !success {
		title = " ⚠️ УСТАНОВКА ЗАВЕРШЕНА С ЗАМЕЧАНИЯМИ "
		titleColor = tcell.NewRGBColor(234, 179, 8) // Yellow
	}

	panel.SetBorder(true).
		SetTitle(title).
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(titleColor).
		SetBorderColor(titleColor)

	text := fmt.Sprintf("\n  [#f8fafc]Служба Tachyon успешно настроена и проверена на роутере.[-]\n\n"+
		"  [#94a3b8]Веб-интерфейс LuCI:[-] [#38bdf8]%s[-]\n\n"+
		"  [#cbd5e1]Нажмите кнопку ниже или клавишу [O], чтобы открыть панель управления в браузере.[-]", luciURL)

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(text)
	textView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	form := tview.NewForm()
	form.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	form.AddButton("🌐 Открыть LuCI в браузере (Enter)", func() {
		_ = browser.OpenURL(luciURL)
		ctx.ConsoleWritef("[#22c55e]✓ Открытие веб-интерфейса в браузере: %s[-]\n", luciURL)
	})
	form.AddButton("📊 Просмотр лога", func() {
		ctx.Pages.RemovePage("finish_dialog")
		ctx.App.SetFocus(ctx.ConsoleView)
	})
	form.AddButton("🚪 Выйти (Esc)", func() {
		ctx.App.Stop()
	})

	form.SetCancelFunc(func() {
		ctx.App.Stop()
	})

	EnableFormArrowNavigation(form)

	panel.AddItem(textView, 6, 1, false)
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(form, 4, 1, true)

	modal := CreateWizardModalCustom(panel, 76, 15)
	ctx.Pages.AddPage("finish_dialog", modal, true, true)
	ctx.App.SetFocus(form)
}
