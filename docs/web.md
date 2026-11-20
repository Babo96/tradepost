# Website and API

The website is served by the same Go API at:

```
https://tradepost.b4b0.de/
```

It is embedded in the server binary (`server/internal/api/web`), so there is no
separate frontend build or static host.

The website reads the same crowd-sourced data as the in-game addon. A player's
companion uploads scans; the server aggregates them; the site shows the shared
prices, history and metadata. The website does not need the companion itself.

## Downloads

<https://tradepost.b4b0.de/download/> links to the latest GitHub release assets:

- Windows companion (`tradepost-companion-windows-amd64.zip`),
- Linux companion (`tradepost-linux-amd64.tar.gz`),
- addon only (`TradePost-addon.zip`).

The links use stable release aliases, so they always point at the newest release.
The companion installs the addon and data pack; the addon-only zip is for
CurseForge/WoWUI or manual installs.

## What you can do

- Pick a market (for example `eu:pve:horde`, `eu:pvp:alliance`).
- Search by item name, item ID or exact key (`i:2589`, `g:12345:70`).
- Filter by quality, class, subclass, inventory type, item level, required
  level, price, stock, and structured stats/resistances.
- Page through 50 results at a time and sort by price, name, item level,
  quality or scan time.
- Hover or click a row for a text-only item card with the full tooltip lines,
  stats, bind, slot, item level, required level, vendor price, max stack and
  current price. No item images are rendered.
- View 12-hour (1-minute), 7-day (15-minute) and all-time (hourly) history.
- Compare the latest price across markets.
- Keep a per-character watchlist, optionally logged in with a one-time code.
- See the live contributor count in the header.
- Download the addon or a companion from the [downloads page](https://tradepost.b4b0.de/download/).

The filter panel collapses on narrow screens; the pager and hover card work on
mobile.

## Stats and resistances

The filter sends the client's structured `C_Item.GetItemStats` keys. The site
accepts a comma-separated list with AND semantics:

```
stats=KEY[:MIN[:MAX]],KEY2[:MIN[:MAX]]
```

Examples:

```
stats=ITEM_MOD_STAMINA_SHORT:10
stats=ITEM_MOD_STAMINA_SHORT:10,ITEM_MOD_INTELLECT_SHORT:5
stats=RESISTANCE2_NAME::20
```

Keys exposed in the UI:

- **Primary:** `ITEM_MOD_STRENGTH_SHORT`, `ITEM_MOD_AGILITY_SHORT`,
  `ITEM_MOD_STAMINA_SHORT`, `ITEM_MOD_INTELLECT_SHORT`,
  `ITEM_MOD_SPIRIT_SHORT`
- **Combat:** `ITEM_MOD_SPELL_POWER_SHORT`, `ITEM_MOD_SPELL_HEALING_DONE_SHORT`,
  `ITEM_MOD_SPELL_DAMAGE_DONE_SHORT`, `ITEM_MOD_ATTACK_POWER_SHORT`,
  `ITEM_MOD_CRIT_RATING_SHORT`, `ITEM_MOD_HIT_RATING_SHORT`,
  `ITEM_MOD_DEFENSE_SKILL_RATING_SHORT`, `ITEM_MOD_DODGE_RATING_SHORT`,
  `ITEM_MOD_PARRY_RATING_SHORT`, `ITEM_MOD_BLOCK_RATING_SHORT`,
  `ITEM_MOD_EXPERTISE_RATING_SHORT`, `ITEM_MOD_RESILIENCE_RATING_SHORT`,
  `ITEM_MOD_MANA_REGENERATION_SHORT`, `ITEM_MOD_HEALTH_REGEN_SHORT`
- **Resistances:** `RESISTANCE0_NAME` (armor), `RESISTANCE1_NAME` (holy),
  `RESISTANCE2_NAME` (fire), `RESISTANCE3_NAME` (nature),
  `RESISTANCE4_NAME` (frost), `RESISTANCE5_NAME` (shadow),
  `RESISTANCE6_NAME` (arcane)

`has_stats=1` requires a non-empty stats object.

## Data flow

1. The addon scans the auction house and the companion uploads the scan and
   item metadata.
2. The server queues the scan, aggregates 1-minute/15-minute/hourly buckets and
   stores the latest price.
3. The website reads `/v1/items`, `/v1/item/detail`, history and market
   endpoints directly. There is no second database for the site.
4. The companion writes the central `TradePost_Data` baseline (startup and
   every 30 minutes) and the 5-minute live ring deltas for the game. The
   website is independent of those local files.
5. The metadata backfill adds full tooltip text and stats for items the game
   client has cached. `GET /v1/items/missing-details` drives the next pass.

## API

```
GET /v1/meta
GET /v1/items?market=eu:pve:horde&q=linen&limit=50&page=1
GET /v1/items?market=eu:pve:horde&quality=2,3&class_id=4&min_ilvl=30
GET /v1/items?market=eu:pve:horde&stats=ITEM_MOD_STAMINA_SHORT:10,ITEM_MOD_INTELLECT_SHORT:5
GET /v1/items?market=eu:pve:horde&has_stats=1&in_stock=1&sort=price
GET /v1/item/detail?market=eu:pve:horde&key=i:2589
GET /v1/items/missing-details?market=eu:pve:horde&limit=500&force=0
GET /v1/item/history?market=eu:pve:horde&key=i:2589&range=12h
GET /v1/item/markets?key=i:2589
GET /v1/live
POST /v1/auth/code
POST /v1/auth/redeem
GET /v1/auth/me
POST /v1/auth/logout
GET /v1/watch
PUT /v1/watch
```

`/v1/items` returns `{items,total,page,limit,market,query}`. Each item includes
the key, item ID, name, quality, latest price/quantity/listings/scan time, the
optional metadata fields, and `has_details`.

Filters:

| Parameter | Meaning |
| --- | --- |
| `q` | name substring, exact key, or item ID |
| `quality` | comma-separated quality IDs |
| `class_id`, `subclass_id`, `inventory_type` | numeric filters |
| `min_ilvl`, `max_ilvl`, `min_req_level`, `max_req_level` | numeric ranges |
| `min_price`, `max_price` | copper |
| `in_stock=1` | quantity > 0 |
| `stats` | comma-separated `KEY[:MIN[:MAX]]`, ANDed |
| `has_stats=1` | non-empty stats object |
| `sort` | `price`, `name`, `ilvl`, `quality`, `scanned` |
| `page`, `limit` | 1-based page; limit max 200 |

`/v1/item/detail` returns the item metadata (`stats`, `details`,
`details_version`) plus the latest price/quantity/listings/scanned.

`/v1/items/missing-details` returns `{items:[ids],count}`; `limit` is capped at
2,000 and `force=1` returns every known item in the market.

`/v1/item/history` returns oldest-to-newest points with min/max/quantity/samples
and a `resolution` of `1m`, `15m` or `hour`.

`/v1/live` returns `{live_clients,window_seconds}`. The count is distinct
clients that requested a login code in the last 10 minutes; the companion
publishes a fresh code every 8 minutes.

The auth/watch endpoints are described in
[watchlist sync](watch.md). The server needs a companion bearer token only for
ingest and code creation; the read API is public.

## Limits

- Details and stats exist only for items the game client has cached. The
  backfill skips the "Retrieving item information" placeholder and retries
  uncached items on later runs.
- The companion writes the data addon on startup and every 30 minutes; the live
  ring carries the 5-minute updates. A new scan can take a few minutes to reach
  the in-game ring.
- The 16 metadata ack slots are progress reporting only; collection is not
  blocked by the slot budget. `/reload` resets the slots.
- A static data pack from
  `/v1/snapshot/<market>/data-addon.zip` gives static prices but no live
  updates and does not satisfy the companion check.
- The website session is per character; log in with another code to switch the
  watchlist.

See the [server README](../server/README.md) for the full API, worker and
schema details.
