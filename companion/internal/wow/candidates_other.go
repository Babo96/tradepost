//go:build !linux && !windows

package wow

// defaultCandidates is empty on platforms without a known WoW install layout.
// Users can always pass --wow or set WOW_DIR.
func defaultCandidates() []string {
	return nil
}
