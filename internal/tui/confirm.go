package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var engineLabels = map[string]string{
	"sing-box-extended":            "sing-box-extended (xHTTP)",
	"sing-box-extended-compressed": "sing-box-extended (сжатый бинарник)",
	"sing-box-tiny":                "sing-box-tiny (минимум RAM)",
	"sing-box-lx":                  "sing-box-lx (Leadaxe)",
	"steer":                        "steer (C-движок, минимум RAM)",
	"steer-extended":               "steer-extended (C-движок, xHTTP)",
	"skip":                         "без ядра (поставить позже)",
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
	fmt.Fprintf(&b, "  %sРоутер:%s          %s%s%s  %s(%s)%s\n", TagMuted, TagReset, TagTextBold, tview.Escape(routerIP), TagReset, TagMuted, tview.Escape(profile.Model), TagReset)
	fmt.Fprintf(&b, "  %sСистема:%s         %sOpenWrt %s, %s%s\n", TagMuted, TagReset, TagText, tview.Escape(profile.Version), tview.Escape(profile.Arch), TagReset)
	fmt.Fprintf(&b, "  %sЯдро прокси:%s     %s%s%s\n", TagMuted, TagReset, TagCyanBold, engineLabel(opts.SelectedEngine), TagReset)
	fmt.Fprintf(&b, "  %sВерсия Tachyon:%s  %s%s%s\n", TagMuted, TagReset, TagVioletBold, tview.Escape(opts.TachyonVersion), TagReset)
	fmt.Fprintf(&b, "  %sЗеркало:%s         %s%s%s\n", TagMuted, TagReset, TagSubText, tview.Escape(mirrorLabel(opts.SelectedMirror)), TagReset)
	fmt.Fprintf(&b, "  %sРусский LuCI:%s    %s%s%s\n", TagMuted, TagReset, TagSubText, lang, TagReset)
	b.WriteString(fmt.Sprintf("\n  %sЧто произойдёт:%s\n", TagCyanBold, TagReset))
	b.WriteString(fmt.Sprintf("  %s1. Файлы скачаются на этот компьютер и проверятся%s\n", TagSubText, TagReset))
	b.WriteString(fmt.Sprintf("  %s2. Конфиг роутера сохранится в резервную копию%s\n", TagSubText, TagReset))
	b.WriteString(fmt.Sprintf("  %s3. Пакеты зальются в /tmp роутера и установятся%s\n", TagSubText, TagReset))
	b.WriteString(fmt.Sprintf("  %s4. Служба запустится и пройдёт диагностику%s\n", TagSubText, TagReset))
	if w := profile.Warnings(); len(w) > 0 {
		fmt.Fprintf(&b, "\n  %s⚠ Предупреждений о роутере: %d (см. диагностику, клавиша D на прошлом экране)%s\n", TagAmberBold, len(w), TagReset)
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
	tv.SetBackgroundColor(ColorBgSpace)

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
