//go:build windows

package picker

import (
	"fmt"
	"os/exec"
	"strings"
)

func pickFolder(title string) (string, error) {
	safeTitle := strings.ReplaceAll(title, "'", "''")
	script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
$dialog.Description = '%s'
$dialog.ShowNewFolderButton = $false
if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
  Write-Output $dialog.SelectedPath
}`, safeTitle)
	out, err := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script).Output()
	if err != nil {
		return "", fmt.Errorf("powershell folder picker: %w", ErrUnavailable)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("folder picker was cancelled")
	}
	return path, nil
}
