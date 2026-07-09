//go:build windows

package agent

import (
	"os/exec"
	"syscall"
)

// newCommand wraps exec.Command with CREATE_NO_WINDOW so that child processes
// (powershell, ffmpeg) never flash a visible console on the desktop.
func newCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd
}
