package downloader_test

import (
	"os"
	"path/filepath"
	"testing"

	"tachyon-installer/internal/downloader"
)

func TestWrapURL(t *testing.T) {
	raw := "https://github.com/Dushnilin/tachyon/releases/download/1.4.9/tachyon_1.4.9.ipk"
	mirror := "https://gh-proxy.com/"

	wrapped := downloader.WrapURL(mirror, raw)
	expected := "https://gh-proxy.com/https://github.com/Dushnilin/tachyon/releases/download/1.4.9/tachyon_1.4.9.ipk"
	if wrapped != expected {
		t.Errorf("WrapURL() = %s, want %s", wrapped, expected)
	}

	// Direct should not modify
	direct := downloader.WrapURL("direct", raw)
	if direct != raw {
		t.Errorf("WrapURL(direct) = %s, want %s", direct, raw)
	}

	// Avoid double-mirror
	doubleWrapped := downloader.WrapURL("https://ghproxy.net/", wrapped)
	expectedReplaced := "https://ghproxy.net/https://github.com/Dushnilin/tachyon/releases/download/1.4.9/tachyon_1.4.9.ipk"
	if doubleWrapped != expectedReplaced {
		t.Errorf("double WrapURL() = %s, want %s", doubleWrapped, expectedReplaced)
	}
}

func TestGetCandidateURLs(t *testing.T) {
	mgr := downloader.NewMirrorManager("auto")
	raw := "https://github.com/Dushnilin/tachyon/releases/download/1.4.9/tachyon_1.4.9.ipk"

	candidates := mgr.GetCandidateURLs(raw)
	if len(candidates) < len(downloader.DefaultMirrors)+1 {
		t.Errorf("expected at least %d candidates, got %d", len(downloader.DefaultMirrors)+1, len(candidates))
	}

	// Last candidate in auto mode should be raw/direct
	last := candidates[len(candidates)-1]
	if last != raw {
		t.Errorf("expected last candidate to be raw %s, got %s", raw, last)
	}
}

func TestFastestMirrorPriority(t *testing.T) {
	mgr := downloader.NewMirrorManager("auto")
	fastest := "https://ghfast.top/"
	mgr.SetFastestMirror(fastest)

	raw := "https://github.com/Dushnilin/tachyon/releases/download/1.4.9/tachyon_1.4.9.ipk"
	candidates := mgr.GetCandidateURLs(raw)

	if len(candidates) == 0 {
		t.Fatalf("expected candidates, got 0")
	}

	expectedFirst := downloader.WrapURL(fastest, raw)
	if candidates[0] != expectedFirst {
		t.Errorf("expected first candidate to be fastest mirror %s, got %s", expectedFirst, candidates[0])
	}
}

func TestParseSHA256Sums_And_Verify(t *testing.T) {
	sumsText := `
# test checksums
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  empty.txt
5d41402abc4b2a76b9719d911017c592  short.txt
2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae *hello.txt
`
	parsed := downloader.ParseSHA256Sums(sumsText)
	if len(parsed) != 2 {
		t.Fatalf("expected 2 valid hashes, got %d", len(parsed))
	}

	if parsed["empty.txt"] != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("unexpected hash for empty.txt: %s", parsed["empty.txt"])
	}
	if parsed["hello.txt"] != "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae" {
		t.Errorf("unexpected hash for hello.txt: %s", parsed["hello.txt"])
	}

	// Verify real file
	tmp := t.TempDir()
	emptyPath := filepath.Join(tmp, "empty.txt")
	os.WriteFile(emptyPath, []byte(""), 0644)

	err := downloader.VerifyFileSHA256(emptyPath, parsed["empty.txt"])
	if err != nil {
		t.Errorf("VerifyFileSHA256 failed on empty.txt: %v", err)
	}

	helloPath := filepath.Join(tmp, "hello.txt")
	os.WriteFile(helloPath, []byte("foo"), 0644) // sha256("foo") is 2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae
	err = downloader.VerifyFileSHA256(helloPath, parsed["hello.txt"])
	if err != nil {
		t.Errorf("VerifyFileSHA256 failed on hello.txt: %v", err)
	}

	// Bad hash
	err = downloader.VerifyFileSHA256(helloPath, "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Errorf("expected error on mismatched hash, got nil")
	}
}

func TestNormalizeArch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"x86_64", "amd64"},
		{"aarch64_cortex-a53", "arm64"},
		{"aarch64_generic", "arm64"},
		{"mipsel_24kc", "mipsle"},
		{"mips_24kc", "mips"},
		{"arm_cortex-a7_neon-vfpv4", "armv7"},
	}

	for _, tt := range tests {
		got := downloader.NormalizeArch(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeArch(%s) = %s, want %s", tt.input, got, tt.want)
		}
	}
}
