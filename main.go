// Tachyon One-Click Express Installer & Diagnostic Wizard
package main

import (
	"github.com/rivo/tview"
	gossh "golang.org/x/crypto/ssh"

	appconfig "tachyon-installer/internal/config"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
	"tachyon-installer/internal/tui"
	"tachyon-installer/internal/tui/widgets"
)

func main() {
	cfg := appconfig.Load("manager_config.json")

	tui.InitTheme()

	app := tview.NewApplication()
	pages := tview.NewPages()

	// Header
	header := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[#38bdf8]🛰️  TACHYON ONE-CLICK EXPRESS INSTALLER & DIAGNOSTIC WIZARD[-]")

	// Console view
	consoleView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	consoleView.SetChangedFunc(func() {
		consoleView.ScrollToEnd()
		app.Draw()
	})
	consoleView.SetBorder(true).SetTitle(" 📊 Лог установки и диагностики ")
	consoleView.SetBorderColor(tui.ZincColor(70))

	// Progress view — custom animated progress bar widget
	progressView := widgets.NewProgressBar(7)
	progressView.SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	progressLayout := tui.BuildConsoleLayout(header, progressView, consoleView)
	pages.AddPage("progress", progressLayout, true, false)

	// Build AppContext
	ctx := &tui.AppContext{
		App:          app,
		Pages:        pages,
		ConsoleView:  consoleView,
		ProgressView: progressView,
		Config:       cfg,
		Reconnect: func() (*gossh.Client, error) {
			return sshpkg.Connect(cfg.RouterIP, cfg.SSHPort, cfg.Username, cfg.Password)
		},
	}

	// Wire callbacks
	ctx.OnProfileReady = func(profile *routerpkg.RouterProfile) {
		handleProfileReady(ctx)
	}

	ctx.OnStartInstall = func(opts tui.InstallOptions) {
		ctx.Installing = true
		tui.RunExpressInstall(ctx, opts)
	}

	ctx.OnSaveSub = func(url string) {
		go tui.RunSaveSubscription(ctx, url)
	}

	ctx.OnSkipSub = func() {
		ctx.Installing = false
		tui.FinishAndExit(ctx, true, "")
	}

	// Graceful Ctrl+C handling
	tui.SetupGracefulShutdown(ctx)

	// Display wizard
	tui.ShowWelcomeWizard(ctx)

	if err := app.SetRoot(pages, true).EnableMouse(false).Run(); err != nil {
		panic(err)
	}
}

// handleProfileReady connects to the router, profiles it, and shows the options wizard.
func handleProfileReady(ctx *tui.AppContext) {
	cfg := ctx.Config

	client, err := sshpkg.Connect(cfg.RouterIP, cfg.SSHPort, cfg.Username, cfg.Password)
	if err != nil {
		ctx.App.QueueUpdateDraw(func() {
			tui.ShowErrorWizard(ctx, err)
		})
		return
	}
	ctx.SSHClient = client

	profile, err := routerpkg.RunPreConnectionCheck(client)
	if err != nil {
		client.Close()
		ctx.SSHClient = nil
		ctx.App.QueueUpdateDraw(func() {
			tui.ShowErrorWizard(ctx, err)
		})
		return
	}

	profileData := tui.ProfileData{
		Model:               profile.Model,
		Version:             profile.Version,
		Arch:                profile.Arch,
		DistribArch:         profile.DistribArch,
		RAMTotal:            profile.RAMTotal,
		RAMFree:             profile.RAMFree,
		FlashFree:           profile.FlashFree,
		Conflicts:           profile.Conflicts,
		Firewall:            profile.Firewall,
		IsAPK:               profile.IsAPK,
		ActiveEngine:        profile.ActiveEngine,
		InstalledTachyonVer: profile.InstalledTachyonVer,
		InstalledEngineVer:  profile.InstalledEngineVer,
	}

	ctx.App.QueueUpdateDraw(func() {
		tui.ShowOptionsWizard(ctx, profileData)
	})
}

