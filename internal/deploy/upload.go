package deploy

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
)

// UploadStagingAndExecute uploads all files in stagingDir to /tmp/tachyon-install on the router,
// then executes install_on_router.sh and streams console output to outputWriter.
func UploadStagingAndExecute(
	client *gossh.Client,
	stagingDir string,
	outputWriter io.Writer,
	logFn func(msg string),
) error {
	remoteDir := "/tmp/tachyon-install"

	logFn("[#cbd5e1]⚡ Подготовка потоковой загрузки файлов на роутер (tar-over-ssh)...[-]\n")

	// 1. Start remote tar command
	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH upload session: %w", err)
	}
	defer sess.Close()

	cmd := fmt.Sprintf("rm -rf '%s' && mkdir -p '%s' && tar -xf - -C '%s'", remoteDir, remoteDir, remoteDir)

	stdin, err := sess.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := sess.Start(cmd); err != nil {
		stdin.Close()
		return fmt.Errorf("start remote tar: %w", err)
	}

	// 2. Stream files into tar writer
	tw := tar.NewWriter(stdin)
	walkErr := filepath.Walk(stagingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(stagingDir, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}
		header.Name = relSlash

		if info.IsDir() {
			return tw.WriteHeader(header)
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})

	_ = tw.Close()
	_ = stdin.Close()

	if walkErr != nil {
		return fmt.Errorf("archive local files: %w", walkErr)
	}

	if err := sess.Wait(); err != nil {
		return fmt.Errorf("remote unpack error: %w", err)
	}

	logFn("[#22c55e]✓ Файлы успешно загружены в /tmp/tachyon-install на роутере[-]\n")

	// 3. Execute installation script
	logFn("[#eab308]⚡ Запуск скрипта установки пакетов на роутере...[-]\n\n")

	execSess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH exec session: %w", err)
	}
	defer execSess.Close()

	execSess.Stdout = outputWriter
	execSess.Stderr = outputWriter

	runnerCmd := fmt.Sprintf(
		"sed -i 's/\\r$//' %s/install_on_router.sh 2>/dev/null || true; "+
			"chmod +x %s/install_on_router.sh && %s/install_on_router.sh",
		remoteDir, remoteDir, remoteDir,
	)

	runErr := execSess.Run(runnerCmd)

	// 4. Cleanup remote temp directory
	cleanSess, errClean := client.NewSession()
	if errClean == nil {
		_ = cleanSess.Run(fmt.Sprintf("rm -rf '%s'", remoteDir))
		cleanSess.Close()
	}

	if runErr != nil {
		return fmt.Errorf("ошибка выполнения скрипта установки на роутере: %w", runErr)
	}

	return nil
}
