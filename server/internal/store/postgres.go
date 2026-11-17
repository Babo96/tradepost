package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations.sql
var migrations string

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if _, err := pool.Exec(ctx, migrations); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Close() {
	p.pool.Close()
}

func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *Postgres) CreateClient(ctx context.Context, id string, tokenHash []byte) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO clients (id, token_hash) VALUES ($1, $2)`,
		id, tokenHash,
	)
	return err
}

func (p *Postgres) ClientByTokenHash(ctx context.Context, tokenHash []byte) (Client, error) {
	var client Client
	err := p.pool.QueryRow(ctx,
		`SELECT id, status, created_at FROM clients WHERE token_hash = $1`,
		tokenHash,
	).Scan(&client.ID, &client.Status, &client.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Client{}, ErrNotFound
	}
	return client, err
}

func (p *Postgres) Ingest(ctx context.Context, clientID string, scan Scan) (IngestResult, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return IngestResult{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO markets (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		scan.Market,
	); err != nil {
		return IngestResult{}, err
	}

	var scanID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO scans (client_id, market, scanned_at, item_count, checksum, client_version)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (client_id, checksum) DO NOTHING
		 RETURNING id`,
		clientID, scan.Market, scan.ScannedAt, len(scan.Items), scan.Checksum, scan.ClientVersion,
	).Scan(&scanID)
	if errors.Is(err, pgx.ErrNoRows) {
		return IngestResult{Rejected: 1}, nil
	}
	if err != nil {
		return IngestResult{}, err
	}

	batch := &pgx.Batch{}
	bucket := scan.ScannedAt.Truncate(time.Hour)
	for _, item := range scan.Items {
		batch.Queue(
			`INSERT INTO prices_latest (market, item_key, price, quantity, listings, scanned_at, samples)
			 VALUES ($1, $2, $3, $4, $5, $6, 1)
			 ON CONFLICT (market, item_key) DO UPDATE SET
			   price = EXCLUDED.price,
			   quantity = EXCLUDED.quantity,
			   listings = EXCLUDED.listings,
			   scanned_at = EXCLUDED.scanned_at,
			   samples = prices_latest.samples + 1
			 WHERE EXCLUDED.scanned_at > prices_latest.scanned_at`,
			scan.Market, item.Key, item.Price, item.Quantity, item.Listings, scan.ScannedAt,
		)
		batch.Queue(
			`INSERT INTO prices_hourly (market, item_key, bucket, min_price, max_price, quantity, samples)
			 VALUES ($1, $2, $3, $4, $4, $5, 1)
			 ON CONFLICT (market, item_key, bucket) DO UPDATE SET
			   min_price = LEAST(prices_hourly.min_price, EXCLUDED.min_price),
			   max_price = GREATEST(prices_hourly.max_price, EXCLUDED.max_price),
			   quantity = GREATEST(prices_hourly.quantity, EXCLUDED.quantity),
			   samples = prices_hourly.samples + 1`,
			scan.Market, item.Key, bucket, item.Price, item.Quantity,
		)
	}

	if batch.Len() > 0 {
		results := tx.SendBatch(ctx, batch)
		for range scan.Items {
			if _, err := results.Exec(); err != nil {
				results.Close()
				return IngestResult{}, err
			}
			if _, err := results.Exec(); err != nil {
				results.Close()
				return IngestResult{}, err
			}
		}
		if err := results.Close(); err != nil {
			return IngestResult{}, err
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE markets SET version = version + 1, updated_at = now() WHERE id = $1`,
		scan.Market,
	); err != nil {
		return IngestResult{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE clients SET last_seen = now() WHERE id = $1`,
		clientID,
	); err != nil {
		return IngestResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return IngestResult{}, err
	}
	return IngestResult{Accepted: 1, Items: len(scan.Items)}, nil
}

func (p *Postgres) Meta(ctx context.Context) ([]MarketMeta, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT m.id,
		        m.version,
		        m.updated_at,
		        (SELECT count(*) FROM prices_latest p WHERE p.market = m.id),
		        COALESCE((SELECT max(scanned_at) FROM prices_latest p WHERE p.market = m.id), to_timestamp(0))
		 FROM markets m
		 ORDER BY m.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MarketMeta
	for rows.Next() {
		var meta MarketMeta
		var updatedAt, lastScan time.Time
		if err := rows.Scan(&meta.Market, &meta.Version, &updatedAt, &meta.Items, &lastScan); err != nil {
			return nil, err
		}
		meta.UpdatedAt = updatedAt.Unix()
		meta.LastScan = lastScan.Unix()
		result = append(result, meta)
	}
	return result, rows.Err()
}

func (p *Postgres) Snapshot(ctx context.Context, market string) (Snapshot, error) {
	var version int64
	err := p.pool.QueryRow(ctx,
		`SELECT version FROM markets WHERE id = $1`, market,
	).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, ErrNotFound
	}
	if err != nil {
		return Snapshot{}, err
	}

	rows, err := p.pool.Query(ctx,
		`SELECT item_key, price, quantity, listings, extract(epoch FROM scanned_at)::bigint
		 FROM prices_latest
		 WHERE market = $1
		 ORDER BY item_key`,
		market,
	)
	if err != nil {
		return Snapshot{}, err
	}
	defer rows.Close()

	snapshot := Snapshot{
		Market:    market,
		Version:   version,
		Generated: time.Now().Unix(),
		Items:     make([]SnapshotItem, 0, 8192),
	}
	for rows.Next() {
		var item SnapshotItem
		if err := rows.Scan(&item.Key, &item.Price, &item.Quantity, &item.Listings, &item.Scanned); err != nil {
			return Snapshot{}, err
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	return snapshot, rows.Err()
}

func (p *Postgres) Markets(ctx context.Context) ([]string, error) {
	rows, err := p.pool.Query(ctx, `SELECT id FROM markets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var markets []string
	for rows.Next() {
		var market string
		if err := rows.Scan(&market); err != nil {
			return nil, err
		}
		markets = append(markets, market)
	}
	return markets, rows.Err()
}
