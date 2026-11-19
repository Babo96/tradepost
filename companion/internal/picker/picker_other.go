//go:build !linux && !windows

package picker

func pickFolder(title string) (string, error) {
	return "", ErrUnavailable
}
