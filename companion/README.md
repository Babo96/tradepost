# bfc — Babo Forever Companion

Linux-first Go companion. No CGO. Windows is a later port.

## Build

```sh
make companion-build      # dist/bfc
```

## Commands

```sh
bfc enroll  --server https://babo-forever.b4b0.de --wow "/games/World of Warcraft"
bfc install --source /path/to/babo-forever --wow "/games/World of Warcraft" --count 64
bfc sync
bfc run
bfc status
```

Config: `~/.config/babo-forever/config.json` (0600)
Upload state: `~/.config/babo-forever/state.json`

## What `run` does

Every 60 seconds:

1. Reads `WTF/Account/<id>/SavedVariables/BaboForever.lua`
2. Parses `BABOFOREVER_EXPORT` scans (ASCII records, no Lua interpreter in the addon)
3. Uploads new scans to `/v1/ingest` (gzip, bearer)
4. Fetches `/v1/meta` and each market snapshot
5. Rewrites `Interface/AddOns/BaboForever_Data/Data.lua` (base snapshot)
6. Writes the current 5-minute ring slot `BaboForever_Data_XX/Data.lua`
7. Updates the dashboard and status

Already-uploaded scans are tracked by SHA-256 in `state.json`, so `/bf reload`
does not re-upload them.

## Dashboard

`http://127.0.0.1:8787`

- server, market, snapshot version/items
- last upload/download/sync
- error state
- “Sync now” button
- raw status JSON

## Discovery

Linux:

- `--wow` / `WOW_DIR`
- `/games/World of Warcraft`
- account directories under `_forever_/WTF/Account`

Windows (later):

- registry `HKLM\SOFTWARE\WOW6432Node\Blizzard Entertainment\World of Warcraft`
- `%ProgramFiles(x86)%\World of Warcraft`

## Files

Reads:

- `_forever_/WTF/Account/<id>/SavedVariables/BaboForever.lua`

Writes:

- `_forever_/Interface/AddOns/BaboForever_Data/Data.lua`
- `_forever_/Interface/AddOns/BaboForever_Data_XX/Data.lua`

All writes are atomic (temp + rename). The companion never touches Auctionator.
