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
	Version     string
	IsAPK       bool
	IsV2        bool
	BackendURL  string
	BackendName string
	AppURL      string
	AppName     string
	I18nURL     string
	I18nName    string
	SHA256URL   string
	SHA256Name  string
}

// IsV2Tag reports whether a release tag belongs to Tachyon 2.0+.
func IsV2Tag(tag string) bool {
	s := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(tag)), "v")
	dot := strings.IndexByte(s, '.')
	if dot <= 0 {
		return false
	}
	majorStr := s[:dot]
	if len(majorStr) == 1 {
		return majorStr[0] >= '2' && majorStr[0] <= '9'
	}
	for i := 0; i < len(majorStr); i++ {
		if majorStr[i] < '0' || majorStr[i] > '9' {
			return false
		}
	}
	return majorStr[0] != '0'
}

// IsPrereleaseTag reports whether a tag contains a semver prerelease suffix (e.g. v2.0.0-beta.7).
func IsPrereleaseTag(tag string) bool {
	s := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	return strings.Contains(s, "-")
}

// IsBetaChannelRequest reports whether tag selects the 2.0 beta release channel.
func IsBetaChannelRequest(tag string) bool {
	t := strings.ToLower(strings.TrimSpace(tag))
	return t == "beta" || t == "latest-beta" || t == "prerelease" || t == "2.0" || t == "2.0-beta"
}

var tachyon2SupportedArchs = []string{
	"aarch64",
	"x86_64",
	"armv7",
	"armv6",
	"mipsel",
	"mips",
	"riscv64",
	"i386_pentium4",
}

// MapToTachyon2Arch maps OpenWrt distribArch/uname architecture names to Tachyon 2.0 package arch names.
func MapToTachyon2Arch(distribArch, rawArch string) string {
	combined := strings.ToLower(strings.TrimSpace(distribArch + " " + rawArch))
	switch {
	case strings.Contains(combined, "aarch64") || strings.Contains(combined, "arm64"):
		return "aarch64"
	case strings.Contains(combined, "x86_64") || strings.Contains(combined, "amd64"):
		return "x86_64"
	case strings.Contains(combined, "riscv64"):
		return "riscv64"
	case strings.Contains(combined, "mipsel") || strings.Contains(combined, "mips32el") || strings.Contains(combined, "mipsle"):
		return "mipsel"
	case strings.Contains(combined, "mips"):
		return "mips"
	case strings.Contains(combined, "arm_cortex") || strings.Contains(combined, "armv7"):
		return "armv7"
	case strings.Contains(combined, "armv6") || strings.Contains(combined, "arm_arm11") || strings.Contains(combined, "arm_mpcore"):
		return "armv6"
	case strings.Contains(combined, "arm"):
		return "armv7"
	case strings.Contains(combined, "i386_pentium4") || strings.Contains(combined, "i686") || strings.Contains(combined, "i586") || strings.Contains(combined, "i386") || strings.Contains(combined, "386") || strings.Contains(combined, "x86"):
		return "i386_pentium4"
	}
	return "aarch64"
}

// FetchTachyonReleases queries Dushnilin/tachyon releases via mirrors.
func FetchTachyonReleases(ctx context.Context, client *Client) ([]TachyonRelease, error) {
	url := "https://api.github.com/repos/Dushnilin/tachyon/releases?per_page=25"
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

// FetchTachyonReleaseByTag queries a specific release tag (or channel like "latest" / "beta") from Dushnilin/tachyon.
func FetchTachyonReleaseByTag(ctx context.Context, client *Client, tag string) (*TachyonRelease, error) {
	trimmed := strings.TrimSpace(tag)
	if trimmed == "" || strings.EqualFold(trimmed, "latest") {
		url := "https://api.github.com/repos/Dushnilin/tachyon/releases/latest"
		var rel TachyonRelease
		_, err := client.FetchJSON(ctx, url, &rel)
		if err == nil && rel.TagName != "" {
			return &rel, nil
		}

		// Fallback: try scraping expanded_assets or direct release page
		targetTag := "1.8.2" // fallback latest
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

	if IsBetaChannelRequest(trimmed) {
		rels, err := FetchTachyonReleases(ctx, client)
		if err == nil && len(rels) > 0 {
			if picked, ok := pickBestV2BetaRelease(rels); ok {
				return picked, nil
			}
		}

		// Fallback to known latest 2.0 beta tag via expanded_assets
		targetTag := "v2.0.0-beta.7"
		var rel TachyonRelease
		scraped, sErr := client.ScrapeExpandedAssets(ctx, "Dushnilin/tachyon", targetTag)
		if sErr == nil && len(scraped) > 0 {
			rel.TagName = targetTag
			rel.Prerelease = true
			for name, u := range scraped {
				rel.Assets = append(rel.Assets, struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
					Size               int64  `json:"size"`
				}{Name: name, BrowserDownloadURL: u})
			}
			return &rel, nil
		}
		return buildDeterministicRelease(targetTag), nil
	}

	url := fmt.Sprintf("https://api.github.com/repos/Dushnilin/tachyon/releases/tags/%s", trimmed)
	var rel TachyonRelease
	_, err := client.FetchJSON(ctx, url, &rel)
	if err == nil && rel.TagName != "" {
		return &rel, nil
	}

	// Fallback: scrape expanded_assets
	scraped, sErr := client.ScrapeExpandedAssets(ctx, "Dushnilin/tachyon", trimmed)
	if sErr == nil && len(scraped) > 0 {
		rel.TagName = trimmed
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
	return buildDeterministicRelease(trimmed), nil
}

func pickBestV2BetaRelease(rels []TachyonRelease) (*TachyonRelease, bool) {
	var firstV2Pre *TachyonRelease
	var firstAny *TachyonRelease
	for i := range rels {
		r := &rels[i]
		if r.Draft || r.TagName == "" {
			continue
		}
		if firstAny == nil {
			firstAny = r
		}
		if IsV2Tag(r.TagName) {
			if !r.Prerelease && !IsPrereleaseTag(r.TagName) {
				return r, true
			}
			if firstV2Pre == nil {
				firstV2Pre = r
			}
		}
	}
	if firstV2Pre != nil {
		return firstV2Pre, true
	}
	if firstAny != nil {
		return firstAny, true
	}
	return nil, false
}

func buildDeterministicRelease(tag string) *TachyonRelease {
	cleanVer := strings.TrimPrefix(tag, "v")
	rel := &TachyonRelease{
		TagName:    tag,
		Prerelease: IsPrereleaseTag(tag),
	}
	var names []string
	if IsV2Tag(tag) {
		for _, arch := range tachyon2SupportedArchs {
			names = append(names,
				fmt.Sprintf("tachyon-%s-v%s.ipk", arch, cleanVer),
				fmt.Sprintf("tachyon-%s-v%s.apk", arch, cleanVer),
			)
		}
		names = append(names, "SHA256SUMS")
	} else {
		names = []string{
			fmt.Sprintf("tachyon_%s.ipk", cleanVer),
			fmt.Sprintf("tachyon_%s.apk", cleanVer),
			fmt.Sprintf("luci-app-tachyon_%s.ipk", cleanVer),
			fmt.Sprintf("luci-app-tachyon_%s.apk", cleanVer),
			fmt.Sprintf("luci-i18n-tachyon-ru_%s.ipk", cleanVer),
			fmt.Sprintf("luci-i18n-tachyon-ru_%s.apk", cleanVer),
			"sha256sums.txt",
		}
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

// ResolveTachyonAssets identifies the matching .ipk or .apk assets in a 1.x release (legacy signature).
func ResolveTachyonAssets(rel *TachyonRelease, isAPK bool, needI18n bool) (*TachyonAssetsToDownload, error) {
	return ResolveTachyonAssetsForArch(rel, isAPK, needI18n, "", "")
}

// ResolveTachyonAssetsForArch identifies matching .ipk or .apk assets for both Tachyon 1.x and 2.0+.
func ResolveTachyonAssetsForArch(rel *TachyonRelease, isAPK bool, needI18n bool, distribArch, rawArch string) (*TachyonAssetsToDownload, error) {
	ext := "ipk"
	if isAPK {
		ext = "apk"
	}

	cleanVer := strings.TrimPrefix(rel.TagName, "v")

	isV2 := IsV2Tag(rel.TagName)
	if !isV2 {
		hasLuci := false
		hasArchPkg := false
		for _, a := range rel.Assets {
			if strings.HasPrefix(a.Name, "luci-app-tachyon_") {
				hasLuci = true
			}
			if strings.HasPrefix(a.Name, "tachyon-") && strings.HasSuffix(a.Name, "."+ext) {
				hasArchPkg = true
			}
		}
		if hasArchPkg && !hasLuci {
			isV2 = true
		}
	}

	if isV2 {
		arch2 := MapToTachyon2Arch(distribArch, rawArch)
		res := &TachyonAssetsToDownload{
			Version: cleanVer,
			IsAPK:   isAPK,
			IsV2:    true,
		}
		exactName := fmt.Sprintf("tachyon-%s-v%s.%s", arch2, cleanVer, ext)
		archPrefix := fmt.Sprintf("tachyon-%s-", arch2)

		for _, a := range rel.Assets {
			name := a.Name
			if name == exactName {
				res.BackendURL = a.BrowserDownloadURL
				res.BackendName = name
			} else if res.BackendURL == "" && strings.HasPrefix(name, archPrefix) && strings.HasSuffix(name, "."+ext) {
				res.BackendURL = a.BrowserDownloadURL
				res.BackendName = name
			} else if name == "SHA256SUMS" || (res.SHA256URL == "" && name == "sha256sums.txt") {
				res.SHA256URL = a.BrowserDownloadURL
				res.SHA256Name = name
			}
		}

		if res.BackendURL == "" {
			res.BackendName = exactName
			res.BackendURL = fmt.Sprintf("https://github.com/Dushnilin/tachyon/releases/download/%s/%s", rel.TagName, res.BackendName)
		}
		if res.SHA256URL == "" {
			res.SHA256Name = "SHA256SUMS"
			res.SHA256URL = fmt.Sprintf("https://github.com/Dushnilin/tachyon/releases/download/%s/SHA256SUMS", rel.TagName)
		}
		return res, nil
	}

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
		} else if name == "sha256sums.txt" || (res.SHA256URL == "" && name == "SHA256SUMS") {
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

	// 1. Download sha256sums.txt / SHA256SUMS if available
	var sha256Map map[string]string
	if assets.SHA256URL != "" {
		sumsName := assets.SHA256Name
		if sumsName == "" {
			sumsName = "sha256sums.txt"
		}
		subTaskFn("Скачивание контрольных сумм...", 0.1)
		shaPath := filepath.Join(destDir, sumsName)
		_, err := client.DownloadFile(ctx, assets.SHA256URL, shaPath, sumsName, nil)
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
