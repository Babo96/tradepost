// Package app owns the companion runtime: config, optional sync service, and
// the first-run setup state used by the dashboard.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/babo96/tradepost/companion/internal/api"
	"github.com/babo96/tradepost/companion/internal/config"
	"github.com/babo96/tradepost/companion/internal/picker"
	syncsvc "github.com/babo96/tradepost/companion/internal/sync"
	"github.com/babo96/tradepost/companion/internal/update"
	"github.com/babo96/tradepost/companion/internal/wow"
)

type App struct {
	mu         sync.Mutex
	cfgPath    string
	cfg        config.Config
	service    *syncsvc.Service
	setup      bool
	setupWhy   string
	cancel     context.CancelFunc
	baseCtx    context.Context
	baseCancel context.CancelFunc
	closeOnce  sync.Once

	version       string
	updateMu      sync.Mutex
	updateCheckMu sync.Mutex
	updateInfo    update.Info
}

func New(cfgPath, version string) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		cfgPath:    cfgPath,
		cfg:        config.Defaults(),
		setup:      true,
		baseCtx:    ctx,
		baseCancel: cancel,
		version:    version,
		updateInfo: update.Info{Current: version},
	}
}

// Load reads the config and either starts the sync service or enters setup
// mode when the config is missing, incomplete, or points at a bad game folder.
func (a *App) Load() error {
	cfg, err := config.Load(a.cfgPath)
	if err != nil {
		return err
	}

	required, reason := setupRequired(cfg)
	if !required {
		if info, err := wow.Validate(cfg.WowDir); err == nil {
			cfg.WowDir = info.Root
		}
	}

	a.mu.Lock()
	a.cfg = cfg
	a.setup = required
	a.setupWhy = reason
	if !required {
		if err := a.startServiceLocked(); err != nil {
			a.setup = true
			a.setupWhy = err.Error()
		}
	}
	a.mu.Unlock()

	go a.updateLoop()
	return nil
}

func setupRequired(cfg config.Config) (bool, string) {
	if missing := cfg.Missing(); len(missing) > 0 {
		return true, "first-run setup required (missing " + strings.Join(missing, ", ") + ")"
	}
	if _, err := wow.Validate(cfg.WowDir); err != nil {
		return true, err.Error()
	}
	return false, ""
}

func (a *App) Config() config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

func (a *App) ConfigPath() string { return a.cfgPath }

func (a *App) SetupRequired() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.setup
}

func (a *App) SetupReason() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.setupWhy
}

func (a *App) Status() syncsvc.Status {
	a.mu.Lock()
	service := a.service
	cfg := a.cfg
	setup := a.setup
	reason := a.setupWhy
	a.mu.Unlock()
	if service == nil {
		status := syncsvc.Status{Market: cfg.Market}
		if setup {
			status.Error = reason
		}
		return status
	}
	return service.Status()
}

func (a *App) Detect() []string {
	a.mu.Lock()
	explicit := a.cfg.WowDir
	a.mu.Unlock()
	found := wow.Detect(explicit)
	if found == nil {
		return []string{}
	}
	return found
}

func (a *App) Candidates() []string {
	a.mu.Lock()
	explicit := a.cfg.WowDir
	a.mu.Unlock()
	return wow.Candidates(explicit)
}

func (a *App) ValidateWow(dir string) (wow.InstallInfo, error) {
	return wow.Validate(dir)
}

func (a *App) PickFolder(title string) (string, error) {
	return picker.PickFolder(title)
}

// CompleteSetup enrolls, installs the addon, saves the config, and starts the
// sync service without requiring a restart.
func (a *App) CompleteSetup(ctx context.Context, req config.SetupRequest) (config.Config, error) {
	info, err := wow.Validate(req.WowDir)
	if err != nil {
		return a.Config(), err
	}

	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()

	cfg.WowDir = info.Root
	applyRequest(&cfg, req)
	if cfg.Server == "" {
		cfg.Server = config.DefaultServer
	}
	cfg.Server = normalizeServer(cfg.Server)
	if cfg.AddonSource == "" {
		cfg.AddonSource = DefaultAddonSource()
	}

	client := api.New(cfg.Server, "")
	enrolled, err := client.Enroll(ctx)
	if err != nil {
		return cfg, fmt.Errorf("enroll: %w", err)
	}
	cfg.Token = enrolled.Token
	cfg.ClientID = enrolled.ClientID

	if err := wow.Install(cfg.AddonSource, cfg.WowDir, cfg.RingSlots); err != nil {
		return cfg, fmt.Errorf("install: %w", err)
	}
	if err := config.Save(a.cfgPath, cfg); err != nil {
		return cfg, err
	}

	a.mu.Lock()
	a.cfg = cfg
	a.setup = false
	a.setupWhy = ""
	a.restartServiceLocked()
	a.mu.Unlock()
	return cfg, nil
}

// UpdateSettings saves changed values and restarts the service with them.
// It does not enroll; use CompleteSetup for a new machine.
func (a *App) UpdateSettings(ctx context.Context, req config.SetupRequest) (config.Config, error) {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()

	if strings.TrimSpace(req.WowDir) != "" {
		info, err := wow.Validate(req.WowDir)
		if err != nil {
			return cfg, err
		}
		cfg.WowDir = info.Root
	}
	applyRequest(&cfg, req)
	cfg.Server = normalizeServer(cfg.Server)
	if cfg.Server == "" {
		cfg.Server = config.DefaultServer
	}
	if cfg.AddonSource == "" {
		cfg.AddonSource = DefaultAddonSource()
	}
	if err := config.Save(a.cfgPath, cfg); err != nil {
		return cfg, err
	}

	a.mu.Lock()
	a.cfg = cfg
	required, reason := setupRequired(cfg)
	a.setup = required
	a.setupWhy = reason
	if !required {
		a.restartServiceLocked()
	} else {
		a.stopServiceLocked()
	}
	a.mu.Unlock()
	return cfg, nil
}

// Reinstall copies the addon, data addon, ring slots, and auth slots again
// using the saved config.
func (a *App) Reinstall(ctx context.Context) error {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()

	info, err := wow.Validate(cfg.WowDir)
	if err != nil {
		return err
	}
	cfg.WowDir = info.Root
	if cfg.AddonSource == "" {
		cfg.AddonSource = DefaultAddonSource()
	}
	if err := wow.Install(cfg.AddonSource, cfg.WowDir, cfg.RingSlots); err != nil {
		return err
	}
	if err := config.Save(a.cfgPath, cfg); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	return nil
}

func (a *App) Sync(ctx context.Context) error {
	a.mu.Lock()
	service := a.service
	setup := a.setup
	a.mu.Unlock()
	if service == nil {
		if setup {
			return errors.New("setup is not complete")
		}
		return errors.New("sync service is not running")
	}
	return service.Sync(ctx)
}

// UpdateStatus returns the last known update state for the dashboard.
func (a *App) UpdateStatus() update.Info {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	return a.updateInfo
}

func (a *App) storeUpdate(info update.Info) {
	a.updateMu.Lock()
	a.updateInfo = info
	a.updateMu.Unlock()
}

// CheckUpdate forces a release check and records the result.
func (a *App) CheckUpdate(ctx context.Context) (update.Info, error) {
	return a.checkUpdate(ctx, false)
}

func (a *App) checkUpdate(ctx context.Context, automatic bool) (update.Info, error) {
	a.updateCheckMu.Lock()
	defer a.updateCheckMu.Unlock()

	if automatic {
		current := a.UpdateStatus()
		if current.Applying {
			return current, nil
		}
	}
	info, err := update.New(a.version).Check(ctx)
	a.storeUpdate(info)
	a.syncAddonLatest(info)
	return info, err
}

// updateLoop checks once at startup and then daily until shutdown. Network
// failures are recorded in the status and never block the companion.
func (a *App) updateLoop() {
	_, _ = a.checkUpdate(a.baseCtx, true)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-a.baseCtx.Done():
			return
		case <-ticker.C:
			_, _ = a.checkUpdate(a.baseCtx, true)
		}
	}
}

// syncAddonLatest pushes a successful update-check result to the running
// service, which writes it into the tiny status addon for the game.
func (a *App) syncAddonLatest(info update.Info) {
	if strings.TrimSpace(info.Latest) == "" {
		return
	}
	a.mu.Lock()
	service := a.service
	a.mu.Unlock()
	if service != nil {
		service.SetAddonLatest(info.Latest, info.HTMLURL)
	}
}

// ApplyUpdate downloads and installs the available release. When the config
// points at a valid game folder, the addon sources from the same release are
// installed too. The running process keeps using the old binary until restart.
func (a *App) ApplyUpdate(ctx context.Context) (update.Info, error) {
	a.updateCheckMu.Lock()
	defer a.updateCheckMu.Unlock()

	info := a.UpdateStatus()
	if strings.TrimSpace(info.AssetURL) == "" || info.Latest == "" {
		checked, err := update.New(a.version).Check(ctx)
		a.storeUpdate(checked)
		if err != nil {
			return checked, err
		}
		info = checked
	}
	if !info.Available {
		return info, update.ErrNoUpdate
	}

	info.Applying = true
	info.Error = ""
	info.Message = "Downloading and installing v" + info.Latest + "…"
	a.storeUpdate(info)

	result, err := update.Apply(ctx, info, update.ApplyOptions{})
	info.Applying = false
	if err != nil {
		info.Error = err.Error()
		info.Message = "Update failed: " + err.Error()
		a.storeUpdate(info)
		return info, err
	}
	defer result.Cleanup()

	info.Applied = true
	info.RestartRequired = result.RestartRequired
	info.Available = false
	message := fmt.Sprintf("Companion v%s installed. Restart the companion to use it.", result.Version)

	cfg := a.Config()
	valid, validErr := wow.Validate(cfg.WowDir)
	if validErr != nil {
		message += " Addon not updated: " + validErr.Error()
	} else if err := wow.Install(result.SourceDir, valid.Root, cfg.RingSlots); err != nil {
		info.Error = err.Error()
		info.Message = message + " Addon install failed: " + err.Error()
		a.storeUpdate(info)
		return info, fmt.Errorf("companion updated but addon install failed: %w", err)
	} else {
		info.AddonUpdated = true
		message += " Addon files updated; use /reload in game or restart the game to apply them."
		a.mu.Lock()
		a.cfg.WowDir = valid.Root
		a.mu.Unlock()
	}

	info.Message = message
	a.storeUpdate(info)
	a.syncAddonLatest(info)
	return info, nil
}

func (a *App) Shutdown() {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.stopServiceLocked()
		a.mu.Unlock()
		a.baseCancel()
	})
}

func applyRequest(cfg *config.Config, req config.SetupRequest) {
	if server := strings.TrimSpace(req.Server); server != "" {
		cfg.Server = server
	}
	if market := strings.TrimSpace(req.Market); market != "" {
		cfg.Market = market
	}
	if req.RingSlots > 0 {
		cfg.RingSlots = req.RingSlots
	}
	if req.IntervalSeconds > 0 {
		cfg.IntervalSeconds = req.IntervalSeconds
	}
	if source := strings.TrimSpace(req.AddonSource); source != "" {
		cfg.AddonSource = source
	}
	if cfg.RingSlots < 1 {
		cfg.RingSlots = config.DefaultRings
	}
	if cfg.RingSlots > 256 {
		cfg.RingSlots = 256
	}
	if cfg.IntervalSeconds < 5 {
		cfg.IntervalSeconds = 60
	}
	if cfg.Market == "" {
		cfg.Market = config.DefaultMarket
	}
}

func normalizeServer(server string) string {
	server = strings.TrimSpace(server)
	server = strings.TrimRight(server, "/")
	if server != "" && !strings.Contains(server, "://") {
		server = "https://" + server
	}
	return server
}

func (a *App) startServiceLocked() error {
	service, err := syncsvc.New(a.cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(a.baseCtx)
	a.service = service
	a.cancel = cancel
	if info := a.UpdateStatus(); info.Latest != "" {
		service.SetAddonLatest(info.Latest, info.HTMLURL)
	}
	interval := time.Duration(a.cfg.IntervalSeconds) * time.Second
	go service.Run(ctx, interval)
	return nil
}

func (a *App) stopServiceLocked() {
	if a.service == nil {
		return
	}
	a.service.SetOnline(false)
	// Only cancel the service context, not the app's base context.
	if a.cancel != nil {
		a.cancel()
	}
	a.cancel = nil
	a.service = nil
}

func (a *App) restartServiceLocked() {
	a.stopServiceLocked()
	if err := a.startServiceLocked(); err != nil {
		a.setup = true
		a.setupWhy = err.Error()
	}
}

// DefaultAddonSource prefers the repository checkout, then the directory
// containing the executable (packaged Windows builds).
func DefaultAddonSource() string {
	if _, err := os.Stat(filepath.Join("addon", "TradePost")); err == nil {
		return "."
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, candidate := range []string{dir, filepath.Dir(dir)} {
			if _, err := os.Stat(filepath.Join(candidate, "addon", "TradePost")); err == nil {
				return candidate
			}
		}
	}
	return "."
}
