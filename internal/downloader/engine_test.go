package downloader_test

import (
	"context"
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
		{"arm_arm1176jzf-s_vfp", "armv6"},
		{"riscv64_generic", "riscv64"},
		{"i386_pentium4", "386"},
	}

	for _, c := range cases {
		got := downloader.NormalizeArch(c.input)
		if got != c.expected {
			t.Errorf("NormalizeArch(%q) = %q; want %q", c.input, got, c.expected)
		}
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

func TestNormalizeEngine(t *testing.T) {
	cases := []struct {
		input    string
		expected downloader.EngineType
	}{
		{"", downloader.EngineTachyonCore},
		{"tachyon-core", downloader.EngineTachyonCore},
		{"core", downloader.EngineTachyonCore},
		{"sing-box", downloader.EngineTachyonCore},
		{"sing-box-extended", downloader.EngineTachyonCore},
		{"sing-box-extended-compressed", downloader.EngineTachyonCore},
		{"sing-box-tiny", downloader.EngineTachyonCore},
		{"sing-box-lx", downloader.EngineTachyonCore},
		{"skip", downloader.EngineSkip},
	}

	for _, c := range cases {
		if got := downloader.NormalizeEngine(c.input); got != c.expected {
			t.Errorf("NormalizeEngine(%q) = %q; want %q", c.input, got, c.expected)
		}
	}
}

func TestFilterTachyonCoreAssets(t *testing.T) {
	assets := []downloader.ReleaseAsset{
		{Name: "tachyon-core-aarch64-linux-android-v0.5.8.tar.gz", BrowserDownloadURL: "https://example.com/android-arm64"},
		{Name: "tachyon-core-armv7-linux-androideabi-v0.5.8.tar.gz", BrowserDownloadURL: "https://example.com/android-armv7"},
		{Name: "tachyon-core-x86_64-linux-android-v0.5.8.tar.gz", BrowserDownloadURL: "https://example.com/android-amd64"},
		{Name: "tachyon-core-ios-arm64-v0.5.8.tar.gz", BrowserDownloadURL: "https://example.com/ios-arm64"},
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

func TestFilterTachyonCoreAssets_V058Packages(t *testing.T) {
	assets := []downloader.ReleaseAsset{
		{Name: "tachyon-core-aarch64-linux-android-v0.5.8.tar.gz", BrowserDownloadURL: "https://example.com/android"},
		{Name: "tachyon-core-ios-arm64-v0.5.8.tar.gz", BrowserDownloadURL: "https://example.com/ios"},
		{Name: "tachyon-core-aarch64-v0.5.8.apk", BrowserDownloadURL: "https://example.com/apk-arm64"},
		{Name: "tachyon-core-x86_64-v0.5.8.apk", BrowserDownloadURL: "https://example.com/apk-amd64"},
		{Name: "tachyon-core-mips-v0.5.8.apk", BrowserDownloadURL: "https://example.com/apk-mips"},
		{Name: "tachyon-core-mipsel-v0.5.8.apk", BrowserDownloadURL: "https://example.com/apk-mipsel"},
		{Name: "tachyon-core-i386_pentium4-v0.5.8.apk", BrowserDownloadURL: "https://example.com/apk-386"},
		{Name: "tachyon-core-riscv64-v0.5.8.ipk", BrowserDownloadURL: "https://example.com/ipk-riscv64"},
	}

	cases := []struct {
		distribArch string
		rawArch     string
		ext         string
		wantURL     string
	}{
		{"aarch64_generic", "arm64", "apk", "https://example.com/apk-arm64"},
		{"x86_64", "amd64", "apk", "https://example.com/apk-amd64"},
		{"mips_24kc", "mips", "apk", "https://example.com/apk-mips"},
		{"mipsel_24kc", "mipsle", "apk", "https://example.com/apk-mipsel"},
		{"i386_pentium4", "386", "apk", "https://example.com/apk-386"},
		{"riscv64_generic", "riscv64", "ipk", "https://example.com/ipk-riscv64"},
	}

	for _, tc := range cases {
		res, err := downloader.FilterTachyonCoreAssets(assets, tc.distribArch, tc.rawArch, tc.ext, "v0.5.8")
		if err != nil {
			t.Fatalf("FilterTachyonCoreAssets(%s, %s) failed: %v", tc.distribArch, tc.ext, err)
		}
		if len(res) != 1 || !res[0].IsPackage || res[0].URL != tc.wantURL {
			t.Errorf("for %s/%s got %+v; want IsPackage=true, URL=%s", tc.distribArch, tc.ext, res, tc.wantURL)
		}
	}
}

