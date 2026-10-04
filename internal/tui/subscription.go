package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ShowSubscriptionSetupForm displays a modal for entering a subscription URL.
func ShowSubscriptionSetupForm(ctx *AppContext) {
	panel := NewSolidFlex()
	panel.SetDirection(tview.FlexRow)
	panel.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	panel.SetBorder(true).
		SetTitle(" 🚀 ДОБАВЛЕНИЕ ССЫЛКИ НА ПОДПИСКУ ").
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(tcell.NewRGBColor(56, 189, 248)).
		SetBorderColor(tcell.NewRGBColor(56, 189, 248))

	text := "\n  [#38bdf8]🛰️  Tachyon успешно установлен![-]\n\n" +
		"  [#cbd5e1]Для обхода блокировок вставьте вашу ссылку на подписку[-]\n" +
		"  [#cbd5e1](VLESS / Shadowsocks / Trojan / VMess) от вашего VPN-провайдера:[-]"

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(text)
	textView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	subInput := tview.NewInputField().
		SetLabel("Ссылка на подписку: ").
		SetText("").
		SetFieldWidth(60)

	form := tview.NewForm()
	form.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	form.SetLabelColor(tview.Styles.SecondaryTextColor)
	form.SetFieldTextColor(tview.Styles.PrimaryTextColor)
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
		ctx.ConsoleWrite("\n[#cbd5e1]⚡ Сохранение подписки в Tachyon...[-]\n")

		if ctx.OnSaveSub != nil {
			ctx.OnSaveSub(url)
		}
	}

	form.AddButton("🚀 Сохранить и запустить", saveAndRun)
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
	panel.AddItem(nil, 1, 0, false)
	panel.AddItem(formLayout(form), 6, 1, true)

	modalForm := NewSolidFlex()
	modalForm.AddItem(nil, 0, 1, false)
	modalForm.AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(panel, 16, 1, true).
		AddItem(nil, 0, 1, false), 76, 1, true)
	modalForm.AddItem(nil, 0, 1, false)
	modalForm.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	ctx.Pages.AddPage("sub_setup", modalForm, true, true)
	ctx.Pages.SwitchToPage("sub_setup")
	ctx.App.SetFocus(subInput)
}
