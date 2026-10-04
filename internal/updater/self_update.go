package updater

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DefaultMirrors lists reliable download endpoints.
var DefaultMirrors = []string{
	"https://github.com/Dushnilin/tachyon-installer/releases/download/%s/%s",
	"https://gh-proxy.com/https://github.com/Dushnilin/tachyon-installer/releases/download/%s/%s",
	"https://ghfast.top/https://github.com/Dushnilin/tachyon-installer/releases/download/%s/%s",
	"https://gh.ddlc.top/https://github.com/Dushnilin/tachyon-installer/releases/download/%s/%s",
}

// SelfUpdate queries GitHub for the newest release, downloads matching binary, verifies SHA256,
// and atomically replaces the currently running executable.
func SelfUpdate(ctx context.Context, currentVersion string, logFn func(string)) (bool, string, error) {
	if logFn == nil {
		logFn = func(string) {}
	}

	logFn("⚡ Проверка наличия новых версий Tachyon Installer на GitHub...\n")
	hasUpdate, latestVer, _, err := CheckForUpdate(ctx, currentVersion)
	if err != nil {
		return false, "", fmt.Errorf("check update: %w", err)
	}
	if !hasUpdate {
		logFn(fmt.Sprintf("✓ У вас уже установлена актуальная версия (%s).\n", currentVersion))
		return false, currentVersion, nil
	}

	logFn(fmt.Sprintf("💡 Найдена новая версия: %s (текущая: %s)\n", latestVer, currentVersion))

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	binaryName := fmt.Sprintf("tachyon-installer-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)

	// 1. Download sha256sums.txt
	logFn("⚡ Загрузка манифеста контрольных сумм sha256sums.txt...\n")
	checksumData, err := downloadWithMirrors(ctx, latestVer, "sha256sums.txt")
	if err != nil {
		return false, "", fmt.Errorf("download sha256sums.txt: %w", err)
	}

	expectedHash, err := parseChecksumForFile(checksumData, binaryName)
	if err != nil {
		return false, "", fmt.Errorf("checksum entry: %w", err)
	}

	// 2. Download binary
	logFn(fmt.Sprintf("⚡ Загрузка обновленного бинарного файла %s...\n", binaryName))
	binData, err := downloadWithMirrors(ctx, latestVer, binaryName)
	if err != nil {
		return false, "", fmt.Errorf("download binary: %w", err)
	}

	// 3. Verify SHA256
	logFn("⚡ Проверка целостности SHA256...\n")
	h := sha256.New()
	h.Write(binData)
	actualHash := hex.EncodeToString(h.Sum(nil))

	if !strings.EqualFold(actualHash, expectedHash) {
		return false, "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}
	logFn("✓ Хэш-сумма проверена и полностью совпадает!\n")

	// 4. In-Place Replacement
	logFn("⚡ Замена исполняемого файла на диске...\n")
	exePath, err := os.Executable()
	if err != nil {
		return false, "", fmt.Errorf("detect executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return false, "", fmt.Errorf("resolve symlinks: %w", err)
	}

	if err := ReplaceExecutable(exePath, binData); err != nil {
		return false, "", fmt.Errorf("replace executable: %w", err)
	}

	logFn(fmt.Sprintf("\n🎉 Программа успешно обновлена до %s!\n", latestVer))
	return true, latestVer, nil
}

// ReplaceExecutable safely replaces the binary at targetPath with newBytes.
func ReplaceExecutable(targetPath string, newBytes []byte) error {
	if runtime.GOOS == "windows" {
		oldPath := targetPath + ".old"
		_ = os.Remove(oldPath)

		if err := os.Rename(targetPath, oldPath); err != nil {
			return fmt.Errorf("rename old executable: %w", err)
		}

		if err := os.WriteFile(targetPath, newBytes, 0755); err != nil {
			// Rollback if failed
			_ = os.Rename(oldPath, targetPath)
			return fmt.Errorf("write new executable: %w", err)
		}

		_ = os.Remove(oldPath)
		return nil
	}

	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, newBytes, 0755); err != nil {
		return fmt.Errorf("write tmp executable: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename executable: %w", err)
	}

	return nil
}

func downloadWithMirrors(ctx context.Context, tag, filename string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	for _, tmpl := range DefaultMirrors {
		url := fmt.Sprintf(tmpl, tag, filename)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "tachyon-installer-updater")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || len(data) == 0 {
			continue
		}

		return data, nil
	}

	return nil, fmt.Errorf("failed to download %s from all available mirrors", filename)
}

func parseChecksumForFile(manifestBytes []byte, targetFile string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(manifestBytes))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			hash := fields[0]
			name := filepath.Base(fields[1])
			if name == targetFile {
				return hash, nil
			}
		}
	}
	return "", fmt.Errorf("target file %s not found in sha256sums.txt", targetFile)
}
