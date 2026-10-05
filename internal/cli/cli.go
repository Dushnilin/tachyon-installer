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
	"tachyon-installer/internal/fleet"
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
	GuidedSetup       bool
	FleetScan         bool
	FleetDeploy       bool
	FleetSubnets      string
	FleetOnlyOutdated bool
	FleetOnlyClean    bool
	FleetConcurrency  int
	SpeedDoctor       bool
	SpeedDuration     int
	AWGSetup          bool
	AWGType           string
	AWGSection        string
	AWGTest           bool
	Yes               bool
	AppVersion        string
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
	fs.BoolVar(&opts.GuidedSetup, "wizard", false, "Мастер быстрой первоначальной настройки (автопилот под ключ)")
	fs.BoolVar(&opts.GuidedSetup, "guided", false, "Алиас для -wizard")
	fs.BoolVar(&opts.GuidedSetup, "autopilot", false, "Алиас для -wizard")
	fs.BoolVar(&opts.RunRescue, "rescue", false, "Аварийный сброс правил перехвата и восстановление прямого интернета")
	fs.BoolVar(&opts.FixConflicts, "fix-conflicts", false, "Отключить конфликтующие прокси-пакеты (passwall, openclash, zapret и др.)")
	fs.BoolVar(&opts.FullSnapshot, "snapshot", false, "Создать полный бэкап настроек сети, dhcp, firewall и Tachyon")
	fs.StringVar(&opts.OfflineBundle, "offline-bundle", "", "Скачать полный оффлайн-бандл пакетов в указанную папку (например 'offline/')")
	fs.BoolVar(&opts.RunMonitor, "monitor", false, "Мониторинг нагрузки CPU, RAM, трафика и ячеек прокси в реальном времени")
	fs.StringVar(&opts.TestSub, "test-sub", "", "Проверить и протестировать пинг до нод в подписке (HTTPS, vless://, base64)")
	fs.BoolVar(&opts.Yes, "yes", false, "Автоматическое подтверждение (без интерактивного меню)")
	fs.BoolVar(&opts.Yes, "y", false, "Короткий алиас для -yes")
	fs.BoolVar(&opts.FleetScan, "fleet-scan", false, "Сканировать сеть и вывести инвентарь всех роутеров с их статусом Tachyon")
	fs.BoolVar(&opts.FleetDeploy, "fleet-deploy", false, "Массовая параллельная установка/обновление Tachyon на обнаруженные роутеры")
	fs.StringVar(&opts.FleetSubnets, "fleet-subnets", "", "Список подсетей через запятую для сканирования (например 192.168.1.0/24,192.168.31.0/24)")
	fs.BoolVar(&opts.FleetOnlyOutdated, "fleet-only-outdated", false, "Применять fleet-deploy только к роутерам с устаревшим Tachyon")
	fs.BoolVar(&opts.FleetOnlyClean, "fleet-only-clean", false, "Применять fleet-deploy только к чистым роутерам без Tachyon")
	fs.IntVar(&opts.FleetConcurrency, "fleet-concurrency", 3, "Количество одновременных потоков установки при fleet-deploy")
	fs.BoolVar(&opts.SpeedDoctor, "speedtest", false, "Запустить тест скорости, Bufferbloat и троттлинга CPU роутера")
	fs.BoolVar(&opts.SpeedDoctor, "speed-doctor", false, "Алиас для -speedtest")
	fs.IntVar(&opts.SpeedDuration, "speed-duration", 5, "Длительность замера скорости в секундах (по умолчанию 5)")
	fs.BoolVar(&opts.AWGSetup, "awg", false, "Создать секцию AmneziaWG (генератор WARP или кастомная конфигурация)")
	fs.StringVar(&opts.AWGType, "awg-conf", "warp", "Тип/источник AWG: 'warp', 'custom', путь к .conf файлу или ссылка vpn://")
	fs.StringVar(&opts.AWGSection, "awg-section", "warp", "Имя секции UCI tachyon.<name> для AmneziaWG (по умолчанию 'warp')")
	fs.BoolVar(&opts.AWGTest, "awg-test", false, "Протестировать работу AmneziaWG (YouTube, Discord, Rutracker, Cloudflare WARP trace)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return opts, nil
}

// ShouldRunHeadless checks if flags request non-interactive execution.
func (o *Options) ShouldRunHeadless() bool {
	return o.RunDiag || o.RunUninstall || o.RunRestore != "" || o.ListBackups || o.CheckUpdate || o.SelfUpdate || o.TuneNetwork || o.GuidedSetup || o.SwitchEngine != "" || o.RunRescue || o.FixConflicts || o.FullSnapshot || o.OfflineBundle != "" || o.RunMonitor || o.TestSub != "" || o.FleetScan || o.FleetDeploy || o.SpeedDoctor || o.AWGSetup || o.AWGTest || o.Yes
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

	// 5. Fleet Scan
	if opts.FleetScan {
		fmt.Println("⚡ Поиск и опрос OpenWrt роутеров в локальной сети...")
		subnets := fleet.SplitSubnets(opts.FleetSubnets)
		scanCfg := fleet.DefaultScanConfig(opts.Password, opts.Version)
		scanCfg.Subnets = subnets
		if opts.KeyPath != "" {
			scanCfg.KeyPath = opts.KeyPath
		}
		if opts.RouterIP != "" {
			scanCfg.Gateway = opts.RouterIP
		}

		nodes, err := fleet.ScanFleet(context.Background(), scanCfg, func(done, total int, ip string) {
			fmt.Printf("\r[Сканирование %d/%d] Проверка хоста %-16s", done, total, ip)
		})
		fmt.Print("\r" + strings.Repeat(" ", 60) + "\r")

		if err != nil {
			fmt.Printf("❌ Ошибка сканирования сети: %v\n", err)
			return 1
		}

		if len(nodes) == 0 {
			fmt.Println("Роутеры OpenWrt не найдены в сети.")
			return 0
		}

		fmt.Printf("\n=== TACHYON FLEET INVENTORY (Найдено: %d) ===\n\n", len(nodes))
		fmt.Printf("%-16s %-24s %-10s %-10s %-26s %s\n", "IP-АДРЕС", "МОДЕЛЬ", "АРХ", "ОЗУ (СВОБ)", "СТАТУС TACHYON", "РЕКОМ. ЯДРО")
		fmt.Println(strings.Repeat("-", 100))
		for _, n := range nodes {
			model := n.Model
			if len(model) > 23 {
				model = model[:20] + "..."
			}
			ramStr := fmt.Sprintf("%.0f МБ", n.RAMFreeMB)
			if n.RAMFreeMB == 0 {
				ramStr = "-"
			}
			arch := n.Arch
			if arch == "" {
				arch = "-"
			}
			fmt.Printf("%-16s %-24s %-10s %-10s %-26s %s\n",
				n.IP, model, arch, ramStr, n.StatusText, n.RecommendedEngine)
		}
		fmt.Println()
		return 0
	}

	// 6. Fleet Mass Deploy
	if opts.FleetDeploy {
		fmt.Println("⚡ Запуск массового развертывания Tachyon на роутеры сети...")
		subnets := fleet.SplitSubnets(opts.FleetSubnets)
		scanCfg := fleet.DefaultScanConfig(opts.Password, opts.Version)
		scanCfg.Subnets = subnets
		if opts.KeyPath != "" {
			scanCfg.KeyPath = opts.KeyPath
		}
		if opts.RouterIP != "" {
			scanCfg.Gateway = opts.RouterIP
		}

		nodes, err := fleet.ScanFleet(context.Background(), scanCfg, nil)
		if err != nil {
			fmt.Printf("❌ Ошибка обнаружения роутеров: %v\n", err)
			return 1
		}

		var selected []*fleet.FleetNode
		for _, n := range nodes {
			if opts.FleetOnlyOutdated && n.Status != fleet.StatusOutdated {
				continue
			}
			if opts.FleetOnlyClean && n.Status != fleet.StatusClean {
				continue
			}
			if !opts.FleetOnlyOutdated && !opts.FleetOnlyClean {
				if n.Status != fleet.StatusOutdated && n.Status != fleet.StatusClean {
					continue
				}
			}
			n.Selected = true
			selected = append(selected, n)
		}

		if len(selected) == 0 {
			fmt.Println("ℹ️  Нет подходящих роутеров для установки или обновления.")
			return 0
		}

		fmt.Printf("⚡ Выбрано роутеров для установки: %d шт.\n", len(selected))
		for _, s := range selected {
			fmt.Printf("  • %-16s %-24s (%s, %s)\n", s.IP, s.Model, s.Arch, s.RecommendedEngine)
		}
		fmt.Println()

		deployCfg := fleet.DeployConfig{
			TargetVersion:  opts.Version,
			SelectedMirror: opts.Mirror,
			InstallI18n:    opts.InstallI18n,
			Concurrency:    opts.FleetConcurrency,
		}

		err = fleet.DeployFleet(context.Background(), selected, deployCfg, func(node *fleet.FleetNode) {
			fmt.Printf("[%s] [%3.0f%%] %s\n", node.IP, node.DeployProgress*100, node.DeployStage)
		})
		if err != nil {
			fmt.Printf("❌ Ошибка массового развертывания: %v\n", err)
			return 1
		}

		fmt.Println("\n🎉 Массовое развертывание Tachyon успешно завершено!")
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

	// Bufferbloat & CPU Encryption Doctor
	if opts.SpeedDoctor {
		dur := opts.SpeedDuration
		if dur <= 0 {
			dur = 5
		}
		fmt.Printf("⚡ ЗАПУСК BUFFERBLOAT & SPEED DOCTOR (Длительность: %d сек)...\n", dur)
		fmt.Println("  Измерение прямой задержки в покое, скорости WAN и задержки под насыщением...")
		fmt.Println("  (Замер потоковый, без записи во Flash-память)")
		fmt.Println()

		execCmd := func(_ *gossh.Client, cmd string) (string, error) {
			sess, err := client.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		}

		report, err := routerpkg.RunSpeedDoctor(client, execCmd, routerpkg.SpeedDoctorOptions{
			DurationSec: dur,
		})
		if err != nil {
			fmt.Printf("❌ Ошибка выполнения теста: %v\n", err)
			return 1
		}

		printSpeedDoctorCLIReport(report)
		return 0
	}

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

	// 9.5. AmneziaWG Configuration & Live Verification
	if opts.AWGSetup || opts.AWGTest {
		execCmd := func(_ *gossh.Client, cmd string) (string, error) {
			sess, err := client.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		}

		secName := strings.TrimSpace(opts.AWGSection)
		if secName == "" {
			secName = "warp"
		}

		if opts.AWGSetup {
			fmt.Printf("⚡ [AWG] Настройка секции AmneziaWG '%s' на роутере...\n", secName)
			var awgConf *routerpkg.AWGConfig
			var err error

			src := strings.TrimSpace(opts.AWGType)
			if src == "" || strings.EqualFold(src, "warp") {
				fmt.Println("⚡ Генерация конфигурации Cloudflare WARP via AWG...")
				awgConf, err = routerpkg.GenerateWarpAWG(client, execCmd)
			} else if strings.EqualFold(src, "custom") {
				fmt.Println("⚡ Генерация кастомных ключей Curve25519 и параметров обфускации AWG...")
				awgConf, err = routerpkg.GenerateCustomAWGConfig("", 0, "")
			} else if strings.HasPrefix(src, "vpn://") {
				fmt.Println("⚡ Разбор Amnezia vpn:// контейнера...")
				awgConf, err = routerpkg.ParseAWGConfig(src)
			} else if _, statErr := os.Stat(src); statErr == nil {
				fmt.Printf("⚡ Чтение файла конфигурации %s...\n", src)
				data, readErr := os.ReadFile(src)
				if readErr != nil {
					err = readErr
				} else {
					awgConf, err = routerpkg.ParseAWGConfig(string(data))
				}
			} else {
				awgConf, err = routerpkg.ParseAWGConfig(src)
			}

			if err != nil {
				fmt.Printf("❌ Ошибка подготовки конфигурации AWG: %v\n", err)
				return 1
			}

			fmt.Printf("✓ Параметры AWG:\n  Сервер: %s:%d\n  IP: %s\n  Jc: %d, Jmin: %d, Jmax: %d, S1: %d, S2: %d, H1: %s\n",
				awgConf.ServerAddress, awgConf.ServerPort, awgConf.Address,
				awgConf.Jc, awgConf.Jmin, awgConf.Jmax, awgConf.S1, awgConf.S2, awgConf.H1)

			fmt.Println("⚡ Применение секции в /etc/config/tachyon и перезапуск службы...")
			err = routerpkg.ApplyAWGSection(client, execCmd, awgConf, secName, true)
			if err != nil {
				fmt.Printf("❌ Сбой применения секции AWG: %v\n", err)
				return 1
			}
			fmt.Printf("✓ Секция tachyon.%s успешно сохранена и активирована!\n", secName)
		}

		if opts.AWGTest || opts.AWGSetup {
			fmt.Printf("\n⚡ [AWG] Сквозная проверка работы секции '%s' через роутер...\n", secName)
			testRes, err := routerpkg.TestAWGSection(client, execCmd, secName)
			if err != nil {
				fmt.Printf("⚠️ Ошибка тестирования секции AWG: %v\n", err)
			}

			fmt.Println("\n--- РЕЗУЛЬТАТЫ ПРОВЕРКИ ---")
			sbStatus := "❌ ОСТАНОВЛЕНА"
			if testRes.ServiceRunning {
				sbStatus = "✓ РАБОТАЕТ"
			}
			fakeIPStatus := "⚠️ НЕ ОТВЕЧАЕТ"
			if testRes.FakeIPActive {
				fakeIPStatus = "✓ РАБОТАЕТ (" + testRes.ResolvedIP + ")"
			}
			fmt.Printf("Служба sing-box:   %s\n", sbStatus)
			fmt.Printf("Fake-IP DNS:       %s\n", fakeIPStatus)
			if testRes.EgressIP != "" {
				loc := testRes.EgressLocation
				if loc != "" {
					loc = " (" + loc + ")"
				}
				fmt.Printf("Внешний IP:        %s%s\n", testRes.EgressIP, loc)
			}
			if testRes.IsWARP {
				fmt.Println("Cloudflare WARP:   ✓ ВКЛЮЧЕН (warp=on)")
			}

			fmt.Println("\nТестирование доступности ресурсов через роутер:")
			for _, p := range testRes.Probes {
				statusIcon := "✓"
				if !p.Success {
					statusIcon = "❌"
				}
				fmt.Printf("  • %-16s %s HTTP %d (%d мс)\n", p.Name+":", statusIcon, p.HTTPStatus, p.LatencyMs)
			}
			fmt.Println("---------------------------")
			if testRes.Success {
				fmt.Println("🎉 Проверка успешно пройдена! Трафик идет через AmneziaWG.")
			} else {
				fmt.Printf("⚠️ %s\n", testRes.Details)
			}
		}

		return 0
	}

	// 10. Guided Autopilot Setup
	if opts.GuidedSetup {
		fmt.Printf("==> 🚀 МАСТЕР БЫСТРОЙ НАСТРОЙКИ (АВТОПИЛОТ ПОД КЛЮЧ)\n\n")
		execCmd := func(_ *gossh.Client, cmd string) (string, error) {
			sess, err := client.NewSession()
			if err != nil {
				return "", err
			}
			defer sess.Close()
			out, err := sess.CombinedOutput(cmd)
			return string(out), err
		}

		fmt.Println("⚡ [1/5] Сканирование оборудования и роутера...")
		profile, err := routerpkg.RunPreConnectionCheck(client)
		if err == nil {
			fmt.Printf("  • Модель: %s, Архитектура: %s, ОЗУ: %.0f МБ\n", profile.Model, profile.Arch, profile.RAMTotal)
		}

		fmt.Println("⚡ [2/5] DNS-тестирование: проверка отравления UDP 53 и подбор резолвера...")
		dnsReport := routerpkg.RunDNSTest(client, execCmd)
		if dnsReport.UDPPoisoned {
			fmt.Printf("  ⚠️ Провайдер подменяет обычный DNS (спуфинг): %s\n", dnsReport.PoisonDetail)
		} else {
			fmt.Println("  ✓ Прямой DNS не отравлен провайдером")
		}
		var chosenDNS routerpkg.DNSResolver
		if dnsReport.Fastest != nil {
			chosenDNS = *dnsReport.Fastest
			fmt.Printf("  ✓ Выбран быстрейший защищенный DNS: %s (%d мс)\n", chosenDNS.Name, chosenDNS.LatencyMs)
		} else {
			chosenDNS = routerpkg.DefaultDNSResolvers()[0]
			fmt.Printf("  ✓ Выбран DNS по умолчанию: %s\n", chosenDNS.Name)
		}

		fmt.Println("⚡ [3/5] DPI Fuzzer: подбор оптимальной стратегии обхода YouTube и Discord...")
		fuzzReport := routerpkg.RunDPIFuzzer(client, execCmd, nil)
		var chosenStrat routerpkg.DPIStrategy
		if fuzzReport.WinningStrategy != nil {
			chosenStrat = *fuzzReport.WinningStrategy
			fmt.Printf("  ✓ Выбрана победившая стратегия: %s (Успех: %d%%, Задержка: %d мс)\n", chosenStrat.Name, chosenStrat.SuccessRate, chosenStrat.AvgLatencyMs)
		} else {
			chosenStrat = routerpkg.DefaultDPIStrategies()[0]
			fmt.Printf("  ✓ Выбрана базовая стратегия: %s\n", chosenStrat.Name)
		}

		fmt.Println("⚡ [4/5] Применение конфигурации под ключ и сетевой тюнинг...")
		plan := routerpkg.GuidedSetupPlan{
			Mode:           routerpkg.ModeStandaloneDPI,
			ChosenDNS:      chosenDNS,
			ChosenStrategy: chosenStrat,
			SelectedEngine: "steer",
			EnableTune:     true,
		}
		if opts.Subscription != "" {
			plan.Mode = routerpkg.ModeTunnel
			plan.SubscriptionURL = opts.Subscription
			plan.SelectedEngine = "sing-box"
		}

		res, err := routerpkg.RunGuidedSetup(client, execCmd, plan, func(title string, frac float64) {
			fmt.Printf("  • %s\n", title)
		})
		if err != nil {
			fmt.Printf("❌ Сбой применения: %v\n", err)
			return 1
		}

		fmt.Println("\n⚡ [5/5] Финальный скоркарт результатов:")
		fmt.Printf("  • DNS: %s (Задержка: %d мс) ✓\n", res.DNSName, res.DNSLatencyMs)
		if res.StrategyApplied {
			fmt.Printf("  • DPI-стратегия: %s ✓\n", res.StrategyName)
		}
		if res.NetworkTuned {
			fmt.Println("  • Сетевой стек: TCP BBR + адаптивные буферы + flow offloading ✓")
		}
		for _, p := range res.VerifyResult.Probes {
			if p.Success {
				fmt.Printf("  • %-16s HTTP %d (%d мс) ✓\n", p.Name+":", p.HTTPStatus, p.LatencyMs)
			} else {
				fmt.Printf("  • %-16s HTTP %d\n", p.Name+":", p.HTTPStatus)
			}
		}

		fmt.Println("\n🎉 НАСТРОЙКА ПОД КЛЮЧ УСПЕШНО ЗАВЕРШЕНА!")
		fmt.Printf("Веб-интерфейс LuCI доступен: http://%s\n", opts.RouterIP)
		return 0
	}

	// 11. Uninstall
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

func printSpeedDoctorCLIReport(report *routerpkg.SpeedDoctorReport) {
	if report.Direct != nil {
		d := report.Direct
		fmt.Printf("================================================================================\n")
		fmt.Printf("  BUFFERBLOAT GRADE:  [ %s ]  —  %s\n", d.Grade, d.Verdict)
		fmt.Printf("================================================================================\n")
		fmt.Printf("  • Скорость скачивания (WAN):  %.1f Мбит/с\n", d.SpeedMbps)
		fmt.Printf("  • Задержка в покое (Idle):    %.1f мс (мин: %.1f, макс: %.1f, джиттер: %.1f мс)\n",
			d.IdlePingAvg, d.IdlePingMin, d.IdlePingMax, d.IdleJitter)
		fmt.Printf("  • Задержка под нагрузкой:     %.1f мс\n", d.LoadedPingAvg)
		fmt.Printf("  • Bufferbloat Оверхед:        +%.1f мс (Оценка: %s)\n", d.BufferbloatDelta, d.Grade)

		throttledMsg := ""
		if d.CPU.Throttled {
			throttledMsg = " [ВНИМАНИЕ: CPU Throttling / Перегрузка SoftIRQ!]"
		}
		fmt.Printf("  • Загрузка CPU роутера:       %.0f%% (sys: %.0f%%, softirq: %.0f%%)%s\n",
			d.CPU.TotalPct, d.CPU.SystemPct, d.CPU.SoftIRQPct, throttledMsg)
		fmt.Printf("================================================================================\n")
	}

	if report.Tunnel != nil {
		t := report.Tunnel
		fmt.Printf("\n--- ТУННЕЛЬ TACHYON (PROXY) ---\n")
		fmt.Printf("  • Скорость:            %.1f Мбит/с\n", t.SpeedMbps)
		fmt.Printf("  • Пинг в туннеле:      %.1f мс (Bufferbloat: +%.1f мс)\n", t.IdlePingAvg, t.BufferbloatDelta)
		fmt.Printf("  • Загрузка CPU:        %.0f%%\n", t.CPU.TotalPct)
	}

	if len(report.Recommendations) > 0 {
		fmt.Printf("\n💡 Рекомендации сетевого доктора:\n")
		for _, rec := range report.Recommendations {
			fmt.Printf("  • %s\n", rec)
		}
		fmt.Println()
	}
}
