# Companion ↔ game protocol

Current as of TradePost **v0.2.7**. This document describes how the addon and
the local companion exchange data. It is the technical counterpart to the
player-facing [README](../README.md) and [architecture](architecture.md).

## Constraints

The addon runs in the standard WoW Lua sandbox:

- no file, socket, HTTP or clipboard **read** API;
- SavedVariables are read at login and written by the client at logout/reload
  and on a throttled `SaveAddOns()`;
- a LoadOnDemand addon can be loaded **once per UI session**; there is no
  unload API;
- new addon folders are discovered at client startup/reload, not at runtime.

The companion therefore uses four channels:

1. SavedVariables for the game → companion direction;
2. a tiny status addon for the companion → game presence flag;
3. a data addon for the full baseline;
4. LoadOnDemand ring/auth/ack slots for small live updates.

## Game → companion (SavedVariables)

The companion watches `WTF/Account/<account>/SavedVariables/TradePost.lua`
every few seconds and parses it with a Lua interpreter. Writing the file is
throttled by the client; the addon calls `SaveExport()` and retries the flush
so scans, batches and requests eventually reach the companion without
`/reload`.

| Global | Meaning |
| --- | --- |
| `TRADEPOST_EXPORT.scans` | compact scan records: `market|unix|key,price,qty,listings;...` |
| `TRADEPOST_EXPORT.companionRequest` | one-time reload request: "refresh the status flag" |
| `TRADEPOST_EXPORT.ringRequest` | `{ market, at }`: force a full ring snapshot |
| `TRADEPOST_ITEMS` | basic item cache: name, quality, class, subclass, icon |
| `TRADEPOST_ITEM_DETAILS` | full tooltip lines + stats for the metadata backfill |
| `TRADEPOST_META_BATCH` | current 500-item metadata batch |
| `TRADEPOST_AUTH` | login-code request (`seq`, `character`, `market`) |
| `TRADEPOST_WATCH` | local watchlist per character |
| `TRADEPOST_ACTIVE` | active character and market (`character`, `market`, `seen`) |

The companion hashes scans and metadata (`state.json`) and only uploads new or
changed records.

## Companion → game

### Status addon (`TradePost_Companion`)

A tiny regular addon written by the companion:

```lua
TRADEPOST_COMPANION = {
  online = true,
  updated = 1795163048,
  addon_latest = "0.2.7",
  addon_url = "https://github.com/Babo96/tradepost/releases/tag/v0.2.7",
}
```

- `online = true` is written when the companion starts.
- `online = false` is written when it stops.
- It is written once more when the addon's `companionRequest` is seen after a
  login/reload.
- `addon_latest`/`addon_url` are written when the companion's update check
  finds a new addon release. The addon compares them with `TP.version` and
  its TOC `## Interface` with `GetBuildInfo()` and shows a chat/Settings
  notice.

There is **no heartbeat** and no timestamp check. The addon loads the flag once
at login/reload:

```lua
if not self:CompanionReady() then
  TRADEPOST_EXPORT.companionRequest = time()
  -- warn and disable live-only options; one request per reload
end
```

If the companion is offline, nothing answers the request and the next load
still warns. A missing addon (standalone CurseForge/WoWUI install) is treated
the same way.

### Data addon (`TradePost_Data`)

The full baseline for every known market:

```lua
TRADEPOST_DATA = {
  schema = 1,
  markets = {
    ["eu:pve:horde"] = {
      version = 17,
      generated = 1795162407,
      items = {
        { k = "i:2589", p = 125, q = 20, n = 3, t = 1795162000 },
      },
    },
  },
}
```

- Loaded by the game at login/reload.
- Written by the companion on startup and every **30 minutes**.
- It is a baseline, not a live feed; the live ring carries the 5-minute data.
- The addon merges it into its central lookup and uses it before the first ring
  window.

### Live ring (`TradePost_Data_XX`)

The ring is a pool of LoadOnDemand addons. A slot is written by the companion
and read by the addon when the window advances:

```
window = floor(unix_time / 300)
slot   = window mod price_slots         -- price_slots = ring_slots - 8
```

With the default 64 slots there are 56 price slots, i.e. 56 windows
(4 h 40 min) of live updates per UI session. The slider allows up to 128
slots (120 price slots, about 10 h). The 8 remaining slots are reserved for
login-code ring payloads.

Each price slot contains either a full snapshot or a cumulative delta:

```lua
TRADEPOST_RING = {
  schema = 1,
  market = "eu:pve:horde",
  window = 5983775,
  generated = 1795162407,
  full = false,              -- full snapshot when true
  base = 1795132453284,      -- seq of the full snapshot this delta builds on
  seq  = 1795132539956,      -- monotonically increasing write id (ms)
  items = {                  -- cumulative changes since base
    { k = "i:2589", p = 125, q = 20, n = 3, t = 1795162000 },
  },
  removed = { "i:6789" },    -- keys removed since base
  pins = { "i:1251" },       -- watchlist pins
  pins_updated = 1795162400,
}
```

Rules:

- **Full snapshot** (`full = true`): the addon replaces its lookup for that
  market. The companion writes a full on startup, on a market change, every 6
  windows (30 minutes), or when the delta exceeds 4,000 changed items.
- **Delta** (`full = false`): requires `base` to match the addon's current
  full. The addon applies `items` and `removed` and updates `seq`. A missed
  window is harmless because each delta is cumulative since the base.
- If `base` does not match, the addon keeps its old data and writes
  `TRADEPOST_EXPORT.ringRequest`; the companion forces a full on the next sync.
- The companion writes the current and next window slots on each sync. The
  addon loads the target window's slot at the 5-minute boundary.
- The companion selects the market from `TRADEPOST_ACTIVE.market`; its
  configured `market` is only the fallback until a character is detected.
- After the ring wraps, the addon asks for `/reload`; all LoadOnDemand addons
  unload and can be reused.

### Login codes

The companion publishes a fresh code every 8 minutes, or answers an explicit
`/tradepost login` request:

- all 16 `TradePost_Auth_XX` slots receive `TRADEPOST_AUTH_REPLY = { seq, code,
  character, expires }`;
- the 8 reserved ring sync slots receive the same code as `auth = { ... }`;
- the addon first tries a cached, unexpired code for the current character
  (instant), then writes a request and polls if none is available.

The 16 auth slots and 8 sync slots are one-shot per UI session. After many
logins without a restart, `/reload` resets them. The companion also keeps the
code in its dashboard state.

### Metadata acknowledgements

Each `TRADEPOST_META_BATCH` (`seq`, `count`, `items`) is uploaded by the
companion. After a successful ingest the companion writes:

```lua
TRADEPOST_META_ACK = { batch = 17, ok = true }
```

into the 16 `TradePost_Ack_XX` LoadOnDemand slots. The addon consumes an unused
slot to advance. The slots are one-shot; after 16 batches the addon finishes or
waits for `/reload`. Acks are progress reporting only — collection is not
blocked by the ack budget.

## Server ingest

`POST /v1/ingest` is authenticated with the companion bearer token and accepts
gzip JSON:

```json
{
  "v": 1,
  "scans": [{
    "market": "eu:pve:horde",
    "scanned_at": 1795162000,
    "client_version": "0.2.6",
    "items": [{"k": "i:2589", "p": 125, "q": 20, "n": 3}]
  }],
  "items": [{"id": 2589, "name": "Linen Cloth", "quality": 1,
             "class_id": 7, "subclass_id": 5, "icon": 132889,
             "stats": {"ITEM_MOD_STAMINA_SHORT": 4}}]
}
```

- `202 Accepted` with `{queued,rejected,meta}`; scans enter the durable
  `ingest_jobs` queue.
- The worker claims jobs with `FOR UPDATE SKIP LOCKED`, writes the audit scan,
  and aggregates into `prices_1m`, `prices_15m`, `prices_hourly` and
  `prices_latest`.
- Duplicate `(client, checksum)` scans are rejected.
- Optional metadata is upserted synchronously; `details` increments
  `details_version`.

## Limits and recovery

- Client scan throttle: roughly 15 minutes per account for `ReplicateItems`.
- Data addon: startup + 30 minutes; not per sync.
- Live ring: 5-minute windows; `price_slots` per session; reload after a wrap.
- Auth/ack slots: 16 each + 8 ring sync slots per session; reload resets.
- The SavedVariables write throttle is survived by `ScheduleExportRetry`;
  pending data is kept across reloads.
- If the companion is not running, the addon warns and keeps local/central
  data; the live ring, login and metadata are disabled.

## Why not the alternatives

The client probes ruled out `LoadBindings` as a reliable mid-session channel,
runtime addon creation (only discovered at reload), TOC metadata changes
(cached) and addon messages (server relay, 255 bytes, spam policy). The ring is
fixed, pre-created, time-indexed and one-shot. The full rationale and probe
results are in [findings.md](findings.md) and [probe.md](probe.md).
