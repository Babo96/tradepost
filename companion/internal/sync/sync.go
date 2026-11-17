package sync

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"babo-forever/companion/internal/addon"
	"babo-forever/companion/internal/api"
	"babo-forever/companion/internal/config"
	"babo-forever/companion/internal/export"
	"babo-forever/companion/internal/wow"
)

const clientVersion = "0.1.0"

type Status struct {
	Running        bool   `json:"running"`
	LastSync       int64  `json:"last_sync"`
	LastUpload     int64  `json:"last_upload"`
	LastDownload   int64  `json:"last_download"`
	Market         string `json:"market"`
	Version        int64  `json:"version"`
	Items          int    `json:"items"`
	Accepted       int    `json:"accepted"`
	Rejected       int    `json:"rejected"`
	Meta           int    `json:"meta"`
	Watch          int    `json:"watch"`
	RingSlot       string `json:"ring_slot"`
	Error          string `json:"error"`
	Account        string `json:"account"`
	SavedVariables string `json:"saved_variables"`
}

type Service struct {
	mu       sync.Mutex
	syncMu   sync.Mutex
	cfg      config.Config
	client   *api.Client
	account  string
	uploaded map[string]bool
	meta     map[string]string
	authSeq  int64
	watch    []string
	watchAt  int64
	status   Status
}

func New(cfg config.Config) (*Service, error) {
	if err := wow.EnsureAuthSlots(cfg.WowDir); err != nil {
		return nil, fmt.Errorf("auth slots: %w", err)
	}
	accounts, err := wow.AccountDirs(cfg.WowDir)
	if err != nil {
		return nil, err
	}
	account := accounts[0]
	// Prefer an account that already has Babo Forever data.
	for _, candidate := range accounts {
		if fileExists(wow.SavedVariablesFile(cfg.WowDir, candidate)) {
			account = candidate
			break
		}
	}
	if account == "" {
		return nil, fmt.Errorf("no WoW account found")
	}

	service := &Service{
		cfg:      cfg,
		client:   api.New(cfg.Server, cfg.Token),
		account:  account,
		uploaded: loadUploaded(cfg.Server, cfg.ClientID),
		meta:     loadMeta(cfg.Server, cfg.ClientID),
		authSeq:  loadAuthSeq(cfg.Server, cfg.ClientID),
		status: Status{
			Account:        account,
			Market:         cfg.Market,
			LastUpload:     loadLastUpload(cfg.Server, cfg.ClientID),
			SavedVariables: wow.SavedVariablesFile(cfg.WowDir, account),
		},
	}
	return service, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) Sync(ctx context.Context) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	s.mu.Lock()
	s.status.Running = true
	s.mu.Unlock()

	err := s.sync(ctx)

	s.mu.Lock()
	s.status.Running = false
	s.status.LastSync = time.Now().Unix()
	if err != nil {
		s.status.Error = err.Error()
	} else {
		s.status.Error = ""
	}
	s.mu.Unlock()
	return err
}

func (s *Service) sync(ctx context.Context) error {
	if s.cfg.Token == "" {
		return fmt.Errorf("no token; run bfc enroll first")
	}

	data, err := export.LoadData(s.status.SavedVariables)
	if err != nil {
		return err
	}
	s.syncAuth(ctx, data)
	s.syncWatch(ctx, data)
	newScans := make([]export.Scan, 0, len(data.Scans))
	for _, scan := range data.Scans {
		if !s.uploaded[scan.Hash] {
			newScans = append(newScans, scan)
		}
	}
	newMeta := make([]api.IngestMeta, 0, len(data.Items))
	updatedMeta := map[string]string{}
	for _, item := range data.Items {
		id := strconv.FormatInt(item.ID, 10)
		hash := item.Hash()
		if s.meta[id] == hash {
			continue
		}
		newMeta = append(newMeta, api.IngestMeta{
			ID:         item.ID,
			Name:       item.Name,
			Quality:    item.Quality,
			ClassID:    item.ClassID,
			SubclassID: item.SubclassID,
			Icon:       item.Icon,
		})
		updatedMeta[id] = hash
	}
	if len(newScans) > 0 || len(newMeta) > 0 {
		payload := make([]api.IngestScan, 0, len(newScans))
		for _, scan := range newScans {
			items := make([]api.IngestItem, 0, len(scan.Items))
			for _, item := range scan.Items {
				items = append(items, api.IngestItem{
					Key:      item.Key,
					Price:    item.Price,
					Quantity: item.Quantity,
					Listings: item.Listings,
				})
			}
			payload = append(payload, api.IngestScan{
				Market:        scan.Market,
				ScannedAt:     scan.ScannedAt,
				ClientVersion: clientVersion,
				Items:         items,
			})
		}
		result, err := s.client.Ingest(ctx, payload, newMeta)
		if err != nil {
			return fmt.Errorf("upload: %w", err)
		}
		for _, scan := range newScans {
			s.uploaded[scan.Hash] = true
		}
		for id, hash := range updatedMeta {
			s.meta[id] = hash
		}
		if err := saveUploaded(s.cfg.Server, s.cfg.ClientID, s.uploaded); err != nil {
			return fmt.Errorf("save upload state: %w", err)
		}
		if err := saveMeta(s.cfg.Server, s.cfg.ClientID, s.meta); err != nil {
			return fmt.Errorf("save metadata state: %w", err)
		}
		now := time.Now().Unix()
		if err := saveLastUpload(s.cfg.Server, s.cfg.ClientID, now); err != nil {
			return fmt.Errorf("save upload time: %w", err)
		}
		s.mu.Lock()
		s.status.LastUpload = now
		s.status.Accepted = result.Accepted
		s.status.Rejected = result.Rejected
		s.status.Meta = result.Meta
		s.mu.Unlock()
	}

	meta, err := s.client.Meta(ctx)
	if err != nil {
		return fmt.Errorf("meta: %w", err)
	}
	if len(meta) == 0 {
		return nil
	}

	snapshots := make([]api.Snapshot, 0, len(meta))
	for _, market := range meta {
		snapshot, err := s.client.Snapshot(ctx, market.Market)
		if err != nil {
			return fmt.Errorf("snapshot %s: %w", market.Market, err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := addon.WriteDataAddon(s.cfg.WowDir, snapshots); err != nil {
		return fmt.Errorf("write data addon: %w", err)
	}

	var selected *api.Snapshot
	for i := range snapshots {
		if snapshots[i].Market == s.cfg.Market {
			selected = &snapshots[i]
			break
		}
	}
	if selected == nil && len(snapshots) > 0 {
		selected = &snapshots[0]
	}

	s.mu.Lock()
	s.status.LastDownload = time.Now().Unix()
	s.status.Market = selected.Market
	s.status.Version = selected.Version
	s.status.Items = len(selected.Items)
	s.mu.Unlock()

	if selected != nil {
		path, err := addon.WriteRingSlot(s.cfg.WowDir, selected.Market, *selected, s.cfg.RingSlots, 0, s.watch, s.watchAt)
		if err != nil {
			return fmt.Errorf("write ring slot: %w", err)
		}
		s.mu.Lock()
		s.status.RingSlot = path
		s.mu.Unlock()
	}
	return nil
}

func (s *Service) syncAuth(ctx context.Context, data export.Data) {
	request := data.Auth
	if request == nil || request.Seq <= s.authSeq {
		return
	}
	if request.Action != "login" {
		s.authSeq = request.Seq
		if err := saveAuthSeq(s.cfg.Server, s.cfg.ClientID, s.authSeq); err != nil {
			log.Printf("auth state: %v", err)
		}
		return
	}
	response, err := s.client.LoginCode(ctx, request.Character)
	if err != nil {
		log.Printf("auth code: %v", err)
		return
	}
	if err := wow.WriteAuthReply(s.cfg.WowDir, request.Seq, response.Code, response.Character, response.ExpiresAt); err != nil {
		log.Printf("auth reply: %v", err)
		return
	}
	market := request.Market
	if market == "" {
		market = s.cfg.Market
	}
	if err := addon.WriteAuthRingSlots(s.cfg.WowDir, market, request.Seq, response.Code, response.Character, response.ExpiresAt, s.cfg.RingSlots); err != nil {
		log.Printf("auth ring reply: %v", err)
	}
	s.authSeq = request.Seq
	if err := saveAuthSeq(s.cfg.Server, s.cfg.ClientID, s.authSeq); err != nil {
		log.Printf("auth state: %v", err)
	}
}

func (s *Service) syncWatch(ctx context.Context, data export.Data) {
	character := data.Active
	if character == "" {
		return
	}
	local := data.Watch[character]
	remote, err := s.client.Watch(ctx, character)
	if err != nil {
		log.Printf("watch: %v", err)
		return
	}
	if local.Updated > remote.UpdatedAt+2 {
		updated, err := s.client.SetWatch(ctx, character, local.Items)
		if err != nil {
			log.Printf("watch upload: %v", err)
			return
		}
		remote = updated
	}
	s.watch = remote.Items
	s.watchAt = remote.UpdatedAt
	s.mu.Lock()
	s.status.Watch = len(remote.Items)
	s.mu.Unlock()
}

func (s *Service) Run(ctx context.Context, interval time.Duration) {
	if interval < 5*time.Second {
		interval = 60 * time.Second
	}
	_ = s.Sync(ctx)

	// The addon calls C_AddOns.SaveAddOns() after a scan, so watching the
	// SavedVariables mtime gives near-live uploads without /reload.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastMod := time.Time{}
	if info, err := os.Stat(s.savedVariablesPath()); err == nil {
		lastMod = info.ModTime()
	}
	lastFull := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(s.savedVariablesPath())
			changed := err == nil && !info.ModTime().Equal(lastMod)
			if changed {
				lastMod = info.ModTime()
			}
			if changed || time.Since(lastFull) >= interval {
				_ = s.Sync(ctx)
				if changed {
					lastFull = time.Now()
				}
			}
		}
	}
}

func (s *Service) savedVariablesPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status.SavedVariables
}
