# server — TradePost API

Go + Postgres API for the crowd-sourced TradePost data. It accepts companion
uploads, aggregates prices, stores item metadata, serves the website, and
publishes per-market snapshots and data addons.

## Local development

```sh
make db-up          # Postgres 18 on localhost:5432 (tradepost/tradepost)
make server-run     # API on :8080
make server-test
```

Environment:

```
TRADEPOST_DATABASE_URL  postgres://tradepost:tradepost@localhost:5432/tradepost?sslmode=disable
TRADEPOST_ADDR          :8080
TRADEPOST_WORKER        optional; set 0 to disable the in-process worker
```

Migrations are embedded and run at startup. The website is embedded in the
binary from `internal/api/web`; there is no separate frontend build.

## API

All `/v1` JSON; the companion uses a bearer token, the website uses an HttpOnly
session cookie.

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/v1/enroll` | create a client ID + bearer token |
| `POST` | `/v1/ingest` | gzip scan/metadata upload (bearer), returns `202` |
| `GET` | `/v1/meta` | markets, versions, item counts, last scan |
| `GET` | `/v1/snapshot/{market}` | compact market snapshot |
| `GET` | `/v1/snapshot/{market}/data-addon.zip` | full `TradePost_Data` zip |
| `GET` | `/v1/items` | filtered, paged item search |
| `GET` | `/v1/items/missing-details` | item IDs that still need tooltip metadata |
| `GET` | `/v1/item/detail` | metadata + latest price for one item key |
| `GET` | `/v1/item/history` | 12 h 1-minute, 7 d 15-minute, or hourly history |
| `GET` | `/v1/item/markets` | latest price per market for a key |
| `GET` | `/v1/stats` | market index, movers, stock and composition (`market`, optional `hours` 1-168) |
| `GET` | `/v1/live` | live contributor count |
| `POST` | `/v1/auth/code` | one-time website login code (bearer) |
| `POST` | `/v1/auth/redeem` | exchange a code for a session cookie |
| `GET` | `/v1/auth/me` | current website character |
| `POST` | `/v1/auth/logout` | clear the session |
| `GET/PUT` | `/v1/watch` | per-character watchlist |
| `GET` | `/` | website |
| `GET` | `/download/` | downloads page |
| `GET` | `/stats/` | market statistics page |
| `GET` | `/healthz` | liveness |

Market keys are `region:ruleset:house`, e.g. `eu:pve:horde`.
`GET /v1/items` accepts `quality`, `class_id`, `subclass_id`,
`inventory_type`, `min_ilvl`, `max_ilvl`, `min_req_level`, `max_req_level`,
`min_price`, `max_price`, `in_stock=1`, `stats=KEY[:MIN[:MAX]],...`,
`has_stats=1`, `sort`, `dir` (`asc`/`desc`), `page` and `limit` (max 200).
Sort keys are `price`, `name`, `ilvl`, `quality` and `scanned`; the default
direction is ascending for price/name and descending for the others.

`/v1/live` counts distinct clients that requested a login code in the last 10
minutes. It is the website's "live contributors" badge, not a per-addon
heartbeat.

## Ingest and the worker

`POST /v1/ingest` validates the payload and returns immediately:

```json
{"queued": 1, "rejected": 0, "meta": 120}
```

- Scans are gzip JSON, max 10 scans, 100k items per scan, 300k items total.
- A scan is stored in `ingest_jobs` with the client, market, timestamp,
  checksum and payload. Duplicate `(client, checksum)` pairs are rejected.
- A worker claims jobs with `FOR UPDATE SKIP LOCKED` (safe with multiple
  replicas), retries transient failures up to 5 times, and processes one job in
  one transaction:
  - audit row in `scans`;
  - bulk copy into a staging table;
  - upsert `prices_1m`, `prices_15m`, `prices_hourly` and `prices_latest`;
  - bump the market version and the client's `last_seen`.
- Item metadata (`items` array) is upserted synchronously. `details` is the
  rendered tooltip JSON; `stats` is the structured `C_Item.GetItemStats` map.
  `details_version` increments when new details arrive.

`TRADEPOST_WORKER=0` disables the in-process worker if you run it separately.

## History and cleanup

| Range | Resolution | Retention |
| --- | --- | --- |
| `12h` | 1 minute (`prices_1m`) | 12 hours |
| `7d` | 15 minutes (`prices_15m`) | 7 days |
| `all` | 1 hour (`prices_hourly`) | forever |

`prices_latest` keeps the newest price per market/item. A newer `scanned_at`
wins; older scans do not overwrite it. Cleanup runs periodically.

## Item metadata and search

`items` stores name, quality, class, subclass, icon, item level, required level,
bind type, inventory type, equip location, armor, damage, delay, vendor price,
max stack, unique flag, set id, JSON `stats`, JSON `details`, and
`details_version`/`details_updated`.

- `GET /v1/items` joins the latest prices to metadata and supports all filters
  above.
- `GET /v1/item/detail` returns metadata plus latest price/quantity/listings.
- `GET /v1/items/missing-details?market=...&limit=500&force=0` returns item IDs
  that still need details, including rows that only contain the client's
  "Retrieving item information" placeholder; `force=1` returns every known item
  ID in the market. The metadata backfill uses this endpoint.
- The data addon endpoint builds the full `TradePost_Data` zip for one market;
  the companion also writes its own all-market baseline.

## Auth and watchlist

- `POST /v1/auth/code` (bearer) creates a short, one-use code for a character.
- `POST /v1/auth/redeem` exchanges the code for a 30-day HttpOnly session
  cookie; no password.
- `GET/PUT /v1/watch` stores a per-character list of item keys. The companion
  can also use its bearer token with `?character=...`.
- The website session character scopes the watchlist; logging in as another
  character gives that character's list.

## Schema

`clients`, `markets`, `scans`, `items`, `prices_latest`, `prices_1m`,
`prices_15m`, `prices_hourly`, `ingest_jobs`, `login_codes`, `web_sessions`,
`watch_lists`. Required extensions: none beyond Postgres core.

## Tests

```sh
cd server
go test ./...
go vet ./...
gofmt -l .
```

The API tests use an in-memory store and cover ingest/worker processing,
history resolutions, metadata/detail/missing-details, filters/paging/stats,
login, sessions and watchlists.

## Deployment

Production is `https://tradepost.b4b0.de`:

- GitHub Actions builds `ghcr.io/babo96/tradepost-server` (`sha-<short>` tags;
  `develop` on `main`).
- Argo CD app `tradepost` runs the API Deployment, a Postgres 18 StatefulSet,
  a Traefik Ingress and cert-manager. The manifests live in the external
  `homelab-argo` repository under `tradepost/`.
- Local Compose: `server/docker-compose.yml`.

See [../deploy/README.md](../deploy/README.md).
