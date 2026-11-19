package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/babo96/tradepost/server/internal/store"
)

// MemStore is an in-memory Store for handler tests and local smoke tests.
type MemStore struct {
	mu        sync.Mutex
	clients   map[string]store.Client
	tokens    map[string]string
	seen      map[string]bool
	processed map[string]bool
	jobs      map[int64]store.IngestJob
	jobSeq    int64
	markets   map[string]map[string]store.SnapshotItem
	versions  map[string]int64
	items     map[int64]store.ItemMeta
	history   map[string]map[string]map[int64]store.HistoryPoint
	quarter   map[string]map[string]map[int64]store.HistoryPoint
	minute    map[string]map[string]map[int64]store.HistoryPoint
	codes     map[string]store.LoginGrant
	used      map[string]bool
	sessions  map[string]store.WebSession
	watches   map[string]store.WatchList
}

func NewMemStore() *MemStore {
	return &MemStore{
		clients:   map[string]store.Client{},
		tokens:    map[string]string{},
		seen:      map[string]bool{},
		processed: map[string]bool{},
		jobs:      map[int64]store.IngestJob{},
		markets:   map[string]map[string]store.SnapshotItem{},
		versions:  map[string]int64{},
		items:     map[int64]store.ItemMeta{},
		history:   map[string]map[string]map[int64]store.HistoryPoint{},
		quarter:   map[string]map[string]map[int64]store.HistoryPoint{},
		minute:    map[string]map[string]map[int64]store.HistoryPoint{},
		codes:     map[string]store.LoginGrant{},
		used:      map[string]bool{},
		sessions:  map[string]store.WebSession{},
		watches:   map[string]store.WatchList{},
	}
}

func (m *MemStore) Ping(context.Context) error { return nil }

func (m *MemStore) CreateClient(_ context.Context, id string, tokenHash []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[id] = store.Client{ID: id, Status: "active", CreatedAt: time.Now()}
	m.tokens[hex.EncodeToString(tokenHash)] = id
	return nil
}

func (m *MemStore) ClientByTokenHash(_ context.Context, tokenHash []byte) (store.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.tokens[hex.EncodeToString(tokenHash)]
	if !ok {
		return store.Client{}, store.ErrNotFound
	}
	client, ok := m.clients[id]
	if !ok {
		return store.Client{}, store.ErrNotFound
	}
	return client, nil
}

func (m *MemStore) EnqueueIngest(_ context.Context, clientID string, job store.IngestJob) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := clientID + ":" + hex.EncodeToString(job.Checksum)
	if m.seen[key] {
		return false, nil
	}
	m.seen[key] = true
	m.jobSeq++
	job.ID = m.jobSeq
	job.ClientID = clientID
	job.Status = "queued"
	job.Attempts = 0
	job.CreatedAt = time.Now()
	job.UpdatedAt = job.CreatedAt
	m.jobs[job.ID] = job
	return true, nil
}

func (m *MemStore) ClaimIngestJobs(_ context.Context, limit int) ([]store.IngestJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit < 1 {
		limit = 1
	}
	ids := make([]int64, 0, len(m.jobs))
	for id, job := range m.jobs {
		if job.Status == "queued" {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > limit {
		ids = ids[:limit]
	}
	jobs := make([]store.IngestJob, 0, len(ids))
	for _, id := range ids {
		job := m.jobs[id]
		job.Status = "processing"
		job.UpdatedAt = time.Now()
		m.jobs[id] = job
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (m *MemStore) ProcessIngestJob(_ context.Context, job store.IngestJob) error {
	payload, err := decodeMemPayload(job.Payload)
	if err != nil {
		return err
	}
	if payload.Market == "" {
		payload.Market = job.Market
	}
	scannedAt := job.ScannedAt
	if payload.ScannedAt != 0 {
		scannedAt = time.Unix(payload.ScannedAt, 0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := job.ClientID + ":" + hex.EncodeToString(job.Checksum)
	if m.processed[key] {
		job.Status = "done"
		m.jobs[job.ID] = job
		return nil
	}
	m.processed[key] = true

	m.applyScanLocked(payload.Market, scannedAt, payload.Items)
	job.Status = "done"
	job.Error = ""
	job.UpdatedAt = time.Now()
	m.jobs[job.ID] = job
	return nil
}

func decodeMemPayload(payload []byte) (store.ScanPayload, error) {
	var scan store.ScanPayload
	gz, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return scan, err
	}
	defer gz.Close()
	data, err := io.ReadAll(io.LimitReader(gz, 64<<20))
	if err != nil {
		return scan, err
	}
	if err := json.Unmarshal(data, &scan); err != nil {
		return scan, err
	}
	return scan, nil
}

func (m *MemStore) applyScanLocked(market string, scannedAt time.Time, items []store.Item) {
	if m.markets[market] == nil {
		m.markets[market] = map[string]store.SnapshotItem{}
	}
	latest := m.markets[market]
	for _, item := range items {
		existing, ok := latest[item.Key]
		if ok && existing.Scanned >= scannedAt.Unix() {
			continue
		}
		latest[item.Key] = store.SnapshotItem{
			Key:      item.Key,
			Price:    item.Price,
			Quantity: item.Quantity,
			Listings: item.Listings,
			Scanned:  scannedAt.Unix(),
		}
	}

	m.aggregateLocked(m.minute, market, scannedAt.Truncate(time.Minute), items)
	m.aggregateLocked(m.quarter, market, scannedAt.Truncate(15*time.Minute), items)
	m.aggregateLocked(m.history, market, scannedAt.Truncate(time.Hour), items)
	m.versions[market]++
}

func (m *MemStore) aggregateLocked(target map[string]map[string]map[int64]store.HistoryPoint, market string, bucket time.Time, items []store.Item) {
	if target[market] == nil {
		target[market] = map[string]map[int64]store.HistoryPoint{}
	}
	for _, item := range items {
		byKey := target[market][item.Key]
		if byKey == nil {
			byKey = map[int64]store.HistoryPoint{}
			target[market][item.Key] = byKey
		}
		key := bucket.Unix()
		point, ok := byKey[key]
		if !ok {
			point = store.HistoryPoint{Bucket: key, Min: item.Price, Max: item.Price}
		}
		if item.Price < point.Min {
			point.Min = item.Price
		}
		if item.Price > point.Max {
			point.Max = item.Price
		}
		if item.Quantity > point.Quantity {
			point.Quantity = item.Quantity
		}
		point.Samples++
		byKey[key] = point
	}
}

func (m *MemStore) CompleteIngestJob(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return store.ErrNotFound
	}
	job.Status = "done"
	job.UpdatedAt = time.Now()
	m.jobs[id] = job
	return nil
}

func (m *MemStore) FailIngestJob(_ context.Context, id int64, attempts int, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return store.ErrNotFound
	}
	job.Attempts = attempts
	job.Error = errMsg
	if attempts >= 5 {
		job.Status = "failed"
	} else {
		job.Status = "queued"
	}
	job.UpdatedAt = time.Now()
	m.jobs[id] = job
	return nil
}

func (m *MemStore) Meta(context.Context) ([]store.MarketMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []store.MarketMeta
	for market, items := range m.markets {
		var last int64
		for _, item := range items {
			if item.Scanned > last {
				last = item.Scanned
			}
		}
		result = append(result, store.MarketMeta{
			Market:    market,
			Version:   m.versions[market],
			Items:     int64(len(items)),
			LastScan:  last,
			UpdatedAt: last,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Market < result[j].Market })
	return result, nil
}

func (m *MemStore) Snapshot(_ context.Context, market string) (store.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	items, ok := m.markets[market]
	if !ok {
		return store.Snapshot{}, store.ErrNotFound
	}
	snapshot := store.Snapshot{
		Market:    market,
		Version:   m.versions[market],
		Generated: time.Now().Unix(),
		Items:     make([]store.SnapshotItem, 0, len(items)),
	}
	for _, item := range items {
		snapshot.Items = append(snapshot.Items, item)
	}
	sort.Slice(snapshot.Items, func(i, j int) bool { return snapshot.Items[i].Key < snapshot.Items[j].Key })
	return snapshot, nil
}

func (m *MemStore) UpsertItems(_ context.Context, items []store.ItemMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range items {
		existing, ok := m.items[item.ID]
		if ok {
			item = mergeItemMeta(existing, item)
		}
		if len(item.Details) > 0 {
			item.DetailsVersion = existing.DetailsVersion + 1
			item.DetailsUpdated = time.Now().Unix()
		}
		m.items[item.ID] = item
	}
	return nil
}

func mergeItemMeta(existing, incoming store.ItemMeta) store.ItemMeta {
	if incoming.ItemLevel == nil {
		incoming.ItemLevel = existing.ItemLevel
	}
	if incoming.RequiredLevel == nil {
		incoming.RequiredLevel = existing.RequiredLevel
	}
	if incoming.BindType == nil {
		incoming.BindType = existing.BindType
	}
	if incoming.InventoryType == nil {
		incoming.InventoryType = existing.InventoryType
	}
	if incoming.EquipLoc == nil {
		incoming.EquipLoc = existing.EquipLoc
	}
	if incoming.Armor == nil {
		incoming.Armor = existing.Armor
	}
	if incoming.DamageMin == nil {
		incoming.DamageMin = existing.DamageMin
	}
	if incoming.DamageMax == nil {
		incoming.DamageMax = existing.DamageMax
	}
	if incoming.Delay == nil {
		incoming.Delay = existing.Delay
	}
	if incoming.VendorPrice == nil {
		incoming.VendorPrice = existing.VendorPrice
	}
	if incoming.MaxStack == nil {
		incoming.MaxStack = existing.MaxStack
	}
	if incoming.UniqueItem == nil {
		incoming.UniqueItem = existing.UniqueItem
	}
	if incoming.SetID == nil {
		incoming.SetID = existing.SetID
	}
	if len(incoming.Stats) == 0 {
		incoming.Stats = existing.Stats
	}
	if len(incoming.Details) == 0 {
		incoming.Details = existing.Details
		incoming.DetailsVersion = existing.DetailsVersion
		incoming.DetailsUpdated = existing.DetailsUpdated
	}
	return incoming
}

func (m *MemStore) SearchItems(_ context.Context, market string, query store.SearchQuery) (store.SearchPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if query.Page < 1 {
		query.Page = 1
	}
	if query.Limit < 1 {
		query.Limit = 50
	}
	if query.Limit > 200 {
		query.Limit = 200
	}
	search := strings.ToLower(strings.TrimSpace(query.Query))

	result := []store.SearchResult{}
	for key, item := range m.markets[market] {
		id := store.ItemID(key)
		meta := m.items[id]
		if search != "" {
			if !strings.Contains(strings.ToLower(meta.Name), search) &&
				!strings.Contains(strings.ToLower(key), search) &&
				key != search && formatItemID(id) != search {
				continue
			}
		}
		if !matchesFilters(meta, item, query) {
			continue
		}
		result = append(result, store.SearchResult{
			Key:           key,
			ItemID:        id,
			Name:          meta.Name,
			Quality:       meta.Quality,
			Price:         item.Price,
			Quantity:      item.Quantity,
			Listings:      item.Listings,
			Scanned:       item.Scanned,
			ItemLevel:     meta.ItemLevel,
			RequiredLevel: meta.RequiredLevel,
			BindType:      meta.BindType,
			InventoryType: meta.InventoryType,
			EquipLoc:      derefString(meta.EquipLoc),
			Armor:         meta.Armor,
			DamageMin:     meta.DamageMin,
			DamageMax:     meta.DamageMax,
			Delay:         meta.Delay,
			VendorPrice:   meta.VendorPrice,
			MaxStack:      meta.MaxStack,
			UniqueItem:    meta.UniqueItem,
			SetID:         meta.SetID,
			HasDetails:    len(meta.Details) > 0,
		})
	}

	sortSearchResults(result, query.Sort)
	total := int64(len(result))
	start := (query.Page - 1) * query.Limit
	if start > len(result) {
		start = len(result)
	}
	end := start + query.Limit
	if end > len(result) {
		end = len(result)
	}
	return store.SearchPage{Items: result[start:end], Total: total}, nil
}

func matchesFilters(meta store.ItemMeta, item store.SnapshotItem, query store.SearchQuery) bool {
	if len(query.Qualities) > 0 {
		found := false
		for _, quality := range query.Qualities {
			if meta.Quality == quality {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if query.ClassID != nil && meta.ClassID != *query.ClassID {
		return false
	}
	if query.SubclassID != nil && meta.SubclassID != *query.SubclassID {
		return false
	}
	if query.InventoryType != nil && (meta.InventoryType == nil || *meta.InventoryType != *query.InventoryType) {
		return false
	}
	if query.MinIlvl != nil && (meta.ItemLevel == nil || *meta.ItemLevel < *query.MinIlvl) {
		return false
	}
	if query.MaxIlvl != nil && (meta.ItemLevel == nil || *meta.ItemLevel > *query.MaxIlvl) {
		return false
	}
	if query.MinReqLevel != nil && (meta.RequiredLevel == nil || *meta.RequiredLevel < *query.MinReqLevel) {
		return false
	}
	if query.MaxReqLevel != nil && (meta.RequiredLevel == nil || *meta.RequiredLevel > *query.MaxReqLevel) {
		return false
	}
	if query.MinPrice != nil && item.Price < *query.MinPrice {
		return false
	}
	if query.MaxPrice != nil && item.Price > *query.MaxPrice {
		return false
	}
	if query.InStock && item.Quantity <= 0 {
		return false
	}
	if !matchesStats(meta, query) {
		return false
	}
	return true
}

func matchesStats(meta store.ItemMeta, query store.SearchQuery) bool {
	if len(query.Stats) == 0 && !query.HasStats {
		return true
	}
	var stats map[string]any
	if len(meta.Stats) > 0 {
		_ = json.Unmarshal(meta.Stats, &stats)
	}
	if query.HasStats && len(stats) == 0 {
		return false
	}
	for _, filter := range query.Stats {
		value, ok := stats[filter.Key]
		if !ok {
			return false
		}
		number, ok := value.(float64)
		if !ok {
			return false
		}
		if filter.Min != nil && number < *filter.Min {
			return false
		}
		if filter.Max != nil && number > *filter.Max {
			return false
		}
	}
	return true
}

func sortSearchResults(items []store.SearchResult, sortKey string) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortKey {
		case "name":
			if a.Name != b.Name {
				return a.Name < b.Name
			}
		case "ilvl":
			av, bv := intValue(a.ItemLevel), intValue(b.ItemLevel)
			if av != bv {
				return av > bv
			}
		case "quality":
			if a.Quality != b.Quality {
				return a.Quality > b.Quality
			}
		case "scanned":
			if a.Scanned != b.Scanned {
				return a.Scanned > b.Scanned
			}
		default:
			if a.Price != b.Price {
				return a.Price < b.Price
			}
			if a.Name != b.Name {
				return a.Name < b.Name
			}
		}
		return a.Key < b.Key
	})
}

func intValue(value *int) int {
	if value == nil {
		return -1
	}
	return *value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatItemID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func (m *MemStore) MissingItemDetails(_ context.Context, market string, force bool, limit int) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit < 1 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	seen := map[int64]bool{}
	result := []int64{}
	for key := range m.markets[market] {
		id := store.ItemID(key)
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		meta := m.items[id]
		if !force && len(meta.Details) > 0 {
			continue
		}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *MemStore) ItemDetail(_ context.Context, market, key string) (store.ItemDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.items[store.ItemID(key)]
	if !ok {
		return store.ItemDetail{}, store.ErrNotFound
	}
	detail := store.ItemDetail{ItemMeta: meta}
	if item, ok := m.markets[market][key]; ok {
		detail.Price = item.Price
		detail.Quantity = item.Quantity
		detail.Listings = item.Listings
		detail.Scanned = item.Scanned
	}
	return detail, nil
}

func (m *MemStore) ItemHistory(ctx context.Context, market, key string, limit int) (store.History, error) {
	return m.HistoryPoints(ctx, market, key, "hour", time.Unix(0, 0), limit)
}

func (m *MemStore) HistoryPoints(_ context.Context, market, key, resolution string, since time.Time, limit int) (store.History, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	history := store.History{Market: market, Key: key, Resolution: resolution, Points: []store.HistoryPoint{}}
	history.Name = m.items[store.ItemID(key)].Name
	if since.IsZero() {
		since = time.Unix(0, 0)
	}

	var source map[string]map[int64]store.HistoryPoint
	switch resolution {
	case "1m", "raw":
		source = m.minute[market]
	case "15m":
		source = m.quarter[market]
	default:
		source = m.history[market]
	}
	for bucket, point := range source[key] {
		if bucket >= since.Unix() {
			point.Bucket = bucket
			history.Points = append(history.Points, point)
		}
	}

	sort.Slice(history.Points, func(i, j int) bool {
		return history.Points[i].Bucket < history.Points[j].Bucket
	})
	if limit > 0 && len(history.Points) > limit {
		history.Points = history.Points[len(history.Points)-limit:]
	}
	return history, nil
}

func (m *MemStore) CleanupHistory(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	minuteCutoff := time.Now().Add(-12 * time.Hour).Unix()
	quarterCutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
	cleanupBuckets(m.minute, minuteCutoff)
	cleanupBuckets(m.quarter, quarterCutoff)
	return nil
}

func cleanupBuckets(target map[string]map[string]map[int64]store.HistoryPoint, cutoff int64) {
	for market, byKey := range target {
		for key, points := range byKey {
			for bucket := range points {
				if bucket < cutoff {
					delete(points, bucket)
				}
			}
			if len(points) == 0 {
				delete(byKey, key)
			}
		}
		if len(byKey) == 0 {
			delete(target, market)
		}
	}
}

func (m *MemStore) ItemMarkets(_ context.Context, key string) ([]store.MarketPrice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []store.MarketPrice{}
	for market, items := range m.markets {
		item, ok := items[key]
		if !ok {
			continue
		}
		result = append(result, store.MarketPrice{
			Market:   market,
			Price:    item.Price,
			Quantity: item.Quantity,
			Listings: item.Listings,
			Scanned:  item.Scanned,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Price != result[j].Price {
			return result[i].Price < result[j].Price
		}
		return result[i].Market < result[j].Market
	})
	return result, nil
}

func (m *MemStore) CreateLoginCode(_ context.Context, codeHash []byte, clientID, character string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codes[hex.EncodeToString(codeHash)] = store.LoginGrant{
		ClientID: clientID, Character: character, ExpiresAt: expires,
	}
	return nil
}

func (m *MemStore) RedeemLoginCode(_ context.Context, codeHash []byte, now time.Time) (store.LoginGrant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hex.EncodeToString(codeHash)
	grant, ok := m.codes[key]
	if !ok || m.used[key] || !grant.ExpiresAt.After(now) {
		return store.LoginGrant{}, store.ErrNotFound
	}
	m.used[key] = true
	return grant, nil
}

func (m *MemStore) CreateWebSession(_ context.Context, tokenHash []byte, clientID, character string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[hex.EncodeToString(tokenHash)] = store.WebSession{
		ClientID: clientID, Character: character, ExpiresAt: expires,
	}
	return nil
}

func (m *MemStore) WebSession(_ context.Context, tokenHash []byte, now time.Time) (store.WebSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[hex.EncodeToString(tokenHash)]
	if !ok || !session.ExpiresAt.After(now) {
		return store.WebSession{}, store.ErrNotFound
	}
	return session, nil
}

func (m *MemStore) DeleteWebSession(_ context.Context, tokenHash []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, hex.EncodeToString(tokenHash))
	return nil
}

func watchKey(clientID, character string) string {
	return clientID + "\x00" + character
}

func (m *MemStore) WatchList(_ context.Context, clientID, character string) (store.WatchList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list, ok := m.watches[watchKey(clientID, character)]
	if !ok {
		return store.WatchList{Character: character, Items: []string{}}, nil
	}
	list.Items = append([]string{}, list.Items...)
	return list, nil
}

func (m *MemStore) SetWatchList(_ context.Context, clientID, character string, items []string) (store.WatchList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if items == nil {
		items = []string{}
	}
	list := store.WatchList{
		Character: character,
		Items:     append([]string{}, items...),
		UpdatedAt: time.Now().Unix(),
	}
	m.watches[watchKey(clientID, character)] = list
	return list, nil
}

func (m *MemStore) Markets(context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var markets []string
	for market := range m.markets {
		markets = append(markets, market)
	}
	sort.Strings(markets)
	return markets, nil
}
