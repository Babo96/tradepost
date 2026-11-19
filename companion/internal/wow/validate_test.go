package wow

import (
	"os"
	"path/filepath"
	"testing"
)

func makeInstall(t *testing.T, account bool) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, foreverDir, "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}
	if account {
		if err := os.MkdirAll(filepath.Join(root, foreverDir, "WTF", "Account", "123#1", "SavedVariables"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestValidateInstall(t *testing.T) {
	root := makeInstall(t, true)
	info, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Root != root {
		t.Fatalf("root = %q, want %q", info.Root, root)
	}
	if len(info.Accounts) != 1 || info.Accounts[0] != filepath.Join(root, foreverDir, "WTF", "Account", "123#1") {
		t.Fatalf("accounts = %v", info.Accounts)
	}
	if !info.Writable {
		t.Fatal("expected writable addon folder")
	}

	// A path ending in _forever_ resolves to the parent root.
	info, err = Validate(filepath.Join(root, foreverDir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Root != root {
		t.Fatalf("normalized root = %q, want %q", info.Root, root)
	}
}

func TestValidateRejectsBadInstalls(t *testing.T) {
	empty := t.TempDir()
	if _, err := Validate(empty); err == nil {
		t.Fatal("empty folder must fail validation")
	}

	noAddons := t.TempDir()
	if err := os.MkdirAll(filepath.Join(noAddons, foreverDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(noAddons); err == nil {
		t.Fatal("missing Interface/AddOns must fail validation")
	}

	noAccount := makeInstall(t, false)
	if _, err := Validate(noAccount); err == nil {
		t.Fatal("missing account directories must fail validation")
	}
}

func TestDetectAndCandidates(t *testing.T) {
	root := makeInstall(t, true)
	if got := Detect(root); len(got) == 0 || got[0] != root {
		t.Fatalf("Detect = %v, want %s first", got, root)
	}
	found := false
	for _, candidate := range Candidates(root) {
		if candidate == root {
			found = true
		}
	}
	if !found {
		t.Fatalf("Candidates did not include %s: %v", root, Candidates(root))
	}
}
