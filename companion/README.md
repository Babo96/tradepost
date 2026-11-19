# tradepost — TradePost Companion

Go companion for Linux and Windows. No CGO, no runtime dependencies.

## Build

```sh
make companion-build           # dist/tradepostc (host platform)
make companion-build-windows   # dist/tradepostc.exe (windows/amd64)
make release-windows           # dist/tradepost-companion-<version>-windows-amd64.zip
```

The Windows zip contains `tradepost.exe`, `setup.cmd`, and the addon sources in the
`addon/` and `data-addon/` layout expected by `tradepost install`.

## Commands

```sh
tradepost enroll  --server https://tradepost.b4b0.de --market eu:pve:horde --count 64
tradepost install --count 64
tradepost sync
tradepost run [--no-browser]
tradepost status
tradepost version
```

`tradepost run` starts the local dashboard at `http://127.0.0.1:8787`. If the
config is missing or incomplete (no token, no game folder, or an invalid game
folder), it enters first-run setup mode, opens the wizard at `/setup` in the
browser, and keeps the dashboard running instead of exiting. Use `--no-browser`
to suppress the automatic browser launch. Completing the wizard enrolls the
machine, installs the addon and slots, saves the config atomically, and starts
the sync service without a restart.

The dashboard has:

- `/` — status, “Sync now”, login code, raw JSON
- `/setup` — three-step setup wizard (game folder, server/market, install)
- `/settings` — change the game folder, server, market, ring slots, interval,
  and re-run the addon install

The dashboard always binds `127.0.0.1`. POST endpoints require a per-process
token embedded in the served page, so another website cannot trigger setup or
settings changes.

`enroll` and `install` auto-detect the game root and save the resolved path to
the config, so later `sync`/`run` calls do not depend on the working directory.
Use `--wow DIR` or `WOW_DIR` to override detection.

`install` defaults `--source` to the current directory when it contains
`addon/TradePost`, otherwise to the directory containing `tradepost`. A packaged
Windows build therefore works from a shortcut or any working directory.

Config:

- Linux: `~/.config/tradepost/config.json`
- Windows: `%AppData%\tradepost\config.json`

Upload state lives next to it as `state.json` (per server/client ID).

## Windows quick start

1. Extract the release zip to a permanent folder, for example `C:\TradePost`.
2. Close World of Warcraft.
3. Double-click `tradepost.exe` (or run `tradepost.exe run`) and complete the
   setup wizard, or run `setup.cmd` for the command-line flow.
4. Start the game, `/reload`, then hover items and use `/tradepost browse`.
5. Keep `tradepost run` open for uploads and live data; dashboard:
   `http://127.0.0.1:8787`.

If auto-detection fails, pass the install root explicitly:

```bat
tradepost.exe enroll --wow "C:\Program Files (x86)\World of Warcraft" ...
tradepost.exe install --source . --wow "C:\Program Files (x86)\World of Warcraft" --count 64
```

The same `--wow` value is accepted when it points at `_forever_` instead of the
root.

See [`../docs/windows.md`](../docs/windows.md) for the full checklist.

## What `run` does

Every 60 seconds:

1. Reads `WTF/Account/<id>/SavedVariables/TradePost.lua`
2. Parses `TRADEPOST_EXPORT` scans (ASCII records, no Lua interpreter in the addon)
3. Uploads new scans to `/v1/ingest` (gzip, bearer)
4. Fetches `/v1/meta` and each market snapshot
5. Rewrites `Interface/AddOns/TradePost_Data/Data.lua` (base snapshot)
6. Writes the current 5-minute ring slot `TradePost_Data_XX/Data.lua`
7. Updates the dashboard and status

Already-uploaded scans are tracked by SHA-256 in `state.json`, so `/tradepost reload`
does not re-upload them.

## Dashboard

`http://127.0.0.1:8787`

- server, market, snapshot version/items
- last upload/download/sync
- error state
- “Sync now” button
- raw status JSON

## Discovery

Explicit first, then environment, then platform defaults:

1. `--wow DIR`
2. `WOW_DIR`
3. Linux: `/games/World of Warcraft`
4. Windows: `%ProgramFiles(x86)%`, `%ProgramFiles%`, `%LOCALAPPDATA%` +
   `\World of Warcraft`, then `C:\World of Warcraft` and
   `C:\Games\World of Warcraft`

A path ending in `_forever_` is accepted and resolved to its parent.

## Files

Reads:

- `_forever_/WTF/Account/<id>/SavedVariables/TradePost.lua`

Writes:

- `_forever_/Interface/AddOns/TradePost_Data/Data.lua`
- `_forever_/Interface/AddOns/TradePost_Data_XX/Data.lua`

All writes are atomic (temp + rename). The companion never touches Auctionator.
