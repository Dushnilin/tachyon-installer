package downloader

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// EngineType represents the routing engine to install.
type EngineType string

const (
	EngineTachyonCore EngineType = "tachyon-core"
	EngineSkip        EngineType = "skip"
)

// NormalizeEngine maps legacy engine identifiers to supported Tachyon 1.8.0+ engines.
func NormalizeEngine(engine string) EngineType {
	e := strings.ToLower(strings.TrimSpace(engine))
	switch e {
	case "", "tachyon-core", "core", "sing-box", "sing-box-extended", "sing-box-extended-compressed", "sing-box-tiny", "sing-box-lx":
		return EngineTachyonCore
	case "skip":
		return EngineSkip
	default:
		return EngineType(e)
	}
}

// EngineDownload holds resolved information for an engine asset.
type EngineDownload struct {
	EngineType EngineType
	Version    string
	URL        string
	Filename   string
	IsPackage  bool // true if .ipk or .apk, false if tar.gz binary archive
}

// ResolveEngineAssets finds all matching assets for the router architecture.
func ResolveEngineAssets(
	ctx context.Context,
	client *Client,
	engine EngineType,
	distribArch string,
	rawArch string,
	isAPK bool,
) ([]*EngineDownload, error) {
	if engine == EngineSkip || engine == "" {
		return []*EngineDownload{{EngineType: EngineSkip}}, nil
	}

	ext := "ipk"
	if isAPK {
		ext = "apk"
	}

	switch NormalizeEngine(string(engine)) {
	case EngineTachyonCore:
		return resolveTachyonCore(ctx, client, distribArch, rawArch, ext)
	default:
		return nil, fmt.Errorf("unsupported engine type: %s", engine)
	}
}

// ResolveEngineAsset finds the best matching asset for the router architecture (returns the primary asset).
func ResolveEngineAsset(
	ctx context.Context,
	client *Client,
	engine EngineType,
	distribArch string,
	rawArch string,
	isAPK bool,
) (*EngineDownload, error) {
	assets, err := ResolveEngineAssets(ctx, client, engine, distribArch, rawArch, isAPK)
	if err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("no engine assets found for engine: %s", engine)
	}
	return assets[0], nil
}

func getArchCandidates(distribArch, normArch string) []string {
	var candidates []string
	cleanDist := strings.ToLower(strings.TrimSpace(distribArch))
	if cleanDist != "" {
		candidates = append(candidates, cleanDist)
	}

	switch normArch {
	case "arm64":
		genericList := []string{"aarch64_generic", "aarch64_cortex-a53", "aarch64_cortex-a72", "aarch64_cortex-a76"}
		for _, c := range genericList {
			if !slices.Contains(candidates, c) {
				candidates = append(candidates, c)
			}
		}
	case "amd64":
		if !slices.Contains(candidates, "x86_64") {
			candidates = append(candidates, "x86_64")
		}
	case "mipsle":
		genericList := []string{"mipsel_24kc", "mipsel_24kf", "mipsel_74kc", "mipsel_mips32", "mipsel"}
		for _, c := range genericList {
			if !slices.Contains(candidates, c) {
				candidates = append(candidates, c)
			}
		}
	case "mips":
		genericList := []string{"mips_24kc", "mips_4kec", "mips_mips32", "mips"}
		for _, c := range genericList {
			if !slices.Contains(candidates, c) {
				candidates = append(candidates, c)
			}
		}
	case "armv7", "arm":
		genericList := []string{"arm_cortex-a7_neon-vfpv4", "arm_cortex-a9_neon", "arm_cortex-a7", "arm_cortex-a9", "arm_cortex-a15_neon-vfpv4"}
		for _, c := range genericList {
			if !slices.Contains(candidates, c) {
				candidates = append(candidates, c)
			}
		}
	case "riscv64":
		genericList := []string{"riscv64_generic", "riscv64", "riscv64gc"}
		for _, c := range genericList {
			if !slices.Contains(candidates, c) {
				candidates = append(candidates, c)
			}
		}
	case "armv6":
		genericList := []string{"arm_arm1176jzf-s_vfp", "arm_mpcore", "armv6"}
		for _, c := range genericList {
			if !slices.Contains(candidates, c) {
				candidates = append(candidates, c)
			}
		}
	case "386":
		genericList := []string{"i386_pentium4", "x86"}
		for _, c := range genericList {
			if !slices.Contains(candidates, c) {
				candidates = append(candidates, c)
			}
		}
	}
	return candidates
}

func isForeignPlatformAsset(lowerName string) bool {
	return strings.Contains(lowerName, "darwin") ||
		strings.Contains(lowerName, "windows") ||
		strings.Contains(lowerName, "android") ||
		strings.Contains(lowerName, "ios")
}

func tachyonCoreArchMatches(lowerName, normArch, distribArch string) bool {
	switch normArch {
	case "arm64":
		return strings.Contains(lowerName, "aarch64") || strings.Contains(lowerName, "arm64")
	case "amd64":
		return strings.Contains(lowerName, "linux-amd64") || strings.Contains(lowerName, "x86_64") || strings.Contains(lowerName, "-amd64")
	case "mipsle":
		return strings.Contains(lowerName, "mipsel") || strings.Contains(lowerName, "mipsle")
	case "mips":
		return (strings.Contains(lowerName, "-mips-") || strings.Contains(lowerName, "-mips.") || strings.Contains(lowerName, "_mips") || strings.Contains(lowerName, "linux-mips")) &&
			!strings.Contains(lowerName, "mipsel") && !strings.Contains(lowerName, "mipsle")
	case "armv7", "arm":
		return strings.Contains(lowerName, "armv7") || strings.Contains(lowerName, "armhf") || strings.Contains(lowerName, "arm_cortex")
	case "armv6":
		return strings.Contains(lowerName, "armv6") || strings.Contains(lowerName, "arm_arm11")
	case "riscv64":
		return strings.Contains(lowerName, "riscv64")
	case "386":
		return (strings.Contains(lowerName, "i386") || strings.Contains(lowerName, "i686") || strings.Contains(lowerName, "386") || strings.Contains(lowerName, "x86")) && !strings.Contains(lowerName, "x86_64")
	}

	d := strings.ToLower(distribArch)
	return d != "" && strings.Contains(lowerName, d)
}

func isTachyonCorePackageMatch(name, normArch, distribArch, ext string) bool {
	n := strings.ToLower(name)
	if !strings.HasPrefix(n, "tachyon-core") || !strings.HasSuffix(n, "."+strings.ToLower(ext)) {
		return false
	}
	if isForeignPlatformAsset(n) {
		return false
	}
	return tachyonCoreArchMatches(n, normArch, distribArch)
}

func isTachyonCoreArchiveMatch(name, normArch, distribArch string) bool {
	n := strings.ToLower(name)
	if !strings.HasPrefix(n, "tachyon-core") {
		return false
	}
	if !strings.HasSuffix(n, ".tar.gz") && !strings.HasSuffix(n, ".tgz") {
		return false
	}
	if isForeignPlatformAsset(n) {
		return false
	}
	return tachyonCoreArchMatches(n, normArch, distribArch)
}

func resolveTachyonCore(ctx context.Context, client *Client, distribArch, rawArch, ext string) ([]*EngineDownload, error) {
	repo := "Dushnilin/tachyon-core"
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	var rel struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	_, err := client.FetchJSON(ctx, apiURL, &rel)
	if err != nil || len(rel.Assets) == 0 {
		scraped, sErr := client.ScrapeExpandedAssets(ctx, repo, "latest")
		if sErr == nil && len(scraped) > 0 {
			rel.TagName = "latest"
			for name, u := range scraped {
				rel.Assets = append(rel.Assets, struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
				}{Name: name, BrowserDownloadURL: u})
			}
		} else {
			return nil, fmt.Errorf("failed to fetch %s release: %v", repo, err)
		}
	}

	var assets []ReleaseAsset
	for _, a := range rel.Assets {
		assets = append(assets, ReleaseAsset{
			Name:               a.Name,
			BrowserDownloadURL: a.BrowserDownloadURL,
		})
	}

	return FilterTachyonCoreAssets(assets, distribArch, rawArch, ext, rel.TagName)
}

// FilterTachyonCoreAssets matches the best tachyon-core package or binary archive for the router architecture.
func FilterTachyonCoreAssets(assets []ReleaseAsset, distribArch, rawArch, ext, tagName string) ([]*EngineDownload, error) {
	normArch := NormalizeArch(rawArch)
	if normArch == "" {
		normArch = NormalizeArch(distribArch)
	}

	// 1. Check classic OpenWrt package naming (tachyon-core_*_<arch>.<ext>)
	candidates := getArchCandidates(distribArch, normArch)
	for _, cand := range candidates {
		suffix := "_" + cand + "." + ext
		for _, a := range assets {
			lowerName := strings.ToLower(a.Name)
			if !isForeignPlatformAsset(lowerName) && strings.HasPrefix(lowerName, "tachyon-core") && strings.HasSuffix(lowerName, suffix) {
				return []*EngineDownload{{
					EngineType: EngineTachyonCore,
					Version:    tagName,
					URL:        a.BrowserDownloadURL,
					Filename:   a.Name,
					IsPackage:  true,
				}}, nil
			}
		}
	}

	// 2. Check tachyon-core native package naming (e.g. tachyon-core-aarch64-v0.5.8.apk / .ipk)
	for _, a := range assets {
		if isTachyonCorePackageMatch(a.Name, normArch, distribArch, ext) {
			return []*EngineDownload{{
				EngineType: EngineTachyonCore,
				Version:    tagName,
				URL:        a.BrowserDownloadURL,
				Filename:   a.Name,
				IsPackage:  true,
			}}, nil
		}
	}

	// 3. Binary archive match (.tar.gz):
	for _, a := range assets {
		if isTachyonCoreArchiveMatch(a.Name, normArch, distribArch) {
			return []*EngineDownload{{
				EngineType: EngineTachyonCore,
				Version:    tagName,
				URL:        a.BrowserDownloadURL,
				Filename:   a.Name,
				IsPackage:  false,
			}}, nil
		}
	}

	return nil, fmt.Errorf("tachyon-core binary archive not found for arch %s / %s in release %s", distribArch, normArch, tagName)
}

// ReleaseAsset represents a GitHub release asset or scraped asset.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// NormalizeArch converts OpenWrt architecture variations to common linux GOARCH names.
func NormalizeArch(arch string) string {
	a := strings.ToLower(strings.TrimSpace(arch))
	if strings.Contains(a, "x86_64") || strings.Contains(a, "amd64") {
		return "amd64"
	}
	if strings.Contains(a, "aarch64") || strings.Contains(a, "arm64") {
		return "arm64"
	}
	if strings.Contains(a, "mipsel") || strings.Contains(a, "mipsle") {
		return "mipsle"
	}
	if strings.Contains(a, "mips") {
		return "mips"
	}
	if strings.Contains(a, "armv6") || strings.Contains(a, "arm11") {
		return "armv6"
	}
	if strings.Contains(a, "arm_cortex") || strings.Contains(a, "armv7") || strings.Contains(a, "arm") {
		return "armv7"
	}
	if strings.Contains(a, "riscv64") {
		return "riscv64"
	}
	if strings.Contains(a, "i386") || strings.Contains(a, "i686") || strings.Contains(a, "x86") {
		return "386"
	}
	return a
}

// DownloadEnginePackage downloads the resolved engine asset to destDir.
func DownloadEnginePackage(
	ctx context.Context,
	client *Client,
	engine *EngineDownload,
	destDir string,
	logFn func(msg string),
	subTaskFn func(label string, frac float64),
) (string, error) {
	if engine == nil || engine.EngineType == EngineSkip || engine.URL == "" {
		return "", nil
	}

	destPath := filepath.Join(destDir, engine.Filename)
	logFn(fmt.Sprintf("[#cbd5e1]Скачивание ядра %s: [#38bdf8]%s[-]...[-]\n", engine.EngineType, engine.Filename))

	usedMirror, err := client.DownloadFile(ctx, engine.URL, destPath, engine.Filename, func(downloaded, total, speed int64) {
		dlMB := float64(downloaded) / 1024 / 1024
		speedKB := float64(speed) / 1024
		if total > 0 {
			totalMB := float64(total) / 1024 / 1024
			pct := float64(downloaded) / float64(total)
			subTaskFn(fmt.Sprintf("Скачивание ядра: %.1f/%.1f МБ (%.0f%%, %.0f КБ/с)", dlMB, totalMB, pct*100, speedKB), pct*0.95)
		} else {
			subTaskFn(fmt.Sprintf("Скачивание ядра: %.1f МБ (%.0f КБ/с)", dlMB, speedKB), 0.5)
		}
	})
	if err != nil {
		return "", fmt.Errorf("ошибка загрузки ядра %s: %w", engine.Filename, err)
	}

	logFn(fmt.Sprintf("[#22c55e]✓ Ядро %s успешно скачано (зеркало: %s)[-]\n", engine.Filename, usedMirror))

	// If tar.gz archive was downloaded, extract binary locally and remove the archive
	if strings.HasSuffix(destPath, ".tar.gz") || strings.HasSuffix(destPath, ".tgz") {
		logFn("[#cbd5e1]Распаковка архива ядра...[-]\n")
		binPath, err := extractTarGzBinary(destPath, destDir, "tachyon-core")
		if err == nil {
			logFn(fmt.Sprintf("[#22c55e]✓ Бинарный файл ядра распакован: %s[-]\n", filepath.Base(binPath)))
			_ = os.Remove(destPath)
			return binPath, nil
		}
		logFn(fmt.Sprintf("[#eab308]! Не удалось распаковать архив локально: %v[-]\n", err))
	}

	return destPath, nil
}

// extractTarGzBinary extracts a named binary from a .tar.gz archive.
func extractTarGzBinary(tarGzPath, destDir, binName string) (string, error) {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	targetPath := filepath.Join(destDir, binName)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		baseName := filepath.Base(header.Name)
		lowerBase := strings.ToLower(baseName)
		if strings.HasSuffix(lowerBase, ".sha256") ||
			strings.HasSuffix(lowerBase, ".sig") ||
			strings.HasSuffix(lowerBase, ".txt") ||
			strings.HasSuffix(lowerBase, ".md") ||
			strings.HasSuffix(lowerBase, ".json") ||
			lowerBase == "license" {
			continue
		}

		isMatch := false
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA || header.Typeflag == 0 {
			if header.Name == binName || strings.HasSuffix(header.Name, "/"+binName) || baseName == binName {
				isMatch = true
			} else if binName == "tachyon-core" && strings.HasPrefix(baseName, "tachyon-core") {
				isMatch = true
			}
		}

		if isMatch {
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return "", err
			}
			_, copyErr := io.Copy(out, tr)
			out.Close()
			return targetPath, copyErr
		}
	}

	return "", fmt.Errorf("binary %s not found in archive", binName)
}
