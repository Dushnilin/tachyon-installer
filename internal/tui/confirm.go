package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var engineLabels = map[string]string{
	"sing-box-extended": "sing-box-extended (рекомендуется)",
	"steer-extended":    "steer-extended",
	"steer":             "steer (лёгкий)",
	"sing-box-lx":       "sing-box-lx",
	"skip":              "без ядра (поставить позже)",
}

func engineLabel(key string) string {
	if l, ok := engineLabels[key]; ok {
		return l
	}
	return key
}

func mirrorLabel(key string) string {
	switch key {
	case "auto":
		return "автовыбор самого быстрого"
	case "direct":
		return "github.com напрямую"
	}
	return strings.TrimSuffix(strings.TrimPrefix(key, "https://"), "/")
}

// confirmText builds the summary shown before installation starts.
func confirmText(routerIP string, profile ProfileData, opts InstallOptions) string {
	lang := "нет"
	if opts.InstallI18n {
		lang = "да, русский языковой пакет LuCI"
	}
	var b strings.Builder
	b.WriteString("\n")
	fmt.Fprintf(&b, "  [#94a3b8]Роутер:[-]          [#f1f5f9]%s[-]  [#64748b](%s)[-]\n", tview.Escape(routerIP), tview.Escape(profile.Model))
	fmt.Fprintf(&b, "  [#94a3b8]Система:[-]         [#f1f5f9]OpenWrt %s, %s[-]\n", tview.Escape(profile.Version), tview.Escape(profile.Arch))
	fmt.Fprintf(&b, "  [#94a3b8]Ядро прокси:[-]     [#f1f5f9]%s[-]\n", engineLabel(opts.SelectedEngine))
	fmt.Fprintf(&b, "  [#94a3b8]Версия Tachyon:[-]  [#f1f5f9]%s[-]\n", tview.Escape(opts.TachyonVersion))
	fmt.Fprintf(&b, "  [#94a3b8]Зеркало:[-]         [#f1f5f9]%s[-]\n", tview.Escape(mirrorLabel(opts.SelectedMirror)))
	fmt.Fprintf(&b, "  [#94a3b8]Русский LuCI:[-]    [#f1f5f9]%s[-]\n", lang)
	b.WriteString("\n  [#38bdf8]Что произойдёт:[-]\n")
	b.WriteString("  [#cbd5e1]1. Файлы скачаются на этот компьютер и проверятся[-]\n")
	b.WriteString("  [#cbd5e1]2. Конфиг роутера сохранится в резервную копию[-]\n")
	b.WriteString("  [#cbd5e1]3. Пакеты зальются в /tmp роутера и установятся[-]\n")
	b.WriteString("  [#cbd5e1]4. Служба запустится и пройдёт диагностику[-]\n")
	if w := profile.Warnings(); len(w) > 0 {
		fmt.Fprintf(&b, "\n  [#eab308]⚠ Предупреждений о роутере: %d (см. диагностику, клавиша D на прошлом экране)[-]\n", len(w))
	}
	return b.String()
}

// showConfirmModal asks the user to confirm the chosen options before the install starts.
func showConfirmModal(ctx *AppContext, profile ProfileData, opts InstallOptions, onConfirm, onCancel func()) {
	panel := buildStepPanel(" ПОДТВЕРЖДЕНИЕ ")

	routerIP := ""
	if ctx.Config != nil {
		routerIP = ctx.Config.RouterIP
	}
	tv := tview.NewTextView().SetDynamicColors(true).SetText(confirmText(routerIP, profile, opts))
	tv.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	form := tview.NewForm()
	StyleForm(form)
	form.SetButtonsAlign(tview.AlignCenter)
	form.AddButton("🚀 Установить", onConfirm)
	form.AddButton("← Изменить", onCancel)
	form.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyLeft, tcell.KeyUp:
			return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
		case tcell.KeyRight, tcell.KeyDown:
			return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
		case tcell.KeyEscape:
			onCancel()
			return nil
		}
		return ev
	})

	panel.AddItem(tv, 0, 1, false)
	panel.AddItem(formLayout(form), 3, 0, true)

	ctx.Pages.AddPage("confirm_wizard", CreateWizardModalCustom(panel, 84, 22), true, true)
	ctx.Pages.SwitchToPage("confirm_wizard")
	ctx.App.SetFocus(form)
}
