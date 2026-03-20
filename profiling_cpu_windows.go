//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	modKernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGetProcessTimes = modKernel32.NewProc("GetProcessTimes")
)

// filetimeToUint64 converts FILETIME hi/lo parts to a single 64-bit tick value.
func filetimeToUint64(ft syscall.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

// readCPUSeconds returns total process CPU time (user+kernel) in seconds.
func readCPUSeconds() float64 {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}

	var creation syscall.Filetime
	var exit syscall.Filetime
	var kernel syscall.Filetime
	var user syscall.Filetime

	r1, _, _ := procGetProcessTimes.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r1 == 0 {
		return 0
	}

	// FILETIME uses 100ns ticks.
	totalTicks := filetimeToUint64(kernel) + filetimeToUint64(user)
	return float64(totalTicks) / 1e7
}
