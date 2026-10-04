package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OfflinePackageInfo holds metadata for a cached package file.
type OfflinePackageInfo struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Category string `json:"category"` // "tachyon", "engine"
	Arch     string `json:"arch,omitempty"`
}

// OfflineManifest records the offline bundle metadata.
type OfflineManifest struct {
	CreatedAt      time.Time            `json:"created_at"`
	TachyonVersion string               `json:"tachyon_version"`
	Packages       []OfflinePackageInfo `json:"packages"`
}

// CommonRouterArchitectures for pre-caching.
var CommonRouterArchitectures = []string{
	"arm64",
	"mipsle",
	"mips",
	"x86_64",
}

// CreateOfflineBundle downloads Tachyon packages and popular engine binaries into a local directory.
func CreateOfflineBundle(
	ctx context.Context,
	dlClient *Client,
	targetDir string,
	version string,
	archList []string,
	logFn func(string),
) (*OfflineManifest, error) {
	if logFn == nil {
		logFn = func(string) {}
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("create bundle dir: %w", err)
	}

	if len(archList) == 0 {
		archList = CommonRouterArchitectures
	}

	manifest := &OfflineManifest{
		CreatedAt:      time.Now().UTC(),
		TachyonVersion: version,
		Packages:       []OfflinePackageInfo{},
	}

	logFn(fmt.Sprintf("⚡ [1/2] Получение релизов Tachyon (%s)...\n", version))
	rel, err := FetchTachyonReleaseByTag(ctx, dlClient, version)
	if err != nil {
		return nil, fmt.Errorf("fetch tachyon release: %w", err)
	}

	// 1. Download base Tachyon packages for both OPKG and APK
	for _, isAPK := range []bool{false, true} {
		assets, errRes := ResolveTachyonAssets(rel, isAPK, true)
		if errRes != nil {
			continue
		}

		type dlItem struct {
			name string
			url  string
		}
		items := []dlItem{
			{assets.BackendName, assets.BackendURL},
			{assets.AppName, assets.AppURL},
			{assets.I18nName, assets.I18nURL},
			{assets.SHA256Name, assets.SHA256URL},
		}

		for _, item := range items {
			if item.name == "" || item.url == "" {
				continue
			}
			dest := filepath.Join(targetDir, item.name)
			if fi, errStat := os.Stat(dest); errStat == nil {
				logFn(fmt.Sprintf("  • Уже скачан: %s\n", item.name))
				manifest.Packages = append(manifest.Packages, OfflinePackageInfo{
					Filename: item.name,
					Size:     fi.Size(),
					Category: "tachyon",
				})
				continue
			}
			logFn(fmt.Sprintf("  -> Скачивание: %s...\n", item.name))
			_, errDl := dlClient.DownloadFile(ctx, item.url, dest, item.name, nil)
			if errDl == nil {
				fi, _ := os.Stat(dest)
				size := int64(0)
				if fi != nil {
					size = fi.Size()
				}
				manifest.Packages = append(manifest.Packages, OfflinePackageInfo{
					Filename: item.name,
					Size:     size,
					Category: "tachyon",
				})
			}
		}
	}

	// 2. Download sing-box and steer for requested architectures
	logFn("⚡ [2/2] Загрузка ядер для распространенных архитектур...\n")
	for _, arch := range archList {
		for _, eng := range []EngineType{EngineSingBoxExtended, EngineSteer} {
			engAssets, errEng := ResolveEngineAssets(ctx, dlClient, eng, arch, arch, false)
			if errEng != nil || len(engAssets) == 0 {
				continue
			}
			for _, ea := range engAssets {
				if ea == nil || ea.URL == "" {
					continue
				}
				dest := filepath.Join(targetDir, ea.Filename)
				if fi, errStat := os.Stat(dest); errStat == nil {
					manifest.Packages = append(manifest.Packages, OfflinePackageInfo{
						Filename: ea.Filename,
						Size:     fi.Size(),
						Category: "engine",
						Arch:     arch,
					})
					continue
				}
				logFn(fmt.Sprintf("  -> Ядро [%s/%s]: %s...\n", eng, arch, ea.Filename))
				_, errDl := dlClient.DownloadFile(ctx, ea.URL, dest, ea.Filename, nil)
				if errDl == nil {
					fi, _ := os.Stat(dest)
					size := int64(0)
					if fi != nil {
						size = fi.Size()
					}
					manifest.Packages = append(manifest.Packages, OfflinePackageInfo{
						Filename: ea.Filename,
						Size:     size,
						Category: "engine",
						Arch:     arch,
					})
				}
			}
		}
	}

	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(filepath.Join(targetDir, "offline_manifest.json"), manifestBytes, 0644)

	logFn(fmt.Sprintf("✓ Оффлайн-бандл готов! Всего сохранено пакетов: %d шт.\n", len(manifest.Packages)))
	return manifest, nil
}

// LoadOfflineBundle reads the offline_manifest.json from the given directory.
func LoadOfflineBundle(dir string) (*OfflineManifest, error) {
	manifestPath := filepath.Join(dir, "offline_manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read offline manifest: %w", err)
	}

	var m OfflineManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse offline manifest: %w", err)
	}

	return &m, nil
}
