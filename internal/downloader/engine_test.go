package downloader_test

import (
	"context"
	"strings"
	"testing"

	"tachyon-installer/internal/downloader"
)

func TestArchNormalization(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"aarch64_generic", "arm64"},
		{"aarch64_cortex-a53", "arm64"},
		{"x86_64", "amd64"},
		{"amd64", "amd64"},
		{"mipsel_24kc", "mipsle"},
		{"mips_24kc", "mips"},
		{"arm_cortex-a7_neon-vfpv4", "armv7"},
		{"i386_pentium4", "386"},
	}

	for _, c := range cases {
		got := downloader.NormalizeArch(c.input)
		if got != c.expected {
			t.Errorf("NormalizeArch(%q) = %q; want %q", c.input, got, c.expected)
		}
	}
}

func TestFilterSteerAssets_Modular20(t *testing.T) {
	assets := []downloader.ReleaseAsset{
		{Name: "steer-core-2.0.0-1_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-core-ipk"},
		{Name: "steer-core-2.0.0-1_aarch64_generic.apk", BrowserDownloadURL: "https://example.com/steer-core-apk"},
		{Name: "steer-core-2.0.0-1_x86_64.ipk", BrowserDownloadURL: "https://example.com/steer-core-x86"},
		{Name: "steer-vless-2.0.0-1_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-vless-ipk"},
		{Name: "steer-vless-2.0.0-1_aarch64_generic.apk", BrowserDownloadURL: "https://example.com/steer-vless-apk"},
		{Name: "steer-hysteria2-2.0.0-1_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-hy2-ipk"},
		{Name: "steer-proxy-2.0.0-1_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-proxy-ipk"},
		{Name: "steer-obfs-2.0.0-1_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-obfs-ipk"},
		{Name: "steer-tgws-2.0.0-1_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-tgws-ipk"},
		{Name: "steer-xsteer-2.0.0-1_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-xsteer-ipk"},
		{Name: "steer-extended-2.0.0-1_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-ext-ipk"},
		{Name: "steer-obfs-2.0.0-aarch64.tar.gz", BrowserDownloadURL: "https://example.com/steer-obfs-tar"},
	}

	// 1. Test steer (standard/modular): should include core + all 6 submodules, but NOT steer-extended
	resStandard, err := downloader.FilterSteerAssets(assets, false, "aarch64_generic", "arm64", "ipk", "v2.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resStandard) != 7 {
		t.Fatalf("expected 7 packages for standard modular steer, got %d", len(resStandard))
	}
	if !strings.HasPrefix(resStandard[0].Filename, "steer-core-") {
		t.Fatalf("expected steer-core to be first, got %s", resStandard[0].Filename)
	}
	for _, pkg := range resStandard {
		if strings.HasPrefix(pkg.Filename, "steer-extended-") {
			t.Errorf("steer-extended must not be present in standard steer suite: %s", pkg.Filename)
		}
		if !strings.HasSuffix(pkg.Filename, "_aarch64_generic.ipk") {
			t.Errorf("wrong architecture or ext: %s", pkg.Filename)
		}
	}

	// 2. Test steer-extended: should include core + 6 submodules + steer-extended (8 packages)
	resExt, err := downloader.FilterSteerAssets(assets, true, "aarch64_generic", "arm64", "ipk", "v2.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resExt) != 8 {
		t.Fatalf("expected 8 packages for steer-extended, got %d", len(resExt))
	}
	hasCore := false
	hasExtended := false
	for _, pkg := range resExt {
		if strings.HasPrefix(pkg.Filename, "steer-core-") {
			hasCore = true
		}
		if strings.HasPrefix(pkg.Filename, "steer-extended-") {
			hasExtended = true
		}
	}
	if !hasCore || !hasExtended {
		t.Fatalf("expected both steer-core and steer-extended in extended suite")
	}

	// 3. Test APK resolution (OpenWrt 25.12+)
	resAPK, err := downloader.FilterSteerAssets(assets, false, "aarch64_generic", "arm64", "apk", "v2.0.0")
	if err != nil {
		t.Fatalf("unexpected error for apk: %v", err)
	}
	if len(resAPK) == 0 {
		t.Fatalf("expected apk packages, got 0")
	}
	for _, pkg := range resAPK {
		if !strings.HasSuffix(pkg.Filename, ".apk") {
			t.Errorf("expected .apk package, got %s", pkg.Filename)
		}
	}

	// 4. Test candidate architecture fallback:
	// Router has "aarch64_cortex-a53", but assets only have "aarch64_generic"
	resFallback, err := downloader.FilterSteerAssets(assets, false, "aarch64_cortex-a53", "arm64", "ipk", "v2.0.0")
	if err != nil {
		t.Fatalf("expected fallback to aarch64_generic, got err: %v", err)
	}
	if len(resFallback) != 7 {
		t.Fatalf("expected 7 packages on fallback, got %d", len(resFallback))
	}
	if !strings.HasSuffix(resFallback[0].Filename, "_aarch64_generic.ipk") {
		t.Errorf("expected fallback to aarch64_generic, got %s", resFallback[0].Filename)
	}
}

func TestFilterSteerAssets_Legacy1x(t *testing.T) {
	legacyAssets := []downloader.ReleaseAsset{
		{Name: "steer-1.5.9_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-1.5.9-ipk"},
		{Name: "steer-extended-1.5.9_aarch64_generic.ipk", BrowserDownloadURL: "https://example.com/steer-ext-1.5.9-ipk"},
	}

	resStandard, err := downloader.FilterSteerAssets(legacyAssets, false, "aarch64_generic", "arm64", "ipk", "v1.5.9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resStandard) != 1 || resStandard[0].Filename != "steer-1.5.9_aarch64_generic.ipk" {
		t.Fatalf("expected 1 legacy package steer-1.5.9, got %+v", resStandard)
	}

	resExt, err := downloader.FilterSteerAssets(legacyAssets, true, "aarch64_generic", "arm64", "ipk", "v1.5.9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resExt) != 1 || resExt[0].Filename != "steer-extended-1.5.9_aarch64_generic.ipk" {
		t.Fatalf("expected 1 legacy package steer-extended-1.5.9, got %+v", resExt)
	}
}

func TestResolveEngineAssets_Skip(t *testing.T) {
	mgr := downloader.NewMirrorManager("direct")
	customClient := downloader.NewClient(mgr)
	skipAssets, err := downloader.ResolveEngineAssets(context.Background(), customClient, downloader.EngineSkip, "aarch64_generic", "arm64", false)
	if err != nil || len(skipAssets) != 1 || skipAssets[0].EngineType != downloader.EngineSkip {
		t.Fatalf("expected EngineSkip handling, got %v, err %v", skipAssets, err)
	}
}

func TestResolveEngineAssets_TinyAndStable(t *testing.T) {
	mgr := downloader.NewMirrorManager("direct")
	customClient := downloader.NewClient(mgr)
	tinyAssets, err := downloader.ResolveEngineAssets(context.Background(), customClient, downloader.EngineSingBoxTiny, "aarch64_generic", "arm64", false)
	if err != nil || len(tinyAssets) != 1 || tinyAssets[0].EngineType != downloader.EngineSingBoxTiny {
		t.Fatalf("expected EngineSingBoxTiny handling, got %v, err %v", tinyAssets, err)
	}

	stableAssets, err := downloader.ResolveEngineAssets(context.Background(), customClient, downloader.EngineSingBoxStable, "aarch64_generic", "arm64", false)
	if err != nil || len(stableAssets) != 1 || stableAssets[0].EngineType != downloader.EngineSingBoxStable {
		t.Fatalf("expected EngineSingBoxStable handling, got %v, err %v", stableAssets, err)
	}
}

func TestFilterTachyonCoreAssets(t *testing.T) {
	assets := []downloader.ReleaseAsset{
		{Name: "tachyon-core-linux-amd64-v0.0.2.tar.gz", BrowserDownloadURL: "https://example.com/amd64"},
		{Name: "tachyon-core-aarch64-v0.0.2.tar.gz", BrowserDownloadURL: "https://example.com/aarch64"},
		{Name: "tachyon-core-armv7-v0.0.2.tar.gz", BrowserDownloadURL: "https://example.com/armv7"},
		{Name: "tachyon-core-armv6-v0.0.2.tar.gz", BrowserDownloadURL: "https://example.com/armv6"},
		{Name: "tachyon-core-riscv64-v0.0.2.tar.gz", BrowserDownloadURL: "https://example.com/riscv64"},
		{Name: "tachyon-core-mipsel-musl-v0.0.1.tar.gz", BrowserDownloadURL: "https://example.com/mipsel"},
		{Name: "tachyon-core-mips-musl-v0.0.1.tar.gz", BrowserDownloadURL: "https://example.com/mips"},
		{Name: "tachyon-core-darwin-amd64-v0.0.2.tar.gz", BrowserDownloadURL: "https://example.com/darwin"},
		{Name: "tachyon-core-windows-amd64-v0.0.2.zip", BrowserDownloadURL: "https://example.com/windows"},
	}

	testCases := []struct {
		distribArch string
		rawArch     string
		expectedURL string
		expectedFn  string
	}{
		{"aarch64_generic", "arm64", "https://example.com/aarch64", "tachyon-core-aarch64-v0.0.2.tar.gz"},
		{"x86_64", "amd64", "https://example.com/amd64", "tachyon-core-linux-amd64-v0.0.2.tar.gz"},
		{"arm_cortex-a7_neon-vfpv4", "armv7", "https://example.com/armv7", "tachyon-core-armv7-v0.0.2.tar.gz"},
		{"arm_arm1176jzf-s_vfp", "armv6", "https://example.com/armv6", "tachyon-core-armv6-v0.0.2.tar.gz"},
		{"mipsel_24kc", "mipsle", "https://example.com/mipsel", "tachyon-core-mipsel-musl-v0.0.1.tar.gz"},
		{"mips_24kc", "mips", "https://example.com/mips", "tachyon-core-mips-musl-v0.0.1.tar.gz"},
		{"riscv64", "riscv64", "https://example.com/riscv64", "tachyon-core-riscv64-v0.0.2.tar.gz"},
	}

	for _, tc := range testCases {
		res, err := downloader.FilterTachyonCoreAssets(assets, tc.distribArch, tc.rawArch, "ipk", "v0.0.2")
		if err != nil {
			t.Errorf("FilterTachyonCoreAssets for %s/%s failed: %v", tc.distribArch, tc.rawArch, err)
			continue
		}
		if len(res) != 1 {
			t.Errorf("expected 1 result for %s/%s, got %d", tc.distribArch, tc.rawArch, len(res))
			continue
		}
		if res[0].EngineType != downloader.EngineTachyonCore {
			t.Errorf("expected EngineTachyonCore, got %s", res[0].EngineType)
		}
		if res[0].URL != tc.expectedURL {
			t.Errorf("for %s/%s expected URL %s, got %s", tc.distribArch, tc.rawArch, tc.expectedURL, res[0].URL)
		}
		if res[0].Filename != tc.expectedFn {
			t.Errorf("for %s/%s expected filename %s, got %s", tc.distribArch, tc.rawArch, tc.expectedFn, res[0].Filename)
		}
		if res[0].IsPackage {
			t.Errorf("expected IsPackage=false for tar.gz, got true")
		}
	}
}

