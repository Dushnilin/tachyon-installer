package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/rivo/tview"

	dlpkg "tachyon-installer/internal/downloader"
)

// verifyStagedFiles checks every downloaded file and prints its size and SHA256.
// It returns false if any file is clearly broken (error page, empty file).
func verifyStagedFiles(ctx *AppContext, dir string) bool {
	ctx.ConsoleWrite("[#cbd5e1]⚡ Проверка файлов перед отправкой на роутер...[-]\n")
	entries, err := os.ReadDir(dir)
	if err != nil {
		ctx.ConsoleWritef("[#ef5350]❌ Не удалось прочитать папку загрузки: %v[-]\n", err)
		return false
	}

	allOK := true
	for _, e := range entries {
		if e.IsDir() || e.Name() == "sha256sums.txt" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := dlpkg.SanityCheckPackage(path); err != nil {
			ctx.ConsoleWritef("  [#ef5350]✗[-] %s: [#ef5350]%v[-]\n", tview.Escape(e.Name()), err)
			allOK = false
			continue
		}
		sum, size, err := fileSHA256(path)
		if err != nil {
			ctx.ConsoleWritef("  [#ef5350]✗[-] %s: [#ef5350]%v[-]\n", tview.Escape(e.Name()), err)
			allOK = false
			continue
		}
		ctx.ConsoleWritef("  [#22c55e]✓[-] %-46s [#94a3b8]%6.1f МБ  sha256:%s…[-]\n",
			tview.Escape(e.Name()), float64(size)/1024/1024, sum[:16])
	}

	if !allOK {
		ctx.ConsoleWrite("[#ef5350]❌ Часть файлов повреждена или загружена неверно. Выберите другое зеркало и повторите установку.[-]\n")
		return false
	}
	ctx.ConsoleWrite("[#22c55e]✓ Файлы целы и готовы к отправке на роутер![-]\n\n")
	return true
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
