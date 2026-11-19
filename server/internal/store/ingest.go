package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
)

// EnqueueIngest stores one compressed scan. It returns false when the client
// already submitted a scan with the same checksum.
func (p *Postgres) EnqueueIngest(ctx context.Context, clientID string, job IngestJob) (bool, error) {
	var id int64
	err := p.pool.QueryRow(ctx,
		`INSERT INTO ingest_jobs (client_id, market, scanned_at, checksum, payload)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (client_id, checksum) DO NOTHING
		 RETURNING id`,
		clientID, job.Market, job.ScannedAt, job.Checksum, job.Payload,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ClaimIngestJobs atomically leases up to limit queued jobs. SKIP LOCKED makes
// concurrent worker replicas safe.
func (p *Postgres) ClaimIngestJobs(ctx context.Context, limit int) ([]IngestJob, error) {
	if limit < 1 {
		limit = 1
	}
	rows, err := p.pool.Query(ctx,
		`WITH claimed AS (
		   SELECT id FROM ingest_jobs
		   WHERE status = 'queued'
		   ORDER BY id
		   FOR UPDATE SKIP LOCKED
		   LIMIT $1
		 )
		 UPDATE ingest_jobs j
		 SET status = 'processing', updated_at = now()
		 FROM claimed c
		 WHERE j.id = c.id
		 RETURNING j.id, j.client_id, j.market, j.scanned_at, j.checksum,
		           j.payload, j.status, j.attempts, COALESCE(j.error, ''),
		           j.created_at, j.updated_at`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []IngestJob
	for rows.Next() {
		var job IngestJob
		if err := rows.Scan(
			&job.ID, &job.ClientID, &job.Market, &job.ScannedAt, &job.Checksum,
			&job.Payload, &job.Status, &job.Attempts, &job.Error,
			&job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (p *Postgres) CompleteIngestJob(ctx context.Context, id int64) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE ingest_jobs SET status = 'done', error = NULL, updated_at = now() WHERE id = $1`,
		id,
	)
	return err
}

func (p *Postgres) FailIngestJob(ctx context.Context, id int64, attempts int, errMsg string) error {
	if attempts < 0 {
		attempts = 0
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE ingest_jobs
		 SET attempts = $2,
		     error = $3,
		     status = CASE WHEN $2 >= 5 THEN 'failed' ELSE 'queued' END,
		     updated_at = now()
		 WHERE id = $1`,
		id, attempts, errMsg,
	)
	return err
}

// ProcessIngestJob applies one compressed scan to the price tables in a single
// transaction. Re-processing a scan whose audit row already exists is a no-op.
func (p *Postgres) ProcessIngestJob(ctx context.Context, job IngestJob) error {
	payload, err := decodePayload(job.Payload)
	if err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if payload.Market == "" {
		payload.Market = job.Market
	}
	if payload.ScannedAt == 0 {
		payload.ScannedAt = job.ScannedAt.Unix()
	}
	scannedAt := job.ScannedAt
	if payload.ScannedAt != 0 {
		scannedAt = time.Unix(payload.ScannedAt, 0)
	}
	if payload.Market == "" {
		return errors.New("payload has no market")
	}
	if len(payload.Items) == 0 {
		return errors.New("payload has no items")
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO markets (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		payload.Market,
	); err != nil {
		return err
	}

	var scanID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO scans (client_id, market, scanned_at, item_count, checksum, client_version)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (client_id, checksum) DO NOTHING
		 RETURNING id`,
		job.ClientID, payload.Market, scannedAt, len(payload.Items), job.Checksum, payload.ClientVersion,
	).Scan(&scanID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already applied by an earlier attempt; still record liveness and finish.
		if _, err := tx.Exec(ctx, `UPDATE clients SET last_seen = now() WHERE id = $1`, job.ClientID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE ingest_jobs SET status = 'done', error = NULL, updated_at = now() WHERE id = $1`,
			job.ID,
		); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`CREATE TEMP TABLE IF NOT EXISTS ingest_stage (
		   market text NOT NULL,
		   item_key text NOT NULL,
		   price bigint NOT NULL,
		   quantity bigint NOT NULL,
		   listings integer NOT NULL,
		   scanned_at timestamptz NOT NULL
		 ) ON COMMIT DROP`,
	); err != nil {
		return err
	}

	rows := make([][]any, 0, len(payload.Items))
	for _, item := range payload.Items {
		rows = append(rows, []any{
			payload.Market, item.Key, item.Price, item.Quantity, item.Listings, scannedAt,
		})
	}
	if _, err := tx.CopyFrom(ctx,
		pgx.Identifier{"ingest_stage"},
		[]string{"market", "item_key", "price", "quantity", "listings", "scanned_at"},
		pgx.CopyFromRows(rows),
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO prices_1m (market, item_key, bucket, min_price, max_price, quantity, samples)
		 SELECT market, item_key, date_trunc('minute', scanned_at),
		        min(price), max(price), max(quantity), count(*)::integer
		 FROM ingest_stage
		 GROUP BY market, item_key, date_trunc('minute', scanned_at)
		 ON CONFLICT (market, item_key, bucket) DO UPDATE SET
		   min_price = LEAST(prices_1m.min_price, EXCLUDED.min_price),
		   max_price = GREATEST(prices_1m.max_price, EXCLUDED.max_price),
		   quantity = GREATEST(prices_1m.quantity, EXCLUDED.quantity),
		   samples = prices_1m.samples + EXCLUDED.samples`,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO prices_15m (market, item_key, bucket, min_price, max_price, quantity, samples)
		 SELECT market, item_key,
		        to_timestamp(floor(extract(epoch FROM scanned_at) / 900) * 900),
		        min(price), max(price), max(quantity), count(*)::integer
		 FROM ingest_stage
		 GROUP BY market, item_key,
		          to_timestamp(floor(extract(epoch FROM scanned_at) / 900) * 900)
		 ON CONFLICT (market, item_key, bucket) DO UPDATE SET
		   min_price = LEAST(prices_15m.min_price, EXCLUDED.min_price),
		   max_price = GREATEST(prices_15m.max_price, EXCLUDED.max_price),
		   quantity = GREATEST(prices_15m.quantity, EXCLUDED.quantity),
		   samples = prices_15m.samples + EXCLUDED.samples`,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO prices_hourly (market, item_key, bucket, min_price, max_price, quantity, samples)
		 SELECT market, item_key, date_trunc('hour', scanned_at),
		        min(price), max(price), max(quantity), count(*)::integer
		 FROM ingest_stage
		 GROUP BY market, item_key, date_trunc('hour', scanned_at)
		 ON CONFLICT (market, item_key, bucket) DO UPDATE SET
		   min_price = LEAST(prices_hourly.min_price, EXCLUDED.min_price),
		   max_price = GREATEST(prices_hourly.max_price, EXCLUDED.max_price),
		   quantity = GREATEST(prices_hourly.quantity, EXCLUDED.quantity),
		   samples = prices_hourly.samples + EXCLUDED.samples`,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO prices_latest (market, item_key, price, quantity, listings, scanned_at, samples)
		 SELECT DISTINCT ON (item_key)
		        market, item_key, price, quantity, listings, scanned_at, 1
		 FROM ingest_stage
		 ORDER BY item_key, scanned_at DESC
		 ON CONFLICT (market, item_key) DO UPDATE SET
		   price = EXCLUDED.price,
		   quantity = EXCLUDED.quantity,
		   listings = EXCLUDED.listings,
		   scanned_at = EXCLUDED.scanned_at,
		   samples = prices_latest.samples + 1
		 WHERE EXCLUDED.scanned_at > prices_latest.scanned_at`,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE markets SET version = version + 1, updated_at = now() WHERE id = $1`,
		payload.Market,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE clients SET last_seen = now() WHERE id = $1`, job.ClientID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE ingest_jobs SET status = 'done', error = NULL, updated_at = now() WHERE id = $1`,
		job.ID,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func decodePayload(payload []byte) (ScanPayload, error) {
	var scan ScanPayload
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
