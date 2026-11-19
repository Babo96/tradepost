# Web browser (v2)

The web UI is served by the same Go API at `/`:

```
https://tradepost.b4b0.de/
```

It is embedded in the server binary (`server/internal/api/web`), so there is no
Node build or separate static host.

## Features

- market selector fed by `/v1/meta`
- search by item name or item ID over `prices_latest`
- cheapest-first results with quality-colored names, item level, price,
  quantity, listings, and scan age
- item detail: selectable history range:
  - `12h` raw scan points (`prices_raw`, retained 12 hours)
  - `7d` 15-minute min/max buckets (`prices_15m`, retained 7 days)
  - `All` hourly min/max buckets (`prices_hourly`, retained indefinitely)
- market comparison across alliance/horde/neutral and rulesets
- watchlist stored in `localStorage` when logged out, or per character on the
  server after `/tradepost login`; see [`watch.md`](watch.md)
- money formatting in gold/silver/copper

## Data flow

1. The addon caches `name/quality/class/subclass/icon` per item ID in
   `TRADEPOST_ITEMS` (browser and scanner both populate it).
2. `tradepost` parses `TRADEPOST_ITEMS` from SavedVariables, hashes each entry, and
   uploads only new/changed metadata with the next ingest.
3. The server upserts `items` and joins prices to names in search results.
4. Price scans continue to populate `prices_latest` and `prices_hourly`.

## API

```
GET /v1/items?market=eu:pve:horde&q=linen&limit=50
GET /v1/item/history?market=eu:pve:horde&key=i:2589&range=12h
GET /v1/item/history?market=eu:pve:horde&key=i:2589&range=7d
GET /v1/item/history?market=eu:pve:horde&key=i:2589&range=all
GET /v1/item/markets?key=i:2589
```

Search `q` matches a name substring, an exact key (`i:2589`), or an item ID.
History returns oldest-to-newest points with min/max/quantity/samples. The
response includes `resolution` (`raw`, `15m`, `hour`) and `range`. Old raw and
15-minute rows are deleted by the server cleanup loop; hourly rows are kept.

## Ingest additions

```json
{
  "v": 1,
  "scans": [ ... ],
  "items": [
    {"id": 2589, "name": "Linen Cloth", "quality": 1,
     "class_id": 7, "subclass_id": 5, "icon": 132889}
  ]
}
```

`items` may be sent without scans. The companion tracks per-item hashes in
`state.json` so unchanged metadata is not re-uploaded.

## Limits / next

- Watchlist is per browser; server pins and addon sync are not implemented yet.
- The chart shows min/max, not individual scans or volume.
- Item icons are stored but not rendered; adding a media proxy is a later step.
- Full offline name search in the addon still needs metadata in the data addon.
