# Architecture

TradePost is a crowd-sourced price service for **World of Warcraft: Forever**.
Every player contributes the auction house scans their client can already
produce; the server pools them per market and returns the result to every
client and to the website. The addon alone only has local/static data — the
companion is the bridge.

## Components

| Component | What it does |
| --- | --- |
| **Addon** (`addon/TradePost`) | Scans the auction house, keeps a local per-market database, renders tooltips, the in-game browser, watchlist, Auctionator bridge and settings. Applies the live ring deltas and the central data addon. |
| **Data addon** (`TradePost_Data`) | Full price baseline for every known market. Written by the companion on startup and every 30 minutes; loaded by the game at login/reload. |
| **Ring slots** (`TradePost_Data_XX`) | Load-on-demand addons that carry the 5-minute live price deltas for the active character's market. |
| **Status addon** (`TradePost_Companion`) | A tiny addon with `TRADEPOST_COMPANION.online`, `addon_latest` and `addon_url`. It is written on companion start/stop, on a reload request, and when the update check finds a new addon release. No heartbeat. |
| **Companion** (`companion/`) | Go service on the player's machine. Setup wizard, local dashboard, SavedVariables parsing, uploads, snapshot/data-addon/ring writing, login codes, watchlist, metadata backfill and self-update. |
| **Server** (`server/`) | Go + Postgres API. Async ingest queue, price aggregation, item metadata, search/filters, history, auth, watchlist, snapshots and the embedded website. |
| **Website** (`https://tradepost.b4b0.de/`) | The same API's web UI: search, filters, stats, history, hover cards, market selector, watchlist and live contributor count. |

The companion and the server are MIT-licensed. The addon has no external Lua
libraries.

## The crowd loop

```
scan (addon) ──► SavedVariables ──► companion ──HTTPS 202──► server queue
                                                              │
                                                              ▼
                                                    worker: aggregate
                                                              │
          addon ◄── ring deltas / data addon ◄── companion ◄── snapshots
                                                              │
                                                              ▼
                                                           website
```

1. **Scan.** The addon harvests the client's auction house data
   (`C_AuctionHouse.ReplicateItems`) when the AH opens or when the user asks.
   The client applies an account-wide ~15-minute throttle to full scans.
2. **Local database.** Results are aggregated per item key (`i:<id>` or
   `g:<id>:<ilvl>`) and per market. Tooltips use the local data immediately.
3. **Upload.** A completed scan is written to `TRADEPOST_EXPORT`. The companion
   reads the SavedVariables file, hashes each scan, and `POST`s it to
   `/v1/ingest` with the client token. Item metadata (names, quality, icons,
   tooltips, stats) travels with the same request.
4. **Aggregate.** The API returns `202 Accepted`, stores the scan in
   `ingest_jobs`, and a worker claims jobs with `FOR UPDATE SKIP LOCKED`. The
   worker writes 1-minute, 15-minute and hourly aggregates plus the latest row.
5. **Distribute.** The companion downloads market snapshots and writes:
   - the **data addon** baseline (all markets, on startup/30 min),
   - the **ring deltas** for the active character's market (5-minute windows),
   - the **status addon** online flag (only on lifecycle changes),
   - login-code and watchlist payloads.
6. **Consume.** The addon applies the data at login/reload and the ring deltas
   while playing. The website reads the same API. Prices improve for everyone
   as more scans arrive.

## Markets

The auction house is split by:

```
region × ruleset × house
```

- **region** — `eu`, `us`, `kr`, `tw`, `cn` (EU at launch).
- **ruleset** — the suffix of `GetRealmName()`: `PvE`, `PvP`, `RP`, `RP-PvP`,
  `Hardcore`. The realm number is **not** part of the key because the auction
  house is shared per ruleset.
- **house** — `alliance`, `horde`, or `neutral` (Blackwater auction houses:
  Gadgetzan, Booty Bay, Everlook).

Examples: `eu:pve:horde`, `eu:pvp:alliance`, `eu:pve:neutral`.

The addon computes the active market from the current character. The companion
follows the active character's market for the live ring; its configured
`market` is only a fallback until a character is detected. The website has a
market selector and keeps every market.

## Storage and history

| Table / range | Resolution | Retention |
| --- | --- | --- |
| `prices_latest` | newest scan per market/item | forever |
| `prices_1m` | 1-minute min/max/quantity/samples | 12 hours |
| `prices_15m` | 15-minute buckets | 7 days |
| `prices_hourly` | hourly buckets | forever |

A cleanup loop removes expired buckets. A newer `scanned_at` replaces
`prices_latest`; older scans do not overwrite a newer price. Duplicate
`(client, checksum)` scans are rejected.

`items` holds optional metadata: name, quality, class, subclass, icon, item
level, required level, bind type, inventory type, equip location, armor, damage,
delay, vendor price, max stack, unique flag, set id, JSON `stats`, and JSON
`details` (the rendered tooltip lines). `details_version` and `details_updated`
track the metadata backfill.

The client-side price DB and item cache are stored as compact per-entry strings
(`prices[key] = "p,q,n,t"`, `items[id] = "name\ticon\tclass\tsubclass\tquality"`).
Legacy table entries are migrated on load; the tables keep their keys, so
lookups and iteration remain the same.

## Companion ↔ game transport

The game sandbox offers no file, socket or HTTP API to Lua. The game does write
and read SavedVariables and can load a pre-created LoadOnDemand addon at most
once per UI session. TradePost uses those two facts:

- **Outbound (game → companion):** SavedVariables (`TRADEPOST_EXPORT`,
  `TRADEPOST_ITEMS`, `TRADEPOST_ITEM_DETAILS`, `TRADEPOST_META_BATCH`,
  `TRADEPOST_AUTH`, `TRADEPOST_WATCH`, `TRADEPOST_ACTIVE` and the
  `companionRequest` flag). The client throttles `SaveAddOns`; the addon
  retries the flush.
- **Inbound (companion → game):** a full **data addon** at login, small
  **live ring deltas**, the tiny **status addon**, and one-shot
  auth/metadata-ack slot addons.

The live ring is cumulative: each slot carries the changes since a full base,
so a missed window does not lose data. A new full is written on companion
startup, every 6 windows (30 minutes), on a market change, or when more than
4,000 items changed. Full snapshots are ~700 KB per market; normal deltas are a
few hundred bytes to a few tens of kilobytes. The default is 64 slots: 56 price windows (about 4 h 40 min) plus 8
reserved for login codes; the slider allows up to 128.

The **status addon** is the presence check, not a heartbeat. The companion
writes `online = true` when it starts, `online = false` when it stops, once
more when the addon writes `TRADEPOST_EXPORT.companionRequest` after a reload,
and when its update check finds a new addon release (`addon_latest` +
`addon_url`). The addon reads the flags once at login/reload. It compares
`TP.version` to `addon_latest` and its TOC `## Interface` to
`GetBuildInfo()`, and shows a chat/Settings update notice. Without the
companion it shows a warning and disables live-only options; local scans,
central data, the browser, watchlist and Auctionator bridge still work.

The data addon is a login-time baseline and is refreshed on startup, every 30
minutes. The 5-minute live data rides the ring.

See [protocol.md](protocol.md) for the payloads and slot rules.

## Metadata backfill

The optional Advanced/Admin backfill is off by default; normal installs do not
carry item details. When enabled, it collects full tooltip text and
`C_Item.GetItemStats` for every known item in 500-item batches. It runs at most
once per day when enabled (manual start/force overrides), skips the client's
"Retrieving item information" placeholder, uses item links captured by
TradePost's own auction-house scan (`GetReplicateItemLink`), requests uncached
item data by ID, and retries equipment whose stats are not cached yet. The
Auctionator bridge is display-only and is not part of metadata collection. The
companion uploads changed details and acknowledges batches through the 16
`TradePost_Ack_XX` slots. The website renders the text lines and stats in the
hover card; `GET /v1/items/missing-details` drives the next pass.

## Website and API

The website is embedded in the server binary; there is no separate frontend
build.

- Search by name, item ID or exact key; filters for quality, class, subclass,
  inventory type, item level, required level, price, stock, and structured
  stats/resistances; page size 50; sort by price/name/ilvl/quality/scanned in
  either direction.
- Text-only hover cards with the full tooltip, stats, bind, slot, vendor price,
  max stack and current price. No item images are rendered.
- History: 12 h at 1-minute, 7 days at 15-minute, all time hourly.
- Market comparison, per-character watchlist, one-time login code, and the live
  contributor count.
- Public API under `/v1/`; see [web.md](web.md) and [server/README.md](../server/README.md).

## Multi-character support

- Scans, local data, the central data addon, watchlists and login codes are
  per character and market.
- The addon publishes the active character's market in
  `TRADEPOST_ACTIVE.market`; the companion selects that market's snapshot for
  the live ring. Switching characters switches the live market after the next
  SavedVariables flush (normally under a minute, at most a few minutes).
- The website keeps all markets and lets you switch; the watchlist is per
  character.

## Security and privacy

- The companion receives a random client ID and bearer token at enrollment.
  Ingest is authenticated, rate-limited and deduplicated by scan checksum.
- A separate `/v1/auth/code` one-time code links a character to a website
  session for the watchlist; the session is an HttpOnly cookie. No password is
  ever used.
- Uploads contain auction prices, quantities, listings, timestamps and item
  metadata. They do not contain character names, account names or Battle.net
  identifiers. The character name is stored only if you log into the website.
- The dashboard binds to `127.0.0.1` and is not exposed to the network.
- No automation, no Auctionator database writes, no monetization.

## Deployment

The production service is `https://tradepost.b4b0.de`:

- Image: `ghcr.io/babo96/tradepost-server` (GitHub Actions, `sha-<short>` tags,
  `develop` on `main`).
- Runtime: Argo CD app `tradepost` with a Postgres 18 StatefulSet, an API
  Deployment, a Traefik Ingress and cert-manager.
- Manifests live in the external `homelab-argo` repository under `tradepost/`.

See [deploy/README.md](../deploy/README.md) and
[server/README.md](../server/README.md) for local development and self-hosting.
