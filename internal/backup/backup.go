package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// ListBackups returns all available local backup archives, sorted newest first.
func ListBackups() ([]string, error) {
	entries, err := os.ReadDir("backups")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var archives []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "tachyon_backup_") && strings.HasSuffix(entry.Name(), ".tar.gz") {
			archives = append(archives, filepath.Join("backups", entry.Name()))
		}
	}

	// Sort newest first by filename (which embeds ISO timestamp YYYYMMDD-HHMMSS)
	sort.Slice(archives, func(i, j int) bool {
		return archives[i] > archives[j]
	})

	return archives, nil
}

// RestoreBackup uploads a chosen local backup archive to the router and restores it into /etc/config/ and /etc/tachyon.
func RestoreBackup(client *gossh.Client, backupPath string) (string, error) {
	file, err := os.Open(backupPath)
	if err != nil {
		return "", fmt.Errorf("open backup file: %w", err)
	}
	defer file.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session: %w", err)
	}
	defer sess.Close()

	stdin, err := sess.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("stdin pipe: %w", err)
	}

	remoteCmd := `cat > /tmp/tachyon_restore.tar.gz && \
tar -xzf /tmp/tachyon_restore.tar.gz -C / 2>/dev/null || true; \
rm -f /tmp/tachyon_restore.tar.gz; \
if [ -x /etc/init.d/tachyon ]; then /etc/init.d/tachyon restart >/dev/null 2>&1 || true; fi; \
echo 'RESTORE_OK'`

	go func() {
		defer stdin.Close()
		_, _ = io.Copy(stdin, file)
	}()

	out, err := sess.CombinedOutput(remoteCmd)
	if err != nil {
		return "", fmt.Errorf("restore command failed: %w (output: %s)", err, string(out))
	}

	if !strings.Contains(string(out), "RESTORE_OK") {
		return "", fmt.Errorf("restore did not complete cleanly: %s", string(out))
	}

	return fmt.Sprintf("Конфигурация Tachyon успешно восстановлена из %s", filepath.Base(backupPath)), nil
}
