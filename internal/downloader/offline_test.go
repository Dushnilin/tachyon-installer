package downloader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOfflineBundle(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := OfflineManifest{
		CreatedAt:      time.Now().UTC(),
		TachyonVersion: "1.4.9",
		Packages: []OfflinePackageInfo{
			{Filename: "tachyon_1.4.9_all.ipk", Size: 12345, Category: "tachyon"},
			{Filename: "sing-box_arm64.tar.gz", Size: 67890, Category: "engine", Arch: "arm64"},
		},
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "offline_manifest.json"), data, 0644); err != nil {
		t.Fatalf("write file error: %v", err)
	}

	loaded, err := LoadOfflineBundle(tmpDir)
	if err != nil {
		t.Fatalf("LoadOfflineBundle failed: %v", err)
	}

	if loaded.TachyonVersion != "1.4.9" {
		t.Errorf("expected version 1.4.9, got %s", loaded.TachyonVersion)
	}
	if len(loaded.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(loaded.Packages))
	}
}

func TestLoadOfflineBundle_NotFound(t *testing.T) {
	_, err := LoadOfflineBundle("nonexistent-dir-12345")
	if err == nil {
		t.Errorf("expected error for nonexistent directory")
	}
}
