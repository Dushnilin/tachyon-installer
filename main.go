// Tachyon One-Click Express Installer & Diagnostic Wizard
package main

import (
	"context"
	"os"

	"github.com/rivo/tview"
	gossh "golang.org/x/crypto/ssh"

	"tachyon-installer/internal/cli"
	appconfig "tachyon-installer/internal/config"
	deploypkg "tachyon-installer/internal/deploy"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
	"tachyon-installer/internal/tui"
	"tachyon-installer/internal/tui/widgets"
	"tachyon-installer/internal/updater"
)

// AppVersion and BuildDate are set at build time via -ldflags.
var (
	AppVersion = "dev"
	BuildDate  = ""
)

func main() {
	cfg := appconfig.Load("manager_config.json")

	// Parse command line flags
	opts, err := cli.ParseFlags(os.Args[1:], cfg, AppVersion)
	if err != nil {
		os.Exit(2)
	}

	// If headless execution or maintenance flags are given, run non-interactively
	if opts.ShouldRunHeadless() {
		os.Exit(cli.Run(opts))
	}

	// Apply CLI flags override into config for TUI mode
	if opts.RouterIP != "" {
		cfg.RouterIP = opts.RouterIP
	}
	if opts.SSHPort > 0 {
		cfg.SSHPort = opts.SSHPort
	}
	if opts.Username != "" {
		cfg.Username = opts.Username
	}
	if opts.Password != "" {
		cfg.Password = opts.Password
	}
	if opts.KeyPath != "" {
		cfg.KeyPath = opts.KeyPath
	}
	if opts.Engine != "" {
		cfg.SelectedEngine = opts.Engine
	}
	if opts.Mirror != "" {
		cfg.SelectedMirror = opts.Mirror
	}
	if opts.Version != "" {
		cfg.TachyonVersion = opts.Version
	}

	tui.InitTheme()

	app := tview.NewApplication()
	app.EnableMouse(true)
	pages := tview.NewPages()

	// Header
	header := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[#38bdf8]🛰️  TACHYON ONE-CLICK EXPRESS INSTALLER & DIAGNOSTIC WIZARD[-] [#64748b]" + AppVersion + "[-]")

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
		AppVersion:   AppVersion,
		Reconnect: func() (*gossh.Client, error) {
			return sshpkg.Connect(cfg.RouterIP, cfg.SSHPort, cfg.Username, cfg.Password)
		},
	}

	// Background release update check
	go func() {
		hasUpd, latestVer, relURL, _ := updater.CheckForUpdate(context.Background(), AppVersion)
		if hasUpd {
			ctx.ConsoleWritef("\n[#eab308]💡 Доступна новая версия tachyon-installer: %s (текущая: %s)!\n   Скачать: %s[-]\n\n", latestVer, AppVersion, relURL)
		}
	}()

	// Wire callbacks
	ctx.OnProfileReady = func(profile *routerpkg.RouterProfile) {
		handleProfileReady(ctx)
	}

	ctx.OnStartInstall = func(opts tui.InstallOptions) {
		ctx.Installing = true
		tui.RunExpressInstall(ctx, opts)
	}

	ctx.OnHotSwap = func(engineKey, mirrorKey string) {
		ctx.Installing = true
		go func() {
			ctx.ConsoleWritef("\n[#38bdf8:b]====================================================\n")
			ctx.ConsoleWritef("⚡ БЫСТРАЯ СМЕНА ЯДРА (HOT-SWAP) НА [%s]\n", engineKey)
			ctx.ConsoleWritef("====================================================[-]\n\n")

			if ctx.ProgressView != nil {
				ctx.ProgressView.SetStep(1, "1/3: Подготовка и загрузка ядра")
				ctx.ProgressView.StartAnimation(ctx.App)
			}

			client := ctx.SSHClient
			if client == nil {
				var err error
				client, err = ctx.Reconnect()
				if err != nil {
					ctx.ConsoleWritef("[#ef5350]❌ Ошибка SSH подключения: %v[-]\n", err)
					tui.FinishAndExit(ctx, false, "")
					return
				}
				ctx.SSHClient = client
			}

			isAPK := routerpkg.IsAPKPackage(client)
			detectedArch := routerpkg.DetectArch(client)
			distribArch := routerpkg.DetectRawArch(client)
			if distribArch == "" {
				distribArch = detectedArch
			}

			logFn := func(msg string) {
				ctx.ConsoleWrite(msg)
			}

			if ctx.ProgressView != nil {
				ctx.ProgressView.SetStep(2, "2/3: Активация ядра и настройка UCI")
			}

			err := deploypkg.HotSwapEngine(
				context.Background(),
				client,
				engineKey,
				mirrorKey,
				isAPK,
				distribArch,
				detectedArch,
				logFn,
			)

			if err != nil {
				ctx.ConsoleWritef("\n[#ef5350]❌ Сбой смены ядра: %v[-]\n", err)
				tui.FinishAndExit(ctx, false, "")
				return
			}

			if ctx.ProgressView != nil {
				ctx.ProgressView.SetStep(3, "3/3: Проверка службы и сквозного обхода")
			}

			execFn := func(_ *gossh.Client, cmd string) (string, error) {
				sess, err := client.NewSession()
				if err != nil {
					return "", err
				}
				defer sess.Close()
				out, err := sess.CombinedOutput(cmd)
				return string(out), err
			}

			progFn := func(label string, frac float64) {}
			checkRes := routerpkg.VerifyAndFallback(client, execFn, progFn)
			if !checkRes.OK {
				ctx.ConsoleWritef("[#eab308]⚠️ Служба сообщила: %s[-]\n", checkRes.Reason)
			} else {
				ctx.ConsoleWrite("[#22c55e]✓ Служба успешно активирована и запущена![-]\n")
			}

			ctx.ConsoleWrite("[#cbd5e1]⚡ Сквозная проверка обхода блокировок и Fake-IP с роутера...[-]\n")
			bpReport := routerpkg.TestBypass(client, execFn)
			if bpReport.Success {
				ctx.ConsoleWritef("[#22c55e]✓ %s[-]\n", bpReport.Details)
			} else {
				ctx.ConsoleWritef("[#eab308]⚠️ %s[-]\n", bpReport.Details)
			}

			tui.FinishAndExit(ctx, checkRes.OK, "")
		}()
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

	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
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

	if ctx.DiagOnly {
		ctx.App.QueueUpdateDraw(func() {
			tui.ShowDiagnosticsWizard(ctx, "welcome_wizard")
		})
		return
	}

	if ctx.RescueOnly {
		ctx.App.QueueUpdateDraw(func() {
			tui.ShowRescueModal(ctx, "welcome_wizard")
		})
		return
	}

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
