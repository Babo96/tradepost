# bfc — Babo Forever Companion

Go companion for Linux and Windows. No CGO, no runtime dependencies.

## Build

```sh
make companion-build           # dist/bfc (host platform)
make companion-build-windows   # dist/bfc.exe (windows/amd64)
make release-windows           # dist/babo-forever-companion-<version>-windows-amd64.zip
```

The Windows zip contains `bfc.exe`, `setup.cmd`, and the addon sources in the
`addon/` and `data-addon/` layout expected by `bfc install`.

## Commands

```sh
bfc enroll  --server https://babo-forever.b4b0.de --market eu:pve:horde --count 64
bfc install --count 64
bfc sync
bfc run
bfc status
bfc version
```

`enroll` and `install` auto-detect the game root and save the resolved path to
the config, so later `sync`/`run` calls do not depend on the working directory.
Use `--wow DIR` or `WOW_DIR` to override detection.

`install` defaults `--source` to the current directory when it contains
`addon/BaboForever`, otherwise to the directory containing `bfc`. A packaged
Windows build therefore works from a shortcut or any working directory.

Config:

- Linux: `~/.config/babo-forever/config.json`
- Windows: `%AppData%\babo-forever\config.json`

Upload state lives next to it as `state.json` (per server/client ID).

## Windows quick start

1. Extract the release zip to a permanent folder, for example `C:\BaboForever`.
2. Close World of Warcraft.
3. Run `setup.cmd` (or the commands above in a terminal).
4. Start the game, `/reload`, then hover items and use `/bf browse`.
5. Keep `bfc run` open for uploads and live data; dashboard:
   `http://127.0.0.1:8787`.

If auto-detection fails, pass the install root explicitly:

```bat
bfc.exe enroll --wow "C:\Program Files (x86)\World of Warcraft" ...
bfc.exe install --source . --wow "C:\Program Files (x86)\World of Warcraft" --count 64
```

The same `--wow` value is accepted when it points at `_forever_` instead of the
root.

See [`../docs/windows.md`](../docs/windows.md) for the full checklist.

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

- `_forever_/WTF/Account/<id>/SavedVariables/BaboForever.lua`

Writes:

- `_forever_/Interface/AddOns/BaboForever_Data/Data.lua`
- `_forever_/Interface/AddOns/BaboForever_Data_XX/Data.lua`

All writes are atomic (temp + rename). The companion never touches Auctionator.
