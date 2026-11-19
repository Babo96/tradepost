//go:build linux

package picker

import (
	"fmt"
	"os/exec"
	"strings"
)

// These variables are seams for unit tests.
var (
	lookPath   = exec.LookPath
	runCommand = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).Output()
	}
)

func pickFolder(title string) (string, error) {
	if path, err := lookPath("zenity"); err == nil {
		out, err := runCommand(path, "--file-selection", "--directory", "--title", title)
		if err != nil {
			return "", fmt.Errorf("zenity: %w", err)
		}
		return clean(out)
	}
	if path, err := lookPath("kdialog"); err == nil {
		out, err := runCommand(path, "--getexistingdirectory", ".", "--title", title)
		if err != nil {
			return "", fmt.Errorf("kdialog: %w", err)
		}
		return clean(out)
	}
	return "", ErrUnavailable
}

func clean(out []byte) (string, error) {
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("folder picker was cancelled")
	}
	return path, nil
}
