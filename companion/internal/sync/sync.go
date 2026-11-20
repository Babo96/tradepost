package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/babo96/tradepost/companion/internal/addon"
	"github.com/babo96/tradepost/companion/internal/api"
	"github.com/babo96/tradepost/companion/internal/config"
	"github.com/babo96/tradepost/companion/internal/export"
	"github.com/babo96/tradepost/companion/internal/wow"
)

const clientVersion = "0.1.0"

// Matches the server-side limit in server/internal/api/api.go.
const maxMetaPerRequest = 20000

// Ring full snapshots every N 5-minute windows, or earlier when the diff is
// large. Between fulls each slot carries the cumulative change since the base,
// so a missed window still recovers when the next delta arrives.
const (
	ringFullEveryWindows = 6
	ringFullChangedLimit = 4000
)

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
	LoginCode      string `json:"login_code,omitempty"`
	LoginCharacter string `json:"login_character,omitempty"`
	LoginExpires   int64  `json:"login_expires,omitempty"`
	RingSlot       string `json:"ring_slot"`
	Error          string `json:"error"`
	Account        string `json:"account"`
	SavedVariables string `json:"saved_variables"`
}

type Service struct {
	mu                sync.Mutex
	syncMu            sync.Mutex
	cfg               config.Config
	client            *api.Client
	account           string
	uploaded          map[string]bool
	meta              map[string]string
	authSeq           int64
	activeCharacter   string
	activeMarket      string
	lastLoginCodeAt   time.Time
	ringBase          map[string]api.SnapshotItem
	ringBaseSeq       int64
	ringFullWindow    int64
	ringSeq           int64
	ringMarket        string
	lastRingRequestAt int64
	lastCompanionReq  int64
	dataAddonAt       time.Time
	watch             []string
	watchAt           int64
	status            Status
}

func New(cfg config.Config) (*Service, error) {
	if err := wow.EnsureAuthSlots(cfg.WowDir); err != nil {
		return nil, fmt.Errorf("auth slots: %w", err)
	}
	if err := wow.EnsureMetaAckSlots(cfg.WowDir); err != nil {
		return nil, fmt.Errorf("meta ack slots: %w", err)
	}
	if err := wow.EnsureCompanionStatus(cfg.WowDir); err != nil {
		return nil, fmt.Errorf("companion status: %w", err)
	}
	if err := wow.WriteCompanionStatus(cfg.WowDir, true); err != nil {
		return nil, fmt.Errorf("companion status: %w", err)
	}
	accounts, err := wow.AccountDirs(cfg.WowDir)
	if err != nil {
		return nil, err
	}
	account := accounts[0]
	// Prefer an account that already has TradePost data.
	for _, candidate := range accounts {
		if fileExists(wow.SavedVariablesFile(cfg.WowDir, candidate)) {
			account = candidate
			break
		}
	}
	if account == "" {
		return nil, fmt.Errorf("no WoW account found")
	}

	loginCode, loginCharacter, loginExpires := loadLogin(cfg.Server, cfg.ClientID)
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
			LoginCode:      loginCode,
			LoginCharacter: loginCharacter,
			LoginExpires:   loginExpires,
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
		return fmt.Errorf("no token; run tradepost enroll first")
	}

	data, err := export.LoadData(s.status.SavedVariables)
	if err != nil {
		return err
	}
	if data.Active != "" {
		s.mu.Lock()
		s.activeCharacter = data.Active
		s.mu.Unlock()
	}
	if data.ActiveMarket != "" {
		s.mu.Lock()
		s.activeMarket = data.ActiveMarket
		s.mu.Unlock()
	}
	if data.CompanionRequest > s.lastCompanionReq {
		s.lastCompanionReq = data.CompanionRequest
		if err := wow.WriteCompanionStatus(s.cfg.WowDir, true); err != nil {
			log.Printf("companion status: %v", err)
		}
	}
	s.applyRingRequest(data.RingRequest)
	s.syncAuth(ctx, data)
	s.syncWatch(ctx, data)
	newScans := make([]export.Scan, 0, len(data.Scans))
	for _, scan := range data.Scans {
		if !s.uploaded[scan.Hash] {
			newScans = append(newScans, scan)
		}
	}
	// Merge basic cache entries, full details, and the pending backfill batch.
	// Later sources win: details override the icon/name cache, and an
	// unacknowledged batch is always re-sent so it can be acknowledged.
	merged := map[int64]export.Meta{}
	for _, item := range data.Items {
		merged[item.ID] = item
	}
	for _, item := range data.Details {
		merged[item.ID] = item
	}
	batch := data.MetaBatch
	batchIDs := map[int64]bool{}
	if batch != nil {
		for _, item := range batch.Items {
			merged[item.ID] = item
			batchIDs[item.ID] = true
		}
	}

	mergedIDs := make([]int64, 0, len(merged))
	for id := range merged {
		mergedIDs = append(mergedIDs, id)
	}
	sort.Slice(mergedIDs, func(i, j int) bool { return mergedIDs[i] < mergedIDs[j] })

	var batchUpload []metaUpload
	var otherUpload []metaUpload
	for _, itemID := range mergedIDs {
		item := merged[itemID]
		id := strconv.FormatInt(item.ID, 10)
		hash := item.Hash()
		if s.meta[id] == hash && !batchIDs[item.ID] {
			continue
		}
		upload := metaUpload{id: id, hash: hash, meta: ingestMetaFrom(item)}
		if batchIDs[item.ID] {
			batchUpload = append(batchUpload, upload)
		} else {
			otherUpload = append(otherUpload, upload)
		}
	}
	// Keep the request under the server's metadata cap. The pending batch is
	// always uploaded in full; other changed entries wait for a later sync.
	uploads := append([]metaUpload{}, batchUpload...)
	if remaining := maxMetaPerRequest - len(uploads); remaining > 0 {
		if len(otherUpload) > remaining {
			otherUpload = otherUpload[:remaining]
		}
		uploads = append(uploads, otherUpload...)
	}
	newMeta := make([]api.IngestMeta, 0, len(uploads))
	updatedMeta := map[string]string{}
	for _, upload := range uploads {
		newMeta = append(newMeta, upload.meta)
		updatedMeta[upload.id] = upload.hash
	}

	if batch != nil && len(batch.Items) == 0 {
		if err := wow.WriteMetaAck(s.cfg.WowDir, batch.Seq); err != nil {
			return fmt.Errorf("meta ack: %w", err)
		}
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
		if batch != nil {
			// The server accepted the batch; let the addon advance. If this
			// write fails, the batch stays pending and will be re-sent.
			if err := wow.WriteMetaAck(s.cfg.WowDir, batch.Seq); err != nil {
				return fmt.Errorf("meta ack: %w", err)
			}
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
		s.status.Accepted = result.Queued
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
	// The data addon is a login-time baseline; the live ring carries updates.
	// Refresh it on startup, every 30 minutes, or when a reload request asks
	// for it, instead of rewriting ~700 KB every sync.
	if s.dataAddonAt.IsZero() || time.Since(s.dataAddonAt) >= 30*time.Minute {
		if err := addon.WriteDataAddon(s.cfg.WowDir, snapshots); err != nil {
			return fmt.Errorf("write data addon: %w", err)
		}
		s.dataAddonAt = time.Now()
	}

	s.mu.Lock()
	activeMarket := s.activeMarket
	s.mu.Unlock()
	selected := pickSnapshot(snapshots, activeMarket)
	if selected == nil {
		selected = pickSnapshot(snapshots, s.cfg.Market)
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
		// Publish this window and the next one. The addon asks for the new
		// window immediately at the boundary, before the next sync has run.
		window := time.Now().Unix() / 300
		payload := s.buildRingPayload(selected, window)
		var currentPath string
		for offset := int64(0); offset < 2; offset++ {
			payload.Window = window + offset
			payload.Pins = s.watch
			payload.PinsUpdated = s.watchAt
			path, err := addon.WriteRingPayload(s.cfg.WowDir, s.cfg.RingSlots, payload)
			if err != nil {
				return fmt.Errorf("write ring slot: %w", err)
			}
			if offset == 0 {
				currentPath = path
			}
		}
		s.mu.Lock()
		s.status.RingSlot = currentPath
		s.mu.Unlock()
	}
	return nil
}

// buildRingPayload returns a full snapshot when there is no base, the market
// changed, the base is old, or the diff is large. Otherwise it returns the
// cumulative delta since the last full base.
func (s *Service) buildRingPayload(snapshot *api.Snapshot, window int64) addon.RingPayload {
	full := s.ringBase == nil ||
		s.ringMarket != snapshot.Market ||
		window-s.ringFullWindow >= ringFullEveryWindows
	var changed []api.SnapshotItem
	var removed []string
	if !full {
		changed, removed = addon.RingDiff(s.ringBase, snapshot.Items)
		if len(changed) > ringFullChangedLimit || len(changed)+len(removed) > ringFullChangedLimit+1000 {
			full = true
		}
	}
	if full {
		seq := time.Now().UnixMilli()
		if seq <= s.ringSeq {
			seq = s.ringSeq + 1
		}
		base := make(map[string]api.SnapshotItem, len(snapshot.Items))
		for _, item := range snapshot.Items {
			base[item.Key] = item
		}
		s.ringBase = base
		s.ringBaseSeq = seq
		s.ringFullWindow = window
		s.ringSeq = seq
		s.ringMarket = snapshot.Market
		return addon.RingPayload{
			Market: snapshot.Market, Window: window, Generated: snapshot.Generated,
			Base: seq, Seq: seq, Full: true, Items: snapshot.Items,
		}
	}
	seq := time.Now().UnixMilli()
	if seq <= s.ringSeq {
		seq = s.ringSeq + 1
	}
	s.ringSeq = seq
	return addon.RingPayload{
		Market: snapshot.Market, Window: window, Generated: snapshot.Generated,
		Base: s.ringBaseSeq, Seq: seq, Full: false, Items: changed, Removed: removed,
	}
}

// applyRingRequest forces a full ring snapshot when the addon reports that it
// missed a base (for example after a fresh login). The request travels through
// SavedVariables, so it can take one flush to arrive.
// SetOnline publishes the one-time companion presence flag.
func (s *Service) SetOnline(online bool) {
	if s.cfg.WowDir == "" {
		return
	}
	if err := wow.WriteCompanionStatus(s.cfg.WowDir, online); err != nil {
		log.Printf("companion status: %v", err)
	}
}

func (s *Service) applyRingRequest(request *export.RingRequest) {
	if request == nil || request.At <= s.lastRingRequestAt {
		return
	}
	s.lastRingRequestAt = request.At
	s.mu.Lock()
	activeMarket := s.activeMarket
	s.mu.Unlock()
	if request.Market == "" ||
		request.Market == s.cfg.Market ||
		request.Market == activeMarket ||
		request.Market == s.ringMarket {
		s.ringBase = nil
		s.ringMarket = ""
	}
}

func pickSnapshot(snapshots []api.Snapshot, market string) *api.Snapshot {
	if market == "" {
		return nil
	}
	for i := range snapshots {
		if snapshots[i].Market == market {
			return &snapshots[i]
		}
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
		s.mu.Lock()
		market = s.activeMarket
		s.mu.Unlock()
	}
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
	if err := saveLogin(s.cfg.Server, s.cfg.ClientID, response.Code, response.Character, response.ExpiresAt); err != nil {
		log.Printf("login state: %v", err)
	}
	log.Printf("login code for %s: %s (expires %d)", response.Character, response.Code, response.ExpiresAt)
	s.mu.Lock()
	s.status.LoginCode = response.Code
	s.status.LoginCharacter = response.Character
	s.status.LoginExpires = response.ExpiresAt
	s.mu.Unlock()
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

type metaUpload struct {
	id   string
	hash string
	meta api.IngestMeta
}

func ingestMetaFrom(item export.Meta) api.IngestMeta {
	meta := api.IngestMeta{
		ID:            item.ID,
		Name:          item.Name,
		Quality:       item.Quality,
		ClassID:       item.ClassID,
		SubclassID:    item.SubclassID,
		Icon:          item.Icon,
		ItemLevel:     item.ItemLevel,
		RequiredLevel: item.RequiredLevel,
		BindType:      item.BindType,
		InventoryType: item.InventoryType,
		EquipLoc:      item.EquipLoc,
		Armor:         item.Armor,
		DamageMin:     item.DamageMin,
		DamageMax:     item.DamageMax,
		Delay:         item.Delay,
		VendorPrice:   item.VendorPrice,
		MaxStack:      item.MaxStack,
		UniqueItem:    item.UniqueItem,
		SetID:         item.SetID,
	}
	if len(item.Stats) > 0 {
		if data, err := json.Marshal(item.Stats); err == nil {
			meta.Stats = data
		}
	}
	if item.Details != nil {
		if data, err := json.Marshal(item.Details); err == nil {
			meta.Details = data
		}
	}
	return meta
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
			s.maybeRefreshLoginCode(ctx)
			info, err := os.Stat(s.savedVariablesPath())
			changed := err == nil && !info.ModTime().Equal(lastMod)
			if changed || time.Since(lastFull) >= interval {
				// Keep lastMod unchanged on failure so a partially written or
				// temporarily unreadable SavedVariables file is retried.
				if syncErr := s.Sync(ctx); syncErr == nil {
					if changed {
						lastMod = info.ModTime()
					}
					lastFull = time.Now()
				}
			}
		}
	}
}

// maybeRefreshLoginCode publishes a fresh login code every 8 minutes. The
// addon can then read it from an auth/ring slot immediately instead of waiting
// for the throttled SavedVariables flush after /tradepost login.
func (s *Service) maybeRefreshLoginCode(ctx context.Context) {
	s.mu.Lock()
	character := s.activeCharacter
	last := s.lastLoginCodeAt
	s.mu.Unlock()
	if character == "" || time.Since(last) < 8*time.Minute {
		return
	}
	response, err := s.client.LoginCode(ctx, character)
	if err != nil {
		log.Printf("login code refresh: %v", err)
		return
	}
	if err := wow.WriteAuthReply(s.cfg.WowDir, 0, response.Code, response.Character, response.ExpiresAt); err != nil {
		log.Printf("login code slot: %v", err)
		return
	}
	s.mu.Lock()
	market := s.activeMarket
	s.mu.Unlock()
	if market == "" {
		market = s.cfg.Market
	}
	if market == "" {
		market = "eu:pve:horde"
	}
	if err := addon.WriteAuthRingSlots(s.cfg.WowDir, market, 0, response.Code, response.Character, response.ExpiresAt, s.cfg.RingSlots); err != nil {
		log.Printf("login code ring: %v", err)
	}
	s.mu.Lock()
	s.lastLoginCodeAt = time.Now()
	s.status.LoginCode = response.Code
	s.status.LoginCharacter = response.Character
	s.status.LoginExpires = response.ExpiresAt
	s.mu.Unlock()
	if err := saveLogin(s.cfg.Server, s.cfg.ClientID, response.Code, response.Character, response.ExpiresAt); err != nil {
		log.Printf("login state: %v", err)
	}
	log.Printf("fresh login code for %s: %s", response.Character, response.Code)
}

func (s *Service) savedVariablesPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status.SavedVariables
}
