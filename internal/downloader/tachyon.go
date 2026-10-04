package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TachyonRelease represents a release from Dushnilin/tachyon.
type TachyonRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// TachyonAssetsToDownload represents the resolved files for a release.
type TachyonAssetsToDownload struct {
	Version      string
	IsAPK        bool
	BackendURL   string
	BackendName  string
	AppURL       string
	AppName      string
	I18nURL      string
	I18nName     string
	SHA256URL    string
	SHA256Name   string
}

// FetchTachyonReleases queries Dushnilin/tachyon releases via mirrors.
func FetchTachyonReleases(ctx context.Context, client *Client) ([]TachyonRelease, error) {
	url := "https://api.github.com/repos/Dushnilin/tachyon/releases?per_page=15"
	var releases []TachyonRelease

	_, err := client.FetchJSON(ctx, url, &releases)
	if err != nil {
		// Fallback: fetch latest single release
		latestURL := "https://api.github.com/repos/Dushnilin/tachyon/releases/latest"
		var single TachyonRelease
		_, errLatest := client.FetchJSON(ctx, latestURL, &single)
		if errLatest == nil && single.TagName != "" {
			return []TachyonRelease{single}, nil
		}
		return nil, fmt.Errorf("failed to fetch Tachyon releases: %w", err)
	}

	return releases, nil
}

// FetchTachyonReleaseByTag queries a specific release tag from Dushnilin/tachyon.
func FetchTachyonReleaseByTag(ctx context.Context, client *Client, tag string) (*TachyonRelease, error) {
	if tag == "" || tag == "latest" {
		url := "https://api.github.com/repos/Dushnilin/tachyon/releases/latest"
		var rel TachyonRelease
		_, err := client.FetchJSON(ctx, url, &rel)
		if err == nil && rel.TagName != "" {
			return &rel, nil
		}

		// Fallback: try scraping expanded_assets or direct release page
		targetTag := "1.4.9" // fallback latest
		scraped, sErr := client.ScrapeExpandedAssets(ctx, "Dushnilin/tachyon", targetTag)
		if sErr == nil && len(scraped) > 0 {
			rel.TagName = targetTag
			for name, u := range scraped {
				rel.Assets = append(rel.Assets, struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
					Size               int64  `json:"size"`
				}{Name: name, BrowserDownloadURL: u})
			}
			return &rel, nil
		}

		// Deterministic fallback
		return buildDeterministicRelease(targetTag), nil
	}

	url := fmt.Sprintf("https://api.github.com/repos/Dushnilin/tachyon/releases/tags/%s", tag)
	var rel TachyonRelease
	_, err := client.FetchJSON(ctx, url, &rel)
	if err == nil && rel.TagName != "" {
		return &rel, nil
	}

	// Fallback: scrape expanded_assets
	scraped, sErr := client.ScrapeExpandedAssets(ctx, "Dushnilin/tachyon", tag)
	if sErr == nil && len(scraped) > 0 {
		rel.TagName = tag
		for name, u := range scraped {
			rel.Assets = append(rel.Assets, struct {
				Name               string `json:"name"`
				BrowserDownloadURL string `json:"browser_download_url"`
				Size               int64  `json:"size"`
			}{Name: name, BrowserDownloadURL: u})
		}
		return &rel, nil
	}

	// Deterministic fallback
	return buildDeterministicRelease(tag), nil
}

func buildDeterministicRelease(tag string) *TachyonRelease {
	cleanVer := strings.TrimPrefix(tag, "v")
	rel := &TachyonRelease{TagName: tag}
	names := []string{
		fmt.Sprintf("tachyon_%s.ipk", cleanVer),
		fmt.Sprintf("tachyon_%s.apk", cleanVer),
		fmt.Sprintf("luci-app-tachyon_%s.ipk", cleanVer),
		fmt.Sprintf("luci-app-tachyon_%s.apk", cleanVer),
		fmt.Sprintf("luci-i18n-tachyon-ru_%s.ipk", cleanVer),
		fmt.Sprintf("luci-i18n-tachyon-ru_%s.apk", cleanVer),
		"sha256sums.txt",
	}
	for _, n := range names {
		u := fmt.Sprintf("https://github.com/Dushnilin/tachyon/releases/download/%s/%s", tag, n)
		rel.Assets = append(rel.Assets, struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{Name: n, BrowserDownloadURL: u})
	}
	return rel
}

// ResolveTachyonAssets identifies the matching .ipk or .apk assets in a release.
func ResolveTachyonAssets(rel *TachyonRelease, isAPK bool, needI18n bool) (*TachyonAssetsToDownload, error) {
	ext := "ipk"
	if isAPK {
		ext = "apk"
	}

	cleanVer := strings.TrimPrefix(rel.TagName, "v")

	backendPrefix := "tachyon_"
	appPrefix := "luci-app-tachyon_"
	i18nPrefix := "luci-i18n-tachyon-ru_"

	res := &TachyonAssetsToDownload{
		Version: cleanVer,
		IsAPK:   isAPK,
	}

	for _, a := range rel.Assets {
		name := a.Name
		if strings.HasPrefix(name, backendPrefix) && strings.HasSuffix(name, "."+ext) {
			res.BackendURL = a.BrowserDownloadURL
			res.BackendName = name
		} else if strings.HasPrefix(name, appPrefix) && strings.HasSuffix(name, "."+ext) {
			res.AppURL = a.BrowserDownloadURL
			res.AppName = name
		} else if strings.HasPrefix(name, i18nPrefix) && strings.HasSuffix(name, "."+ext) {
			res.I18nURL = a.BrowserDownloadURL
			res.I18nName = name
		} else if name == "sha256sums.txt" {
			res.SHA256URL = a.BrowserDownloadURL
			res.SHA256Name = name
		}
	}

	// Validate / fallback to deterministic asset URLs
	if res.BackendURL == "" {
		res.BackendName = fmt.Sprintf("tachyon_%s.%s", cleanVer, ext)
		res.BackendURL = fmt.Sprintf("https://github.com/Dushnilin/tachyon/releases/download/%s/%s", rel.TagName, res.BackendName)
	}
	if res.AppURL == "" {
		res.AppName = fmt.Sprintf("luci-app-tachyon_%s.%s", cleanVer, ext)
		res.AppURL = fmt.Sprintf("https://github.com/Dushnilin/tachyon/releases/download/%s/%s", rel.TagName, res.AppName)
	}
	if needI18n && res.I18nURL == "" {
		res.I18nName = fmt.Sprintf("luci-i18n-tachyon-ru_%s.%s", cleanVer, ext)
		res.I18nURL = fmt.Sprintf("https://github.com/Dushnilin/tachyon/releases/download/%s/%s", rel.TagName, res.I18nName)
	}
	if res.SHA256URL == "" {
		res.SHA256Name = "sha256sums.txt"
		res.SHA256URL = fmt.Sprintf("https://github.com/Dushnilin/tachyon/releases/download/%s/sha256sums.txt", rel.TagName)
	}

	return res, nil
}

// DownloadTachyonPackages downloads all resolved Tachyon packages to destDir and validates SHA256.
func DownloadTachyonPackages(
	ctx context.Context,
	client *Client,
	assets *TachyonAssetsToDownload,
	needI18n bool,
	destDir string,
	logFn func(msg string),
	subTaskFn func(label string, frac float64),
) ([]string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, err
	}

	var downloadedFiles []string

	// 1. Download sha256sums.txt if available
	var sha256Map map[string]string
	if assets.SHA256URL != "" {
		subTaskFn("Скачивание контрольных сумм...", 0.1)
		shaPath := filepath.Join(destDir, "sha256sums.txt")
		_, err := client.DownloadFile(ctx, assets.SHA256URL, shaPath, "sha256sums.txt", nil)
		if err == nil {
			content, readErr := os.ReadFile(shaPath)
			if readErr == nil {
				sha256Map = ParseSHA256Sums(string(content))
				logFn(fmt.Sprintf("[#22c55e]✓ Загружены контрольные суммы (SHA256) для %d файлов[-]\n", len(sha256Map)))
			}
		}
	}

	type dlItem struct {
		name string
		url  string
	}
	var items []dlItem
	if assets.BackendURL != "" {
		items = append(items, dlItem{assets.BackendName, assets.BackendURL})
	}
	if assets.AppURL != "" {
		items = append(items, dlItem{assets.AppName, assets.AppURL})
	}
	if needI18n && assets.I18nURL != "" {
		items = append(items, dlItem{assets.I18nName, assets.I18nURL})
	}

	totalItems := len(items)
	for i, item := range items {
		destPath := filepath.Join(destDir, item.name)
		logFn(fmt.Sprintf("[#cbd5e1]⚡ Скачивание: [#38bdf8]%s[-]...[-]\n", item.name))

		stepBase := float64(i) / float64(totalItems)
		stepWeight := 1.0 / float64(totalItems)

		usedMirror, err := client.DownloadFile(ctx, item.url, destPath, item.name, func(downloaded, total, speed int64) {
			dlMB := float64(downloaded) / 1024 / 1024
			speedKB := float64(speed) / 1024
			if total > 0 {
				totalMB := float64(total) / 1024 / 1024
				pct := float64(downloaded) / float64(total)
				subTaskFn(fmt.Sprintf("Скачивание %s: %.1f/%.1f МБ (%.0f%%, %.0f КБ/с)", item.name, dlMB, totalMB, pct*100, speedKB), stepBase+pct*stepWeight)
			} else {
				subTaskFn(fmt.Sprintf("Скачивание %s: %.1f МБ (%.0f КБ/с)", item.name, dlMB, speedKB), stepBase+0.5*stepWeight)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("ошибка загрузки %s: %w", item.name, err)
		}

		downloadedFiles = append(downloadedFiles, destPath)
		logFn(fmt.Sprintf("[#22c55e]✓ Успешно скачан: %s (зеркало: %s)[-]\n", item.name, usedMirror))

		// Check SHA256 if available
		if sha256Map != nil {
			if expectedHash, ok := sha256Map[item.name]; ok {
				if err := VerifyFileSHA256(destPath, expectedHash); err != nil {
					return nil, fmt.Errorf("ошибка проверки контрольной суммы для %s: %w", item.name, err)
				}
				logFn(fmt.Sprintf("[#22c55e]✓ SHA256 контрольная сумма подтверждена для %s[-]\n", item.name))
			}
		}
	}

	return downloadedFiles, nil
}
