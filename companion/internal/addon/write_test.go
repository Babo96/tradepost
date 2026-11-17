package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"babo-forever/companion/internal/api"
	"babo-forever/companion/internal/wow"
)

func setupWow(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	addons := filepath.Join(dir, "_forever_", "Interface", "AddOns")
	for _, name := range []string{"BaboForever_Data", "BaboForever_Data_02"} {
		if err := os.MkdirAll(filepath.Join(addons, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestWriteDataAddon(t *testing.T) {
	wowDir := setupWow(t)
	snapshot := api.Snapshot{
		Market:    "eu:pve:horde",
		Version:   7,
		Generated: 1794947776,
		Items: []api.SnapshotItem{
			{Key: "i:2589", Price: 1234, Quantity: 50, Listings: 2, Scanned: 1794947776},
		},
	}
	if err := WriteDataAddon(wowDir, []api.Snapshot{snapshot}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(wow.DataAddonPath(wowDir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `["eu:pve:horde"]`) || !strings.Contains(string(data), `k = "i:2589"`) {
		t.Fatalf("unexpected data addon:\n%s", data)
	}
}

func TestWriteRingSlot(t *testing.T) {
	wowDir := setupWow(t)
	snapshot := api.Snapshot{
		Market:    "eu:pve:horde",
		Version:   9,
		Generated: 1794947776,
		Items: []api.SnapshotItem{
			{Key: "i:2318", Price: 4567, Quantity: 100, Listings: 3, Scanned: 1794947776},
		},
	}
	path, err := WriteRingSlot(wowDir, "eu:pve:horde", snapshot, 8, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "BaboForever_Data_02/Data.lua") {
		t.Fatalf("unexpected slot path: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `window = 10`) || !strings.Contains(string(data), `seq = 9`) {
		t.Fatalf("unexpected ring slot:\n%s", data)
	}
}
