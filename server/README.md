# server — Babo Forever API

Phase 3. Go + Postgres 17.

## Endpoints

```
POST /v1/enroll                      client id + bearer token
POST /v1/ingest                      batched scans (gzip JSON)
GET  /v1/meta                        markets, versions, freshness
GET  /v1/snapshot/{market}           immutable gzip JSON
GET  /v1/snapshot/{market}/data-addon.zip
GET  /v1/stats                       public status page data
GET  /healthz  /metrics
```

## Data

- `markets`, `clients`, `scans`
- `prices_latest` (upsert, conditional on scan time)
- `prices_hourly` (partitioned, 7 days)
- `prices_daily` (365 days)
- `snapshots` (immutable, sha256, bytes)
- later: `items`, `pins`

## Scope

EU only at launch. Market key: `eu:<ruleset>:<house>`, e.g. `eu:pve:horde`.

## Deployment

`deploy/k8s` (ArgoCD-compatible) and `deploy/docker-compose.yml` for local
development. Hostname: `babo-forever.b4b0.de`.
