package store

import (
	"context"
	"errors"
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

type Store interface {
	Ping(ctx context.Context) error
	CreateClient(ctx context.Context, id string, tokenHash []byte) error
	ClientByTokenHash(ctx context.Context, tokenHash []byte) (Client, error)
	Ingest(ctx context.Context, clientID string, scan Scan) (IngestResult, error)
	Meta(ctx context.Context) ([]MarketMeta, error)
	Snapshot(ctx context.Context, market string) (Snapshot, error)
	Markets(ctx context.Context) ([]string, error)
}
