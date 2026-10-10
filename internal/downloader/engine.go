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
	EngineTachyonCore               EngineType = "tachyon-core"
	EngineSingBoxExtended           EngineType = "sing-box-extended"
	EngineSingBoxExtendedCompressed EngineType = "sing-box-extended-compressed"
	EngineSingBoxTiny               EngineType = "sing-box-tiny"
	EngineSingBoxLX                 EngineType = "sing-box-lx"
	EngineSingBoxStable             EngineType = "sing-box"
	EngineSteerExtended             EngineType = "steer-extended"
	EngineSteer                     EngineType = "steer"
	EngineSkip                      EngineType = "skip"
)

// EngineDownload holds resolved information for an engine asset.
type EngineDownload struct {
	EngineType EngineType
	Version    string
	URL        string
	Filename   string
	IsPackage  bool // true if .ipk or .apk, false if tar.gz binary archive
}

// ResolveEngineAssets finds all matching assets for the router architecture.
// For modular engines like Steer 2.0+, this returns the core package and all protocol modules.
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

	switch engine {
	case EngineTachyonCore:
		return resolveTachyonCore(ctx, client, distribArch, rawArch, ext)
	case EngineSingBoxExtended:
		return resolveSingBoxExtended(ctx, client, distribArch, rawArch, ext, false)
	case EngineSingBoxExtendedCompressed:
		return resolveSingBoxExtended(ctx, client, distribArch, rawArch, ext, true)
	case EngineSingBoxTiny:
		return resolveSingBoxTiny(ctx, client, distribArch, rawArch, ext)
	case EngineSingBoxStable:
		return resolveSingBoxStable(ctx, client, distribArch, rawArch, ext)
	case EngineSteerExtended:
		return resolveSteer(ctx, client, true, distribArch, rawArch, ext)
	case EngineSteer:
		return resolveSteer(ctx, client, false, distribArch, rawArch, ext)
	case EngineSingBoxLX:
		return resolveSingBoxLX(ctx, client, distribArch, rawArch, ext)
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
		genericList := []string{"riscv64", "riscv64gc"}
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

func isLinuxBinaryArchiveMatch(name, normArch string) bool {
	n := strings.ToLower(name)
	if !strings.HasSuffix(n, ".tar.gz") && !strings.HasSuffix(n, ".tgz") {
		return false
	}
	if !strings.Contains(n, "linux") {
		return false
	}

	switch normArch {
	case "arm64":
		return strings.Contains(n, "linux-arm64") || strings.Contains(n, "linux-aarch64")
	case "amd64":
		return strings.Contains(n, "linux-amd64") || strings.Contains(n, "linux-x86_64")
	case "mipsle":
		return strings.Contains(n, "linux-mipsle") || strings.Contains(n, "linux-mipsel")
	case "mips":
		return strings.Contains(n, "linux-mips") && !strings.Contains(n, "mipsle") && !strings.Contains(n, "mipsel")
	case "armv7", "arm":
		return strings.Contains(n, "linux-armv7") || strings.Contains(n, "linux-armhf") || strings.Contains(n, "linux-arm")
	case "386":
		return strings.Contains(n, "linux-386") || strings.Contains(n, "linux-x86")
	}
	return false
}

func isTachyonCoreArchiveMatch(name, normArch, distribArch string) bool {
	n := strings.ToLower(name)
	if !strings.HasSuffix(n, ".tar.gz") && !strings.HasSuffix(n, ".tgz") {
		return false
	}
	if strings.Contains(n, "darwin") || strings.Contains(n, "windows") {
		return false
	}

	switch normArch {
	case "arm64":
		return strings.Contains(n, "aarch64") || strings.Contains(n, "arm64")
	case "amd64":
		return strings.Contains(n, "linux-amd64") || strings.Contains(n, "x86_64") || strings.Contains(n, "-amd64")
	case "mipsle":
		return strings.Contains(n, "mipsel") || strings.Contains(n, "mipsle")
	case "mips":
		return (strings.Contains(n, "-mips-") || strings.Contains(n, "-mips.") || strings.Contains(n, "_mips") || strings.Contains(n, "linux-mips")) &&
			!strings.Contains(n, "mipsel") && !strings.Contains(n, "mipsle")
	case "armv7", "arm":
		return strings.Contains(n, "armv7") || strings.Contains(n, "armhf")
	case "armv6":
		return strings.Contains(n, "armv6")
	case "riscv64":
		return strings.Contains(n, "riscv64")
	case "386":
		return (strings.Contains(n, "386") || strings.Contains(n, "x86")) && !strings.Contains(n, "x86_64")
	}

	d := strings.ToLower(distribArch)
	if d != "" && strings.Contains(n, d) {
		return true
	}

	return false
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

	// 1. Check if OpenWrt package exists (e.g. tachyon-core_*_<arch>.<ext>)
	candidates := getArchCandidates(distribArch, normArch)
	for _, cand := range candidates {
		suffix := "_" + cand + "." + ext
		for _, a := range assets {
			if strings.HasPrefix(a.Name, "tachyon-core") && strings.HasSuffix(a.Name, suffix) {
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

	// 2. Binary archive match (.tar.gz):
	for _, a := range assets {
		if !strings.HasPrefix(a.Name, "tachyon-core") {
			continue
		}
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

func resolveSingBoxExtended(ctx context.Context, client *Client, distribArch, rawArch, ext string, compressedOnly bool) ([]*EngineDownload, error) {
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
	if normArch == "" {
		normArch = NormalizeArch(distribArch)
	}

	if compressedOnly {
		// Priority: compressed tar.gz binary archive
		for _, a := range rel.Assets {
			if strings.HasPrefix(a.Name, "sing-box-") &&
				strings.Contains(a.Name, "-compressed") &&
				isLinuxBinaryArchiveMatch(a.Name, normArch) {
				return []*EngineDownload{{
					EngineType: EngineSingBoxExtendedCompressed,
					Version:    rel.TagName,
					URL:        a.BrowserDownloadURL,
					Filename:   a.Name,
					IsPackage:  false,
				}}, nil
			}
		}
		return nil, fmt.Errorf("sing-box-extended-compressed archive not found for arch %s / %s", distribArch, normArch)
	}

	candidates := getArchCandidates(distribArch, normArch)

	// 1. Try matching OpenWrt package: sing-box-extended_.*_openwrt_<arch>.<ext>
	for _, cand := range candidates {
		suffix := "_openwrt_" + cand + "." + ext
		for _, a := range rel.Assets {
			if strings.HasPrefix(a.Name, "sing-box-extended_") && strings.HasSuffix(a.Name, suffix) {
				return []*EngineDownload{{
					EngineType: EngineSingBoxExtended,
					Version:    rel.TagName,
					URL:        a.BrowserDownloadURL,
					Filename:   a.Name,
					IsPackage:  true,
				}}, nil
			}
		}
	}

	// 2. Fallback: compressed tar.gz binary archive
	for _, a := range rel.Assets {
		if strings.HasPrefix(a.Name, "sing-box-") &&
			strings.Contains(a.Name, "-compressed") &&
			isLinuxBinaryArchiveMatch(a.Name, normArch) {
			return []*EngineDownload{{
				EngineType: EngineSingBoxExtended,
				Version:    rel.TagName,
				URL:        a.BrowserDownloadURL,
				Filename:   a.Name,
				IsPackage:  false,
			}}, nil
		}
	}
	for _, a := range rel.Assets {
		if strings.HasPrefix(a.Name, "sing-box-") &&
			isLinuxBinaryArchiveMatch(a.Name, normArch) {
			return []*EngineDownload{{
				EngineType: EngineSingBoxExtended,
				Version:    rel.TagName,
				URL:        a.BrowserDownloadURL,
				Filename:   a.Name,
				IsPackage:  false,
			}}, nil
		}
	}

	return nil, fmt.Errorf("sing-box-extended package not found for arch %s / %s", distribArch, normArch)
}

func resolveSingBoxTiny(ctx context.Context, client *Client, distribArch, rawArch, ext string) ([]*EngineDownload, error) {
	return []*EngineDownload{{
		EngineType: EngineSingBoxTiny,
		Version:    "openwrt",
		URL:        "",
		Filename:   "sing-box-tiny (через менеджер пакетов роутера)",
		IsPackage:  true,
	}}, nil
}

func resolveSingBoxStable(ctx context.Context, client *Client, distribArch, rawArch, ext string) ([]*EngineDownload, error) {
	return []*EngineDownload{{
		EngineType: EngineSingBoxStable,
		Version:    "openwrt",
		URL:        "",
		Filename:   "sing-box (через менеджер пакетов роутера)",
		IsPackage:  true,
	}}, nil
}

func resolveSteer(ctx context.Context, client *Client, extended bool, distribArch, rawArch, ext string) ([]*EngineDownload, error) {
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

	var assets []ReleaseAsset
	for _, a := range rel.Assets {
		assets = append(assets, ReleaseAsset{
			Name:               a.Name,
			BrowserDownloadURL: a.BrowserDownloadURL,
		})
	}

	return FilterSteerAssets(assets, extended, distribArch, rawArch, ext, rel.TagName)
}

// ReleaseAsset represents a GitHub release asset or scraped asset.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// FilterSteerAssets matches and collects all required Steer packages for an architecture.
// In Steer 2.0+, packages are modular (steer-core, steer-vless, steer-hysteria2, etc.).
func FilterSteerAssets(assets []ReleaseAsset, extended bool, distribArch, rawArch, ext, tagName string) ([]*EngineDownload, error) {
	targetEngine := EngineSteer
	if extended {
		targetEngine = EngineSteerExtended
	}

	normArch := NormalizeArch(rawArch)
	if normArch == "" {
		normArch = NormalizeArch(distribArch)
	}
	candidates := getArchCandidates(distribArch, normArch)

	// 1. Find the best matching architecture candidate that exists in release assets
	var matchedCand string
	for _, cand := range candidates {
		suffix := "_" + cand + "." + ext
		for _, a := range assets {
			name := a.Name
			// Matches steer-core (2.0+) or steer- (1.x / monolithic)
			if (strings.HasPrefix(name, "steer-core-") || strings.HasPrefix(name, "steer-")) &&
				strings.HasSuffix(name, suffix) {
				if !extended && strings.HasPrefix(name, "steer-extended-") {
					continue
				}
				matchedCand = cand
				break
			}
		}
		if matchedCand != "" {
			break
		}
	}

	if matchedCand == "" {
		return nil, fmt.Errorf("steer package not found for arch %s (candidates: %s, ext: .%s)",
			distribArch, strings.Join(candidates, ", "), ext)
	}

	suffix := "_" + matchedCand + "." + ext
	var results []*EngineDownload

	isModular := false
	for _, a := range assets {
		if strings.HasPrefix(a.Name, "steer-core-") && strings.HasSuffix(a.Name, suffix) {
			isModular = true
			break
		}
	}

	for _, a := range assets {
		name := a.Name
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		if !strings.HasPrefix(name, "steer-") {
			continue
		}

		if isModular {
			// Steer 2.0+: modular suite
			if !extended && strings.HasPrefix(name, "steer-extended-") {
				continue
			}
		} else {
			// Steer 1.x: monolithic packages
			if extended && !strings.HasPrefix(name, "steer-extended-") {
				continue
			}
			if !extended && strings.HasPrefix(name, "steer-extended-") {
				continue
			}
		}

		results = append(results, &EngineDownload{
			EngineType: targetEngine,
			Version:    tagName,
			URL:        a.BrowserDownloadURL,
			Filename:   name,
			IsPackage:  true,
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("steer packages not found for arch %s (suffix %s)", distribArch, suffix)
	}

	// Sort so steer-core is first, modules alphabetically, and steer-extended last
	slices.SortStableFunc(results, func(a, b *EngineDownload) int {
		if strings.HasPrefix(a.Filename, "steer-core-") {
			return -1
		}
		if strings.HasPrefix(b.Filename, "steer-core-") {
			return 1
		}
		if strings.HasPrefix(a.Filename, "steer-extended-") {
			return 1
		}
		if strings.HasPrefix(b.Filename, "steer-extended-") {
			return -1
		}
		return strings.Compare(a.Filename, b.Filename)
	})

	return results, nil
}

func resolveSingBoxLX(ctx context.Context, client *Client, distribArch, rawArch, ext string) ([]*EngineDownload, error) {
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
	if normArch == "" {
		normArch = NormalizeArch(distribArch)
	}

	// 1. Check OpenWrt packages if any exist
	candidates := getArchCandidates(distribArch, normArch)
	for _, cand := range candidates {
		suffix := "_" + cand + "." + ext
		for _, a := range rel.Assets {
			if strings.HasSuffix(a.Name, suffix) {
				return []*EngineDownload{{
					EngineType: EngineSingBoxLX,
					Version:    rel.TagName,
					URL:        a.BrowserDownloadURL,
					Filename:   a.Name,
					IsPackage:  true,
				}}, nil
			}
		}
	}

	// 2. Binary archive match:
	for _, a := range rel.Assets {
		if strings.HasPrefix(a.Name, "sing-box-") && isLinuxBinaryArchiveMatch(a.Name, normArch) {
			return []*EngineDownload{{
				EngineType: EngineSingBoxLX,
				Version:    rel.TagName,
				URL:        a.BrowserDownloadURL,
				Filename:   a.Name,
				IsPackage:  false,
			}}, nil
		}
	}

	return nil, fmt.Errorf("sing-box-lx package not found for arch %s", distribArch)
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
		return strings.Contains(n, "mipsel") || strings.Contains(n, "mipsle")
	case "mips":
		return (strings.Contains(n, "mips_24kc") || strings.Contains(n, "mips")) && !strings.Contains(n, "mipsel") && !strings.Contains(n, "mipsle")
	case "armv7", "arm":
		return strings.Contains(n, "arm_cortex") || strings.Contains(n, "armv7") || strings.Contains(n, "armhf") || strings.Contains(n, "arm")
	case "386":
		return strings.Contains(n, "i386") || strings.Contains(n, "x86")
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

	// If tar.gz archive was downloaded, extract binary locally and remove the archive
	if strings.HasSuffix(destPath, ".tar.gz") {
		logFn("[#cbd5e1]Распаковка архива ядра...[-]\n")
		binTarget := "sing-box"
		if engine.EngineType == EngineTachyonCore {
			binTarget = "tachyon-core"
		}
		binPath, err := extractTarGzBinary(destPath, destDir, binTarget)
		if err == nil {
			logFn(fmt.Sprintf("[#22c55e]✓ Бинарный файл ядра распакован: %s[-]\n", filepath.Base(binPath)))
			_ = os.Remove(destPath)
			if engine.EngineType == EngineTachyonCore {
				// Also create a copy as sing-box for universal compatibility with any tachyon LuCI calls
				sbPath := filepath.Join(destDir, "sing-box")
				if data, rErr := os.ReadFile(binPath); rErr == nil {
					_ = os.WriteFile(sbPath, data, 0755)
				}
			}
			return binPath, nil
		}
		logFn(fmt.Sprintf("[#eab308]⚠️ Не удалось распаковать архив локально: %v[-]\n", err))
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
		isMatch := false
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA || header.Typeflag == 0 {
			if header.Name == binName || strings.HasSuffix(header.Name, "/"+binName) || baseName == binName {
				isMatch = true
			} else if binName == "tachyon-core" && (strings.HasPrefix(baseName, "tachyon-core") || baseName == "sing-box") {
				isMatch = true
			} else if binName == "sing-box" && (strings.HasPrefix(baseName, "sing-box") || strings.HasPrefix(baseName, "tachyon-core")) {
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
