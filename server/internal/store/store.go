package store

import (
	"context"
	"encoding/json"
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
	Key      string `json:"k"`
	Price    int64  `json:"p"`
	Quantity int64  `json:"q"`
	Listings int32  `json:"n"`
}

// ScanPayload is the compressed JSON body stored in an ingest job.
type ScanPayload struct {
	Market        string `json:"market"`
	ScannedAt     int64  `json:"scanned_at"`
	ClientVersion string `json:"client_version"`
	Items         []Item `json:"items"`
}

// IngestJob is one queued scan. Payload is gzip-compressed ScanPayload JSON.
type IngestJob struct {
	ID        int64
	ClientID  string
	Market    string
	ScannedAt time.Time
	Checksum  []byte
	Payload   []byte
	Status    string
	Attempts  int
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
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
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Quality        int             `json:"quality"`
	ClassID        int             `json:"class_id"`
	SubclassID     int             `json:"subclass_id"`
	Icon           int64           `json:"icon"`
	ItemLevel      *int            `json:"item_level,omitempty"`
	RequiredLevel  *int            `json:"required_level,omitempty"`
	BindType       *int            `json:"bind_type,omitempty"`
	InventoryType  *int            `json:"inventory_type,omitempty"`
	EquipLoc       *string         `json:"equip_loc,omitempty"`
	Armor          *int            `json:"armor,omitempty"`
	DamageMin      *int            `json:"damage_min,omitempty"`
	DamageMax      *int            `json:"damage_max,omitempty"`
	Delay          *int            `json:"delay,omitempty"`
	VendorPrice    *int64          `json:"vendor_price,omitempty"`
	MaxStack       *int            `json:"max_stack,omitempty"`
	UniqueItem     *bool           `json:"unique_item,omitempty"`
	SetID          *int            `json:"set_id,omitempty"`
	Stats          json.RawMessage `json:"stats,omitempty"`
	Details        json.RawMessage `json:"details,omitempty"`
	DetailsVersion int             `json:"details_version,omitempty"`
	DetailsUpdated int64           `json:"details_updated,omitempty"`
}

// StatFilter constrains the numeric value of a single item stat.
type StatFilter struct {
	Key string
	Min *float64
	Max *float64
}

// SearchQuery is the filter set used by the website item browser.
type SearchQuery struct {
	Query         string
	Qualities     []int
	ClassID       *int
	SubclassID    *int
	InventoryType *int
	MinIlvl       *int
	MaxIlvl       *int
	MinReqLevel   *int
	MaxReqLevel   *int
	MinPrice      *int64
	MaxPrice      *int64
	InStock       bool
	Stats         []StatFilter
	HasStats      bool
	Sort          string
	Dir           string
	Page          int
	Limit         int
}

type SearchPage struct {
	Items []SearchResult
	Total int64
}

// SearchResult joins a latest price with optional item metadata.
type SearchResult struct {
	Key           string `json:"key"`
	ItemID        int64  `json:"item_id"`
	Name          string `json:"name"`
	Quality       int    `json:"quality"`
	Price         int64  `json:"price"`
	Quantity      int64  `json:"quantity"`
	Listings      int32  `json:"listings"`
	Scanned       int64  `json:"scanned"`
	ItemLevel     *int   `json:"item_level,omitempty"`
	RequiredLevel *int   `json:"required_level,omitempty"`
	BindType      *int   `json:"bind_type,omitempty"`
	InventoryType *int   `json:"inventory_type,omitempty"`
	EquipLoc      string `json:"equip_loc,omitempty"`
	Armor         *int   `json:"armor,omitempty"`
	DamageMin     *int   `json:"damage_min,omitempty"`
	DamageMax     *int   `json:"damage_max,omitempty"`
	Delay         *int   `json:"delay,omitempty"`
	VendorPrice   *int64 `json:"vendor_price,omitempty"`
	MaxStack      *int   `json:"max_stack,omitempty"`
	UniqueItem    *bool  `json:"unique_item,omitempty"`
	SetID         *int   `json:"set_id,omitempty"`
	HasDetails    bool   `json:"has_details"`
}

// ItemDetail is item metadata plus its latest price on one market.
type ItemDetail struct {
	ItemMeta
	Price    int64 `json:"price"`
	Quantity int64 `json:"quantity"`
	Listings int32 `json:"listings"`
	Scanned  int64 `json:"scanned"`
}

// HistoryPoint is one hourly min/max bucket.
type HistoryPoint struct {
	Bucket   int64 `json:"t"`
	Min      int64 `json:"min"`
	Max      int64 `json:"max"`
	Quantity int64 `json:"quantity"`
	Listings int32 `json:"listings,omitempty"`
	Samples  int64 `json:"samples"`
}

type History struct {
	Market     string         `json:"market"`
	Key        string         `json:"key"`
	Name       string         `json:"name"`
	Resolution string         `json:"resolution"`
	Range      string         `json:"range"`
	Points     []HistoryPoint `json:"points"`
}

type Stats struct {
	Market       string           `json:"market"`
	GeneratedAt  int64            `json:"generated_at"`
	Window       StatsWindow      `json:"window"`
	Index        StatsIndex       `json:"index"`
	Totals       StatsTotals      `json:"totals"`
	Breadth      StatsBreadth     `json:"breadth"`
	NewItems     StatsNewItems    `json:"new_items"`
	Delisted     int64            `json:"delisted"`
	Movers       StatsMovers      `json:"movers"`
	Stock        StatsStock       `json:"stock"`
	Classes      []StatsClass     `json:"classes"`
	Quality      []StatsQuality   `json:"quality"`
	Bands        []StatsBand      `json:"bands"`
	Heavyweights []StatsItemValue `json:"heavyweights"`
	Freshness    StatsFreshness   `json:"freshness"`
}

type StatsWindow struct {
	TargetHours     int     `json:"target_hours"`
	CurrentBucket   int64   `json:"current_bucket"`
	ReferenceBucket int64   `json:"reference_bucket"`
	ReferenceHours  float64 `json:"reference_hours"`
	FirstBucket     int64   `json:"first_bucket"`
}

type StatsPoint struct {
	T     int64   `json:"t"`
	Value float64 `json:"value"`
}

type StatsIndex struct {
	ChangePct float64      `json:"change_pct"`
	Matched   int          `json:"matched"`
	Points    []StatsPoint `json:"points"`
}

type StatsTotals struct {
	Items        int   `json:"items"`
	ItemsPrev    int   `json:"items_prev"`
	Offers       int   `json:"offers"`
	OffersPrev   int   `json:"offers_prev"`
	Quantity     int64 `json:"quantity"`
	QuantityPrev int64 `json:"quantity_prev"`
	Value        int64 `json:"value"`
	ValuePrev    int64 `json:"value_prev"`
}

type StatsBreadth struct {
	Up, Down, Flat, Matched int
}

type StatsNewItems struct {
	Count int         `json:"count"`
	Items []StatsItem `json:"items"`
}

type StatsItem struct {
	ItemID   int64  `json:"item_id"`
	Key      string `json:"key,omitempty"`
	Name     string `json:"name"`
	Quality  int    `json:"quality"`
	Price    int64  `json:"price"`
	Quantity int64  `json:"quantity"`
}

type StatsMovers struct {
	Gainers []StatsMove `json:"gainers"`
	Losers  []StatsMove `json:"losers"`
}

type StatsMove struct {
	ItemID       int64   `json:"item_id"`
	Name         string  `json:"name"`
	Quality      int     `json:"quality"`
	Price        int64   `json:"price"`
	PricePrev    int64   `json:"price_prev"`
	ChangePct    float64 `json:"change_pct"`
	Quantity     int64   `json:"quantity"`
	QuantityPrev int64   `json:"quantity_prev"`
}

type StatsStock struct {
	Added   []StatsStockMove `json:"added"`
	Removed []StatsStockMove `json:"removed"`
}

type StatsStockMove struct {
	ItemID       int64  `json:"item_id"`
	Name         string `json:"name"`
	Quality      int    `json:"quality"`
	Price        int64  `json:"price"`
	Quantity     int64  `json:"quantity"`
	QuantityPrev int64  `json:"quantity_prev"`
	Delta        int64  `json:"delta"`
}

type StatsClass struct {
	ClassID   int     `json:"class_id"`
	Items     int     `json:"items"`
	Quantity  int64   `json:"quantity"`
	Value     int64   `json:"value"`
	ChangePct float64 `json:"change_pct"`
}

type StatsQuality struct {
	Quality  int   `json:"quality"`
	Items    int   `json:"items"`
	Offers   int   `json:"offers"`
	Quantity int64 `json:"quantity"`
	Value    int64 `json:"value"`
}

type StatsBand struct {
	Label string `json:"label"`
	Items int    `json:"items"`
	Value int64  `json:"value"`
}

type StatsItemValue struct {
	ItemID   int64   `json:"item_id"`
	Name     string  `json:"name"`
	Value    int64   `json:"value"`
	SharePct float64 `json:"share_pct"`
}

type StatsFreshness struct {
	LastScan       int64   `json:"last_scan"`
	LastScanAgeMin float64 `json:"last_scan_age_min"`
	Scans24h       int     `json:"scans_24h"`
	Clients24h     int     `json:"clients_24h"`
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
	EnqueueIngest(ctx context.Context, clientID string, job IngestJob) (bool, error)
	ClaimIngestJobs(ctx context.Context, limit int) ([]IngestJob, error)
	ProcessIngestJob(ctx context.Context, job IngestJob) error
	CompleteIngestJob(ctx context.Context, id int64) error
	FailIngestJob(ctx context.Context, id int64, attempts int, errMsg string) error
	Meta(ctx context.Context) ([]MarketMeta, error)
	Snapshot(ctx context.Context, market string) (Snapshot, error)
	Markets(ctx context.Context) ([]string, error)
	UpsertItems(ctx context.Context, items []ItemMeta) error
	SearchItems(ctx context.Context, market string, query SearchQuery) (SearchPage, error)
	MissingItemDetails(ctx context.Context, market string, force bool, limit int) ([]int64, error)
	ItemDetail(ctx context.Context, market, key string) (ItemDetail, error)
	ItemHistory(ctx context.Context, market, key string, limit int) (History, error)
	HistoryPoints(ctx context.Context, market, key, resolution string, since time.Time, limit int) (History, error)
	Stats(ctx context.Context, market string, hours int) (Stats, error)
	CleanupHistory(ctx context.Context) error
	ItemMarkets(ctx context.Context, key string) ([]MarketPrice, error)

	CreateLoginCode(ctx context.Context, codeHash []byte, clientID, character string, expires time.Time) error
	RedeemLoginCode(ctx context.Context, codeHash []byte, now time.Time) (LoginGrant, error)
	CountLiveClients(ctx context.Context, since time.Time) (int, error)
	CreateWebSession(ctx context.Context, tokenHash []byte, clientID, character string, expires time.Time) error
	WebSession(ctx context.Context, tokenHash []byte, now time.Time) (WebSession, error)
	DeleteWebSession(ctx context.Context, tokenHash []byte) error
	WatchList(ctx context.Context, clientID, character string) (WatchList, error)
	SetWatchList(ctx context.Context, clientID, character string, items []string) (WatchList, error)
}
