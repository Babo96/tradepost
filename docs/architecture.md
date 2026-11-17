# Architecture

## Goal

Give every player Auctionator-style price tooltips that are correct for the
market they are standing in, and make those prices fresh through crowd-sourced
scans instead of every player scanning alone.

## Markets

Forever has no realms. The auction house is identified by:

```
region (EU for launch) × ruleset (suffix of GetRealmName, e.g. "PvE") × house
```

Houses:

- `alliance` — faction capitals
- `horde` — faction capitals
- `neutral` — Blackwater houses (Gadgetzan, Booty Bay, Everlook)

The addon computes a market key such as `eu:pve:horde` and uses it for both
local data and the central snapshot.

Phase 0 showed that `GetRealmName()` is the full string
`CAMWOW EU Realm 3 PvE`, not just `PvE`. The ruleset is the suffix
(`PvE`, `PvP`, `RP`, `RP-PvP`, later `Hardcore`); the realm number is not part
of the market key because the auction house is shared per ruleset.

## Components

```
WoW client (_forever_)
  BaboForever (Lua)
    scanner        C_AuctionHouse.ReplicateItems -> per-item min/qty
    local DB       own scans, instant tooltips
    central DB     loaded from BaboForever_Data
    tooltips       TooltipDataProcessor + fallbacks
    live mailbox   LoadBindings deltas (phase 0 verifies)
    export buffer  SavedVariables + optional bindings uplink

  BaboForever_Data (LoadOnDemand where possible)
    full snapshot at login

bfc companion (Go, Linux first)
  reads SavedVariables, uploads scans
  downloads snapshots, writes BaboForever_Data
  writes mailbox deltas while the game runs
  local web dashboard on 127.0.0.1:8787

server (Go + Postgres 17)
  POST /v1/ingest
  GET  /v1/meta
  GET  /v1/snapshot/{market}
  snapshot builder + status page
```

## Data flow

1. A player opens the auction house. The addon harvests an existing replicate
   if another addon already ran one, otherwise calls `ReplicateItems` (15 min
   account-wide throttle).
2. The scan is aggregated per item key (`i:<id>` or `g:<id>:<ilvl>`) and stored
   locally. Tooltips use it immediately.
3. The scan is written to the export buffer. The companion uploads it to
   `/v1/ingest` and the server merges it into the market.
4. The snapshot builder publishes a compact per-market snapshot. The companion
   downloads it, writes `BaboForever_Data/Data.lua`, and (if the live channel
   works) feeds deltas into the bindings mailbox.
5. The addon applies the delta without `/reload`; tooltips and the Auctionator
   display bridge update on the next hover.

## Freshness

| Source | Age |
|---|---|
| Targeted AH search while the AH is open | seconds |
| Search/browse results captured from the AH UI | seconds |
| Own full scan | ≤15 min (client throttle) |
| Central snapshot via mailbox delta | ~1–2 min |
| Central snapshot at login/`/reload` | 2–15 min |

The addon cannot make HTTP requests and the companion cannot push data through
a socket. Phase 0 rejected the `LoadBindings` mailbox and confirmed a
**LoadOnDemand ring**: a slot written on disk while the game runs is loaded
with its current content when first loaded. The ring is pre-created, fixed-size
and time-indexed (64 slots, 5-minute windows, cumulative deltas). New addon
folders are not discovered at runtime, and TOC metadata is cached, so the ring
must be installed with the addon. True real-time prices still come from
targeted AH searches while the AH is open.

## Auctionator

Auctionator stays the display for the auction house UI. The addon installs a
runtime read bridge around `Auctionator.Database.GetPrice`, `GetFirstPrice`,
`GetMeanPrice` and `GetPriceAge`, maps keys, and returns Babo Forever data when
it is fresher. Auctionator's database and SavedVariables are never modified.

## Safety

- Public write API uses per-companion bearer tokens, rate limits and dedupe.
- Price outliers need independent confirmation before they become "latest".
- No character names or account IDs are required. The client ID is random.
- All addon data is free; no automation, no monetization.
