//go:build windows

package main

import (
	"log"
	"os"
	"syscall"
)

// attachConsole reconnects stdio to the parent console when the GUI-subsystem
// executable is launched from cmd/PowerShell. Double-clicking without a parent
// console leaves the process GUI-only.
func attachConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	// ATTACH_PARENT_PROCESS
	if ok, _, _ := attach.Call(^uintptr(0)); ok == 0 {
		return
	}
	if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = out
		os.Stderr = out
		log.SetOutput(out)
	}
	if in, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = in
	}
}
