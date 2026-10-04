package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/crypto/ssh"

	appconfig "tachyon-installer/internal/config"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
	"tachyon-installer/internal/tui/widgets"
)

// ansiRegex is compiled once for stripping ANSI escape codes from remote output.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes ANSI escape sequences from a string.
func StripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

// InstallOptions holds the parameters chosen by the user in the wizard.
type InstallOptions struct {
	TachyonVersion string
	SelectedEngine string
	SelectedMirror string
	InstallI18n    bool
	InstallZRAM    bool
}

// AppContext holds all shared state for the TUI application.
type AppContext struct {
	App         *tview.Application
	Pages       *tview.Pages
	ConsoleView *tview.TextView
	Config      *appconfig.Config
	SSHClient   *ssh.Client

	// Callbacks provided by the installer orchestrator.
	OnProfileReady func(profile *routerpkg.RouterProfile)
	OnStartInstall func(opts InstallOptions)
	OnHotSwap      func(engineKey, mirrorKey string)
	OnSaveSub      func(subURL string)
	OnSkipSub      func()

	// Reconnect function for resilient SSH.
	Reconnect func() (*ssh.Client, error)

	// ProgressView displays the current installation step/progress bar.
	ProgressView *widgets.ProgressBar

	// CancelModalShown tracks whether the Ctrl+C modal is visible.
	CancelModalShown bool
	Installing       bool

	// Final Results for native terminal output
	HasFinalResult  bool
	FinalSuccess    bool
	FinalArchiveVer string

	// FirewallNotReady is set when the router reported fw4 rule errors during install.
	FirewallNotReady bool
	// LogPath is the file with the full raw installer output.
	LogPath string
	// ReportPath is the saved post-install diagnostics report.
	ReportPath string

	// DiagOnly makes the wizard run diagnostics instead of an installation.
	DiagOnly bool
	// RescueOnly makes the wizard run emergency rescue directly after connecting.
	RescueOnly bool
	// GuidedOnly makes the wizard open the hand-in-hand autopilot setup modal directly.
	GuidedOnly bool
	// AppVersion is the version of tachyon-installer.
	AppVersion string
}

// ExecSSH executes a command on the router with reconnection support.
func (ctx *AppContext) ExecSSH(cmd string) (string, error) {
	return sshpkg.Exec(ctx.SSHClient, cmd, ctx.Reconnect)
}

// ConsoleWrite writes text to the console view (keeps tview color tags).
func (ctx *AppContext) ConsoleWrite(text string) {
	clean := strings.ReplaceAll(text, "\r", "")
	go ctx.App.QueueUpdateDraw(func() {
		ctx.ConsoleView.Write([]byte(clean))
		ctx.ConsoleView.ScrollToEnd()
	})
}

// ConsoleWritef writes formatted text to the console view (keeps tview color tags).
func (ctx *AppContext) ConsoleWritef(format string, args ...any) {
	ctx.ConsoleWrite(fmt.Sprintf(format, args...))
}

// SetSubTask updates the sub-task progress bar.
func (ctx *AppContext) SetSubTask(label string, fraction float64) {
	if ctx.ProgressView != nil {
		ctx.ProgressView.SetSubTask(label, fraction)
	}
}

// SetupGracefulShutdown intercepts Ctrl+C and shows a confirmation modal.
func SetupGracefulShutdown(ctx *AppContext) {
	ctx.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			if ctx.Installing && !ctx.CancelModalShown {
				showCancelModal(ctx)
				return nil
			}
			ctx.App.Stop()
			return nil
		}
		return event
	})
}

func showCancelModal(ctx *AppContext) {
	ctx.CancelModalShown = true

	modal := tview.NewFlex().SetDirection(tview.FlexRow)
	modal.SetBorder(true).
		SetTitle(" ⚠️  ПРЕДУПРЕЖДЕНИЕ ").
		SetTitleAlign(tview.AlignCenter).
		SetTitleColor(tcell.NewRGBColor(234, 179, 8)).
		SetBorderColor(tcell.NewRGBColor(234, 179, 8))
	modal.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("\n[#eab308]Установка ещё не завершена![-]\n[#cbd5e1]Вы уверены, что хотите прервать процесс?[-]\n")
	textView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	form := tview.NewForm()
	form.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	form.SetLabelColor(tview.Styles.SecondaryTextColor)
	form.SetFieldTextColor(tview.Styles.PrimaryTextColor)
	form.AddButton("❌ Отменить установку", func() {
		ctx.Installing = false
		ctx.App.Stop()
	})
	form.AddButton("✅ Продолжить установку", func() {
		ctx.CancelModalShown = false
		ctx.Pages.RemovePage("cancel_modal")
	})

	modal.AddItem(nil, 0, 1, false)
	modal.AddItem(textView, 5, 1, false)
	modal.AddItem(nil, 1, 0, false)
	modal.AddItem(form, 4, 1, true)

	overlay := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(modal, 60, 1, true).
			AddItem(nil, 0, 1, false), 12, 1, true).
		AddItem(nil, 0, 1, false)

	ctx.Pages.AddPage("cancel_modal", overlay, true, true)
	ctx.App.SetFocus(form)
}

func PrintFinalResultsToConsole(ctx *AppContext, success bool, archiveVer string) {
	ctx.ConsoleWrite("\n[#38bdf8]======================================================[-]\n")
	if success {
		ctx.ConsoleWrite("[#22c55e] 🎉 УСТАНОВКА TACHYON УСПЕШНО ЗАВЕРШЕНА[-]\n")
	} else {
		ctx.ConsoleWrite("[#eab308] ⚠️ УСТАНОВКА ЗАВЕРШЕНА С ЗАМЕЧАНИЯМИ[-]\n")
	}
	ctx.ConsoleWrite("[#38bdf8]======================================================[-]\n\n")

	ip := ""
	if ctx.Config != nil {
		ip = ctx.Config.RouterIP
	}
	luciURL := fmt.Sprintf("http://%s/cgi-bin/luci/admin/services/tachyon", ip)
	ctx.ConsoleWrite(fmt.Sprintf("  [#22c55e]✅[-] Tachyon Core: [#f8fafc]v%s[-]\n", archiveVer))
	ctx.ConsoleWrite(fmt.Sprintf("  [#22c55e]✅[-] LuCI Web: [#f8fafc]%s[-]\n", luciURL))
	ctx.ConsoleWrite(fmt.Sprintf("  [#22c55e]✅[-] SSH: [#f8fafc]ssh root@%s[-]\n\n", ip))
	if ctx.FirewallNotReady {
		ctx.ConsoleWrite("  [#eab308]⚠ Правила firewall fw4 не применились: таблица ещё не была загружена.[-]\n")
		ctx.ConsoleWrite("  [#cbd5e1]Это не ломает установку. Если прокси не заработает, выполните на роутере:[-]\n")
		ctx.ConsoleWrite("  [#38bdf8]/etc/init.d/firewall restart && /etc/init.d/tachyon restart[-]\n\n")
	}
	if ctx.ReportPath != "" {
		ctx.ConsoleWrite(fmt.Sprintf("  [#94a3b8]Отчёт диагностики: %s[-]\n", ctx.ReportPath))
	}
	if ctx.LogPath != "" {
		ctx.ConsoleWrite(fmt.Sprintf("  [#94a3b8]Полный лог установки: %s[-]\n\n", ctx.LogPath))
	}
	ctx.ConsoleWrite("  [#38bdf8:b]🚀 Нажмите [O] (или выберите в окне), чтобы открыть LuCI в браузере![-]\n")
	ctx.ConsoleWrite("  [#64748b]Клавиши: [O] открыть браузер · [Enter / Esc / Q] закрыть программу[-]\n\n")
}
