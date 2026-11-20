package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/babo96/tradepost/companion/internal/api"
	apppkg "github.com/babo96/tradepost/companion/internal/app"
	"github.com/babo96/tradepost/companion/internal/browser"
	"github.com/babo96/tradepost/companion/internal/config"
	"github.com/babo96/tradepost/companion/internal/dashboard"
	syncsvc "github.com/babo96/tradepost/companion/internal/sync"
	"github.com/babo96/tradepost/companion/internal/update"
	"github.com/babo96/tradepost/companion/internal/wow"
)

var version = "0.2.13"

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
	case "update":
		err = cmdUpdate(os.Args[2:])
	case "version":
		fmt.Println("tradepost " + version)
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
	fmt.Fprintf(os.Stderr, `TradePost Companion

  tradepost enroll  --server URL [--wow DIR] [--market eu:pve:horde]
  tradepost install [--source DIR] [--wow DIR] [--count 64]
  tradepost sync    [--config PATH]
  tradepost run     [--config PATH] [--no-browser]
  tradepost status  [--config PATH]
  tradepost update  [--check] [--force] [--yes] [--config PATH]
  tradepost version

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
		cfg.AddonSource = apppkg.DefaultAddonSource()
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
	fmt.Printf("installed TradePost + %d ring slots into %s\n", cfg.RingSlots, wow.AddonsDir(cfg.WowDir))
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
	noBrowser := fs.Bool("no-browser", false, "do not open the setup dashboard in a browser")
	_ = fs.Parse(args)

	application := apppkg.New(*cfgPath, version)
	if err := application.Load(); err != nil {
		return err
	}
	defer application.Shutdown()

	cfg := application.Config()
	dash := dashboard.New(application)
	server := dash.Start(cfg.DashboardAddr)

	if application.SetupRequired() {
		setupURL := dashboard.SetupURL(cfg.DashboardAddr)
		log.Printf("setup required: %s", setupURL)
		if !*noBrowser {
			if err := browser.Open(setupURL); err != nil {
				log.Printf("could not open browser: %v (open %s manually)", err, setupURL)
			}
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Printf("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	return nil
}

func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config path")
	checkOnly := fs.Bool("check", false, "only check for a newer release")
	force := fs.Bool("force", false, "apply even when the version is already current")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	_ = fs.Parse(args)

	cfgExists := true
	if _, err := os.Stat(*cfgPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		cfgExists = false
	}

	fmt.Printf("current: %s\n", version)
	info, err := update.New(version).Check(context.Background())
	if err != nil {
		fmt.Printf("latest: unavailable (%v)\n", err)
		return err
	}
	fmt.Printf("latest: v%s\n", info.Latest)
	fmt.Printf("available: %t\n", info.Available)
	if *checkOnly {
		return nil
	}
	if !info.Available && !*force {
		fmt.Println("already up to date")
		return nil
	}
	if strings.TrimSpace(info.AssetURL) == "" {
		return fmt.Errorf("no release asset available for this platform")
	}
	if !*yes {
		fmt.Printf("Install TradePost companion v%s? [y/N] ", info.Latest)
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("aborted")
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := update.Apply(ctx, info, update.ApplyOptions{})
	if err != nil {
		return err
	}
	defer result.Cleanup()

	fmt.Printf("installed: %s\n", result.BinaryPath)
	if result.RestartRequired {
		fmt.Println("restart the companion to use the new version")
	}

	if !cfgExists {
		fmt.Println("addon: skipped (no config found)")
		return nil
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	installInfo, err := wow.Validate(cfg.WowDir)
	if err != nil {
		fmt.Printf("addon: skipped (%v)\n", err)
		return nil
	}
	if err := wow.Install(result.SourceDir, installInfo.Root, cfg.RingSlots); err != nil {
		return fmt.Errorf("addon install: %w", err)
	}
	fmt.Printf("addon: installed into %s (use /reload in game or restart the game)\n", wow.AddonsDir(installInfo.Root))
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

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
