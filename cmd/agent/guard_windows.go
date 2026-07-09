//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	seKernelObject          = 6
	daclSecurityInformation = 0x00000004
	aclRevision             = 2
	winWorldSid             = 1

	processTerminate     = 0x0001
	processCreateThread  = 0x0002
	processVmOperation   = 0x0008
	processVmWrite       = 0x0020
	processSuspendResume = 0x0800

	denyMask = processTerminate | processCreateThread |
		processVmOperation | processVmWrite | processSuspendResume
)

var (
	modAdvapi32 = syscall.NewLazyDLL("advapi32.dll")

	procCreateWellKnownSid = modAdvapi32.NewProc("CreateWellKnownSid")
	procInitializeAcl      = modAdvapi32.NewProc("InitializeAcl")
	procAddAccessDeniedAce = modAdvapi32.NewProc("AddAccessDeniedAce")
	procSetSecurityInfo    = modAdvapi32.NewProc("SetSecurityInfo")
)

// currentProcessHandle is the Windows pseudo-handle for the current process.
const currentProcessHandle = ^uintptr(0)

func protectProcess() error {
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

	r, _, _ = procAddAccessDeniedAce.Call(
		uintptr(unsafe.Pointer(&aclBuf[0])),
		uintptr(aclRevision),
		uintptr(denyMask),
		uintptr(unsafe.Pointer(&sidBuf[0])),
	)
	if r == 0 {
		return fmt.Errorf("AddAccessDeniedAce: %w", syscall.GetLastError())
	}

	ret, _, _ := procSetSecurityInfo.Call(
		currentProcessHandle,
		uintptr(seKernelObject),
		uintptr(daclSecurityInformation),
		0, 0,
		uintptr(unsafe.Pointer(&aclBuf[0])),
		0,
	)
	if ret != 0 {
		return fmt.Errorf("SetSecurityInfo returned %d", ret)
	}

	return nil
}
