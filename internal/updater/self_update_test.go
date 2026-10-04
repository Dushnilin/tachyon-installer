package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseChecksumForFile(t *testing.T) {
	manifest := []byte(`
325f0389f202524a6e40e32e4ccffe801b04434afd176f1760ee47c23ba62549  tachyon-installer-windows-amd64.exe
e70eff3600a8ee147b699b9a0d4aa7028ce0485009fdb8f2b8242a2983320c92  tachyon-installer-linux-amd64
`)

	hash, err := parseChecksumForFile(manifest, "tachyon-installer-windows-amd64.exe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "325f0389f202524a6e40e32e4ccffe801b04434afd176f1760ee47c23ba62549" {
		t.Errorf("expected matching hash, got %s", hash)
	}

	_, errNotFound := parseChecksumForFile(manifest, "non-existent.exe")
	if errNotFound == nil {
		t.Errorf("expected error for non-existent file")
	}
}

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "mytestbinary.exe")

	initialContent := []byte("v1.0.0-binary-content")
	if err := os.WriteFile(exePath, initialContent, 0755); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	newContent := []byte("v2.0.0-updated-binary-content")
	if err := ReplaceExecutable(exePath, newContent); err != nil {
		t.Fatalf("ReplaceExecutable failed: %v", err)
	}

	data, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read replaced file failed: %v", err)
	}

	if string(data) != string(newContent) {
		t.Errorf("expected replaced content %s, got %s", string(newContent), string(data))
	}
}
