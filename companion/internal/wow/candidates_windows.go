//go:build windows

package wow

import (
	"os"
	"path/filepath"
)

func defaultCandidates() []string {
	roots := []string{
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("ProgramFiles"),
		os.Getenv("LOCALAPPDATA"),
	}
	candidates := make([]string, 0, len(roots)+2)
	for _, root := range roots {
		if root == "" {
			continue
		}
		candidates = append(candidates, filepath.Join(root, "World of Warcraft"))
	}
	candidates = append(candidates,
		`C:\World of Warcraft`,
		`C:\Games\World of Warcraft`,
	)
	return candidates
}
