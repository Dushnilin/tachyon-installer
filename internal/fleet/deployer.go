package fleet

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"

	deploypkg "tachyon-installer/internal/deploy"
	dlpkg "tachyon-installer/internal/downloader"
	routerpkg "tachyon-installer/internal/router"
	sshpkg "tachyon-installer/internal/ssh"
)

// DeployConfig controls parameters for mass deployment.
type DeployConfig struct {
	TargetVersion string
	SelectedMirror string
	InstallI18n   bool
	Concurrency   int
}

// DeployFleet runs mass concurrent deployment across all selected FleetNodes.
func DeployFleet(ctx context.Context, nodes []*FleetNode, cfg DeployConfig, updateCb func(node *FleetNode)) error {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 3
	}
	if cfg.TargetVersion == "" {
		cfg.TargetVersion = "latest"
	}
	if cfg.SelectedMirror == "" {
		cfg.SelectedMirror = "auto"
	}

	// Filter selected nodes
	var targets []*FleetNode
	for _, n := range nodes {
		if n.Selected {
			targets = append(targets, n)
		}
	}
	if len(targets) == 0 {
		return fmt.Errorf("нет выбранных роутеров для установки")
	}

	mirrorMgr := dlpkg.NewMirrorManager(cfg.SelectedMirror)
	dlClient := dlpkg.NewClient(mirrorMgr)

	// Fetch target release metadata once
	tachyonRel, err := dlpkg.FetchTachyonReleaseByTag(ctx, dlClient, cfg.TargetVersion)
	if err != nil {
		return fmt.Errorf("ошибка получения релиза Tachyon (%s): %w", cfg.TargetVersion, err)
	}

	// Shared cache for downloaded packages by key: "arch_isAPK_engine"
	var (
		cacheMu sync.Mutex
		pkgCache = map[string]string{}
	)

	defer func() {
		cacheMu.Lock()
		for _, dir := range pkgCache {
			_ = os.RemoveAll(dir)
		}
		cacheMu.Unlock()
	}()

	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	for _, n := range targets {
		select {
		case <-ctx.Done():
			break
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(node *FleetNode) {
			defer wg.Done()
			defer func() { <-sem }()

			deployToNode(ctx, node, cfg, dlClient, tachyonRel, &cacheMu, pkgCache, updateCb)
		}(n)
	}

	wg.Wait()
	return nil
}

func deployToNode(
	ctx context.Context,
	node *FleetNode,
	cfg DeployConfig,
	dlClient *dlpkg.Client,
	tachyonRel *dlpkg.TachyonRelease,
	cacheMu *sync.Mutex,
	pkgCache map[string]string,
	updateCb func(node *FleetNode),
) {
	notify := func(stage string, progress float64) {
		node.DeployStage = stage
		node.DeployProgress = progress
		if updateCb != nil {
			updateCb(node)
		}
	}

	notify("Подключение по SSH...", 0.1)

	// 1. SSH Connect
	client, err := sshpkg.Connect(node.IP, node.Port, node.Username, node.Password)
	if err != nil {
		node.DeployError = fmt.Sprintf("SSH: %v", err)
		notify(fmt.Sprintf("Ошибка SSH: %v", err), 0.0)
		return
	}
	defer client.Close()

	// Sync router time to prevent TLS errors
	_, _ = routerpkg.SyncRouterTime(client)

	// 2. Prepare & Cache packages for this router's architecture
	cacheKey := fmt.Sprintf("%s_%v_%s_%v", node.Arch, node.IsAPK, node.RecommendedEngine, cfg.InstallI18n)

	cacheMu.Lock()
	cachedDir, hasCached := pkgCache[cacheKey]
	cacheMu.Unlock()

	if !hasCached {
		notify("Сканирование и загрузка пакетов...", 0.25)
		stagingDir, errStaging := os.MkdirTemp("", "tachyon_fleet_pkg_*")
		if errStaging != nil {
			node.DeployError = errStaging.Error()
			notify(fmt.Sprintf("Ошибка: %v", errStaging), 0.0)
			return
		}

		// Download Tachyon packages
		tachyonAssets, errRes := dlpkg.ResolveTachyonAssets(tachyonRel, node.IsAPK, cfg.InstallI18n)
		if errRes != nil {
			_ = os.RemoveAll(stagingDir)
			node.DeployError = errRes.Error()
			notify(fmt.Sprintf("Ошибка Tachyon: %v", errRes), 0.0)
			return
		}

		_, errDl := dlpkg.DownloadTachyonPackages(
			ctx,
			dlClient,
			tachyonAssets,
			cfg.InstallI18n,
			stagingDir,
			func(string) {},
			func(string, float64) {},
		)
		if errDl != nil {
			_ = os.RemoveAll(stagingDir)
			node.DeployError = errDl.Error()
			notify(fmt.Sprintf("Ошибка загрузки Tachyon: %v", errDl), 0.0)
			return
		}

		// Download Engine packages
		if node.RecommendedEngine != "skip" && node.RecommendedEngine != "" {
			engineAssets, errEng := dlpkg.ResolveEngineAssets(
				ctx,
				dlClient,
				dlpkg.EngineType(node.RecommendedEngine),
				node.DistribArch,
				node.Arch,
				node.IsAPK,
			)
			if errEng == nil && len(engineAssets) > 0 {
				for _, ea := range engineAssets {
					if ea != nil && ea.URL != "" {
						_, _ = dlpkg.DownloadEnginePackage(ctx, dlClient, ea, stagingDir, func(string) {}, func(string, float64) {})
					}
				}
			}
		}

		cacheMu.Lock()
		pkgCache[cacheKey] = stagingDir
		cachedDir = stagingDir
		cacheMu.Unlock()
	}

	// 3. Create per-router staging dir
	routerStaging, errStg := deploypkg.NewStagingArea()
	if errStg != nil {
		node.DeployError = errStg.Error()
		notify(fmt.Sprintf("Ошибка: %v", errStg), 0.0)
		return
	}
	defer routerStaging.Cleanup()

	// Copy cached packages to this router's staging
	files, _ := os.ReadDir(cachedDir)
	for _, f := range files {
		src := filepath.Join(cachedDir, f.Name())
		dst := filepath.Join(routerStaging.Dir, f.Name())
		_ = copyFile(src, dst)
	}

	// 4. Generate runner script tailored to router's recommended engine and zRAM
	notify("Генерация установочного скрипта...", 0.45)
	_, err = routerStaging.WriteRunnerScript(node.RecommendedEngine, node.IsAPK, node.RecommendedZRAM)
	if err != nil {
		node.DeployError = err.Error()
		notify(fmt.Sprintf("Ошибка скрипта: %v", err), 0.0)
		return
	}

	// 5. Upload packages and execute installation on the router
	notify("Передача пакетов и установка на роутер...", 0.65)
	err = deploypkg.UploadStagingAndExecute(client, routerStaging.Dir, io.Discard, func(string) {})
	if err != nil {
		node.DeployError = err.Error()
		notify(fmt.Sprintf("Ошибка установки: %v", err), 0.0)
		return
	}

	// 6. Validation
	notify("Проверка запуска службы...", 0.90)
	time.Sleep(2 * time.Second)

	execFn := func(_ *gossh.Client, cmd string) (string, error) {
		return sshpkg.Exec(client, cmd, nil)
	}
	res := routerpkg.VerifyAndFallback(client, execFn, func(string, float64) {})

	if res.OK {
		node.DeploySuccess = true
		node.DeployError = ""
		node.InstalledVersion = strings.TrimPrefix(tachyonRel.TagName, "v")
		node.Status = StatusUpToDate
		node.StatusText = fmt.Sprintf("Актуален (v%s)", node.InstalledVersion)
		notify("Успешно настроен ✓", 1.0)
	} else {
		node.DeploySuccess = true
		node.InstalledVersion = strings.TrimPrefix(tachyonRel.TagName, "v")
		notify("Установлен (проверьте в LuCI) ✓", 1.0)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
