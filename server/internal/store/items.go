package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) UpsertItems(ctx context.Context, items []ItemMeta) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for _, item := range items {
		batch.Queue(
			`INSERT INTO items (item_id, name, quality, class_id, subclass_id, icon, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, now())
			 ON CONFLICT (item_id) DO UPDATE SET
			   name = EXCLUDED.name,
			   quality = EXCLUDED.quality,
			   class_id = EXCLUDED.class_id,
			   subclass_id = EXCLUDED.subclass_id,
			   icon = EXCLUDED.icon,
			   updated_at = now()`,
			item.ID, item.Name, item.Quality, item.ClassID, item.SubclassID, item.Icon,
		)
	}
	results := tx.SendBatch(ctx, batch)
	for range items {
		if _, err := results.Exec(); err != nil {
			results.Close()
			return err
		}
	}
	if err := results.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) SearchItems(ctx context.Context, market, query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	rows, err := p.pool.Query(ctx,
		`SELECT p.item_key,
		        COALESCE(NULLIF(split_part(p.item_key, ':', 2), '')::bigint, 0),
		        COALESCE(i.name, ''),
		        COALESCE(i.quality, 1),
		        p.price,
		        p.quantity,
		        p.listings,
		        extract(epoch FROM p.scanned_at)::bigint
		 FROM prices_latest p
		 LEFT JOIN items i
		   ON i.item_id = NULLIF(split_part(p.item_key, ':', 2), '')::bigint
		 WHERE p.market = $1
		   AND ($2 = ''
		        OR p.item_key = $2
		        OR i.name ILIKE '%' || $2 || '%'
		        OR split_part(p.item_key, ':', 2) = $2)
		 ORDER BY p.price ASC, i.name ASC, p.item_key ASC
		 LIMIT $3`,
		market, query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]SearchResult, 0, limit)
	for rows.Next() {
		var item SearchResult
		if err := rows.Scan(
			&item.Key, &item.ItemID, &item.Name, &item.Quality,
			&item.Price, &item.Quantity, &item.Listings, &item.Scanned,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (p *Postgres) ItemHistory(ctx context.Context, market, key string, limit int) (History, error) {
	return p.HistoryPoints(ctx, market, key, "hour", time.Unix(0, 0), limit)
}

func (p *Postgres) HistoryPoints(ctx context.Context, market, key, resolution string, since time.Time, limit int) (History, error) {
	history := History{Market: market, Key: key, Resolution: resolution, Points: []HistoryPoint{}}
	if since.IsZero() {
		since = time.Unix(0, 0)
	}
	if limit < 1 {
		limit = 1000
	}

	itemID := ItemID(key)
	if itemID > 0 {
		_ = p.pool.QueryRow(ctx,
			`SELECT name FROM items WHERE item_id = $1`, itemID,
		).Scan(&history.Name)
	}

	var query string
	switch resolution {
	case "raw":
		query = `SELECT extract(epoch FROM scanned_at)::bigint, price, price, quantity, listings, 1
		         FROM prices_raw
		         WHERE market = $1 AND item_key = $2 AND scanned_at >= $3
		         ORDER BY scanned_at DESC
		         LIMIT $4`
	case "15m":
		query = `SELECT extract(epoch FROM bucket)::bigint, min_price, max_price, quantity, 0, samples
		         FROM prices_15m
		         WHERE market = $1 AND item_key = $2 AND bucket >= $3
		         ORDER BY bucket DESC
		         LIMIT $4`
	default:
		query = `SELECT extract(epoch FROM bucket)::bigint, min_price, max_price, quantity, 0, samples
		         FROM prices_hourly
		         WHERE market = $1 AND item_key = $2 AND bucket >= $3
		         ORDER BY bucket DESC
		         LIMIT $4`
	}

	rows, err := p.pool.Query(ctx, query, market, key, since, limit)
	if err != nil {
		return History{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var point HistoryPoint
		if err := rows.Scan(&point.Bucket, &point.Min, &point.Max, &point.Quantity, &point.Listings, &point.Samples); err != nil {
			return History{}, err
		}
		history.Points = append(history.Points, point)
	}
	if err := rows.Err(); err != nil {
		return History{}, err
	}
	for i, j := 0, len(history.Points)-1; i < j; i, j = i+1, j-1 {
		history.Points[i], history.Points[j] = history.Points[j], history.Points[i]
	}
	return history, nil
}

func (p *Postgres) CleanupHistory(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx,
		`DELETE FROM prices_raw WHERE scanned_at < now() - interval '12 hours'`,
	); err != nil {
		return err
	}
	_, err := p.pool.Exec(ctx,
		`DELETE FROM prices_15m WHERE bucket < now() - interval '7 days'`,
	)
	return err
}

func (p *Postgres) ItemMarkets(ctx context.Context, key string) ([]MarketPrice, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT market, price, quantity, listings, extract(epoch FROM scanned_at)::bigint
		 FROM prices_latest
		 WHERE item_key = $1
		 ORDER BY price ASC`,
		key,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []MarketPrice{}
	for rows.Next() {
		var price MarketPrice
		if err := rows.Scan(&price.Market, &price.Price, &price.Quantity, &price.Listings, &price.Scanned); err != nil {
			return nil, err
		}
		result = append(result, price)
	}
	return result, rows.Err()
}
