// Package cli handles non-interactive, headless command-line execution.
package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	backuppkg "tachyon-installer/internal/backup"
	appconfig "tachyon-installer/internal/config"
	deploypkg "tachyon-installer/internal/deploy"
	"tachyon-installer/internal/diag"
	dlpkg "tachyon-installer/internal/downloader"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
	"tachyon-installer/internal/updater"
)

// Options holds command line parameters for headless execution.
type Options struct {
	RouterIP       string
	SSHPort        int
	Username       string
	Password       string
	KeyPath        string
	Engine         string
	Mirror         string
	Version        string
	InstallI18n    bool
	InstallZRAM    bool
	Subscription   string
	RunDiag        bool
	ExportDiagPath string
	RunUninstall   bool
	RunRestore     string
	ListBackups    bool
	CheckUpdate    bool
	SwitchEngine   string
	RunRescue      bool
	FixConflicts   bool
	FullSnapshot   bool
	OfflineBundle  string
	RunMonitor     bool
	TestSub        string
	SelfUpdate     bool
	TuneNetwork    bool
	Yes            bool
	AppVersion     string
}

// ParseFlags parses command line arguments and populates Options.
func ParseFlags(args []string, defaultCfg *appconfig.Config, appVersion string) (*Options, error) {
	fs := flag.NewFlagSet("tachyon-installer", flag.ContinueOnError)

	opts := &Options{
		AppVersion: appVersion,
	}

	ipDef := defaultCfg.RouterIP
	if ipDef == "" {
		ipDef = "192.168.1.1"
	}

	fs.StringVar(&opts.RouterIP, "ip", ipDef, "IP-адрес роутера OpenWrt")
	fs.IntVar(&opts.SSHPort, "port", defaultCfg.SSHPort, "SSH порт роутера")
	fs.StringVar(&opts.Username, "user", defaultCfg.Username, "SSH имя пользователя")
	fs.StringVar(&opts.Password, "pass", "", "SSH пароль")
	fs.StringVar(&opts.KeyPath, "key", defaultCfg.KeyPath, "Путь к SSH приватному ключу")
	fs.StringVar(&opts.Engine, "engine", defaultCfg.SelectedEngine, "Ядро прокси: sing-box-extended, sing-box-extended-compressed, sing-box-tiny, sing-box-lx, steer, steer-extended, skip")
	fs.StringVar(&opts.SwitchEngine, "switch-engine", "", "Горячая смена ядра прокси на роутере без переустановки LuCI")
	fs.StringVar(&opts.Mirror, "mirror", defaultCfg.SelectedMirror, "Зеркало загрузки GitHub: auto, direct, https://gh-proxy.com/ и др.")
	fs.StringVar(&opts.Version, "version", defaultCfg.TachyonVersion, "Версия Tachyon: latest или тег релиза (например 1.4.9)")
	fs.BoolVar(&opts.InstallI18n, "i18n", defaultCfg.InstallI18n, "Установить русскую локализацию LuCI")
	fs.BoolVar(&opts.InstallZRAM, "zram", defaultCfg.InstallZRAM, "Включить zRAM-swap (сжатый SWAP в ОЗУ)")
	fs.StringVar(&opts.Subscription, "sub", "", "Ссылка на подписку (HTTPS, vless:// или base64)")
	fs.BoolVar(&opts.RunDiag, "diag", false, "Запустить расширенную диагностику роутера и вывести отчет")
	fs.StringVar(&opts.ExportDiagPath, "export-diag", "", "Экспортировать отчет диагностики в файл")
	fs.BoolVar(&opts.RunUninstall, "uninstall", false, "Полное чистое удаление Tachyon с роутера")
	fs.StringVar(&opts.RunRestore, "restore", "", "Восстановить настройки из файла бэкапа или 'latest'")
	fs.BoolVar(&opts.ListBackups, "list-backups", false, "Показать список доступных локальных бэкапов")
	fs.BoolVar(&opts.CheckUpdate, "check-update", false, "Проверить наличие обновлений программы на GitHub")
	fs.BoolVar(&opts.SelfUpdate, "self-update", false, "Автоматическое обновление программы установщика до последней версии с GitHub")
	fs.BoolVar(&opts.TuneNetwork, "tune-network", false, "Оптимизировать сетевой стек роутера (TCP BBR, fq_codel, rmem/wmem, conntrack, offloading)")
	fs.BoolVar(&opts.RunRescue, "rescue", false, "Аварийный сброс правил перехвата и восстановление прямого интернета")
	fs.BoolVar(&opts.FixConflicts, "fix-conflicts", false, "Отключить конфликтующие прокси-пакеты (passwall, openclash, zapret и др.)")
	fs.BoolVar(&opts.FullSnapshot, "snapshot", false, "Создать полный бэкап настроек сети, dhcp, firewall и Tachyon")
	fs.StringVar(&opts.OfflineBundle, "offline-bundle", "", "Скачать полный оффлайн-бандл пакетов в указанную папку (например 'offline/')")
	fs.BoolVar(&opts.RunMonitor, "monitor", false, "Мониторинг нагрузки CPU, RAM, трафика и ячеек прокси в реальном времени")
	fs.StringVar(&opts.TestSub, "test-sub", "", "Проверить и протестировать пинг до нод в подписке (HTTPS, vless://, base64)")
	fs.BoolVar(&opts.Yes, "yes", false, "Автоматическое подтверждение (без интерактивного меню)")
	fs.BoolVar(&opts.Yes, "y", false, "Короткий алиас для -yes")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return opts, nil
}

// ShouldRunHeadless checks if flags request non-interactive execution.
func (o *Options) ShouldRunHeadless() bool {
	return o.RunDiag || o.RunUninstall || o.RunRestore != "" || o.ListBackups || o.CheckUpdate || o.SelfUpdate || o.TuneNetwork || o.SwitchEngine != "" || o.RunRescue || o.FixConflicts || o.FullSnapshot || o.OfflineBundle != "" || o.RunMonitor || o.TestSub != "" || o.Yes
}

// Run executes non-interactive CLI operations.
func Run(opts *Options) int {
	fmt.Printf("==> Tachyon Express Installer %s (Headless Mode)\n\n", opts.AppVersion)

	// 1. Check Update
	if opts.CheckUpdate {
		fmt.Println("⚡ Проверка наличия обновлений на GitHub...")
		hasUpdate, latestVer, relURL, err := updater.CheckForUpdate(context.Background(), opts.AppVersion)
		if err != nil {
			fmt.Printf("⚠️ Ошибка проверки обновлений: %v\n", err)
			return 1
		}
		if hasUpdate {
			fmt.Printf("💡 Доступна новая версия: %s (текущая: %s)\nСсылка для загрузки: %s\n", latestVer, opts.AppVersion, relURL)
		} else {
			fmt.Printf("✓ У вас установлена актуальная версия Tachyon Installer (%s)\n", opts.AppVersion)
		}
		return 0
	}

	// In-Place Self-Update
	if opts.SelfUpdate {
		logFn := func(msg string) { fmt.Print(msg) }
		updated, newVer, err := updater.SelfUpdate(context.Background(), opts.AppVersion, logFn)
		if err != nil {
			fmt.Printf("❌ Ошибка самообновления: %v\n", err)
			return 1
		}
		if updated {
			fmt.Printf("✓ Программа успешно обновлена до %s. Пожалуйста, перезапустите установщик.\n", newVer)
		}
		return 0
	}

	// 2. List Backups
	if opts.ListBackups {
		fmt.Println("⚡ Поиск сохраненных локальных резервных копий (backups/)...")
		backups, err := backuppkg.ListBackups()
		if err != nil {
			fmt.Printf("❌ Ошибка чтения директории бэкапов: %v\n", err)
			return 1
		}
		if len(backups) == 0 {
			fmt.Println("Резервных копий пока нет.")
			return 0
		}
		fmt.Printf("Найдено резервных копий: %d шт.\n", len(backups))
		for i, b := range backups {
			fi, _ := os.Stat(b)
			sizeKB := float64(0)
			if fi != nil {
				sizeKB = float64(fi.Size()) / 1024
			}
			fmt.Printf("  [%d] %-48s (%.1f КБ)\n", i+1, filepath.Base(b), sizeKB)
		}
		return 0
	}

	// 3. Offline Bundle Downloader
	if opts.OfflineBundle != "" {
		fmt.Printf("⚡ Создание оффлайн-пакета в директории: %s...\n", opts.OfflineBundle)
		mirrorMgr := dlpkg.NewMirrorManager(opts.Mirror)
		dlClient := dlpkg.NewClient(mirrorMgr)
		manifest, err := dlpkg.CreateOfflineBundle(
			context.Background(),
			dlClient,
			opts.OfflineBundle,
			opts.Version,
			nil,
			func(msg string) { fmt.Print(msg) },
		)
		if err != nil {
			fmt.Printf("❌ Ошибка создания оффлайн-пакета: %v\n", err)
			return 1
		}
		fmt.Printf("\n✓ Оффлайн-бандл успешно сформирован! Всего пакетов: %d\n", len(manifest.Packages))
		return 0
	}

	// 4. Test Subscription Nodes (Ping & Benchmark)
	if opts.TestSub != "" {
		fmt.Println("⚡ Анализ и тестирование задержки узлов подписки...")
		nodes, err := routerpkg.ResolveNodesFromInput(context.Background(), opts.TestSub)
		if err != nil {
			fmt.Printf("❌ Ошибка разбора подписки: %v\n", err)
			return 1
		}
		if len(nodes) == 0 {
			fmt.Println("❌ В подписке не найдено поддерживаемых узлов")
			return 1
		}
		fmt.Printf("✓ Найдено узлов: %d. Запуск TCP handshake пинга...\n\n", len(nodes))
		results := routerpkg.BenchmarkNodes(context.Background(), nodes, 3*time.Second)

		fmt.Printf(" %-4s | %-28s | %-8s | %-24s | %s\n", "#", "Имя узла", "Протокол", "Хост:Порт", "Пинг / Статус")
		fmt.Println(strings.Repeat("-", 82))
		for i, r := range results {
			status := fmt.Sprintf("%d ms", r.LatencyMs)
			if !r.Success {
				status = "TIMEOUT / FAIL"
			}
			fmt.Printf(" [%2d] | %-28.28s | %-8s | %-24.24s | %s\n", i+1, r.Name, r.Protocol, fmt.Sprintf("%s:%d", r.Host, r.Port), status)
		}
		return 0
	}

	// Connect to router over SSH
	fmt.Printf("⚡ Подключение к роутеру %s:%d (%s)...\n", opts.RouterIP, opts.SSHPort, opts.Username)
	client, err := sshpkg.Connect(opts.RouterIP, opts.SSHPort, opts.Username, opts.Password)
	if err != nil {
		fmt.Printf("❌ Ошибка SSH-подключения: %v\n", err)
		return 1
	}
	defer client.Close()
	fmt.Println("✓ SSH подключение установлено.")

	isAPK := routerpkg.IsAPKPackage(client)

	// 5. Emergency Rescue
	if opts.RunRescue {
		fmt.Println("⚡ АВАРИЙНЫЙ СБРОС И ВОССТАНОВЛЕНИЕ СЕТИ РОУТЕРА...")
		execCmd := func(_ *gossh.Client, cmd string) (string, error) {
			sess, err := client.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		}
		rep := routerpkg.EmergencyRescue(client, execCmd)
		for _, a := range rep.ActionsTaken {
			fmt.Printf("  • %s\n", a)
		}
		if rep.Success {
			fmt.Println("\n✓ Правила перехвата и службы сброшены.")
			fmt.Printf("  • Пинг до шлюза провайдера: %v\n", rep.GatewayPing)
			fmt.Printf("  • Пинг в интернет (8.8.8.8): %v\n", rep.InternetPing)
			if rep.InternetPing {
				fmt.Println("✓ Доступ в Интернет успешно восстановлен напрямую!")
			} else {
				fmt.Println("⚠️ Интернет не отвечает по прямому пингу (проверьте WAN/кабель).")
			}
			return 0
		}
		fmt.Printf("❌ Сбой аварийного сброса: %s\n", rep.Detail)
		return 1
	}

	// 6. Conflict Auto-Fix
	if opts.FixConflicts {
		fmt.Println("⚡ Поиск и отключение конфликтующих прокси-пакетов...")
		execCmd := func(_ *gossh.Client, cmd string) (string, error) {
			sess, err := client.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		}
		rep := routerpkg.DisableConflicts(client, execCmd, nil)
		if !rep.Success {
			fmt.Printf("❌ Ошибка: %s\n", rep.Details)
			return 1
		}
		if rep.DisabledCount > 0 {
			fmt.Printf("✓ %s\n", rep.Details)
		} else {
			fmt.Println("✓ Конфликтующих служб (passwall, openclash, zapret и др.) не обнаружено.")
		}
		return 0
	}

	// 7. Full System Snapshot
	if opts.FullSnapshot {
		fmt.Println("⚡ Создание полного снимка настроек роутера (сеть, firewall, dhcp, tachyon)...")
		msg, err := backuppkg.CreateFullSnapshot(client, opts.RouterIP)
		if err != nil {
			fmt.Printf("❌ Ошибка создания полного бэкапа: %v\n", err)
			return 1
		}
		fmt.Printf("✓ %s\n", msg)
		return 0
	}

	// 8. Live Real-Time Monitor
	if opts.RunMonitor {
		fmt.Printf("⚡ Запуск мониторинга роутера %s (Ctrl+C для выхода)...\n\n", opts.RouterIP)
		execCmd := func(_ *gossh.Client, cmd string) (string, error) {
			sess, err := client.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		}

		for {
			stats, err := routerpkg.CollectLiveStats(client, execCmd)
			if err != nil {
				fmt.Printf("⚠️ Ошибка сбора метрик: %v\n", err)
			} else {
				ramPercent := float64(0)
				if stats.RAMTotalMB > 0 {
					ramPercent = (stats.RAMUsedMB / stats.RAMTotalMB) * 100.0
				}
				engInfo := "не запущен"
				if stats.EngineName != "" {
					engInfo = fmt.Sprintf("%s (PID: %s, VSZ: %s)", stats.EngineName, stats.EnginePID, stats.EngineVSZ)
				}
				fmt.Printf("\r[Load: %.2f %.2f %.2f] [RAM: %.1f/%.1f MB (%.0f%%)] [Uptime: %s] [Ядро: %s] [Соединений: %d]   ",
					stats.Load1, stats.Load5, stats.Load15,
					stats.RAMUsedMB, stats.RAMTotalMB, ramPercent,
					stats.UptimeString(),
					engInfo,
					stats.Connections,
				)
			}
			time.Sleep(2 * time.Second)
		}
	}

	// 9. Network Optimization & Sysctl Tuning
	if opts.TuneNetwork {
		fmt.Println("⚡ ОПТИМИЗАЦИЯ СЕТЕВОГО СТЕКА И SYSCTL РОУТЕРА...")
		execCmd := func(_ *gossh.Client, cmd string) (string, error) {
			sess, err := client.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		}
		rep := routerpkg.TuneNetwork(client, execCmd)
		if !rep.Success {
			fmt.Printf("❌ Сбой оптимизации: %s\n", rep.Details)
			return 1
		}
		for _, r := range rep.AppliedRules {
			fmt.Printf("  • %s\n", r)
		}
		fmt.Printf("\n✓ Конфигурация сохранена в %s\n", rep.PersistPath)
		fmt.Printf("✓ %s\n", rep.Details)
		return 0
	}

	// 10. Uninstall
	if opts.RunUninstall {
		fmt.Println("⚡ Запуск чистого удаления Tachyon...")
		out, err := deploypkg.UninstallTachyon(client, isAPK)
		fmt.Println(out)
		if err != nil {
			fmt.Printf("❌ Сбой удаления: %v\n", err)
			return 1
		}
		fmt.Println("✓ Tachyon успешно удален с роутера.")
		return 0
	}

	// 4. Restore
	if opts.RunRestore != "" {
		backupPath := opts.RunRestore
		if strings.ToLower(backupPath) == "latest" {
			backups, err := backuppkg.ListBackups()
			if err != nil || len(backups) == 0 {
				fmt.Println("❌ Не найдены локальные файлы резервных копий в папке backups/")
				return 1
			}
			backupPath = backups[0]
		}
		fmt.Printf("⚡ Восстановление конфигурации из архива: %s...\n", backupPath)
		msg, err := backuppkg.RestoreBackup(client, backupPath)
		if err != nil {
			fmt.Printf("❌ Ошибка восстановления: %v\n", err)
			return 1
		}
		fmt.Printf("✓ %s\n", msg)
		return 0
	}

	// 5. Diagnostics Only
	if opts.RunDiag {
		fmt.Println("⚡ Запуск диагностики роутера...")
		execCmd := func(cmd string) (string, error) {
			sess, err := client.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		}
		results := diag.RunRouter(execCmd, func(r diag.Result) {
			fmt.Printf("  %-8s %s: %s\n", r.Status.Icon(), r.Title, r.Detail)
		})
		formatted := diag.Format(fmt.Sprintf("Отчет диагностики роутера %s", opts.RouterIP), results)
		fmt.Println("\n" + formatted)

		if opts.ExportDiagPath != "" {
			p, err := diag.ExportToFile("Диагностика роутера "+opts.RouterIP, results, opts.ExportDiagPath)
			if err != nil {
				fmt.Printf("⚠️ Ошибка экспорта отчета: %v\n", err)
			} else {
				fmt.Printf("✓ Отчет успешно сохранен в файл: %s\n", p)
			}
		}
		return 0
	}

	// 6. Switch Engine (Hot-Swap)
	if opts.SwitchEngine != "" {
		fmt.Printf("⚡ Горячая смена ядра на [%s]...\n", opts.SwitchEngine)
		detectedArch := routerpkg.DetectArch(client)
		distribArch := routerpkg.DetectRawArch(client)
		if distribArch == "" {
			distribArch = detectedArch
		}
		logFn := func(msg string) { fmt.Print(msg) }
		err := deploypkg.HotSwapEngine(
			context.Background(),
			client,
			opts.SwitchEngine,
			opts.Mirror,
			isAPK,
			distribArch,
			detectedArch,
			logFn,
		)
		if err != nil {
			fmt.Printf("❌ Сбой горячей смены ядра: %v\n", err)
			return 1
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

		fmt.Println("\n⚡ Проверка сквозного обхода через роутер...")
		bypassRes := routerpkg.TestBypass(client, execFn)
		if bypassRes.Success {
			fmt.Printf("  ✓ %s\n", bypassRes.Details)
		} else {
			fmt.Printf("  ⚠️ %s\n", bypassRes.Details)
		}

		luciURL := fmt.Sprintf("http://%s/cgi-bin/luci/admin/services/tachyon", opts.RouterIP)
		fmt.Printf("\n✓ Горячая смена ядра завершена!\nВеб-интерфейс: %s\n", luciURL)
		return 0
	}

	// 7. Automated Express Installation
	if opts.Yes {
		return runHeadlessInstall(client, isAPK, opts)
	}

	return 0
}

func runHeadlessInstall(client *gossh.Client, isAPK bool, opts *Options) int {
	fmt.Println("\n==========================================")
	fmt.Println("🚀 ЗАПУСК АВТОМАТИЧЕСКОЙ ЭКСПРЕСС-УСТАНОВКИ")
	fmt.Println("==========================================")

	// Step 1: Clock synchronization
	fmt.Println("⚡ [1/7] Синхронизация системного времени роутера с ПК (защита TLS)...")
	if dateStr, err := routerpkg.SyncRouterTime(client); err == nil {
		fmt.Printf("✓ Время синхронизировано: %s\n", dateStr)
	} else {
		fmt.Printf("⚠️ Не удалось синхронизировать время: %v (продолжаем)\n", err)
	}

	// Step 2: System and Connectivity Checks
	fmt.Println("⚡ [2/7] Оценка ресурсов и сетевой связности роутера...")
	_, _, connDetails, _ := routerpkg.CheckRouterConnectivity(client)
	fmt.Printf("  • Сеть роутера: %s\n", connDetails)

	detectedArch := routerpkg.DetectArch(client)
	distribArch := routerpkg.DetectRawArch(client)
	if distribArch == "" {
		distribArch = detectedArch
	}
	fmt.Printf("  • Архитектура:  %s (пакетная: %s)\n", detectedArch, distribArch)
	fmt.Printf("  • Пакетный менеджер: %s\n", map[bool]string{true: "apk", false: "opkg"}[isAPK])

	// Auto zRAM recommendation if not explicitly passed
	installZRAM := opts.InstallZRAM

	// Step 3: Mirrors & Downloads
	fmt.Printf("⚡ [3/7] Подготовка зеркал загрузки (%s)...\n", opts.Mirror)
	mirrorMgr := dlpkg.NewMirrorManager(opts.Mirror)
	if opts.Mirror == "auto" {
		fastest := mirrorMgr.TestFastestMirrorDetailed(context.Background(), func(res dlpkg.MirrorPingResult) {
			if res.Err == nil {
				fmt.Printf("    %-30s %d ms ✓\n", res.Mirror, res.Latency.Milliseconds())
			}
		})
		fmt.Printf("✓ Выбрано быстрейшее зеркало: %s\n", fastest)
	}
	dlClient := dlpkg.NewClient(mirrorMgr)

	staging, err := deploypkg.NewStagingArea()
	if err != nil {
		fmt.Printf("❌ Ошибка создания папки staging: %v\n", err)
		return 1
	}
	defer staging.Cleanup()

	fmt.Printf("⚡ Получение информации о релизе Tachyon (%s)...\n", opts.Version)
	rel, err := dlpkg.FetchTachyonReleaseByTag(context.Background(), dlClient, opts.Version)
	if err != nil {
		fmt.Printf("❌ Ошибка запроса релиза Tachyon: %v\n", err)
		return 1
	}
	fmt.Printf("✓ Релиз: %s\n", rel.TagName)

	assets, err := dlpkg.ResolveTachyonAssets(rel, isAPK, opts.InstallI18n)
	if err != nil {
		fmt.Printf("❌ Ошибка разрешения файлов Tachyon: %v\n", err)
		return 1
	}

	fmt.Println("⚡ Скачивание пакетов Tachyon на компьютер...")
	logFn := func(msg string) { fmt.Print(msg) }
	progFn := func(label string, frac float64) {}
	_, err = dlpkg.DownloadTachyonPackages(context.Background(), dlClient, assets, opts.InstallI18n, staging.Dir, logFn, progFn)
	if err != nil {
		fmt.Printf("❌ Сбой скачивания Tachyon: %v\n", err)
		return 1
	}

	if opts.Engine != "skip" && opts.Engine != "" {
		fmt.Printf("⚡ Поиск и скачивание пакетов ядра (%s)...\n", opts.Engine)
		engAssets, err := dlpkg.ResolveEngineAssets(context.Background(), dlClient, dlpkg.EngineType(opts.Engine), distribArch, detectedArch, isAPK)
		if err == nil && len(engAssets) > 0 {
			for _, ea := range engAssets {
				if ea == nil || ea.URL == "" {
					continue
				}
				fmt.Printf("  -> Загрузка %s...\n", ea.Filename)
				_, _ = dlpkg.DownloadEnginePackage(context.Background(), dlClient, ea, staging.Dir, logFn, progFn)
			}
		}
	}

	// Step 4: Checksum integrity
	fmt.Println("\n⚡ [4/7] Проверка целостности SHA256...")
	if !verifyFiles(staging.Dir) {
		fmt.Println("❌ Ошибка целостности скачанных пакетов")
		return 1
	}
	fmt.Println("✓ Все файлы проверены и целостны.")

	// Step 5: Backup
	fmt.Println("⚡ [5/7] Создание локальной резервной копии настроек...")
	msg, _ := backuppkg.CreateLocalBackup(client, opts.RouterIP)
	fmt.Printf("✓ %s\n", msg)

	// Step 6: Deploy & Execute
	fmt.Println("⚡ [6/7] Загрузка и выполнение установки на роутере...")
	_, err = staging.WriteRunnerScript(opts.Engine, isAPK, installZRAM)
	if err != nil {
		fmt.Printf("❌ Ошибка генерации runner скрипта: %v\n", err)
		return 1
	}

	err = deploypkg.UploadStagingAndExecute(client, staging.Dir, os.Stdout, logFn)
	if err != nil {
		fmt.Printf("❌ Сбой установки на роутере: %v\n", err)
		return 1
	}
	fmt.Println("\n✓ Пакеты успешно развернуты на роутере!")

	// Step 7: Subscription & Service check
	fmt.Println("⚡ [7/7] Проверка и настройка службы...")
	execCmd := func(c *gossh.Client, cmd string) (string, error) {
		sess, err := c.NewSession()
		if err != nil {
			return "", err
		}
		defer sess.Close()
		out, err := sess.CombinedOutput(cmd)
		return string(out), err
	}

	if opts.Subscription != "" {
		fmt.Printf("⚡ Сохранение подписки: %s...\n", opts.Subscription)
		if err := routerpkg.SaveSubscription(client, execCmd, opts.Subscription); err != nil {
			fmt.Printf("⚠️ Ошибка сохранения подписки: %v\n", err)
		} else {
			fmt.Println("✓ Подписка сохранена.")
		}
	}

	time.Sleep(3 * time.Second)
	checkRes := routerpkg.VerifyAndFallback(client, execCmd, progFn)

	fmt.Println("\n⚡ Проверка сквозного обхода блокировок через роутер...")
	bypassRes := routerpkg.TestBypass(client, execCmd)
	if bypassRes.Success {
		fmt.Printf("  ✓ %s\n", bypassRes.Details)
	} else {
		fmt.Printf("  ⚠️ %s\n", bypassRes.Details)
	}

	luciURL := fmt.Sprintf("http://%s/cgi-bin/luci/admin/services/tachyon", opts.RouterIP)
	if checkRes.OK {
		fmt.Println("\n🎉 УСТАНОВКА УСПЕШНО ЗАВЕРШЕНА!")
		fmt.Printf("Панель управления доступна в веб-интерфейсе LuCI: %s\n", luciURL)
		return 0
	}

	fmt.Printf("\n⚠️ Установка завершена, но служба сообщила: %s\nВеб-интерфейс: %s\n", checkRes.Reason, luciURL)
	return 0
}

func verifyFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "sha256sums.txt" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if err := dlpkg.SanityCheckPackage(p); err != nil {
			fmt.Printf("  ✗ %s: %v\n", e.Name(), err)
			return false
		}
		sum, size, err := fileSHA256Sum(p)
		if err != nil {
			fmt.Printf("  ✗ %s: %v\n", e.Name(), err)
			return false
		}
		fmt.Printf("  ✓ %-42s (%.1f МБ, sha256:%s…)\n", e.Name(), float64(size)/1024/1024, sum[:12])
	}
	return true
}

func fileSHA256Sum(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}
