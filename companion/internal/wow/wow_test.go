package wow

import (
	"os"
	"path/filepath"
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
