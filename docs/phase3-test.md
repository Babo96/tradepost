# Phase 3 local test checklist

Local services started on this machine:

| Service | Address |
|---|---|
| Postgres | `localhost:5432` (`babo` / `babo`) |
| API | `http://127.0.0.1:18080` |
| Companion dashboard | `http://127.0.0.1:8787` |
| Companion config | `~/.config/babo-forever/config.json` |

The API uses `18080` locally because `8080` was already occupied. In the
homelab it will use `8080` behind Traefik.

The companion has already uploaded the existing `BABOFOREVER_EXPORT` scan:

```
market:   eu:pve:horde
version:  1
items:    9,758
ring:     BaboForever_Data_59
```

## 1. Restart WoW

The 64 ring slots and the current `BaboForever_Data` were installed after the
client started. Restart WoW completely.

On the character select screen, enable:

- `BaboForever`
- `BaboForever_Data`
- `BaboForever_Data_00` … `BaboForever_Data_63`

## 2. Configure the ring for 64 slots

The addon previously used 8 slots for testing:

```
/bf ring count 64
```

Then:

```
/bf status
/bf ring
```

Expected:

- market `eu:pve:horde`
- central data `base v1`
- ring `slots=64`
- live data may be empty until a ring slot is loaded

## 3. Test central data

To prove the tooltip works from the server snapshot rather than your local
scan, clear the local cache for this market:

```
/bf clear
```

Hover an item that was in the uploaded scan (for example something you scanned
earlier). Expected: a BF/Auctionator price with a `C` marker (central base) or
`R` (ring), and an age around the scan time.

Then scan again:

```
/bf scan
```

Wait for the summary, then `/reload` (or log out) so `BaboForever.lua` is
written.

## 4. Test the companion upload

Watch the dashboard:

```
http://127.0.0.1:8787
```

Within ~5 seconds after `SaveAddOns` (or ~60 seconds for a periodic refresh):

- `last upload` updates
- `accepted` increments by 1
- `version` increments on the server
- `BaboForever_Data/Data.lua` and a new `BaboForever_Data_XX/Data.lua` are
  rewritten

Check:

```sh
curl -sS http://127.0.0.1:18080/v1/meta
```

Expected: `items`/`version` reflect the new snapshot.

The same scan must not upload twice; `state.json` tracks hashes and the server
also deduplicates.

## 5. Test the ring

The companion writes the **current** 5-minute slot. The addon loads the
**previous completed** window at the next boundary.

For an immediate test, read the window from the slot file and force it:

```sh
grep -E 'window|seq' "/games/World of Warcraft/_forever_/Interface/AddOns/BaboForever_Data_XX/Data.lua"
```

```
/bf ring now <window>
```

Then hover an item in that slot. Expected: price updates and the source marker
is `R`.

## 6. Test Auctionator bridge

With the bridge active, Auctionator’s `Auction` line uses our price and the
`Auction Age` line gets a source marker (`R`, `C`, or `L`).

To compare:

```
/bf bridgetest 2318
/bf bridge off
/bf bridgetest 2318
/bf bridge on
```

`current` should follow our data when the bridge is on and revert to `original`
when it is off.

## What to report

- `/bf status`, `/bf ring`
- dashboard JSON (`curl -sS http://127.0.0.1:8787/api/status`)
- server meta (`curl -sS http://127.0.0.1:18080/v1/meta`)
- any `/bf lasterror` output
- whether the base and ring prices appear after `/bf clear` and `/bf ring now`

## Stop the local stack

```sh
kill "$(cat /tmp/opencode/bfc-local.pid)" 2>/dev/null || true
kill "$(cat /tmp/opencode/bfd-local.pid)" 2>/dev/null || true
make db-down
```
