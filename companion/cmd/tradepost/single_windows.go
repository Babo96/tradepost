//go:build windows

package main

import (
	"log"
	"syscall"
	"unsafe"
)

var singleInstanceHandle syscall.Handle

// acquireSingleInstance succeeds only for the first companion run in this
// Windows session. The named mutex is released automatically when the process
// exits.
func acquireSingleInstance() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createMutex := kernel32.NewProc("CreateMutexW")
	name, _ := syscall.UTF16PtrFromString(`Local\TradePostCompanion`)
	h, _, err := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		log.Printf("single instance: CreateMutex failed: %v", err)
		return true // fail open rather than refuse to start
	}
	if errno, ok := err.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
		syscall.CloseHandle(syscall.Handle(h))
		return false
	}
	singleInstanceHandle = syscall.Handle(h)
	return true
}
