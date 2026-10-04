package downloader

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// EngineType represents the routing engine to install.
type EngineType string

const (
	EngineSingBoxExtended EngineType = "sing-box-extended"
	EngineSteerExtended   EngineType = "steer-extended"
	EngineSteer           EngineType = "steer"
	EngineSingBoxLX       EngineType = "sing-box-lx"
	EngineSkip            EngineType = "skip"
)

// EngineDownload holds resolved information for an engine asset.
type EngineDownload struct {
	EngineType EngineType
	Version    string
	URL        string
	Filename   string
	IsPackage  bool // true if .ipk or .apk, false if tar.gz binary archive
}

// ResolveEngineAsset finds the best matching asset for the router architecture.
func ResolveEngineAsset(
	ctx context.Context,
	client *Client,
	engine EngineType,
	distribArch string,
	rawArch string,
	isAPK bool,
) (*EngineDownload, error) {
	if engine == EngineSkip || engine == "" {
		return &EngineDownload{EngineType: EngineSkip}, nil
	}

	ext := "ipk"
	if isAPK {
		ext = "apk"
	}

	switch engine {
	case EngineSingBoxExtended:
		return resolveSingBoxExtended(ctx, client, distribArch, rawArch, ext)
	case EngineSteerExtended:
		return resolveSteer(ctx, client, true, distribArch, ext)
	case EngineSteer:
		return resolveSteer(ctx, client, false, distribArch, ext)
	case EngineSingBoxLX:
		return resolveSingBoxLX(ctx, client, distribArch, rawArch, ext)
	default:
		return nil, fmt.Errorf("unsupported engine type: %s", engine)
	}
}

func resolveSingBoxExtended(ctx context.Context, client *Client, distribArch, rawArch, ext string) (*EngineDownload, error) {
	repo := "shtorm-7/sing-box-extended"
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
		// Try HTML scraping fallback
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

	normArch := NormalizeArch(rawArch)
	// Try matching OpenWrt package:
	// 1. Exact distribArch: sing-box-extended_.*_openwrt_<distribArch>.<ext>
	for _, a := range rel.Assets {
		if strings.HasPrefix(a.Name, "sing-box-extended_") &&
			strings.HasSuffix(a.Name, "."+ext) &&
			strings.Contains(a.Name, "_openwrt_"+distribArch+".") {
			return &EngineDownload{
				EngineType: EngineSingBoxExtended,
				Version:    rel.TagName,
				URL:        a.BrowserDownloadURL,
				Filename:   a.Name,
				IsPackage:  true,
			}, nil
		}
	}

	// 2. Normalized arch in openwrt package name (e.g. openwrt_x86_64 or aarch64_generic)
	for _, a := range rel.Assets {
		if strings.HasPrefix(a.Name, "sing-box-extended_") &&
			strings.HasSuffix(a.Name, "."+ext) &&
			strings.Contains(a.Name, "_openwrt_") &&
			archMatches(a.Name, distribArch, normArch) {
			return &EngineDownload{
				EngineType: EngineSingBoxExtended,
				Version:    rel.TagName,
				URL:        a.BrowserDownloadURL,
				Filename:   a.Name,
				IsPackage:  true,
			}, nil
		}
	}

	// 3. Fallback: compressed tar.gz binary archive
	// e.g. sing-box-extended_.*_linux-<normArch>-compressed.tar.gz
	for _, a := range rel.Assets {
		if strings.HasPrefix(a.Name, "sing-box-extended_") &&
			strings.Contains(a.Name, "linux-"+normArch) &&
			strings.HasSuffix(a.Name, ".tar.gz") {
			return &EngineDownload{
				EngineType: EngineSingBoxExtended,
				Version:    rel.TagName,
				URL:        a.BrowserDownloadURL,
				Filename:   a.Name,
				IsPackage:  false,
			}, nil
		}
	}

	return nil, fmt.Errorf("sing-box-extended package not found for arch %s / %s", distribArch, normArch)
}

func resolveSteer(ctx context.Context, client *Client, extended bool, distribArch, ext string) (*EngineDownload, error) {
	repo := "xyzmean/steer"
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
			return nil, fmt.Errorf("failed to fetch steer release: %v", err)
		}
	}

	prefix := "steer-"
	targetEngine := EngineSteer
	if extended {
		prefix = "steer-extended-"
		targetEngine = EngineSteerExtended
	}

	suffix := "_" + distribArch + "." + ext

	for _, a := range rel.Assets {
		name := a.Name
		if !extended && strings.HasPrefix(name, "steer-extended-") {
			continue // skip extended when looking for standard
		}
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			return &EngineDownload{
				EngineType: targetEngine,
				Version:    rel.TagName,
				URL:        a.BrowserDownloadURL,
				Filename:   name,
				IsPackage:  true,
			}, nil
		}
	}

	// Fallback: match without exact sub-arch if generic
	for _, a := range rel.Assets {
		name := a.Name
		if !extended && strings.HasPrefix(name, "steer-extended-") {
			continue
		}
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, "."+ext) &&
			archMatches(name, distribArch, NormalizeArch(distribArch)) {
			return &EngineDownload{
				EngineType: targetEngine,
				Version:    rel.TagName,
				URL:        a.BrowserDownloadURL,
				Filename:   name,
				IsPackage:  true,
			}, nil
		}
	}

	return nil, fmt.Errorf("%s package not found for arch %s (OpenWrt .%s)", prefix, distribArch, ext)
}

func resolveSingBoxLX(ctx context.Context, client *Client, distribArch, rawArch, ext string) (*EngineDownload, error) {
	repo := "Leadaxe/sing-box-lx"
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

	normArch := NormalizeArch(rawArch)
	for _, a := range rel.Assets {
		if archMatches(a.Name, distribArch, normArch) {
			isPkg := strings.HasSuffix(a.Name, "."+ext)
			return &EngineDownload{
				EngineType: EngineSingBoxLX,
				Version:    rel.TagName,
				URL:        a.BrowserDownloadURL,
				Filename:   a.Name,
				IsPackage:  isPkg,
			}, nil
		}
	}

	return nil, fmt.Errorf("sing-box-lx package not found for arch %s", distribArch)
}

// NormalizeArch converts OpenWrt architecture variations to common linux GOARCH names.
func NormalizeArch(arch string) string {
	a := strings.ToLower(arch)
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
	if strings.Contains(a, "arm_cortex") || strings.Contains(a, "armv7") || strings.Contains(a, "arm") {
		return "armv7"
	}
	if strings.Contains(a, "i386") || strings.Contains(a, "x86") {
		return "386"
	}
	return a
}

func archMatches(name, distribArch, normArch string) bool {
	n := strings.ToLower(name)
	d := strings.ToLower(distribArch)

	if d != "" && strings.Contains(n, d) {
		return true
	}
	switch normArch {
	case "amd64":
		return strings.Contains(n, "x86_64") || strings.Contains(n, "amd64")
	case "arm64":
		return strings.Contains(n, "aarch64") || strings.Contains(n, "arm64")
	case "mipsle":
		return strings.Contains(n, "mipsel") || strings.Contains(n, "mipsle") || strings.Contains(n, "24kc")
	case "mips":
		return strings.Contains(n, "mips_24kc") || (strings.Contains(n, "mips") && !strings.Contains(n, "mipsel") && !strings.Contains(n, "mipsle"))
	case "armv7":
		return strings.Contains(n, "arm_cortex") || strings.Contains(n, "armv7") || strings.Contains(n, "armhf")
	}
	return false
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
	logFn(fmt.Sprintf("[#cbd5e1]⚡ Скачивание ядра %s: [#38bdf8]%s[-]...[-]\n", engine.EngineType, engine.Filename))

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

	// If tar.gz archive was downloaded, extract binary locally if needed or keep for upload
	if strings.HasSuffix(destPath, ".tar.gz") {
		logFn("[#cbd5e1]Распаковка архива ядра...[-]\n")
		binPath, err := extractTarGzBinary(destPath, destDir, "sing-box")
		if err == nil {
			logFn(fmt.Sprintf("[#22c55e]✓ Бинарный файл ядра распакован: %s[-]\n", filepath.Base(binPath)))
		}
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

		if header.Typeflag == tar.TypeReg && (header.Name == binName || strings.HasSuffix(header.Name, "/"+binName)) {
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
