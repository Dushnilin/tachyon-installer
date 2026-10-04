package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// CreateLocalBackup pulls configuration files from the router and saves them as a tar.gz archive on the host.
func CreateLocalBackup(client *gossh.Client, routerIP string) (string, error) {
	if err := os.MkdirAll("backups", 0755); err != nil {
		return "", fmt.Errorf("create backups directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	safeIP := routerIP
	if safeIP == "" {
		safeIP = "router"
	}
	archiveFilename := fmt.Sprintf("tachyon_backup_%s_%s.tar.gz", safeIP, timestamp)
	localPath := filepath.Join("backups", archiveFilename)

	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session for backup: %w", err)
	}
	defer sess.Close()

	// Tar existing tachyon config files if they exist on the router
	remoteCmd := "tar -cz -C / etc/config/tachyon etc/tachyon 2>/dev/null || tar -cz -C / etc/config/tachyon 2>/dev/null || echo ''"
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}

	if err := sess.Start(remoteCmd); err != nil {
		return "", fmt.Errorf("start remote tar: %w", err)
	}

	outFile, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create backup file %s: %w", localPath, err)
	}
	defer outFile.Close()

	n, copyErr := io.Copy(outFile, stdout)
	_ = sess.Wait()

	if copyErr != nil || n < 32 {
		// No preexisting config or empty stream, create minimal placeholder backup
		_ = os.Remove(localPath)
		return createEmptyPlaceholderBackup(localPath, routerIP)
	}

	return fmt.Sprintf("Резервная копия настроек сохранена: %s (%.1f КБ)", archiveFilename, float64(n)/1024), nil
}

func createEmptyPlaceholderBackup(localPath, routerIP string) (string, error) {
	f, err := os.Create(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	note := fmt.Sprintf("Tachyon Fresh Install Backup - Router %s - %s\nNo pre-existing Tachyon configuration found.\n", routerIP, time.Now().String())
	hdr := &tar.Header{
		Name:    "README_BACKUP.txt",
		Mode:    0644,
		Size:    int64(len(note)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return "", err
	}
	_, _ = tw.Write([]byte(note))

	return fmt.Sprintf("Первичная установка: создана точка восстановления %s", filepath.Base(localPath)), nil
}
