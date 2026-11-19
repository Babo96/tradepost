package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultServer is the public TradePost server used for new installs.
	DefaultServer = "https://tradepost.b4b0.de"
	DefaultMarket = "eu:pve:horde"
	DefaultRings  = 64
	DefaultAddr   = "127.0.0.1:8787"
)

type Config struct {
	Server          string `json:"server"`
	Token           string `json:"token"`
	ClientID        string `json:"client_id"`
	WowDir          string `json:"wow_dir"`
	Market          string `json:"market"`
	RingSlots       int    `json:"ring_slots"`
	IntervalSeconds int    `json:"interval_seconds"`
	DashboardAddr   string `json:"dashboard_addr"`
	AddonSource     string `json:"addon_source"`
}

// SetupRequest is the payload submitted by the setup wizard or the settings
// page. Empty fields keep the current (or default) value.
type SetupRequest struct {
	WowDir          string `json:"wow_dir"`
	Server          string `json:"server"`
	Market          string `json:"market"`
	RingSlots       int    `json:"ring_slots"`
	IntervalSeconds int    `json:"interval_seconds"`
	AddonSource     string `json:"addon_source"`
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tradepost", "config.json")
}

func Defaults() Config {
	return Config{
		Server:          DefaultServer,
		WowDir:          "",
		Market:          DefaultMarket,
		RingSlots:       DefaultRings,
		IntervalSeconds: 60,
		DashboardAddr:   DefaultAddr,
		AddonSource:     "",
	}
}

// Missing returns the names of the fields that prevent the companion from
// syncing. A missing token or game folder means first-run setup is required.
func (c Config) Missing() []string {
	var missing []string
	if strings.TrimSpace(c.Server) == "" {
		missing = append(missing, "server")
	}
	if strings.TrimSpace(c.Token) == "" {
		missing = append(missing, "token")
	}
	if strings.TrimSpace(c.WowDir) == "" {
		missing = append(missing, "wow_dir")
	}
	return missing
}

// Complete reports whether the config has everything needed to sync. It does
// not check that the game folder still exists; use wow.Validate for that.
func (c Config) Complete() bool {
	return len(c.Missing()) == 0
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Server == "" {
		cfg.Server = DefaultServer
	}
	if cfg.Market == "" {
		cfg.Market = DefaultMarket
	}
	if cfg.RingSlots < 1 {
		cfg.RingSlots = DefaultRings
	}
	if cfg.IntervalSeconds < 5 {
		cfg.IntervalSeconds = 60
	}
	if cfg.DashboardAddr == "" {
		cfg.DashboardAddr = DefaultAddr
	}
	return cfg, nil
}

// Save writes the config atomically (temp file in the same directory, fsync,
// rename) so a crash cannot leave a half-written file behind.
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
