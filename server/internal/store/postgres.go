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
