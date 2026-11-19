// Package browser opens a URL in the user's default browser.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open launches url with the platform's default browser.
func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	// Reap the child so it does not become a zombie; do not wait on the
	// browser process itself.
	go func() { _ = cmd.Wait() }()
	return nil
}
