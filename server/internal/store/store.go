package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrDuplicate    = errors.New("duplicate scan")
	ErrUnauthorized = errors.New("unauthorized")
)

type Client struct {
	ID        string
	Status    string
	CreatedAt time.Time
}

type Item struct {
	Key      string
	Price    int64
	Quantity int64
	Listings int32
}

type Scan struct {
	Market        string
	ScannedAt     time.Time
	ClientVersion string
	Checksum      []byte
	Items         []Item
}

type IngestResult struct {
	Accepted int
	Rejected int
	Items    int
}

type MarketMeta struct {
	Market    string `json:"market"`
	Version   int64  `json:"version"`
	Items     int64  `json:"items"`
	LastScan  int64  `json:"last_scan"`
	UpdatedAt int64  `json:"updated_at"`
}

type SnapshotItem struct {
	Key      string `json:"k"`
	Price    int64  `json:"p"`
	Quantity int64  `json:"q"`
	Listings int32  `json:"n"`
	Scanned  int64  `json:"t"`
}

type Snapshot struct {
	Market    string         `json:"market"`
	Version   int64          `json:"version"`
	Generated int64          `json:"generated"`
	Items     []SnapshotItem `json:"items"`
}

// ItemMeta is client-known item information. It is keyed by item ID, not by
// price key, so equipment variants share one name/icon.
type ItemMeta struct {
	ID         int64
	Name       string
	Quality    int
	ClassID    int
	SubclassID int
	Icon       int64
}

// SearchResult joins a latest price with optional item metadata.
type SearchResult struct {
	Key      string `json:"key"`
	ItemID   int64  `json:"item_id"`
	Name     string `json:"name"`
	Quality  int    `json:"quality"`
	Price    int64  `json:"price"`
	Quantity int64  `json:"quantity"`
	Listings int32  `json:"listings"`
	Scanned  int64  `json:"scanned"`
}

// HistoryPoint is one hourly min/max bucket.
type HistoryPoint struct {
	Bucket   int64 `json:"t"`
	Min      int64 `json:"min"`
	Max      int64 `json:"max"`
	Quantity int64 `json:"quantity"`
	Samples  int64 `json:"samples"`
}

type History struct {
	Market string         `json:"market"`
	Key    string         `json:"key"`
	Name   string         `json:"name"`
	Points []HistoryPoint `json:"points"`
}

// MarketPrice compares one key across markets.
type MarketPrice struct {
	Market   string `json:"market"`
	Price    int64  `json:"price"`
	Quantity int64  `json:"quantity"`
	Listings int32  `json:"listings"`
	Scanned  int64  `json:"scanned"`
}

// ItemID extracts the numeric item ID from i:<id> or g:<id>:<ilvl> keys.
func ItemID(key string) int64 {
	parts := strings.Split(key, ":")
	if len(parts) < 2 {
		return 0
	}
	id, _ := strconv.ParseInt(parts[1], 10, 64)
	return id
}

// LoginGrant is the result of redeeming a short login code.
type LoginGrant struct {
	ClientID  string
	Character string
	ExpiresAt time.Time
}

// WebSession is a browser session bound to one character.
type WebSession struct {
	ClientID  string
	Character string
	ExpiresAt time.Time
}

// WatchList is one character's pinned item keys.
type WatchList struct {
	Character string   `json:"character"`
	Items     []string `json:"items"`
	UpdatedAt int64    `json:"updated_at"`
}

type Store interface {
	Ping(ctx context.Context) error
	CreateClient(ctx context.Context, id string, tokenHash []byte) error
	ClientByTokenHash(ctx context.Context, tokenHash []byte) (Client, error)
	Ingest(ctx context.Context, clientID string, scan Scan) (IngestResult, error)
	Meta(ctx context.Context) ([]MarketMeta, error)
	Snapshot(ctx context.Context, market string) (Snapshot, error)
	Markets(ctx context.Context) ([]string, error)
	UpsertItems(ctx context.Context, items []ItemMeta) error
	SearchItems(ctx context.Context, market, query string, limit int) ([]SearchResult, error)
	ItemHistory(ctx context.Context, market, key string, limit int) (History, error)
	ItemMarkets(ctx context.Context, key string) ([]MarketPrice, error)

	CreateLoginCode(ctx context.Context, codeHash []byte, clientID, character string, expires time.Time) error
	RedeemLoginCode(ctx context.Context, codeHash []byte, now time.Time) (LoginGrant, error)
	CreateWebSession(ctx context.Context, tokenHash []byte, clientID, character string, expires time.Time) error
	WebSession(ctx context.Context, tokenHash []byte, now time.Time) (WebSession, error)
	DeleteWebSession(ctx context.Context, tokenHash []byte) error
	WatchList(ctx context.Context, clientID, character string) (WatchList, error)
	SetWatchList(ctx context.Context, clientID, character string, items []string) (WatchList, error)
}
