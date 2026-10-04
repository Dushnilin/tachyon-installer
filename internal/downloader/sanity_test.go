package downloader_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"tachyon-installer/internal/downloader"
)

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSanityCheckPackage(t *testing.T) {
	big := bytes.Repeat([]byte{0x1f, 0x8b, 0x08, 0x00}, 400)
	html := append([]byte("<!DOCTYPE html><html><body>Bad gateway</body></html>"), bytes.Repeat([]byte(" "), 800)...)
	json := append([]byte(`{"message":"Not Found"}`), bytes.Repeat([]byte(" "), 800)...)

	cases := []struct {
		name    string
		file    string
		data    []byte
		wantErr bool
	}{
		{"valid gzip package", "a.ipk", big, false},
		{"valid binary", "tachyon-bin", big, false},
		{"empty", "a.apk", nil, true},
		{"tiny", "a.ipk", []byte("oops"), true},
		{"html error page", "a.ipk", html, true},
		{"html even without extension rules", "sing-box", html, true},
		{"json error for package", "a.apk", json, true},
		{"json body is fine for non-package", "info.txt", json, false},
	}
	for _, c := range cases {
		err := downloader.SanityCheckPackage(writeFile(t, c.file, c.data))
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", c.name, err, c.wantErr)
		}
	}
	if err := downloader.SanityCheckPackage(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing file must error")
	}
}
