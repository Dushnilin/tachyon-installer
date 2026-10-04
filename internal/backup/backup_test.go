package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListBackups(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	_ = os.Chdir(tmpDir)

	_ = os.MkdirAll("backups", 0755)
	f1 := filepath.Join("backups", "tachyon_backup_192.168.1.1_20260101-100000.tar.gz")
	f2 := filepath.Join("backups", "tachyon_backup_192.168.1.1_20260201-120000.tar.gz")
	f3 := filepath.Join("backups", "tachyon_snapshot_192.168.1.1_20260301-150000.tar.gz")
	fOther := filepath.Join("backups", "unrelated.txt")

	_ = os.WriteFile(f1, []byte("test1"), 0644)
	_ = os.WriteFile(f2, []byte("test2"), 0644)
	_ = os.WriteFile(f3, []byte("test3"), 0644)
	_ = os.WriteFile(fOther, []byte("ignore"), 0644)

	list, err := ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() returned error: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("ListBackups() returned %d items, want 3", len(list))
	}

	// Should be sorted newest first (f3, then f2, then f1)
	if list[0] != f3 || list[1] != f2 || list[2] != f1 {
		t.Errorf("ListBackups() wrong sorting: got %v, want [%s, %s, %s]", list, f3, f2, f1)
	}
}
