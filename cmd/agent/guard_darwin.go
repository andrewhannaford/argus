//go:build darwin

package main

import (
	"os/signal"
	"syscall"
)

func protectProcess() error {
	// macOS doesn't expose prctl; signal masking is the available user-space
	// protection without kernel extensions or entitlements.
	// Full protection relies on the LaunchDaemon running as root (see installDarwin).
	signal.Ignore(syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGUSR2)
	return nil
}
