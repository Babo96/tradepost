package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babo96/tradepost/companion/internal/api"
	"github.com/babo96/tradepost/companion/internal/wow"
)

func setupWow(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	addons := filepath.Join(dir, "_forever_", "Interface", "AddOns")
	for _, name := range []string{"TradePost_Data", "TradePost_Data_02"} {
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
	path, err := WriteRingSlot(wowDir, "eu:pve:horde", snapshot, 8, 10, []string{"i:2589"}, 1794947800)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "TradePost_Data_02/Data.lua") {
		t.Fatalf("unexpected slot path: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `window = 10`) || !strings.Contains(string(data), `seq = 9`) {
		t.Fatalf("unexpected ring slot:\n%s", data)
	}
	if !strings.Contains(string(data), `"i:2589"`) || !strings.Contains(string(data), `pins_updated = 1794947800`) {
		t.Fatalf("unexpected pins in ring slot:\n%s", data)
	}
}

func TestWriteAuthRingSlots(t *testing.T) {
	wowDir := setupWow(t)
	if err := WriteAuthRingSlots(wowDir, "eu:pve:horde", 5, "ABCD2345", "Schmoo-Boo", 1794949000, 64); err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < wow.SyncSlots; offset++ {
		index := wow.SyncSlotIndex(64, offset)
		data, err := os.ReadFile(wow.SlotPath(wowDir, index))
		if err != nil {
			t.Fatalf("slot %d: %v", index, err)
		}
		if !strings.Contains(string(data), `code = "ABCD2345"`) || !strings.Contains(string(data), `seq = 5`) {
			t.Fatalf("slot %d missing auth payload:\n%s", index, data)
		}
	}
}

func TestPriceRingSkipsSyncSlots(t *testing.T) {
	wowDir := setupWow(t)
	snapshot := api.Snapshot{
		Market: "eu:pve:horde", Version: 7, Generated: 1794947776,
		Items: []api.SnapshotItem{{Key: "i:1", Price: 1, Quantity: 1, Listings: 1, Scanned: 1794947776}},
	}
	path, err := WriteRingSlot(wowDir, "eu:pve:horde", snapshot, 64, 60, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "TradePost_Data_04/Data.lua") {
		t.Fatalf("price ring used a reserved sync slot: %s", path)
	}
}
