package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) CreateLoginCode(ctx context.Context, codeHash []byte, clientID, character string, expires time.Time) error {
	_, _ = p.pool.Exec(ctx, `DELETE FROM login_codes WHERE expires_at < now()`)
	_, err := p.pool.Exec(ctx,
		`INSERT INTO login_codes (code_hash, client_id, character, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		codeHash, clientID, character, expires,
	)
	return err
}

func (p *Postgres) RedeemLoginCode(ctx context.Context, codeHash []byte, now time.Time) (LoginGrant, error) {
	var grant LoginGrant
	err := p.pool.QueryRow(ctx,
		`UPDATE login_codes
		 SET used_at = $2
		 WHERE code_hash = $1
		   AND used_at IS NULL
		   AND expires_at > $2
		 RETURNING client_id, character, expires_at`,
		codeHash, now,
	).Scan(&grant.ClientID, &grant.Character, &grant.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginGrant{}, ErrNotFound
	}
	return grant, err
}

func (p *Postgres) CreateWebSession(ctx context.Context, tokenHash []byte, clientID, character string, expires time.Time) error {
	_, _ = p.pool.Exec(ctx, `DELETE FROM web_sessions WHERE expires_at < now()`)
	_, err := p.pool.Exec(ctx,
		`INSERT INTO web_sessions (token_hash, client_id, character, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		tokenHash, clientID, character, expires,
	)
	return err
}

func (p *Postgres) WebSession(ctx context.Context, tokenHash []byte, now time.Time) (WebSession, error) {
	var session WebSession
	err := p.pool.QueryRow(ctx,
		`SELECT client_id, character, expires_at
		 FROM web_sessions
		 WHERE token_hash = $1 AND expires_at > $2`,
		tokenHash, now,
	).Scan(&session.ClientID, &session.Character, &session.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return WebSession{}, ErrNotFound
	}
	return session, err
}

func (p *Postgres) DeleteWebSession(ctx context.Context, tokenHash []byte) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM web_sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (p *Postgres) WatchList(ctx context.Context, clientID, character string) (WatchList, error) {
	list := WatchList{Character: character, Items: []string{}}
	var updated time.Time
	err := p.pool.QueryRow(ctx,
		`SELECT items, updated_at
		 FROM watch_lists
		 WHERE client_id = $1 AND character = $2`,
		clientID, character,
	).Scan(&list.Items, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return list, nil
	}
	if err != nil {
		return WatchList{}, err
	}
	list.UpdatedAt = updated.Unix()
	if list.Items == nil {
		list.Items = []string{}
	}
	return list, nil
}

func (p *Postgres) SetWatchList(ctx context.Context, clientID, character string, items []string) (WatchList, error) {
	if items == nil {
		items = []string{}
	}
	list := WatchList{Character: character}
	var updated time.Time
	err := p.pool.QueryRow(ctx,
		`INSERT INTO watch_lists (client_id, character, items, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (client_id, character) DO UPDATE SET
		   items = EXCLUDED.items,
		   updated_at = now()
		 RETURNING items, updated_at`,
		clientID, character, items,
	).Scan(&list.Items, &updated)
	if err != nil {
		return WatchList{}, err
	}
	list.UpdatedAt = updated.Unix()
	return list, nil
}
