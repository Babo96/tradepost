# TradePost

**Crowd-sourced auction house prices for World of Warcraft: Forever.**

TradePost shows market-aware auction house prices in your tooltips, adds a
searchable in-game price browser, and keeps a public price history on the
website. It works because players share the auction house scans their client
already makes.

> **The more people use it, the better it gets.** A single client can only ask
> the auction house for a fresh full scan about every 15 minutes. TradePost
> merges everyone's scans on the server, so prices refresh continuously instead
> of once per player.

[Website](https://tradepost.b4b0.de/) ·
[Download the latest release](https://github.com/Babo96/tradepost/releases/latest) ·
[Windows](#windows) · [Linux](#linux) · [How the crowd works](#how-the-crowd-works) ·
[Documentation](docs/README.md)

## What you get

| In game | On the website |
| --- | --- |
| Price, quantity and age in item tooltips | Search with rarity, class, slot, level, price and stat filters |
| A searchable browser with history and watchlist | Text-only item cards with tooltips, stats and resistances |
| Auctionator price bridge (read-only) | 12 h, 7 d and all-time price history |
| Multi-character support (Horde/Alliance, PvE/PvP) | Watchlist sync per character |
| Optional one-time website login code | Market comparison and live contributor count |

The addon has no account system. A random client ID is created when the
companion enrolls; only auction house prices and item data are uploaded. A
character name is stored only if you choose to use the website watchlist.

## How the crowd works

```
Your WoW client + TradePost addon
        │  local files (SavedVariables / live ring)
        ▼
TradePost companion  ──HTTPS──►  https://tradepost.b4b0.de  ──►  website
        ▲                                      │
        └──── data addon + 5-minute price deltas
```

1. **You play normally.** When you open the auction house, the addon reads the
   auctions your client already has (or asks for a fresh scan, subject to the
   client's ~15-minute throttle) and stores them in a local database.
2. **The companion uploads the scan.** It sends item keys, prices, quantities,
   listings, timestamps, and item metadata (name, quality, icon, tooltips,
   stats) to the server. There is no character name, account, or personal data
   in an upload.
3. **The server pools every player.** It merges scans per market, keeps
   1-minute, 15-minute and hourly history, and publishes the latest prices.
4. **The companion sends it back.** It refreshes the baseline data addon and
   writes small 5-minute live deltas into the game. Your tooltips, browser and
   the website update from the same pool everyone contributed to.
5. **The addon never touches Auctionator's database.** The bridge only reads
   Auctionator's display functions and returns TradePost prices.

Markets are split by region, ruleset and faction, for example
`eu:pve:horde` or `eu:pvp:alliance`. The active character's market is detected
automatically; the companion's configured market is only a fallback. Alts are
supported, and the website has a market selector.

### Do I need the companion?

Yes for the full experience. The companion is the only component that talks to
the server and writes live data into the game. The addon alone can be installed
from CurseForge/WoWUI, but it will:

- show a chat warning that not all functionality is available;
- disable the live price ring, web login and metadata backfill options;
- keep local scans, the browser, the watchlist, the Auctionator bridge and
  tooltips from any installed central data pack working.

The addon checks a tiny `TradePost_Companion` status addon that the companion
installs. There is no heartbeat: the companion writes an `online` flag when it
starts, when it stops, and once when the addon asks after a reload.

## Windows

1. Download **`tradepost-companion-<version>-windows-amd64.zip`** from the
   [latest release](https://github.com/Babo96/tradepost/releases/latest).
2. Extract it somewhere permanent, for example `C:\TradePost`.
3. Close World of Warcraft.
4. Run `tradepost.exe` (or `setup.cmd`). The setup wizard opens at
   `http://127.0.0.1:8787/setup`. If your browser does not open, visit that
   address manually.
5. In the wizard:
   - **Game folder:** choose the folder that contains `_forever_`, for example
     `C:\World of Warcraft`. The wizard checks `_forever_/Interface/AddOns` and
     the account folders.
   - **Server:** keep `https://tradepost.b4b0.de`.
   - **Default market:** `eu:pve:horde` is a fallback; the active character's
     region, ruleset (PvE/PvP/RP) and faction select the live market
     automatically.
   - **Ring slots:** 64 is the default; up to 128 gives a longer live session.
6. Click **Install**. The companion enrolls this machine, copies the addon, the
   data addon, the status addon and the ring/auth slots, and starts syncing.
7. Start WoW. **Restart the client once** after the first install so the new
   addon folders are discovered. Later addon code updates only need `/reload`.
8. In game, open `/tradepost` for settings, or use the coin button on the
   minimap (left click: browser, right click: settings). Open the auction house
   to make your first scan.

Keep the companion running while you play. Its dashboard is
`http://127.0.0.1:8787`; it shows the last sync, the active market, the login
code, and available updates.

> **Windows SmartScreen:** the binary is not code-signed. Choose
> **More info → Run anyway**. The source and build workflow are in this
> repository.

The companion stores its config and upload state in
`%AppData%\tradepost\config.json`. It does not install a Windows service; you
start it when you want to play, or add a shortcut to your startup folder.

For screenshots, advanced CLI flags and troubleshooting, see the
[Windows guide](docs/windows.md).

## Linux

1. Download **`tradepost-<version>-linux-amd64.tar.gz`** from the
   [latest release](https://github.com/Babo96/tradepost/releases/latest).
2. Extract it somewhere permanent, for example `~/TradePost`:
   ```sh
   mkdir -p ~/TradePost
   tar -xzf tradepost-<version>-linux-amd64.tar.gz -C ~/TradePost
   ```
3. Close World of Warcraft.
4. Start the companion:
   ```sh
   cd ~/TradePost
   ./tradepost
   ```
   The setup wizard opens at `http://127.0.0.1:8787/setup` (or add
   `--no-browser` and open it yourself).
5. Pick the game folder that contains `_forever_`, for example
   `/games/World of Warcraft`. Steam/Proton installations use the same Linux
   path; the companion writes the addon files directly and does not need to run
   inside Proton.
6. Keep the default server, set the fallback market and ring slots, then click
   **Install**. Start WoW afterwards. Restart the client once after the first
   install; `/reload` is enough for code updates.
7. Keep `./tradepost` running while you play. The dashboard is
   `http://127.0.0.1:8787`.

The binary is a static x86_64 build with no CGO or runtime dependencies. Your
user needs write access to the game's `_forever_/Interface/AddOns` folder; no
root access is required.

To run it in the background, a user service is enough:

```ini
# ~/.config/systemd/user/tradepost.service
[Unit]
Description=TradePost companion
After=network-online.target

[Service]
ExecStart=%h/TradePost/tradepost run --no-browser
Restart=on-failure

[Install]
WantedBy=default.target
```

```sh
systemctl --user enable --now tradepost
```

Config and upload state live in `~/.config/tradepost/`. See the
[Linux guide](docs/linux.md) for details.

## Addon only (CurseForge / WoWUI)

The release also contains **`TradePost-<version>.zip`**, the standalone addon
package. Install it like any other addon. Without the companion it shows the
warning and disables live-only options as described above. Install and run the
companion for live prices, website login, watchlist sync and metadata.

The `TradePost_Data` price pack is normally written by the companion. A static
data pack can be downloaded from
`https://tradepost.b4b0.de/v1/snapshot/<market>/data-addon.zip`; it gives
static prices but no live updates.

See [Publishing the addon](docs/publishing.md) for the CurseForge/WoWUI flow.

## Troubleshooting

**"Companion not detected" in game.**
Start the companion and check `http://127.0.0.1:8787` first. Then `/reload`. Make
sure the `TradePost_Companion` addon is present and enabled. The status addon is
created by the companion, so the first install needs a client restart.

**The addon or ring slots are missing.**
Restart the game client, not just `/reload`, after the first companion install.
LoadOnDemand addons that were created while the client was running are only
discovered at startup.

**No live prices yet.**
Open the auction house once so the addon scans. The companion uploads the scan,
the server aggregates it, and the next ring window (5 minutes) or data addon
refresh (30 minutes) delivers it. The website should show your market
immediately after the upload.

**`/tradepost login` does not show a code.**
The companion must be running. It publishes a fresh code every 8 minutes. If a
code still does not appear after a few `/reload`s, the 16 load-on-demand auth
slots are exhausted for this session; restart the client or wait for the
companion's next code refresh.

**How do I update?**
The companion checks GitHub once a day and shows an update card on its
dashboard. Use **Install update**, or run `tradepost update` (Windows:
`tradepost.exe update`). The binary is replaced; restart the companion. The
addon files are refreshed too; `/reload` in game (or restart once if new addon
folders were added).

The addon also warns in chat and in Settings when its `## Interface` is older
than the client, or when the companion reports a newer addon release. Standalone
addon users get the release notice from their addon manager (CurseForge/WoWUI).

**Windows flags the download.**
The binary is unsigned. Use **More info → Run anyway** if you trust the source,
or build it yourself with the instructions in the
[companion README](companion/README.md).

**Linux cannot write the addon folder.**
Make sure your user owns the game folder, or move the game to a location your
user can write. The companion never needs root.

**The game folder is rejected.**
Choose the folder that contains `_forever_` (the World of Warcraft root), not
the `_forever_` folder itself. The wizard validates
`_forever_/Interface/AddOns`, the account folders and write access.

## More resources

| I want to… | Read |
| --- | --- |
| See every document | [Documentation index](docs/README.md) |
| Install on Windows in detail | [Windows guide](docs/windows.md) |
| Install on Linux in detail | [Linux guide](docs/linux.md) |
| Use the website and API | [Website and API](docs/web.md) |
| Understand watchlists and login | [Watchlist sync](docs/watch.md) |
| Understand the architecture | [Architecture](docs/architecture.md) |
| Understand the companion↔game transport | [Protocol](docs/protocol.md) |
| Publish the addon on CurseForge/WoWUI | [Publishing](docs/publishing.md) |
| Run or extend the server | [Server README](server/README.md) |
| Deploy the server | [Deployment notes](deploy/README.md) |
| Build the companion | [Companion README](companion/README.md) |
| Work on the addon | [Addon README](addon/TradePost/README.md) |
| Browse prices | <https://tradepost.b4b0.de/> |
| Report a bug | <https://github.com/Babo96/tradepost/issues> |

## For developers

Repository layout:

```
addon/TradePost/     Lua addon (scan, local DB, tooltips, browser, ring, bridge)
data-addon/          generated price data addon (written by the companion)
companion/           Go companion (Linux + Windows, local dashboard)
server/              Go API + Postgres
deploy/              deployment notes
docs/                architecture, protocol, install and publishing guides
tools/               ring slot and release packaging helpers
```

Common commands:

```sh
make lint                  # luac syntax check for addon/data/probe
make test                  # server + companion + protocol tests
make check                 # lint + tests
make companion-build       # dist/tradepost (host)
make companion-build-windows
make server-run            # local API on :8080
make db-up                 # local Postgres on :5432
```

The companion is a normal Go module and the addon has no external libraries.
CI builds the Linux/Windows companions, the addon package and the public
`ghcr.io/babo96/tradepost-server` image. The live server runs at
`https://tradepost.b4b0.de`.

The companion and server are MIT-licensed. World of Warcraft and Blizzard
Entertainment are trademarks of Blizzard Entertainment, Inc. TradePost is an
unofficial fan project and is not affiliated with Blizzard.
