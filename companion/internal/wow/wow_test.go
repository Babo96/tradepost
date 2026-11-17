package wow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavedVariablesFile(t *testing.T) {
	wowDir := "/games/World of Warcraft"
	got := SavedVariablesFile(wowDir, "123#1")
	want := filepath.Join(wowDir, "_forever_", "WTF", "Account", "123#1", "SavedVariables", "BaboForever.lua")
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}

	absolute := "/tmp/account"
	got = SavedVariablesFile(wowDir, absolute)
	want = filepath.Join(absolute, "SavedVariables", "BaboForever.lua")
	if got != want {
		t.Fatalf("absolute: got %s want %s", got, want)
	}
}

func TestNormalizeRoot(t *testing.T) {
	cases := map[string]string{
		"/games/World of Warcraft":              "/games/World of Warcraft",
		"/games/World of Warcraft/_forever_":    "/games/World of Warcraft",
		"/games/World of Warcraft/_forever_/":   "/games/World of Warcraft",
		`C:\Games\World of Warcraft`:            `C:\Games\World of Warcraft`,
		`C:\Games\World of Warcraft\_forever_`:  `C:\Games\World of Warcraft`,
		`C:\Games\World of Warcraft\_forever_\`: `C:\Games\World of Warcraft`,
		"":                                      "",
	}
	for input, want := range cases {
		if got := normalizeRoot(input); got != want {
			t.Errorf("normalizeRoot(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFindAcceptsForeverDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "_forever_", "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Find(filepath.Join(root, "_forever_"))
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("got %s, want %s", got, root)
	}
}

func TestInstall(t *testing.T) {
	source := t.TempDir()
	addon := filepath.Join(source, "addon", "BaboForever")
	dataAddon := filepath.Join(source, "data-addon", "BaboForever_Data")
	liveAddon := filepath.Join(source, "data-addon", "BaboForever_LiveData")
	if err := os.MkdirAll(addon, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataAddon, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(liveAddon, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addon, "BaboForever.toc"), []byte("## Interface: 16001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataAddon, "BaboForever_Data.toc"), []byte("## Interface: 16001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveAddon, "BaboForever_LiveData.toc"), []byte("## Interface: 16001\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wowDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wowDir, "_forever_", "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(source, wowDir, 3); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"BaboForever", "BaboForever_Data", "BaboForever_LiveData", "BaboForever_Data_00", "BaboForever_Data_02"} {
		if _, err := os.Stat(filepath.Join(AddonsDir(wowDir), name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	slot := SlotPath(wowDir, 1)
	data, err := os.ReadFile(slot)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("empty slot")
	}
}

func TestAuthSlots(t *testing.T) {
	wowDir := t.TempDir()
	addons := AddonsDir(wowDir)
	if err := os.MkdirAll(addons, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAuthSlots(wowDir); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < AuthSlots; index++ {
		if _, err := os.Stat(filepath.Join(addons, AuthSlotName(index), AuthSlotName(index)+".toc")); err != nil {
			t.Fatalf("missing slot TOC %d: %v", index, err)
		}
	}
	if err := WriteAuthReply(wowDir, 9, "ABCD2345", "Schmoo-Boo", 1794949000); err != nil {
		t.Fatal(err)
	}
	path := AuthSlotPath(wowDir, 1)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `code = "ABCD2345"`) || !strings.Contains(string(data), `seq = 9`) {
		t.Fatalf("unexpected auth reply:\n%s", data)
	}

	// Re-ensuring must not overwrite a pending reply.
	if err := EnsureAuthSlots(wowDir); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(data) {
		t.Fatal("EnsureAuthSlots overwrote an existing reply")
	}
}
