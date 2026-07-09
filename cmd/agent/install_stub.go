//go:build !windows

package main

import "fmt"

// installWindows and uninstallWindows are no-ops on non-Windows platforms.
// They exist so main.go can reference them unconditionally inside runtime.GOOS
// switch arms without breaking cross-platform compilation.

func installWindows(_, _, _, _ string) error {
	return fmt.Errorf("Windows install not supported on %s", "this platform")
}

func uninstallWindows() error {
	return fmt.Errorf("Windows uninstall not supported on this platform")
}
