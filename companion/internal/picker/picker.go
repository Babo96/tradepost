// Package picker opens the operating system's native folder chooser.
package picker

import "errors"

// ErrUnavailable means no native folder picker could be found. Callers should
// show the message and let the user type the path manually.
var ErrUnavailable = errors.New("no native folder picker available; type the folder path instead")

// PickFolder opens a folder chooser and returns the selected absolute path.
func PickFolder(title string) (string, error) {
	return pickFolder(title)
}
