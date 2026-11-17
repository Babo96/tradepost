package api

import (
	"context"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"

	"babo-forever/server/internal/store"
)

// MemStore is an in-memory Store for handler tests and local smoke tests.
type MemStore struct {
	mu       sync.Mutex
	clients  map[string]store.Client
	tokens   map[string]string
	seen     map[string]bool
	markets  map[string]map[string]store.SnapshotItem
	versions map[string]int64
	items    map[int64]store.ItemMeta
	history  map[string]map[string]map[int64]store.HistoryPoint
	quarter  map[string]map[string]map[int64]store.HistoryPoint
	raw      map[string]map[string][]store.HistoryPoint
	codes    map[string]store.LoginGrant
	used     map[string]bool
	sessions map[string]store.WebSession
	watches  map[string]store.WatchList
}

func NewMemStore() *MemStore {
	return &MemStore{
		clients:  map[string]store.Client{},
		tokens:   map[string]string{},
		seen:     map[string]bool{},
		markets:  map[string]map[string]store.SnapshotItem{},
		versions: map[string]int64{},
		items:    map[int64]store.ItemMeta{},
		history:  map[string]map[string]map[int64]store.HistoryPoint{},
		quarter:  map[string]map[string]map[int64]store.HistoryPoint{},
		raw:      map[string]map[string][]store.HistoryPoint{},
		codes:    map[string]store.LoginGrant{},
		used:     map[string]bool{},
		sessions: map[string]store.WebSession{},
		watches:  map[string]store.WatchList{},
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

func (m *MemStore) Ingest(_ context.Context, clientID string, scan store.Scan) (store.IngestResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := clientID + ":" + hex.EncodeToString(scan.Checksum)
	if m.seen[key] {
		return store.IngestResult{Rejected: 1}, nil
	}
	m.seen[key] = true
	if m.markets[scan.Market] == nil {
		m.markets[scan.Market] = map[string]store.SnapshotItem{}
	}
	latest := m.markets[scan.Market]
	for _, item := range scan.Items {
		existing, ok := latest[item.Key]
		if ok && existing.Scanned >= scan.ScannedAt.Unix() {
			continue
		}
		latest[item.Key] = store.SnapshotItem{
			Key:      item.Key,
			Price:    item.Price,
			Quantity: item.Quantity,
			Listings: item.Listings,
			Scanned:  scan.ScannedAt.Unix(),
		}
	}
	if m.history[scan.Market] == nil {
		m.history[scan.Market] = map[string]map[int64]store.HistoryPoint{}
	}
	bucket := scan.ScannedAt.Truncate(time.Hour).Unix()
	for _, item := range scan.Items {
		byKey := m.history[scan.Market][item.Key]
		if byKey == nil {
			byKey = map[int64]store.HistoryPoint{}
			m.history[scan.Market][item.Key] = byKey
		}
		point, ok := byKey[bucket]
		if !ok {
			point = store.HistoryPoint{Bucket: bucket, Min: item.Price, Max: item.Price}
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
		byKey[bucket] = point
	}

	if m.quarter[scan.Market] == nil {
		m.quarter[scan.Market] = map[string]map[int64]store.HistoryPoint{}
	}
	quarterBucket := scan.ScannedAt.Truncate(15 * time.Minute).Unix()
	if m.raw[scan.Market] == nil {
		m.raw[scan.Market] = map[string][]store.HistoryPoint{}
	}
	for _, item := range scan.Items {
		byKey := m.quarter[scan.Market][item.Key]
		if byKey == nil {
			byKey = map[int64]store.HistoryPoint{}
			m.quarter[scan.Market][item.Key] = byKey
		}
		point, ok := byKey[quarterBucket]
		if !ok {
			point = store.HistoryPoint{Bucket: quarterBucket, Min: item.Price, Max: item.Price}
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
		byKey[quarterBucket] = point

		m.raw[scan.Market][item.Key] = append(m.raw[scan.Market][item.Key], store.HistoryPoint{
			Bucket: scan.ScannedAt.Unix(), Min: item.Price, Max: item.Price,
			Quantity: item.Quantity, Listings: item.Listings, Samples: 1,
		})
	}

	m.versions[scan.Market]++
	return store.IngestResult{Accepted: 1, Items: len(scan.Items)}, nil
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
		m.items[item.ID] = item
	}
	return nil
}

func (m *MemStore) SearchItems(_ context.Context, market, query string, limit int) ([]store.SearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := []store.SearchResult{}
	for key, item := range m.markets[market] {
		id := store.ItemID(key)
		meta := m.items[id]
		name := meta.Name
		if query != "" {
			if !strings.Contains(strings.ToLower(name), query) &&
				!strings.Contains(key, query) &&
				key != query {
				continue
			}
		}
		result = append(result, store.SearchResult{
			Key:      key,
			ItemID:   id,
			Name:     name,
			Quality:  meta.Quality,
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
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Key < result[j].Key
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
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

	switch resolution {
	case "raw":
		for _, point := range m.raw[market][key] {
			if point.Bucket >= since.Unix() {
				history.Points = append(history.Points, point)
			}
		}
	case "15m":
		for bucket, point := range m.quarter[market][key] {
			if bucket >= since.Unix() {
				point.Bucket = bucket
				history.Points = append(history.Points, point)
			}
		}
	default:
		for bucket, point := range m.history[market][key] {
			if bucket >= since.Unix() {
				point.Bucket = bucket
				history.Points = append(history.Points, point)
			}
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
	rawCutoff := time.Now().Add(-12 * time.Hour).Unix()
	quarterCutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
	for market, byKey := range m.raw {
		for key, points := range byKey {
			kept := points[:0]
			for _, point := range points {
				if point.Bucket >= rawCutoff {
					kept = append(kept, point)
				}
			}
			if len(kept) == 0 {
				delete(byKey, key)
			} else {
				byKey[key] = kept
			}
		}
		if len(byKey) == 0 {
			delete(m.raw, market)
		}
	}
	for market, byKey := range m.quarter {
		for key, points := range byKey {
			for bucket := range points {
				if bucket < quarterCutoff {
					delete(points, bucket)
				}
			}
			if len(points) == 0 {
				delete(byKey, key)
			}
		}
		if len(byKey) == 0 {
			delete(m.quarter, market)
		}
	}
	return nil
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
