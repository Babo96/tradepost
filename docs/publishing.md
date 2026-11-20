# Publishing the addon (CurseForge / WoWUI)

TradePost is distributed in two ways:

1. **The addon** — the Lua UI. Publish `TradePost-<version>.zip` from the
   GitHub release on CurseForge/WoWUI.
2. **The companion** — the local Go service that supplies live prices,
   watchlist/login sync, and metadata. Download it from
   <https://github.com/Babo96/tradepost/releases>. The addon links to it.

## Standalone behavior

The addon works without the companion at a reduced level:

- it prints a chat warning at login/reload that not all functionality is
  available;
- it disables the live price ring, web login and metadata backfill options in
  Settings;
- local scans, the browser, tooltips from the central data addon, the watchlist,
  and the Auctionator bridge keep working.

The companion installs a tiny **`TradePost_Companion`** status addon containing
`TRADEPOST_COMPANION = { online = true/false }`. It is written once when the
companion starts, once when it stops, and once when the addon writes
`TRADEPOST_EXPORT.companionRequest` after a reload. There is **no heartbeat**;
the addon checks the flag once at login/reload. A missing status addon is
treated as "companion not running".

The price data addon (`TradePost_Data`) is a login-time baseline refreshed on
companion startup and every 30 minutes; the 5-minute live data rides the ring.
A static data pack does not count as a running companion.

## Release flow

1. Bump the version in:
   - `addon/TradePost/TradePost.toc`
   - `addon/TradePost/Core.lua` (`TP.version`)
   - `data-addon/TradePost_Data/TradePost_Data.toc`
   - `data-addon/TradePost_LiveData/TradePost_LiveData.toc`
   - `companion/cmd/tradepost/main.go` (default version; release ldflags also
     set it)
2. Tag and push (`vX.Y.Z`). The **Release** workflow builds:
   - `TradePost-X.Y.Z.zip` (addon only; upload to CurseForge/WoWUI),
   - `tradepost-X.Y.Z-linux-amd64.tar.gz`,
   - `tradepost-companion-X.Y.Z-windows-amd64.zip`.
3. The **Docker Build and Push** workflow builds
   `ghcr.io/babo96/tradepost-server:sha-<commit>` on `main`; update the Argo
   manifest tag if the server changed.

## CurseForge / WoWUI listing

- **Game version:** WoW Forever 1.60.1 (`## Interface: 16001`).
- **Category:** Auction/Economy.
- **File:** `TradePost-<version>.zip`; it contains a single `TradePost/` folder
  with the addon core (no `TradePost_Data` or status addon; the companion
  installs those).
- **Description:** use [`addon/TradePost/README.md`](../addon/TradePost/README.md).
  Keep the companion link and the standalone warning.
- **Companion link:** <https://github.com/Babo96/tradepost>
- **Website:** <https://tradepost.b4b0.de/>

The TOC carries `## X-Website`, `## X-License: MIT`, and a `## Notes` line
explaining that the companion is required for live prices, login, watch sync
and metadata.

## Optional static data pack

The companion writes the live data addon. A static pack can be offered as a
separate download:

```
https://tradepost.b4b0.de/v1/snapshot/<market>/data-addon.zip
```

It provides static prices but no live updates and does not satisfy the
companion status check. The addon will still show the warning and disable
live-only options.

## Companion hint

Put the companion link in the project description and in the first screenshot
caption if the platform requires one:

<https://github.com/Babo96/tradepost>

The addon itself prints it on load when the companion is missing.
