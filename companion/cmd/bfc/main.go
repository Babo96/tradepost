package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"babo-forever/companion/internal/api"
	"babo-forever/companion/internal/config"
	"babo-forever/companion/internal/dashboard"
	syncsvc "babo-forever/companion/internal/sync"
	"babo-forever/companion/internal/wow"
)

var version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "enroll":
		err = cmdEnroll(os.Args[2:])
	case "install":
		err = cmdInstall(os.Args[2:])
	case "sync":
		err = cmdSync(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "version":
		fmt.Println("bfc " + version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Babo Forever Companion

  bfc enroll  --server URL [--wow DIR] [--market eu:pve:horde]
  bfc install [--source DIR] [--wow DIR] [--count 64]
  bfc sync    [--config PATH]
  bfc run     [--config PATH]
  bfc status  [--config PATH]

Config: %s
`, config.DefaultPath())
}

func cmdEnroll(args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config path")
	server := fs.String("server", "", "server URL")
	wowDir := fs.String("wow", "", "WoW install root")
	market := fs.String("market", "", "market key")
	count := fs.Int("count", 0, "ring slots")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	if *server != "" {
		cfg.Server = *server
	}
	if *wowDir != "" {
		cfg.WowDir = *wowDir
	}
	if *market != "" {
		cfg.Market = *market
	}
	if *count > 0 {
		cfg.RingSlots = *count
	}
	found, err := wow.Find(cfg.WowDir)
	if err != nil {
		return err
	}
	cfg.WowDir = found

	client := api.New(cfg.Server, "")
	enrolled, err := client.Enroll(context.Background())
	if err != nil {
		return fmt.Errorf("enroll: %w", err)
	}
	cfg.Token = enrolled.Token
	cfg.ClientID = enrolled.ClientID
	if err := config.Save(*cfgPath, cfg); err != nil {
		return err
	}
	fmt.Printf("enrolled client %s\nconfig: %s\n", enrolled.ClientID, *cfgPath)
	return nil
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config path")
	source := fs.String("source", "", "repository root containing addon/ and data-addon/")
	wowDir := fs.String("wow", "", "WoW install root")
	count := fs.Int("count", 0, "ring slots")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	if *source != "" {
		cfg.AddonSource = *source
	}
	if *wowDir != "" {
		cfg.WowDir = *wowDir
	}
	if *count > 0 {
		cfg.RingSlots = *count
	}
	if cfg.AddonSource == "" {
		cfg.AddonSource = defaultAddonSource()
	}
	found, err := wow.Find(cfg.WowDir)
	if err != nil {
		return err
	}
	cfg.WowDir = found
	if err := wow.Install(cfg.AddonSource, cfg.WowDir, cfg.RingSlots); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	if err := config.Save(*cfgPath, cfg); err != nil {
		return err
	}
	fmt.Printf("installed BaboForever + %d ring slots into %s\n", cfg.RingSlots, wow.AddonsDir(cfg.WowDir))
	return nil
}

func cmdSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config path")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	found, err := wow.Find(cfg.WowDir)
	if err != nil {
		return err
	}
	cfg.WowDir = found
	service, err := syncsvc.New(cfg)
	if err != nil {
		return err
	}
	if err := service.Sync(context.Background()); err != nil {
		return err
	}
	return printJSON(service.Status())
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config path")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	found, err := wow.Find(cfg.WowDir)
	if err != nil {
		return err
	}
	cfg.WowDir = found
	service, err := syncsvc.New(cfg)
	if err != nil {
		return err
	}

	dash := dashboard.New(service, cfg)
	server := dash.Start(cfg.DashboardAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go service.Run(ctx, time.Duration(cfg.IntervalSeconds)*time.Second)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Printf("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	return nil
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config path")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	if found, err := wow.Find(cfg.WowDir); err == nil {
		cfg.WowDir = found
	}
	redacted := cfg
	if redacted.Token != "" {
		redacted.Token = "***"
	}
	fmt.Println("config:")
	if err := printJSON(redacted); err != nil {
		return err
	}
	service, err := syncsvc.New(cfg)
	if err != nil {
		return nil
	}
	fmt.Println("status:")
	return printJSON(service.Status())
}

// defaultAddonSource prefers the current directory when it looks like a
// repository checkout, otherwise the directory containing bfc. That makes a
// packaged Windows build work when started from a shortcut or another CWD.
func defaultAddonSource() string {
	if _, err := os.Stat(filepath.Join("addon", "BaboForever")); err == nil {
		return "."
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, candidate := range []string{dir, filepath.Dir(dir)} {
			if _, err := os.Stat(filepath.Join(candidate, "addon", "BaboForever")); err == nil {
				return candidate
			}
		}
	}
	return "."
}

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
