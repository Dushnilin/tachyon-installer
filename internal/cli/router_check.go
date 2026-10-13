package cli

import (
	"context"
	"fmt"
	"strings"

	gossh "golang.org/x/crypto/ssh"

	dlpkg "tachyon-installer/internal/downloader"
	routerpkg "tachyon-installer/internal/router"
	"tachyon-installer/internal/updater"
)

// runRouterCheck inspects what is installed on the router, fetches the latest
// Tachyon release from GitHub and reports whether an update is available.
func runRouterCheck(opts *Options, client *gossh.Client) int {
	fmt.Println("⚡ Сбор информации об установленных компонентах на роутере...")
	profile, err := routerpkg.RunPreConnectionCheck(client)
	if err != nil {
		fmt.Printf("❌ Не удалось получить профиль роутера: %v\n", err)
		return 1
	}

	fmt.Println("=== СОСТОЯНИЕ РОУТЕРА ===")
	fmt.Printf("  Модель:        %s\n", profile.Model)
	fmt.Printf("  Платформа:     %s\n", profile.Version)
	fmt.Printf("  Архитектура:   %s\n", profile.Arch)
	fmt.Printf("  Ядро прокси:   %s", profile.ActiveEngine)
	if profile.InstalledEngineVer != "" {
		fmt.Printf(" (%s)", profile.InstalledEngineVer)
	}
	fmt.Println()

	installed := profile.InstalledTachyonVer
	if installed == "" || installed == "none" {
		fmt.Println("  Tachyon:       не установлен")
		fmt.Println("\nℹ️ Tachyon на роутере не установлен — обновлять нечего, доступна свежая установка.")
		return 0
	}

	fmt.Println("⚡ Запрос последней версии Tachyon с GitHub...")
	mirrorMgr := dlpkg.NewMirrorManager(opts.Mirror)
	dlClient := dlpkg.NewClient(mirrorMgr)
	rel, err := dlpkg.FetchTachyonReleaseByTag(context.Background(), dlClient, "latest")
	if err != nil {
		fmt.Printf("⚠️ Не удалось получить последнюю версию: %v\n", err)
		return 1
	}

	latest := strings.TrimPrefix(rel.TagName, "v")

	if installed == "installed" {
		installed = "" // версия не определилась, но пакет стоит
	}
	if installed != "" {
		fmt.Printf("  Tachyon:       v%s\n", installed)
	} else {
		fmt.Println("  Tachyon:       установлен (версия неизвестна)")
	}
	fmt.Printf("  Последний релиз: v%s\n", latest)

	if installed == "" || updater.IsNewerVersion(latest, installed) {
		fmt.Printf("\n💡 Доступно обновление: v%s → v%s\n", installed, latest)
		fmt.Println("   Обновление: запустите установщик и выберите установку на этот роутер.")
	} else {
		fmt.Printf("\n✓ Установлена актуальная версия Tachyon (v%s).\n", installed)
	}
	return 0
}
