# Web browser (v2)

The web UI is served by the same Go API at `/`:

```
https://tradepost.b4b0.de/
```

It is embedded in the server binary (`server/internal/api/web`), so there is no
Node build or separate static host.

## Features

- market selector fed by `/v1/meta`
- search by item name, item ID, or exact key (`i:2589`, `g:1234:0`)
- filters: quality (multi-select), class, subclass, inventory type,
  item level, required level, price (gold), in-stock, sort, and structured
  **stats/resistances** (`stat`, `min_stat`, `max_stat`, `has_stats`)
- 50 results per page with Prev/Next and `X–Y of N`
- cheapest-first (or name/ilvl/quality/scanned) results with quality-colored
  names, item level, price, quantity, listings, and scan age
- text-only hover/detail card with the item's full tooltip lines, stats,
  bind type, slot, ilvl, required level, vendor price, max stack and market
  price; no item images or screenshots
- item detail: selectable history range:
  - `12h` 1-minute min/max buckets (`prices_1m`, retained 12 hours)
  - `7d` 15-minute buckets (`prices_15m`, retained 7 days)
  - `All` hourly buckets (`prices_hourly`, retained indefinitely)
- market comparison across alliance/horde/neutral and rulesets
- watchlist stored in `localStorage` when logged out, or per character on the
  server after `/tradepost login`; see [`watch.md`](watch.md)
- money formatting in gold/silver/copper

## Stats filter keys

The client reports `C_Item.GetItemStats` keys verbatim. The web filter exposes
the useful subset:

- primary: `ITEM_MOD_STRENGTH_SHORT`, `ITEM_MOD_AGILITY_SHORT`,
  `ITEM_MOD_STAMINA_SHORT`, `ITEM_MOD_INTELLECT_SHORT`,
  `ITEM_MOD_SPIRIT_SHORT`
- combat: `ITEM_MOD_SPELL_POWER_SHORT`, `ITEM_MOD_SPELL_HEALING_DONE_SHORT`,
  `ITEM_MOD_SPELL_DAMAGE_DONE_SHORT`, `ITEM_MOD_ATTACK_POWER_SHORT`,
  `ITEM_MOD_CRIT_RATING_SHORT`, `ITEM_MOD_HIT_RATING_SHORT`,
  `ITEM_MOD_DEFENSE_SKILL_RATING_SHORT`, `ITEM_MOD_DODGE_RATING_SHORT`,
  `ITEM_MOD_PARRY_RATING_SHORT`, `ITEM_MOD_BLOCK_RATING_SHORT`,
  `ITEM_MOD_EXPERTISE_RATING_SHORT`, `ITEM_MOD_RESILIENCE_RATING_SHORT`,
  `ITEM_MOD_MANA_REGENERATION_SHORT`, `ITEM_MOD_HEALTH_REGEN_SHORT`
- resistances: `RESISTANCE0_NAME` (armor), `RESISTANCE1_NAME` (holy),
  `RESISTANCE2_NAME` (fire), `RESISTANCE3_NAME` (nature),
  `RESISTANCE4_NAME` (frost), `RESISTANCE5_NAME` (shadow),
  `RESISTANCE6_NAME` (arcane)

## Data flow

1. The addon caches `name/quality/class/subclass/icon` per item ID in
   `TRADEPOST_ITEMS` (browser and scanner both populate it).
2. The admin metadata backfill (Settings → Advanced / Admin) collects full
   tooltip lines and `C_Item.GetItemStats` into `TRADEPOST_ITEM_DETAILS`,
   in 500-item batches. It runs at most once per day when enabled, or on
   `/tradepost meta start|force`.
3. `tradepost` parses the details from SavedVariables, hashes each entry, and
   uploads changed metadata with the next ingest. Items the client has not
   cached yet render as `Retrieving item information`; the addon skips those
   and retries them in later passes.
4. The server upserts `items` (including `stats` and `details`) and joins
   prices to names and metadata in search results.

## API

```
GET /v1/items?market=eu:pve:horde&q=linen&limit=50&page=1
GET /v1/items?market=eu:pve:horde&quality=2,3&class_id=4&min_ilvl=30
GET /v1/items?market=eu:pve:horde&stat=ITEM_MOD_STAMINA_SHORT&min_stat=10
GET /v1/items?market=eu:pve:horde&has_stats=1&in_stock=1&sort=price
GET /v1/item/detail?market=eu:pve:horde&key=i:2589
GET /v1/items/missing-details?market=eu:pve:horde&limit=500&force=0
GET /v1/item/history?market=eu:pve:horde&key=i:2589&range=12h
GET /v1/item/markets?key=i:2589
```

`/v1/items` returns `{items,total,page,limit,market,query}`. `stat` takes an
exact `stats` key (`^[A-Za-z0-9_]{1,64}$`); `min_stat`/`max_stat` are optional
numbers and apply only with `stat`; `has_stats=1` requires a non-empty stats
object. Invalid `stat` or a non-numeric range returns HTTP 400.

History returns oldest-to-newest points with min/max/quantity/samples. The
response includes `resolution` (`1m`, `15m`, `hour`) and `range`.

## Ingest additions

```json
{
  "v": 1,
  "scans": [ ... ],
  "items": [
    {"id": 2589, "name": "Linen Cloth", "quality": 1,
     "class_id": 7, "subclass_id": 5, "icon": 132889,
     "stats": {"ITEM_MOD_STAMINA_SHORT": 4},
     "details": {"lines": [{"left": "Linen Cloth", "type": 0}]}}
  ]
}
```

`items` may be sent without scans. The companion tracks per-item hashes in
`state.json` so unchanged metadata is not re-uploaded. Ingest returns `202`
with `{queued,rejected,meta}`; scans are processed asynchronously by the
worker.

## Limits

- Details and stats exist only for items the game client has cached; uncached
  items are deferred and retried on later daily runs.
- The 16 `TradePost_Ack_XX` load-on-demand slots are progress reporting only.
  They are one-shot per UI session; `/reload` resets them. Metadata collection
  no longer waits for an ack, so slot exhaustion cannot stall a run.
- The chart shows min/max buckets, not individual scans or volume.
- Item icons are stored but not rendered; adding a media proxy is a later step.
