# Linux companion guide

The Linux companion is a static x86_64 Go binary. It runs as your normal user,
writes the addon files into the game folder, and serves a local dashboard at
`http://127.0.0.1:8787`. It does not need Wine or Proton for itself — it writes
the files directly, and the game (native or Proton) reads them.

Download the latest **`tradepost-<version>-linux-amd64.tar.gz`** from
<https://github.com/Babo96/tradepost/releases/latest>.

## Extract and first run

```sh
mkdir -p ~/TradePost
tar -xzf tradepost-<version>-linux-amd64.tar.gz -C ~/TradePost
cd ~/TradePost
./tradepost
```

The setup wizard opens at `http://127.0.0.1:8787/setup` (using `xdg-open`). Add
`--no-browser` if you prefer to open the URL yourself.

## First install

1. **Close World of Warcraft.**
2. Start the companion as above.
3. **Game folder.** Select the folder that contains `_forever_`, for example
   `/games/World of Warcraft`. Steam/Proton installations use the same Linux
   path, for example
   `~/.steam/steam/steamapps/common/World of Warcraft` or
   `~/Games/World of Warcraft`. The wizard validates
   `_forever_/Interface/AddOns`, the account folders and write access.
4. **Server and market.** Keep `https://tradepost.b4b0.de`. The default market
   is a fallback; the active character's region, ruleset and faction select the
   live market. Ring slots default to 64 (up to 128).
5. **Install.** The companion enrolls, writes the addon, the data addon, the
   status addon and the ring/auth slots, and starts syncing.
6. **Start WoW.** Restart the client once after the first install so the new
   addon folders are discovered. Open `/tradepost` or the minimap TradePost button,
   then open the auction house for the first scan.

## Everyday use

- Keep `./tradepost` running while you play. The dashboard is
  `http://127.0.0.1:8787`.
- Scans are uploaded when the addon writes them and the companion sees the
  SavedVariables change.
- The data addon is refreshed on startup and every 30 minutes; the live ring
  carries the 5-minute updates.
- The tiny `TradePost_Companion` status addon carries the `online` flag. No
  heartbeat. If the companion is not running, the addon warns and disables
  live-only options.

## Run in the background

A user service is enough:

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
mkdir -p ~/.config/systemd/user
# save the unit above as ~/.config/systemd/user/tradepost.service
systemctl --user daemon-reload
systemctl --user enable --now tradepost
```

To use the browser-based wizard with the service, run the first install
manually without `--no-browser`; after setup the service can run headless.

## Updates

The companion checks GitHub once a day and shows an update card on the
dashboard. Use **Install update** or run:

```sh
cd ~/TradePost
./tradepost update
```

The binary at `~/TradePost/tradepost` is replaced; restart the companion (or
the user service) to use the new version. The addon files are refreshed too;
`/reload` in game, or restart once if new addon folders were added.

`./tradepost update --check` prints the current and latest versions without
changing anything.
The addon also shows a chat and Settings notice when its `## Interface` is
older than the client, or when the companion reports a newer addon release.
CurseForge/WoWUI managers can update the standalone addon.

## Config and state

- Config: `~/.config/tradepost/config.json`
- Upload/metadata state: `~/.config/tradepost/state.json`
- The companion writes only into the game's `_forever_` AddOns folder and its
  own config folder.

## Permissions

The binary is a static x86_64 build with no CGO or runtime dependencies. Your
user needs write access to:

```
<game>/_forever_/Interface/AddOns
```

No root access is required. If the game is installed under another user or a
read-only location, move it or adjust ownership. If the AddOns folder is in a
Flatpak sandbox, point the wizard at the real host path.

## Proton / Steam

The companion does not need to run inside Proton. Point it at the Linux folder
that contains `_forever_`; it writes the addon files there. The game reads them
when it starts. A native Linux client and a Proton client use the same
`_forever_/Interface/AddOns` layout.

## Addon-only install

The addon can be installed from CurseForge/WoWUI without the companion. It will
warn that live prices, web login and metadata are disabled; local scans,
central-data tooltips, the browser, the watchlist and the Auctionator bridge
still work. Install the companion for the full experience.

## Troubleshooting

**The wizard does not open.** Run with `--no-browser` and open
`http://127.0.0.1:8787/setup` manually. If the port is busy, close the other
companion instance.

**The game folder is rejected.** Choose the folder that contains `_forever_`;
check `_forever_/Interface/AddOns`, the account folders and write permission.

**The addon or slots are missing in game.** Restart the client after the first
install. New addon folders are discovered at startup, not at runtime.

**"Companion not detected" in game.** Start the companion and `/reload`. Check
the dashboard and that `TradePost_Companion` is enabled in the AddOns list.

**No prices.** Open the auction house once. The upload and the next ring/data
refresh take a few minutes.

**Login code does not appear.** The companion must be running; it publishes a
code every 8 minutes. If the 16 auth slots are exhausted, restart the client or
wait for the next refresh.
