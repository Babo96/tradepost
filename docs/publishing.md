# Publishing the addon (CurseForge / WoWUI)

TradePost is distributed twice:

1. **The addon** — the Lua UI. Publish `TradePost-<version>.zip` from the
   GitHub release on CurseForge/WoWUI.
2. **The companion** — the local Go service that supplies live prices,
   watchlist/login sync, and metadata. It is downloaded from
   <https://github.com/Babo96/tradepost/releases>. The addon links to it.

## Standalone behavior

The addon works without the companion at a reduced level:

- it prints a chat warning at login/reload that not all functionality is
  available;
- it disables the live price ring, web login and metadata options in Settings;
- local scans, the browser, tooltips from the central data addon, the
  watchlist, and the Auctionator bridge keep working.

The companion installs a tiny `TradePost_Companion` status addon containing
`TRADEPOST_COMPANION = { online = true/false }`. The companion writes it once
when it starts, once when it stops, and once when the addon asks for a refresh
through `TRADEPOST_EXPORT.companionRequest` on login/reload. There is **no
heartbeat**; the addon checks the flag once at login/reload. A static data pack
does not count as a running companion.

The price data addon is a login-time baseline and refreshes on companion
startup, every 30 minutes, or on an explicit reload request; the live ring
carries the 5-minute updates.

## Release flow

1. Bump the version in:
   - `addon/TradePost/TradePost.toc`
   - `addon/TradePost/Core.lua` (`TP.version`)
   - `data-addon/TradePost_Data/TradePost_Data.toc`
   - `data-addon/TradePost_LiveData/TradePost_LiveData.toc`
2. Tag and push (`vX.Y.Z`). The release workflow builds:
   - `TradePost-X.Y.Z.zip` (addon only; upload to CurseForge/WoWUI)
   - Linux/Windows companion archives.
3. On CurseForge/WoWUI:
   - Game version: **WoW Forever 1.60.1** (`## Interface: 16001`).
   - Category: Auction/Economy.
   - Description: use `addon/TradePost/README.md`; keep the companion link and
     the standalone warning.
   - Upload the addon zip; the companion is a separate download.
4. Optional static data pack: `https://tradepost.b4b0.de/v1/snapshot/<market>/data-addon.zip`
   can be offered as a data-only download. It gives static prices but no live
   updates and does not satisfy companion detection.

## Companion hint

Put the companion link in the project description and in the first screenshot
caption if the platform requires one:

<https://github.com/Babo96/tradepost>

The addon itself prints it on load when the companion is missing.
