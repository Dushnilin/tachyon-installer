package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	backuppkg "tachyon-installer/internal/backup"
	deploypkg "tachyon-installer/internal/deploy"
	dlpkg "tachyon-installer/internal/downloader"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
)

// RunExpressInstall orchestrates the full installation pipeline:
// SSH Connect -> Sanity Check -> Host-Side Download -> Checksums -> Backup -> Upload & Install -> Verify -> Done.
func RunExpressInstall(ctx *AppContext, opts InstallOptions) {
	ctx.ConsoleView.SetText("[#eab308]⚡ Инициализация экспресс-установки Tachyon...[-]\n\n")

	if ctx.ProgressView != nil {
		ctx.ProgressView.StartAnimation(ctx.App)
	}

	go func() {
		cfg := ctx.Config

		// ==========================================
		// 1. SSH-ПОДКЛЮЧЕНИЕ (1/7)
		// ==========================================
		if ctx.ProgressView != nil {
			ctx.ProgressView.SetStep(0, "1/7: SSH-подключение")
		}
		ctx.SetSubTask("Подключение к роутеру...", 0.2)
		ctx.ConsoleWritef("[#cbd5e1]⚡ Проверка SSH-подключения к роутеру [#38bdf8]%s[-]...[-]\n", cfg.RouterIP)

		client, err := sshpkg.EnsureClient(ctx.SSHClient, ctx.Reconnect)
		if err != nil {
			client, err = sshpkg.Connect(cfg.RouterIP, cfg.SSHPort, cfg.Username, cfg.Password)
			if err != nil {
				ctx.ConsoleWritef(
					"[#ef5350]❌ Ошибка подключения: %v[-]\n\n[#eab308]💡 Совет: Проверьте питание роутера, кабель/Wi-Fi и учетные данные.[-]\n",
					err,
				)
				time.Sleep(3 * time.Second)
				ShowWelcomeWizard(ctx)
				return
			}
		}
		ctx.SSHClient = client
		sshClient := client

		// Save current config
		cfg.Save("manager_config.json")

		ctx.SetSubTask("Аутентификация успешна...", 0.8)
		ctx.ConsoleWrite("[#22c55e]✓ Успешное SSH-подключение к роутеру![-]\n\n")
		ctx.SetSubTask("SSH подключено ✓", 1.0)

		// ==========================================
		// 2. ПРОВЕРКА ПАМЯТИ И СИСТЕМЫ (2/7)
		// ==========================================
		if ctx.ProgressView != nil {
			ctx.ProgressView.SetStep(1, "2/7: Проверка памяти и системы")
		}
		ctx.SetSubTask("Проверка ресурсов роутера...", 0.2)
		ctx.ConsoleWrite("[#cbd5e1]⚡ Оценка дискового пространства и оперативной памяти роутера...[-]\n")

		spaceOut, _ := ctx.ExecSSH(`df -k /overlay 2>/dev/null | awk 'END{print $4}'`)
		spaceKB := parseKB(spaceOut)
		if spaceKB == 0 {
			fallback, _ := ctx.ExecSSH(`df -k / 2>/dev/null | awk 'END{print $4}'`)
			spaceKB = parseKB(fallback)
		}

		ramOut, _ := ctx.ExecSSH(`awk '/MemTotal/ {print $2}' /proc/meminfo`)
		ramKB := parseKB(ramOut)

		spaceMB := float64(spaceKB) / 1024.0
		ramMB := float64(ramKB) / 1024.0

		ctx.ConsoleWritef("[#cbd5e1]  • Общая память (RAM):   [#f1f5f9]%.1f МБ[-] [-]\n", ramMB)
		ctx.ConsoleWritef("[#cbd5e1]  • Свободный Flash диск: [#f1f5f9]%.1f МБ[-] [-]\n", spaceMB)

		if spaceKB > 0 && spaceKB < 8*1024 {
			ctx.ConsoleWritef("[#ef5350]❌ ОШИБКА: Недостаточно свободного места на роутере (%.1f МБ)! Требуется минимум 8 МБ.[-]\n", spaceMB)
			return
		}

		ctx.ConsoleWrite("[#22c55e]✓ Проверка памяти успешно пройдена![-]\n\n")
		ctx.SetSubTask("Память проверена ✓", 1.0)

		// Sync router time to prevent TLS certificate verification errors during apk/opkg or https requests
		ctx.ConsoleWrite("[#cbd5e1]⚡ Синхронизация системного времени роутера с ПК...[-]\n")
		if timeStr, errTime := routerpkg.SyncRouterTime(sshClient); errTime == nil {
			ctx.ConsoleWritef("[#22c55e]✓ Системное время роутера синхронизировано: [#f1f5f9]%s[-] (защита TLS/HTTPS)[-]\n", timeStr)
		} else {
			ctx.ConsoleWritef("[#eab308]⚠️ Не удалось синхронизировать время: %v (продолжаем)[-]\n", errTime)
		}

		// Check router WAN & DNS connectivity
		ctx.ConsoleWrite("[#cbd5e1]⚡ Проверка сетевой связности роутера (WAN/DNS)...[-]\n")
		_, _, connDetails, errConn := routerpkg.CheckRouterConnectivity(sshClient)
		if errConn == nil {
			ctx.ConsoleWritef("[#22c55e]✓ Связность сети роутера: [#f1f5f9]%s[-][-]\n\n", connDetails)
		} else {
			ctx.ConsoleWritef("[#eab308]⚠️ Проверка связности: %v (продолжаем)[-]\n\n", errConn)
		}

		// Detect system details
		isAPK := routerpkg.IsAPKPackage(sshClient)
		detectedArch := routerpkg.DetectArch(sshClient)
		distribArch := routerpkg.DetectRawArch(sshClient)
		if distribArch == "" {
			distribArch = detectedArch
		}

		ctx.ConsoleWritef("[#cbd5e1]⚡ Архитектура роутера: [#38bdf8]%s[-] (пакетная: [#38bdf8]%s[-]), пакетный менеджер: [#38bdf8]%s[-][-]\n\n",
			detectedArch, distribArch, map[bool]string{true: "apk (OpenWrt 25+)", false: "opkg (OpenWrt <= 24)"}[isAPK])

		// ==========================================
		// 3. СКАЧИВАНИЕ ПАКЕТОВ В ОС ЧЕРЕЗ ЗЕРКАЛА (3/7)
		// ==========================================
		if ctx.ProgressView != nil {
			ctx.ProgressView.SetStep(2, "3/7: Скачивание пакетов в ОС")
		}
		ctx.SetSubTask("Инициализация зеркал загрузки...", 0.1)
		ctx.ConsoleWrite("[#cbd5e1]⚡ Настройка зеркал и скачивание пакетов на ваш компьютер...[-]\n")

		mirrorMgr := dlpkg.NewMirrorManager(opts.SelectedMirror)
		if opts.SelectedMirror == "auto" {
			ctx.ConsoleWrite("[#cbd5e1]⚡ Замер задержки и проверка доступности зеркал GitHub:[-]\n")
			fastest := mirrorMgr.TestFastestMirrorDetailed(context.Background(), func(res dlpkg.MirrorPingResult) {
				if res.Err == nil {
					ctx.ConsoleWritef("   [#22c55e]●[-] %-32s [#38bdf8]%d ms[-] ✓\n", res.Mirror, res.Latency.Milliseconds())
				} else {
					ctx.ConsoleWritef("   [#94a3b8]●[-] %-32s [#ef5350]недоступно[-]\n", res.Mirror)
				}
			})
			ctx.ConsoleWritef("\n[#22c55e]✓ Автоматически выбрано быстрейшее зеркало: [#f1f5f9]%s[-][-]\n\n", fastest)
		} else if opts.SelectedMirror == "direct" {
			ctx.ConsoleWrite("[#cbd5e1]⚡ Используется прямое подключение к GitHub (без зеркал)...[-]\n\n")
		} else {
			ctx.ConsoleWritef("[#22c55e]✓ Выбрано зеркало: [#f1f5f9]%s[-][-]\n\n", opts.SelectedMirror)
		}

		cfg.SelectedMirror = opts.SelectedMirror
		cfg.SelectedEngine = opts.SelectedEngine
		cfg.TachyonVersion = opts.TachyonVersion
		cfg.InstallI18n = opts.InstallI18n
		cfg.Save("manager_config.json")

		dlClient := dlpkg.NewClient(mirrorMgr)

		// Create local staging directory on the host
		staging, err := deploypkg.NewStagingArea()
		if err != nil {
			ctx.ConsoleWritef("[#ef5350]❌ Ошибка создания временной папки: %v[-]\n", err)
			return
		}
		defer staging.Cleanup()

		// 3a. Resolve Tachyon Release
		ctx.ConsoleWritef("[#cbd5e1]⚡ Запрос релиза Tachyon (%s)...[-]\n", opts.TachyonVersion)
		tachyonRel, err := dlpkg.FetchTachyonReleaseByTag(context.Background(), dlClient, opts.TachyonVersion)
		if err != nil {
			ctx.ConsoleWritef("[#ef5350]❌ Ошибка получения релиза Tachyon: %v[-]\n", err)
			return
		}
		ctx.ConsoleWritef("[#22c55e]✓ Найден релиз: [#f1f5f9]Tachyon %s[-][-]\n", tachyonRel.TagName)

		tachyonAssets, err := dlpkg.ResolveTachyonAssets(tachyonRel, isAPK, opts.InstallI18n)
		if err != nil {
			ctx.ConsoleWritef("[#ef5350]❌ Ошибка разрешения файлов Tachyon: %v[-]\n", err)
			return
		}

		// Download Tachyon packages locally
		_, err = dlpkg.DownloadTachyonPackages(
			context.Background(),
			dlClient,
			tachyonAssets,
			opts.InstallI18n,
			staging.Dir,
			ctx.ConsoleWrite,
			ctx.SetSubTask,
		)
		if err != nil {
			ctx.ConsoleWritef("[#ef5350]❌ Ошибка скачивания пакетов Tachyon: %v[-]\n", err)
			return
		}

		// 3b. Resolve & Download Routing Engine if requested
		if opts.SelectedEngine != "skip" && opts.SelectedEngine != "" {
			archLabel := detectedArch
			if distribArch != "" && distribArch != detectedArch {
				archLabel = fmt.Sprintf("%s (%s)", detectedArch, distribArch)
			}
			ctx.ConsoleWritef("\n[#cbd5e1]⚡ Разрешение ядра прокси [#38bdf8]%s[-] для архитектуры [#38bdf8]%s[-]...[-]\n", opts.SelectedEngine, archLabel)
			engineAssets, err := dlpkg.ResolveEngineAssets(
				context.Background(),
				dlClient,
				dlpkg.EngineType(opts.SelectedEngine),
				distribArch,
				detectedArch,
				isAPK,
			)
			if err != nil {
				ctx.ConsoleWritef("[#eab308]⚠️ Не удалось автоматически найти пакет ядра: %v[-]\n", err)
				ctx.ConsoleWrite("[#eab308]   Установка продолжится, ядро можно будет установить позже через веб-интерфейс Tachyon.[-]\n")
			} else if len(engineAssets) > 0 {
				ctx.ConsoleWritef("[#22c55e]✓ Найдено пакетов ядра (%s): %d шт.[-]\n", opts.SelectedEngine, len(engineAssets))
				for idx, ea := range engineAssets {
					if ea == nil || ea.URL == "" {
						continue
					}
					ctx.ConsoleWritef("[#38bdf8]  [%d/%d] %s[-]\n", idx+1, len(engineAssets), ea.Filename)
					_, errDl := dlpkg.DownloadEnginePackage(
						context.Background(),
						dlClient,
						ea,
						staging.Dir,
						ctx.ConsoleWrite,
						ctx.SetSubTask,
					)
					if errDl != nil {
						ctx.ConsoleWritef("[#eab308]⚠️ Сбой загрузки пакета %s: %v (продолжаем)[-]\n", ea.Filename, errDl)
					}
				}
			}
		}

		ctx.SetSubTask("Все пакеты успешно скачаны ✓", 1.0)
		ctx.ConsoleWrite("[#22c55e]✓ Все дистрибутивные пакеты успешно скачаны на компьютер![-]\n\n")

		// ==========================================
		// 4. ПРОВЕРКА КОНТРОЛЬНЫХ СУММ (4/7)
		// ==========================================
		if ctx.ProgressView != nil {
			ctx.ProgressView.SetStep(3, "4/7: Проверка целостности (SHA256)")
		}
		ctx.SetSubTask("Проверка целостности файлов...", 0.5)
		if !verifyStagedFiles(ctx, staging.Dir) {
			return
		}
		ctx.SetSubTask("Целостность проверена ✓", 1.0)

		// ==========================================
		// 5. РЕЗЕРВНАЯ КОПИЯ НАСТРОЕК (5/7)
		// ==========================================
		if ctx.ProgressView != nil {
			ctx.ProgressView.SetStep(4, "5/7: Резервная копия настроек")
		}
		ctx.SetSubTask("Создание резервной копии...", 0.3)
		ctx.ConsoleWrite("[#cbd5e1]⚡ Создание локальной резервной копии конфигурации роутера...[-]\n")

		backupMsg, _ := backuppkg.CreateLocalBackup(sshClient, cfg.RouterIP)
		ctx.ConsoleWritef("[#22c55e]✓ %s[-]\n\n", backupMsg)
		ctx.SetSubTask("Резервная копия сохранена ✓", 1.0)

		// ==========================================
		// 6. ЗАГРУЗКА И УСТАНОВКА НА РОУТЕР (6/7)
		// ==========================================
		if ctx.ProgressView != nil {
			ctx.ProgressView.SetStep(5, "6/7: Загрузка и установка на роутер")
		}
		ctx.SetSubTask("Загрузка пакетов на роутер по SSH...", 0.2)

		// Generate install_on_router.sh
		_, err = staging.WriteRunnerScript(opts.SelectedEngine, isAPK, opts.InstallZRAM)
		if err != nil {
			ctx.ConsoleWritef("[#ef5350]❌ Ошибка генерации установочного скрипта: %v[-]\n", err)
			return
		}

		consoleWriter := newTviewWriter(ctx.ConsoleView, ctx.App)
		defer consoleWriter.Close()
		err = deploypkg.UploadStagingAndExecute(sshClient, staging.Dir, consoleWriter, ctx.ConsoleWrite)
		consoleWriter.Flush()
		ctx.FirewallNotReady = consoleWriter.hinted
		ctx.LogPath = consoleWriter.path
		if err != nil {
			ctx.ConsoleWritef("\n[#ef5350]❌ Ошибка при установке пакетов на роутер: %v[-]\n", err)
			return
		}

		ctx.SetSubTask("Пакеты установлены на роутер ✓", 1.0)
		ctx.ConsoleWritef("\n[#22c55e]✓ Пакеты Tachyon v%s успешно установлены на роутер![-]\n\n", tachyonAssets.Version)

		// ==========================================
		// 7. ПРОВЕРКА И ЗАПУСК СЛУЖБ (7/7)
		// ==========================================
		if ctx.ProgressView != nil {
			ctx.ProgressView.SetStep(6, "7/7: Проверка и запуск Tachyon")
		}
		ctx.SetSubTask("Запуск диагностики и проверка сети...", 0.2)
		ctx.ConsoleWrite("[#cbd5e1]⚡ Запуск послеустановочной диагностики и проверка работы сети...[-]\n")

		time.Sleep(3 * time.Second)

		reconnectFn := func() (*gossh.Client, error) {
			c, err := sshpkg.Connect(cfg.RouterIP, cfg.SSHPort, cfg.Username, cfg.Password)
			if err == nil {
				ctx.SSHClient = c
				sshClient = c
			}
			return c, err
		}
		execFn := func(_ *gossh.Client, cmd string) (string, error) {
			return sshpkg.Exec(sshClient, cmd, reconnectFn)
		}

		subStatus := routerpkg.CheckSubscriptions(sshClient, execFn)

		if subStatus.HasEmptySubscription {
			ctx.ConsoleWrite("[#eab308]⚠️  ВНИМАНИЕ: Ссылка на подписку еще не добавлена в Tachyon.[-]\n")
			ctx.App.QueueUpdateDraw(func() {
				ShowSubscriptionSetupForm(ctx)
			})
		} else {
			result := routerpkg.VerifyAndFallback(sshClient, execFn, ctx.SetSubTask)
			if result.OK {
				ctx.ConsoleWrite("[#22c55e]🎉 ПОЛНЫЙ УСПЕХ: Служба Tachyon запущена и проверена![-]\n")
				postInstallDiagnostics(ctx, func(cmd string) (string, error) { return execFn(sshClient, cmd) })
				FinishAndExit(ctx, true, tachyonAssets.Version)
			} else {
				ctx.ConsoleWritef("[#eab308]⚠️  Проверка завершилась с предупреждением: %s[-]\n", result.Reason)
				postInstallDiagnostics(ctx, func(cmd string) (string, error) { return execFn(sshClient, cmd) })
				FinishAndExit(ctx, false, tachyonAssets.Version)
			}
		}
	}()
}

// RunSaveSubscription saves a subscription link and restarts Tachyon.
func RunSaveSubscription(ctx *AppContext, subURL string) {
	cfg := ctx.Config
	ctx.ConsoleWritef("\n[#cbd5e1]⚡ Сохранение подписки в Tachyon: [#38bdf8]%s[-]...[-]\n", subURL)

	reconnectFn := func() (*gossh.Client, error) {
		c, err := sshpkg.Connect(cfg.RouterIP, cfg.SSHPort, cfg.Username, cfg.Password)
		if err == nil {
			ctx.SSHClient = c
		}
		return c, err
	}
	execFn := func(_ *gossh.Client, cmd string) (string, error) {
		return sshpkg.Exec(ctx.SSHClient, cmd, reconnectFn)
	}

	err := routerpkg.SaveSubscription(ctx.SSHClient, execFn, subURL)
	if err != nil {
		ctx.ConsoleWritef("[#ef5350]❌ Ошибка сохранения подписки: %v[-]\n", err)
		FinishAndExit(ctx, false, "")
		return
	}

	ctx.ConsoleWrite("[#22c55e]✓ Подписка сохранена. Ожидание запуска службы...[-]\n")
	time.Sleep(5 * time.Second)

	result := routerpkg.VerifyAndFallback(ctx.SSHClient, execFn, ctx.SetSubTask)
	if result.OK {
		ctx.ConsoleWrite("[#22c55e]🎉 ПОДПИСКА АКТИВНА: Обход блокировок запущен и проверен![-]\n")
		FinishAndExit(ctx, true, "")
	} else {
		ctx.ConsoleWritef("[#eab308]⚠️ Подписка сохранена, статус проверки: %s[-]\n", result.Reason)
		FinishAndExit(ctx, false, "")
	}
}

func parseKB(s string) int64 {
	var kb int64
	for _, c := range strings.TrimSpace(s) {
		if c >= '0' && c <= '9' {
			kb = kb*10 + int64(c-'0')
		} else if kb > 0 {
			break
		}
	}
	return kb
}
