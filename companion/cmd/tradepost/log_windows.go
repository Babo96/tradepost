//go:build windows

package main

import (
	"log"
	"os"
	"path/filepath"
)

// setupLogging keeps a log next to the companion config because the Windows
// GUI build has no console.
func setupLogging() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "tradepost")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "companion.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		log.SetOutput(f)
	}
}
