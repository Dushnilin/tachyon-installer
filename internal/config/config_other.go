//go:build !windows

package config

import "os/exec"

func setHideWindow(cmd *exec.Cmd) {
	// no-op on non-Windows platforms
	_ = cmd
}
