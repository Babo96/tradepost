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
	want := filepath.Join(wowDir, "_forever_", "WTF", "Account", "123#1", "SavedVariables", "TradePost.lua")
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}

	absolute := "/tmp/account"
	got = SavedVariablesFile(wowDir, absolute)
	want = filepath.Join(absolute, "SavedVariables", "TradePost.lua")
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

func TestMetaAckSlots(t *testing.T) {
	wowDir := t.TempDir()
	if err := os.MkdirAll(AddonsDir(wowDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureMetaAckSlots(wowDir); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < MetaAckSlots; index++ {
		tocPath := filepath.Join(AddonsDir(wowDir), MetaAckSlotName(index), MetaAckSlotName(index)+".toc")
		data, err := os.ReadFile(tocPath)
		if err != nil {
			t.Fatalf("missing ack TOC %d: %v", index, err)
		}
		if !strings.Contains(string(data), "## LoadOnDemand: 1") ||
			!strings.Contains(string(data), "## Group: TradePost_LiveData") {
			t.Fatalf("unexpected ack TOC %d:\n%s", index, data)
		}
	}

	if err := WriteMetaAck(wowDir, 17); err != nil {
		t.Fatal(err)
	}
	// Every slot carries the acknowledgement, so any unloaded slot the addon
	// consumes yields the current batch.
	for index := 0; index < MetaAckSlots; index++ {
		data, err := os.ReadFile(MetaAckSlotPath(wowDir, index))
		if err != nil {
			t.Fatalf("missing ack payload %d: %v", index, err)
		}
		if !strings.Contains(string(data), "batch = 17") || !strings.Contains(string(data), "ok = true") {
			t.Fatalf("unexpected ack payload %d:\n%s", index, data)
		}
	}
	// 17 % 16 == 1
	data, err := os.ReadFile(MetaAckSlotPath(wowDir, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "batch = 17") || !strings.Contains(string(data), "ok = true") {
		t.Fatalf("unexpected ack payload:\n%s", data)
	}

	// Re-ensuring must not overwrite a pending ack.
	if err := EnsureMetaAckSlots(wowDir); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(MetaAckSlotPath(wowDir, 1))
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(data) {
		t.Fatal("EnsureMetaAckSlots overwrote an existing ack")
	}
}

func TestInstall(t *testing.T) {
	source := t.TempDir()
	addon := filepath.Join(source, "addon", "TradePost")
	dataAddon := filepath.Join(source, "data-addon", "TradePost_Data")
	liveAddon := filepath.Join(source, "data-addon", "TradePost_LiveData")
	if err := os.MkdirAll(addon, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataAddon, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(liveAddon, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addon, "TradePost.toc"), []byte("## Interface: 16001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataAddon, "TradePost_Data.toc"), []byte("## Interface: 16001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveAddon, "TradePost_LiveData.toc"), []byte("## Interface: 16001\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wowDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wowDir, "_forever_", "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(source, wowDir, 3); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"TradePost", "TradePost_Data", "TradePost_LiveData", "TradePost_Data_00", "TradePost_Data_02"} {
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

func TestCompanionStatus(t *testing.T) {
	wowDir := t.TempDir()
	if err := os.MkdirAll(AddonsDir(wowDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteCompanionStatus(wowDir, true, "0.2.16", "https://github.com/Babo96/tradepost/releases/tag/v0.2.16"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(CompanionStatusPath(wowDir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "online = true") ||
		!strings.Contains(string(data), `addon_latest = "0.2.16"`) {
		t.Fatalf("status did not publish online/latest:\n%s", data)
	}
	toc, err := os.ReadFile(filepath.Join(AddonsDir(wowDir), CompanionStatusAddon, CompanionStatusAddon+".toc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(toc), "## Title: TradePost Companion") ||
		!strings.Contains(string(toc), `## IconTexture: Interface\AddOns\TradePost\Media\icon`) {
		t.Fatalf("unexpected status TOC:\n%s", toc)
	}

	// Upgrading an older generated status addon must add the icon metadata.
	if err := os.WriteFile(filepath.Join(AddonsDir(wowDir), CompanionStatusAddon, CompanionStatusAddon+".toc"), []byte("## Interface: 16001\n## Title: TradePost Companion\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureCompanionStatus(wowDir); err != nil {
		t.Fatal(err)
	}
	toc, err = os.ReadFile(filepath.Join(AddonsDir(wowDir), CompanionStatusAddon, CompanionStatusAddon+".toc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(toc), "## IconTexture:") {
		t.Fatalf("status TOC was not upgraded:\n%s", toc)
	}
	if err := WriteCompanionStatus(wowDir, false, "", ""); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(CompanionStatusPath(wowDir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "online = false") {
		t.Fatalf("status did not publish online=false:\n%s", data)
	}
}
