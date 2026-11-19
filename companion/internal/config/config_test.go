package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigCompleteness(t *testing.T) {
	cfg := Defaults()
	if cfg.Complete() {
		t.Fatal("defaults without a token or game folder must not be complete")
	}
	if got, want := cfg.Missing(), []string{"token", "wow_dir"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}

	cfg.Token = "abc"
	cfg.WowDir = "/games/World of Warcraft"
	if !cfg.Complete() {
		t.Fatalf("config should be complete, missing %v", cfg.Missing())
	}

	cfg.Server = "   "
	if cfg.Complete() {
		t.Fatal("blank server must not count as complete")
	}
	if got := cfg.Missing(); len(got) == 0 || got[0] != "server" {
		t.Fatalf("missing = %v, want server first", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	cfg := Defaults()
	cfg.Token = "secret"
	cfg.WowDir = "/games/World of Warcraft"
	cfg.RingSlots = 32
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("loaded %+v, want %+v", loaded, cfg)
	}
}

func TestLoadFillsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"token":"t","wow_dir":"/wow","ring_slots":0,"interval_seconds":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RingSlots != DefaultRings || cfg.IntervalSeconds != 60 || cfg.DashboardAddr != DefaultAddr {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if cfg.Server != DefaultServer || cfg.Market != DefaultMarket {
		t.Fatalf("server/market defaults not applied: %+v", cfg)
	}
	if !cfg.Complete() {
		t.Fatalf("expected complete, missing %v", cfg.Missing())
	}
}
