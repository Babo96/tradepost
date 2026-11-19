# Windows companion

The companion is the same Go program on Linux and Windows. The Windows build is
`tradepost.exe`, cross-compiled with `CGO_ENABLED=0`, and ships with the addon sources
in one zip.

## Release zip

```sh
make release-windows
```

Output: `dist/tradepost-companion-<version>-windows-amd64.zip`

Contents:

```
tradepost.exe
setup.cmd
README.txt
addon/TradePost/...
data-addon/TradePost_Data/...
data-addon/TradePost_LiveData/...
```

## Install

1. Extract to a permanent folder. Avoid `Program Files` unless you want to run
   the companion as administrator; `C:\TradePost` is a good default.
2. Close World of Warcraft.
3. Run `setup.cmd`.

`setup.cmd` runs:

```bat
tradepost.exe enroll --server https://tradepost.b4b0.de --market eu:pve:horde --count 64
tradepost.exe install --source "%~dp0" --count 64
tradepost.exe run
```

`enroll` stores the API token in `%AppData%\tradepost\config.json` and saves
the detected game root. `install` writes the addon and 64 live ring slots.
`run` keeps syncing; the dashboard is at `http://127.0.0.1:8787`.

## WoW path detection

Order:

1. `--wow DIR` (also accepts the `_forever_` directory)
2. `WOW_DIR`
3. `%ProgramFiles(x86)%\World of Warcraft`
4. `%ProgramFiles%\World of Warcraft`
5. `%LOCALAPPDATA%\World of Warcraft`
6. `C:\World of Warcraft`, `C:\Games\World of Warcraft`

If the client is somewhere else, pass it explicitly:

```bat
tradepost.exe enroll --wow "D:\Games\WoW Forever" ...
tradepost.exe install --wow "D:\Games\WoW Forever" --count 64
```

## Everyday use

- Run `tradepost.exe run` while playing (console window can be minimized).
- The addon writes scans when the auction house opens and when `/tradepost export` is
  used; the companion uploads them on the next 5-second SavedVariables check and
  the next sync tick.
- Snapshot and ring updates are written into the addon folders; `/reload` picks
  them up in game.
- Config and upload state are in `%AppData%\tradepost\`.

## Firewall and security

- The dashboard binds to `127.0.0.1:8787` only.
- Outbound access is HTTPS to `tradepost.b4b0.de`.
- The executable is not code-signed; Windows SmartScreen may ask for
  confirmation the first time. Use “More info → Run anyway” for your own build.

## Notes

- The companion never writes Auctionator data.
- `tradepost install` replaces `TradePost`, `TradePost_Data`,
  `TradePost_LiveData`, and the generated ring slots. Other addons are
  untouched.
- If the game is running during install, close it first: Windows may keep files
  open and make the copy incomplete.
