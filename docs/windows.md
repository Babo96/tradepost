# Windows companion guide

The Windows companion is a single `tradepost.exe` plus the addon sources and a
setup wizard. It installs the addon, the data addon, the status addon and the
ring/auth slots into your World of Warcraft: Forever installation, then keeps
syncing while you play.

Download the latest **`tradepost-companion-<version>-windows-amd64.zip`** from
the [downloads page](https://tradepost.b4b0.de/download/) or
<https://github.com/Babo96/tradepost/releases/latest>.

## What is in the zip

```
tradepost.exe
setup.cmd
README.txt
addon/TradePost/...
data-addon/TradePost_Data/...
data-addon/TradePost_LiveData/...
```

The addon sources are included so the companion can install them without a
separate download.

## First install

1. **Extract** the zip somewhere permanent, for example `C:\TradePost`.
   Avoid `Program Files` unless you want to run it as administrator.
2. **Close World of Warcraft.** The installer copies files into the game
   folder; Windows can keep them open if the game is running.
3. **Run `tradepost.exe`** (or `setup.cmd`). It starts in the Windows
   notification area without a terminal window. Left-click the tray icon to
   open the dashboard; the setup wizard is at `http://127.0.0.1:8787/setup`.
   If the browser does not open, visit that URL manually. `setup.cmd` is a
   batch file, so Windows may briefly flash a console window for it;
   `tradepost.exe` itself never opens one.
4. **Game folder.** Select the folder that contains `_forever_`, for example
   `C:\World of Warcraft`. The wizard validates:
   - `_forever_/Interface/AddOns`,
   - the account folders under `_forever_/WTF/Account`,
   - write access to the AddOns folder.
5. **Server and market.** Keep `https://tradepost.b4b0.de`. The default market
   `eu:pve:horde` is only a fallback until a character is detected; the active
   character's region, ruleset and faction choose the live market
   automatically. Ring slots default to 64 (up to 128 for a longer session).
6. **Install.** The companion enrolls this machine, writes the addon, the data
   addon, the status addon and the ring/auth slots, saves the config, and starts
   syncing.
7. **Start WoW.** Restart the client once after the first install so the new
   addon folders are discovered. Open `/tradepost` or the minimap TradePost button,
   then open the auction house for the first scan.

## Everyday use

- Run `tradepost.exe` while playing. It stays in the notification area; no
  terminal window is shown. Running it with no arguments is the same as
  `tradepost.exe run`.
- **Left-click** the tray icon to open the dashboard. **Right-click** for
  **Open dashboard**, **Sync now** and **Quit**.
- **Quit** removes the tray icon and stops the companion; scans are uploaded
  again the next time it starts.
- Only one companion instance can run. Launching `tradepost.exe` again opens
  the existing dashboard and exits, so there is only one tray icon.
- The dashboard is `http://127.0.0.1:8787`:
  - last sync, active market, item count, errors;
  - **Sync now**, **login code**, the update card;
  - the raw status JSON.
- Scans are uploaded when the addon writes them and the companion sees the
  SavedVariables change (the client throttles writes; the addon retries).
- The data addon is refreshed on companion startup and every 30 minutes. Live
  prices arrive in the 5-minute ring windows.
- The tiny `TradePost_Companion` status addon carries the `online` flag. There
  is no heartbeat. If the companion is not running, the addon warns once and
  disables live-only options.
- The companion does not install a Windows service. Start it when you want to
  play, or place a shortcut in your startup folder.

## Updates

The companion checks GitHub once a day and shows an **update available** card
on the dashboard. Use **Install update** or run:

```bat
tradepost.exe update
```

The binary is replaced; the running process keeps using the old code until you
restart it. The addon files are refreshed too, so `/reload` in game (or a
client restart if new addon folders were created). `tradepost.exe update
--check` prints the current and latest versions without changing anything.
The addon also shows a chat and Settings notice when its `## Interface` is
older than the client, or when the companion reports a newer addon release.
CurseForge/WoWUI managers can update the standalone addon.

## Config and state

- Config: `%AppData%\tradepost\config.json`
- Upload/metadata state: `%AppData%\tradepost\state.json`
- Log: `%AppData%\tradepost\companion.log` (append-only; delete it while the
  companion is stopped if it grows large). The GUI build has no console, so
  this file is the first place to look when startup fails silently.
- The companion writes only into the game's `_forever_` AddOns folder and its
  own config folder.

## Uninstall

1. Right-click the tray icon and choose **Quit**.
2. Delete the folder where you extracted the companion (for example
   `C:\TradePost`).
3. Remove every `TradePost*` folder from `_forever_\Interface\AddOns`.
4. Optionally delete `%AppData%\tradepost` (config, state and log).

The companion installs no service, driver or registry entry, so there is
nothing else to remove.

## Firewall and security

- The dashboard binds to `127.0.0.1:8787` only. It is not reachable from the
  LAN.
- Outbound traffic is HTTPS to `tradepost.b4b0.de` (and GitHub for the update
  check).
- The executable is not code-signed. Windows SmartScreen may ask for
  confirmation the first time: choose **More info → Run anyway** if you trust
  the source. The source and CI build are in this repository.
- The companion never writes Auctionator's database or SavedVariables. The
  Auctionator integration is a read-only display bridge.

## Advanced command line

The wizard is the normal path. For scripts:

```bat
tradepost.exe enroll --server https://tradepost.b4b0.de --wow "D:\Games\World of Warcraft" --market eu:pve:horde --count 64
tradepost.exe install --source "%~dp0" --wow "D:\Games\World of Warcraft" --count 64
tradepost.exe run
```

`--wow` accepts either the game root or its `_forever_` directory. Without it,
the companion tries `%ProgramFiles(x86)%\World of Warcraft`,
`%ProgramFiles%\World of Warcraft`, `%LOCALAPPDATA%\World of Warcraft`,
`C:\World of Warcraft`, and `C:\Games\World of Warcraft`.

The Windows build is a GUI application. When a command is started from
`cmd.exe` or PowerShell it reattaches to that console, so `status`, `version`,
`update --check` and the other commands print normally. Double-clicking the
`.exe` cannot show output; use the dashboard or run the command from a
terminal. `tradepost.exe` with no arguments is the default `run` and only
writes to the log file.

## Addon-only install

If you install TradePost from CurseForge/WoWUI without the companion, it will
warn that not all functionality is available and disable the live ring, web
login and metadata options. Local scans, the browser, central-data tooltips,
the watchlist and the Auctionator bridge keep working. Install and run the
companion for the full experience.

## Troubleshooting

**The wizard does not open.** Open `http://127.0.0.1:8787/setup` manually. If
the port is busy, close the other companion instance.

**The tray icon does not appear.** Check `%AppData%\tradepost\companion.log`
for a startup error. A second launch does not create a second icon; it opens
the dashboard of the running instance. On Windows 11 the icon may be in the
notification-area overflow menu (the `^` next to the clock); drag it onto the
taskbar to pin it.

**The game folder is rejected.** Pick the folder that contains `_forever_`;
make sure the account folders and `_forever_/Interface/AddOns` exist and are
writable.

**The addon or slots are missing in game.** Restart the client after the first
install. LoadOnDemand addons created while the client was running are only
discovered at startup.

**"Companion not detected" in game.** Start the companion, then `/reload`.
Check the dashboard status and that `TradePost_Companion` is enabled.

**No prices.** Open the auction house once so the addon scans. The upload and
the next ring/data refresh take a few minutes.

**Login code does not appear.** The companion must be running. It publishes a
code every 8 minutes; if the 16 auth slots are exhausted, restart the client or
wait for the next refresh.

**Windows Defender/SmartScreen.** The binary is unsigned. Use **More info →
Run anyway** if you trust the source, or build it yourself.
