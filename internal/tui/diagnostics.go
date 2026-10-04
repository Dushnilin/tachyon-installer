package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	gossh "golang.org/x/crypto/ssh"

	"tachyon-installer/internal/diag"
	sshpkg "tachyon-installer/internal/ssh"
)

func diagIcon(s diag.Status) string {
	switch s {
	case diag.OK:
		return "[#22c55e]✓[-]"
	case diag.Info:
		return "[#38bdf8]ℹ[-]"
	case diag.Warn:
		return "[#eab308]⚠[-]"
	default:
		return "[#ef5350]✗[-]"
	}
}

// renderDiagResults formats results for a dynamic-colors TextView.
func renderDiagResults(results []diag.Result, running bool) string {
	var b strings.Builder
	b.WriteString("\n")
	for _, r := range results {
		fmt.Fprintf(&b, "  %s [#f1f5f9]%s[-]\n", diagIcon(r.Status), tview.Escape(r.Title))
		if r.Detail != "" {
			for _, l := range strings.Split(r.Detail, "\n") {
				fmt.Fprintf(&b, "      [#94a3b8]%s[-]\n", tview.Escape(l))
			}
		}
		if r.Hint != "" {
			fmt.Fprintf(&b, "      [#38bdf8]→ %s[-]\n", tview.Escape(r.Hint))
		}
	}
	if running {
		b.WriteString("\n  [#eab308]⏳ Выполняются проверки…[-]\n")
	} else if len(results) > 0 {
		ok, warn, fail := diag.Summary(results)
		fmt.Fprintf(&b, "\n  [#22c55e]OK: %d[-]   [#eab308]предупреждений: %d[-]   [#ef5350]ошибок: %d[-]\n", ok, warn, fail)
	}
	return b.String()
}

// ShowDiagnosticsWizard opens the built-in diagnostics screen and runs all checks.
// back is the page to return to.
func ShowDiagnosticsWizard(ctx *AppContext, back string) {
	panel := buildStepPanel(" ДИАГНОСТИКА ")

	tv := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	tv.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	note := tview.NewTextView().SetDynamicColors(true)
	note.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	note.SetText("  [#64748b]↑↓ PgUp/PgDn — прокрутка · ←→ — кнопки · Enter — выбрать · Esc — назад[-]")

	var (
		results []diag.Result
		running bool
	)

	var runChecks func()
	runChecks = func() {
		if running {
			return
		}
		running = true
		results = nil
		tv.SetText(renderDiagResults(nil, true))

		go func() {
			cfg := ctx.Config
			push := func(r diag.Result) {
				ctx.App.QueueUpdateDraw(func() {
					results = append(results, r)
					tv.SetText(renderDiagResults(results, true))
					tv.ScrollToEnd()
				})
			}

			diag.RunHost(cfg.RouterIP, cfg.SSHPort, push)

			reconnect := ctx.Reconnect
			if reconnect == nil {
				reconnect = func() (*gossh.Client, error) {
					return sshpkg.Connect(cfg.RouterIP, cfg.SSHPort, cfg.Username, cfg.Password)
				}
			}
			client, err := sshpkg.EnsureClient(ctx.SSHClient, reconnect)
			if err != nil {
				push(diag.Result{ID: "ssh", Title: "SSH-подключение", Status: diag.Fail,
					Detail: err.Error(),
					Hint:   "Проверьте логин, пароль или ключ. Проверки роутера пропущены."})
			} else {
				ctx.SSHClient = client
				run := func(cmd string) (string, error) {
					return sshpkg.Exec(ctx.SSHClient, cmd, reconnect)
				}
				push(diag.Result{ID: "ssh", Title: "SSH-подключение", Status: diag.OK,
					Detail: fmt.Sprintf("%s@%s", cfg.Username, cfg.RouterIP)})
				diag.RunRouter(run, push)
			}

			ctx.App.QueueUpdateDraw(func() {
				running = false
				tv.SetText(renderDiagResults(results, false))
				tv.ScrollToBeginning()
			})
		}()
	}

	goBack := func() {
		ctx.Pages.RemovePage("diag_wizard")
		ctx.Pages.SwitchToPage(back)
		ctx.App.SetFocus(ctx.Pages.GetPage(back))
	}

	saveReport := func() {
		if running {
			note.SetText("  [#eab308]Дождитесь окончания проверок[-]")
			return
		}
		if len(results) == 0 {
			note.SetText("  [#eab308]Нечего сохранять[-]")
			return
		}
		name := fmt.Sprintf("tachyon-diagnostics-%s-%s.txt",
			strings.NewReplacer(":", "_", "/", "_").Replace(ctx.Config.RouterIP), time.Now().Format("20060102-150405"))
		dir, err := os.Getwd()
		if err != nil {
			dir = os.TempDir()
		}
		path := filepath.Join(dir, name)
		text := diag.Format("Tachyon Installer: диагностика роутера "+ctx.Config.RouterIP, results)
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			note.SetText("  [#ef5350]Не удалось сохранить: " + tview.Escape(err.Error()) + "[-]")
			return
		}
		note.SetText("  [#22c55e]✓ Отчёт сохранён: " + tview.Escape(path) + "[-]")
	}

	form := tview.NewForm()
	StyleForm(form)
	form.SetButtonsAlign(tview.AlignCenter)
	form.AddButton("🔄 Повторить", func() { runChecks() })
	form.AddButton("💾 Сохранить отчёт", saveReport)
	form.AddButton("← Назад", goBack)
	form.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		row, col := tv.GetScrollOffset()
		switch ev.Key() {
		case tcell.KeyUp:
			if row > 0 {
				tv.ScrollTo(row-1, col)
			}
			return nil
		case tcell.KeyDown:
			tv.ScrollTo(row+1, col)
			return nil
		case tcell.KeyPgUp:
			tv.ScrollTo(max(row-10, 0), col)
			return nil
		case tcell.KeyPgDn:
			tv.ScrollTo(row+10, col)
			return nil
		case tcell.KeyLeft:
			return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
		case tcell.KeyRight:
			return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
		case tcell.KeyEscape:
			goBack()
			return nil
		}
		return ev
	})

	panel.AddItem(tv, 0, 1, false)
	panel.AddItem(note, 1, 0, false)
	panel.AddItem(formLayout(form), 3, 0, true)

	ctx.Pages.AddPage("diag_wizard", CreateWizardModalCustom(panel, 92, 28), true, true)
	ctx.Pages.SwitchToPage("diag_wizard")
	ctx.App.SetFocus(form)
	runChecks()
}

// postInstallDiagnostics runs the router checks after installation, prints problems
// into the console and saves the full report to a file.
func postInstallDiagnostics(ctx *AppContext, run diag.Exec) {
	ctx.SetSubTask("Итоговая диагностика...", 0.5)
	ctx.ConsoleWrite("\n[#cbd5e1]⚡ Итоговая диагностика роутера...[-]\n")

	results := diag.RunRouter(run, nil)
	for _, r := range results {
		if r.Status != diag.Warn && r.Status != diag.Fail {
			continue
		}
		ctx.ConsoleWritef("  %s [#f1f5f9]%s[-]\n", diagIcon(r.Status), tview.Escape(r.Title))
		if r.Detail != "" {
			ctx.ConsoleWritef("      [#94a3b8]%s[-]\n", tview.Escape(strings.ReplaceAll(r.Detail, "\n", " | ")))
		}
		if r.Hint != "" {
			ctx.ConsoleWritef("      [#38bdf8]→ %s[-]\n", tview.Escape(r.Hint))
		}
	}
	ok, warn, fail := diag.Summary(results)
	ctx.ConsoleWritef("  [#94a3b8]Проверок: OK %d, предупреждений %d, ошибок %d[-]\n\n", ok, warn, fail)

	path := filepath.Join(os.TempDir(), "tachyon-diagnostics.txt")
	text := diag.Format("Tachyon Installer: диагностика роутера "+ctx.Config.RouterIP, results)
	if err := os.WriteFile(path, []byte(text), 0644); err == nil {
		ctx.ReportPath = path
	}
}
