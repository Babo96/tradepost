package export

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "BaboForever.lua")
	data := `BABOFOREVER_DB = {}
BABOFOREVER_EXPORT = {
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
	path := filepath.Join(dir, "BaboForever.lua")
	data := `BABOFOREVER_EXPORT = {
  version = 1,
  scans = {
    "eu:pve:horde|1794947776|i:2589,1234,50,2",
  },
}
BABOFOREVER_ITEMS = {
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
