package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	routerpkg "tachyon-installer/internal/router"
)

// ShowSubscriptionSetupForm displays a modal for entering a subscription URL or direct proxy link(s).
func ShowSubscriptionSetupForm(ctx *AppContext) {
	panel := buildStepPanel("Добавление подписки")

	text := "\n  [#38bdf8]Tachyon установлен на роутер[-]\n\n" +
		"  [#a1a1a6]Вставьте ссылку на подписку (HTTPS / Happ crypt4), конфигурационную ссылку[-]\n" +
		"  [#a1a1a6](VLESS, Trojan, Shadowsocks, VMess, Hysteria2, TUIC, AnyTLS, FPTN, CSQTT, WDTT, Mieru) или base64:[-]"

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(text)
	textView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetText("  [#71717a]Поддерживаются HTTPS/Happ ссылки, прямые URI (vless://, hy2://, fptn:, csqtt://, wdtt://) и base64[-] ")
	statusView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	subInput := tview.NewInputField().
		SetLabel("Ссылка на подписку: ").
		SetText("").
		SetFieldWidth(60)

	subInput.SetChangedFunc(func(text string) {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			statusView.SetText("  [#71717a]Поддерживаются HTTPS/Happ ссылки, прямые URI (vless://, hy2://, fptn:, csqtt://, wdtt://) и base64[-] ")
			return
		}
		analysis := routerpkg.AnalyzeSubscription(trimmed)
		if analysis.Valid {
			statusView.SetText("  [#4cc38a]✓ " + analysis.Summary + "[-] ")
		} else {
			statusView.SetText("  [#e5484d]! " + analysis.ErrorMsg + "[-] ")
		}
	})

	form := tview.NewForm()
	StyleForm(form)
	form.AddFormItem(subInput)

	saveAndRun := func() {
		url := strings.TrimSpace(subInput.GetText())
		if url == "" {
			if ctx.OnSkipSub != nil {
				ctx.OnSkipSub()
			}
			return
		}

		ctx.Pages.SwitchToPage("progress")
		ctx.ConsoleWrite("\n[#a1a1a6]Сохранение подписки в Tachyon...[-]\n")

		if ctx.OnSaveSub != nil {
			ctx.OnSaveSub(url)
		}
	}

	form.AddButton("Сохранить и запустить", saveAndRun)
	form.AddButton("Пропустить (настроить в LuCI)", func() {
		if ctx.OnSkipSub != nil {
			ctx.OnSkipSub()
		}
	})

	subInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			saveAndRun()
		}
	})

	EnableFormArrowNavigation(form)

	panel.AddItem(textViewLayout(textView), 5, 1, false)
	panel.AddItem(statusView, 1, 0, false)
	panel.AddItem(formLayout(form), 6, 1, true)

	modalForm := CreateWizardModalCustom(panel, 88, 18)

	ctx.Pages.AddPage("sub_setup", modalForm, true, true)
	ctx.Pages.SwitchToPage("sub_setup")
	ctx.App.SetFocus(subInput)
}
