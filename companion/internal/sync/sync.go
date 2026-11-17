package sync

import (
	"context"
	"fmt"
	"os"
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
	status   Status
}

func New(cfg config.Config) (*Service, error) {
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
		uploaded: loadUploaded(),
		status: Status{
			Account:        account,
			Market:         cfg.Market,
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

	scans, err := export.Load(s.status.SavedVariables)
	if err != nil {
		return err
	}
	newScans := make([]export.Scan, 0, len(scans))
	for _, scan := range scans {
		if !s.uploaded[scan.Hash] {
			newScans = append(newScans, scan)
		}
	}
	if len(newScans) > 0 {
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
		result, err := s.client.Ingest(ctx, payload)
		if err != nil {
			return fmt.Errorf("upload: %w", err)
		}
		for _, scan := range newScans {
			s.uploaded[scan.Hash] = true
		}
		if err := saveUploaded(s.uploaded); err != nil {
			return fmt.Errorf("save upload state: %w", err)
		}
		s.mu.Lock()
		s.status.LastUpload = time.Now().Unix()
		s.status.Accepted = result.Accepted
		s.status.Rejected = result.Rejected
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
		path, err := addon.WriteRingSlot(s.cfg.WowDir, selected.Market, *selected, s.cfg.RingSlots, 0)
		if err != nil {
			return fmt.Errorf("write ring slot: %w", err)
		}
		s.mu.Lock()
		s.status.RingSlot = path
		s.mu.Unlock()
	}
	return nil
}

func (s *Service) Run(ctx context.Context, interval time.Duration) {
	if interval < 5*time.Second {
		interval = 60 * time.Second
	}
	_ = s.Sync(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.Sync(ctx)
		}
	}
}
