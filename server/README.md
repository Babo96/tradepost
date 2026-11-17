# server — Babo Forever API

Go + Postgres. Aggregates scans and serves per-market snapshots.

## Local

```sh
make db-up          # Postgres 18 on localhost:5432 (babo/babo)
make server-run     # API on :8080
make server-test
```

Environment:

```
BFD_DATABASE_URL  postgres://babo:babo@localhost:5432/babo?sslmode=disable
BFD_ADDR          :8080
```

## Endpoints

```
POST /v1/enroll                      -> client_id + bearer token
POST /v1/ingest                      -> gzip JSON scans (bearer)
GET  /v1/meta                        -> markets, versions, counts, freshness
GET  /v1/snapshot/{market}           -> gzip JSON snapshot + ETag
GET  /v1/snapshot/{market}/data-addon.zip
GET  /v1/items?market=..&q=..        -> item search with latest prices
GET  /v1/item/history?market=..&key= -> hourly min/max history
GET  /v1/item/markets?key=..         -> latest price per market
POST /v1/auth/code                   -> one-time web login code (bearer)
POST /v1/auth/redeem                 -> exchange code for a session cookie
GET  /v1/auth/me                     -> current website character
POST /v1/auth/logout
GET/PUT /v1/watch                    -> per-character watchlist
GET  /                                -> web UI (search, history, watchlist)
GET  /healthz
```

The web UI is embedded in the binary; no separate frontend build.

Market key: `eu:<ruleset>:<house>`, e.g. `eu:pve:horde`.

## Ingest payload

```json
{
  "v": 1,
  "scans": [{
    "market": "eu:pve:horde",
    "scanned_at": 1794947776,
    "client_version": "0.1.0",
    "items": [
      {"k": "i:2589", "p": 1234, "q": 50, "n": 2},
      {"k": "g:12345:70", "p": 4200, "q": 1, "n": 1}
    ]
  }],
  "items": [
    {"id": 2589, "name": "Linen Cloth", "quality": 1,
     "class_id": 7, "subclass_id": 5, "icon": 132889}
  ]
}
```

`items` is optional and can be sent without scans. It is keyed by item ID and
uses the client item cache; metadata is uploaded only when it changes.

- Bearer token from `/v1/enroll`
- `Content-Encoding: gzip` supported
- max 10 scans, 100k items per scan, 300k total
- duplicate `(client, checksum)` rejected
- newer `scanned_at` replaces `prices_latest`; older scans do not
- hourly min/max/quantity aggregates are kept for history

## Schema

`clients`, `markets`, `scans`, `prices_latest`, `prices_hourly`, `items`.
Migrations are embedded and run at startup.

History is kept as hourly min/max buckets per market/key, which feeds the web
chart. The web watchlist is stored in browser `localStorage`; server-side pins
shared with the addon are a later step.

## Deployment

`deploy/k8s` will contain the ArgoCD manifests. Hostname:
`babo-forever.b4b0.de`.
