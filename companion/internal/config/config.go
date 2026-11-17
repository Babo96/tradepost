package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "babo-forever", "config.json")
}

func Defaults() Config {
	return Config{
		Server:          "http://127.0.0.1:8080",
		WowDir:          "",
		Market:          "eu:pve:horde",
		RingSlots:       64,
		IntervalSeconds: 60,
		DashboardAddr:   "127.0.0.1:8787",
		AddonSource:     "",
	}
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
	if cfg.RingSlots < 1 {
		cfg.RingSlots = 64
	}
	if cfg.IntervalSeconds < 5 {
		cfg.IntervalSeconds = 60
	}
	if cfg.DashboardAddr == "" {
		cfg.DashboardAddr = "127.0.0.1:8787"
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
