//go:build linux

package picker

import (
	"errors"
	"testing"
)

func TestPickFolderNoPicker(t *testing.T) {
	originalLook := lookPath
	t.Cleanup(func() { lookPath = originalLook })
	lookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}
	if _, err := PickFolder("test"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
}

func TestPickFolderFallsBackToKDialog(t *testing.T) {
	originalLook := lookPath
	originalRun := runCommand
	t.Cleanup(func() {
		lookPath = originalLook
		runCommand = originalRun
	})
	lookPath = func(name string) (string, error) {
		if name == "kdialog" {
			return "/usr/bin/kdialog", nil
		}
		return "", errors.New("not found")
	}
	var gotName string
	var gotArgs []string
	runCommand = func(name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = args
		return []byte("/games/World of Warcraft\n"), nil
	}
	path, err := PickFolder("Select WoW")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/games/World of Warcraft" {
		t.Fatalf("got %q", path)
	}
	if gotName != "/usr/bin/kdialog" {
		t.Fatalf("ran %q", gotName)
	}
	if len(gotArgs) < 2 || gotArgs[0] != "--getexistingdirectory" {
		t.Fatalf("args %v", gotArgs)
	}
}

func TestPickFolderPrefersZenity(t *testing.T) {
	originalLook := lookPath
	originalRun := runCommand
	t.Cleanup(func() {
		lookPath = originalLook
		runCommand = originalRun
	})
	lookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	runCommand = func(name string, args ...string) ([]byte, error) {
		if name != "/usr/bin/zenity" {
			t.Fatalf("ran %q, want zenity", name)
		}
		return []byte("/wow\n"), nil
	}
	path, err := PickFolder("Select WoW")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/wow" {
		t.Fatalf("got %q", path)
	}
}
