package downloader_test

import (
	"testing"

	"tachyon-installer/internal/downloader"
)

func TestTagClassification(t *testing.T) {
	cases := []struct {
		tag     string
		wantV2  bool
		wantPre bool
	}{
		{"v2.0.0-beta.7", true, true},
		{"v2.0.0", true, false},
		{"2.0.0-rc1", true, true},
		{"1.8.2", false, false},
		{"v1.8.2", false, false},
		{"1.7.33", false, false},
	}

	for _, tc := range cases {
		if got := downloader.IsV2Tag(tc.tag); got != tc.wantV2 {
			t.Errorf("IsV2Tag(%q) = %v, want %v", tc.tag, got, tc.wantV2)
		}
		if got := downloader.IsPrereleaseTag(tc.tag); got != tc.wantPre {
			t.Errorf("IsPrereleaseTag(%q) = %v, want %v", tc.tag, got, tc.wantPre)
		}
	}
}

func TestMapToTachyon2Arch(t *testing.T) {
	cases := []struct {
		distrib string
		raw     string
		want    string
	}{
		{"aarch64_cortex-a53", "arm64", "aarch64"},
		{"x86_64", "amd64", "x86_64"},
		{"i386_pentium4", "386", "i386_pentium4"},
		{"arm_cortex-a7_neon-vfpv4", "armv7", "armv7"},
		{"arm_arm1176jzf-s_vfp", "armv6", "armv6"},
		{"mipsel_24kc", "mipsle", "mipsel"},
		{"mips_24kc", "mips", "mips"},
		{"riscv64_generic", "riscv64", "riscv64"},
	}

	for _, tc := range cases {
		if got := downloader.MapToTachyon2Arch(tc.distrib, tc.raw); got != tc.want {
			t.Errorf("MapToTachyon2Arch(%q, %q) = %q, want %q", tc.distrib, tc.raw, got, tc.want)
		}
	}
}

func TestResolveTachyonAssetsForArch_V2Beta(t *testing.T) {
	rel := &downloader.TachyonRelease{
		TagName:    "v2.0.0-beta.7",
		Prerelease: true,
		Assets: []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		}{
			{Name: "SHA256SUMS", BrowserDownloadURL: "https://example.com/SHA256SUMS"},
			{Name: "tachyon-aarch64-v2.0.0-beta.7.apk", BrowserDownloadURL: "https://example.com/tachyon-aarch64.apk"},
			{Name: "tachyon-aarch64-v2.0.0-beta.7.ipk", BrowserDownloadURL: "https://example.com/tachyon-aarch64.ipk"},
			{Name: "tachyon-x86_64-v2.0.0-beta.7.apk", BrowserDownloadURL: "https://example.com/tachyon-x86_64.apk"},
			{Name: "tachyon-mipsel-v2.0.0-beta.7.ipk", BrowserDownloadURL: "https://example.com/tachyon-mipsel.ipk"},
		},
	}

	apkRes, err := downloader.ResolveTachyonAssetsForArch(rel, true, true, "aarch64_cortex-a53", "arm64")
	if err != nil {
		t.Fatalf("ResolveTachyonAssetsForArch apk failed: %v", err)
	}
	if !apkRes.IsV2 || apkRes.BackendName != "tachyon-aarch64-v2.0.0-beta.7.apk" || apkRes.SHA256Name != "SHA256SUMS" {
		t.Errorf("unexpected v2 apk assets: %+v", apkRes)
	}
	if apkRes.AppName != "" || apkRes.I18nName != "" {
		t.Errorf("expected no separate luci/i18n packages in v2, got app=%q i18n=%q", apkRes.AppName, apkRes.I18nName)
	}

	ipkRes, err := downloader.ResolveTachyonAssetsForArch(rel, false, true, "mipsel_24kc", "mipsle")
	if err != nil {
		t.Fatalf("ResolveTachyonAssetsForArch ipk failed: %v", err)
	}
	if !ipkRes.IsV2 || ipkRes.BackendName != "tachyon-mipsel-v2.0.0-beta.7.ipk" {
		t.Errorf("unexpected v2 ipk assets: %+v", ipkRes)
	}
}
