package export

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TradePost.lua")
	data := `TRADEPOST_DB = {}
TRADEPOST_EXPORT = {
  version = 1,
  scans = {
    "eu:pve:horde|1794947776|i:2589,1234,50,2;i:2318,4567,100,3",
    "eu:pve:horde|1794947777|g:12345:70,4200,1,1",
  },
}
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	scans, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != 2 {
		t.Fatalf("expected 2 scans, got %d", len(scans))
	}
	if scans[0].Market != "eu:pve:horde" || scans[0].ScannedAt != 1794947776 {
		t.Fatalf("unexpected scan: %+v", scans[0])
	}
	if len(scans[0].Items) != 2 || scans[0].Items[0].Key != "i:2589" {
		t.Fatalf("unexpected items: %+v", scans[0].Items)
	}
	if scans[0].Items[0].Price != 1234 || scans[0].Items[1].Listings != 3 {
		t.Fatalf("unexpected item values: %+v", scans[0].Items)
	}
}

func TestLoadMissing(t *testing.T) {
	scans, err := Load(filepath.Join(t.TempDir(), "missing.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scans) != 0 {
		t.Fatalf("expected no scans, got %d", len(scans))
	}
}

func TestParseMalformed(t *testing.T) {
	if _, err := parseRecord("broken"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := parseRecord("eu:pve:horde|notatime|i:1,1,1,1"); err == nil {
		t.Fatal("expected time error")
	}
	if _, err := parseRecord("eu:pve:horde|1|i:1,1"); err == nil {
		t.Fatal("expected item error")
	}
}

func TestLoadDataItems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TradePost.lua")
	data := `TRADEPOST_EXPORT = {
  version = 1,
  scans = {
    "eu:pve:horde|1794947776|i:2589,1234,50,2",
  },
}
TRADEPOST_ITEMS = {
  [2589] = { n = "Linen Cloth", q = 1, c = 7, s = 5, i = 132889 },
  [12345] = { n = "Test Blade", q = 3, c = 2, s = 7, i = 135274 },
}
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadData(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Scans) != 1 {
		t.Fatalf("expected 1 scan, got %d", len(loaded.Scans))
	}
	if len(loaded.Items) != 2 {
		t.Fatalf("expected 2 metadata entries, got %d", len(loaded.Items))
	}
	if loaded.Items[0].Name != "Linen Cloth" || loaded.Items[0].ID != 2589 {
		t.Fatalf("unexpected metadata: %+v", loaded.Items[0])
	}
	if loaded.Items[1].Icon != 135274 || loaded.Items[1].Quality != 3 {
		t.Fatalf("unexpected metadata: %+v", loaded.Items[1])
	}
	if loaded.Items[0].Hash() == loaded.Items[1].Hash() {
		t.Fatal("expected different metadata hashes")
	}
}

func TestLoadMetaBatchAndDetails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TradePost.lua")
	data := `TRADEPOST_ITEM_DETAILS = {
  [2589] = {
    id = 2589, name = "Linen Cloth", quality = 1,
    class_id = 7, subclass_id = 5, icon = 132889,
    item_level = 5, required_level = 1, bind_type = 0,
    inventory_type = 0, equip_loc = "", vendor_price = 12,
    max_stack = 20, unique_item = false, set_id = 0,
    stats = { ITEM_MOD_STAMINA = 5, ITEM_MOD_ARMOR = 12 },
    details = {
      lines = {
        { left = "Linen Cloth", right = "", type = 0 },
        { left = "5 Armor", right = "", type = 1 },
      },
    },
  },
}
TRADEPOST_META_BATCH = {
  seq = 7,
  count = 1,
  items = {
    {
      id = 12345, name = "Test Blade", quality = 3,
      class_id = 2, subclass_id = 7, icon = 135274,
      item_level = 70, required_level = 60, bind_type = 1,
      inventory_type = 13, equip_loc = "MAINHAND", armor = 0,
      damage_min = 10, damage_max = 20, delay = 2000,
      vendor_price = 999, max_stack = 1, unique_item = true, set_id = 42,
      stats = { ITEM_MOD_AGILITY = 7 },
      details = { lines = { { left = "Test Blade", right = "", type = 0 } } },
    },
  },
}
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadData(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Details) != 1 {
		t.Fatalf("expected 1 detail entry, got %d", len(loaded.Details))
	}
	detail := loaded.Details[0]
	if detail.ID != 2589 || detail.Stats["ITEM_MOD_STAMINA"] != 5 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Details == nil || len(detail.Details.Lines) != 2 || detail.Details.Lines[1].Left != "5 Armor" {
		t.Fatalf("unexpected detail lines: %+v", detail.Details)
	}
	if detail.ItemLevel == nil || *detail.ItemLevel != 5 || detail.RequiredLevel == nil || *detail.RequiredLevel != 1 {
		t.Fatalf("unexpected optional levels: %+v", detail)
	}

	if loaded.MetaBatch == nil || loaded.MetaBatch.Seq != 7 {
		t.Fatalf("unexpected batch: %+v", loaded.MetaBatch)
	}
	if len(loaded.MetaBatch.Items) != 1 {
		t.Fatalf("expected 1 batch item, got %d", len(loaded.MetaBatch.Items))
	}
	item := loaded.MetaBatch.Items[0]
	if item.ID != 12345 || item.EquipLoc == nil || *item.EquipLoc != "MAINHAND" {
		t.Fatalf("unexpected batch item: %+v", item)
	}
	if item.SetID == nil || *item.SetID != 42 || item.UniqueItem == nil || !*item.UniqueItem {
		t.Fatalf("unexpected batch optional fields: %+v", item)
	}
	if item.Stats["ITEM_MOD_AGILITY"] != 7 || item.Details == nil || len(item.Details.Lines) != 1 {
		t.Fatalf("unexpected batch stats/details: %+v", item)
	}

	// Two otherwise identical records with different details must hash apart.
	other := item
	other.Details = &Details{Lines: []DetailLine{{Left: "Different", Type: 1}}}
	if item.Hash() == other.Hash() {
		t.Fatal("expected details to affect metadata hash")
	}
}

func TestLoadDataAuthWatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TradePost.lua")
	data := `TRADEPOST_AUTH = {
  seq = 3,
  request = {
    action = "login",
    character = "Schmoo-Boo",
    market = "eu:pve:horde",
    at = 1794947776,
  },
}
TRADEPOST_WATCH = {
  ["Schmoo-Boo"] = {
    updated = 1794947800,
    items = {
      ["i:1251"] = true,
      ["i:2589"] = true,
    },
  },
}
TRADEPOST_ACTIVE = { character = "Schmoo-Boo", seen = 1794947801 }
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadData(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth == nil || loaded.Auth.Seq != 3 || loaded.Auth.Character != "Schmoo-Boo" {
		t.Fatalf("unexpected auth: %+v", loaded.Auth)
	}
	if loaded.Active != "Schmoo-Boo" {
		t.Fatalf("unexpected active character: %q", loaded.Active)
	}
	watch := loaded.Watch["Schmoo-Boo"]
	if watch.Updated != 1794947800 || len(watch.Items) != 2 || watch.Items[0] != "i:1251" {
		t.Fatalf("unexpected watch: %+v", watch)
	}
}

func TestLoadRingRequest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TradePost.lua")
	data := `TRADEPOST_EXPORT = { ringRequest = { market = "eu:pve:horde", at = 1794947776 } }`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := LoadData(path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.RingRequest == nil || parsed.RingRequest.Market != "eu:pve:horde" || parsed.RingRequest.At != 1794947776 {
		t.Fatalf("unexpected ring request: %+v", parsed.RingRequest)
	}
}

func TestLoadActiveMarket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TradePost.lua")
	data := `TRADEPOST_ACTIVE = { character = "Schmoo-Boo", market = "eu:pvp:alliance", seen = 1794947776 }`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := LoadData(path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Active != "Schmoo-Boo" || parsed.ActiveMarket != "eu:pvp:alliance" {
		t.Fatalf("unexpected active character/market: %q / %q", parsed.Active, parsed.ActiveMarket)
	}
}
