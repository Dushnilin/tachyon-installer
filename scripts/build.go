package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Target struct {
	OS   string
	Arch string
	Ext  string
}

var allTargets = []Target{
	// Windows Desktop
	{OS: "windows", Arch: "amd64", Ext: ".exe"},
	{OS: "windows", Arch: "arm64", Ext: ".exe"},
	// Linux Desktop & Gateways
	{OS: "linux", Arch: "amd64", Ext: ""},
	{OS: "linux", Arch: "arm64", Ext: ""},
	// macOS (Darwin)
	{OS: "darwin", Arch: "arm64", Ext: ""}, // Apple Silicon (M1/M2/M3/M4)
	{OS: "darwin", Arch: "amd64", Ext: ""}, // Intel Mac
}

func main() {
	var version string
	var targetOS string
	var targetArch string

	flag.StringVar(&version, "v", "1.0.0", "Release version string")
	flag.StringVar(&targetOS, "os", "all", "Target OS (windows, linux, darwin, all)")
	flag.StringVar(&targetArch, "arch", "all", "Target Arch (amd64, arm64, all)")
	flag.Parse()

	distDir := "dist"
	if err := os.MkdirAll(distDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dist directory: %v\n", err)
		os.Exit(1)
	}

	buildTime := time.Now().UTC().Format(time.RFC3339)
	ldflags := fmt.Sprintf("-s -w -X main.AppVersion=%s -X main.BuildDate=%s", version, buildTime)

	var targets []Target
	for _, t := range allTargets {
		if targetOS != "all" && t.OS != targetOS {
			continue
		}
		if targetArch != "all" && t.Arch != targetArch {
			continue
		}
		targets = append(targets, t)
	}

	if len(targets) == 0 {
		fmt.Printf("No matching targets for os=%s arch=%s\n", targetOS, targetArch)
		return
	}

	fmt.Printf("🚀 Building clean standalone Tachyon Installer binaries v%s for %d desktop targets...\n\n", version, len(targets))

	for _, t := range targets {
		binName := fmt.Sprintf("tachyon-installer-%s-%s%s", t.OS, t.Arch, t.Ext)
		binPath := filepath.Join(distDir, binName)

		fmt.Printf("  • Compiling %s/%s -> %s ... ", t.OS, t.Arch, binName)

		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", binPath, ".")
		cmd.Env = append(os.Environ(),
			"CGO_ENABLED=0",
			"GOOS="+t.OS,
			"GOARCH="+t.Arch,
		)

		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("FAILED ❌\n%s\n", string(out))
			os.Exit(1)
		}

		st, _ := os.Stat(binPath)
		mb := float64(st.Size()) / 1024 / 1024
		fmt.Printf("OK ✓ (%.1f MB)\n", mb)
	}

	// Generate SHA256 sums
	fmt.Printf("\n🔒 Generating SHA256 checksums (sha256sums.txt)...\n")
	checksumFile := filepath.Join(distDir, "sha256sums.txt")
	f, err := os.Create(checksumFile)
	if err == nil {
		defer f.Close()
		entries, _ := os.ReadDir(distDir)
		for _, e := range entries {
			if e.IsDir() || e.Name() == "sha256sums.txt" {
				continue
			}
			filePath := filepath.Join(distDir, e.Name())
			hash, err := fileSHA256(filePath)
			if err == nil {
				f.WriteString(fmt.Sprintf("%s  %s\n", hash, e.Name()))
			}
		}
		fmt.Printf("✓ SHA256 checksums written to %s\n", checksumFile)
	}

	fmt.Printf("\n✨ All desktop binaries ready in %s/ (clean standalone binaries without archives)\n", distDir)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
