package api

import (
	"context"
	"encoding/hex"
	"sort"
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
}

func NewMemStore() *MemStore {
	return &MemStore{
		clients:  map[string]store.Client{},
		tokens:   map[string]string{},
		seen:     map[string]bool{},
		markets:  map[string]map[string]store.SnapshotItem{},
		versions: map[string]int64{},
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
