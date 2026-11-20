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
2. Tag and push (`vX.Y.Z`). The **Release** workflow builds and attaches all
   three to the GitHub release:
   - `TradePost-X.Y.Z.zip` (the addon; also uploaded to CurseForge when configured),
   - `tradepost-X.Y.Z-linux-amd64.tar.gz`,
   - `tradepost-companion-X.Y.Z-windows-amd64.zip`.
3. The workflow also attaches stable asset names
   (`TradePost-addon.zip`, `tradepost-linux-amd64.tar.gz`,
   `tradepost-companion-windows-amd64.zip`) for the website downloads page.
4. The **Docker Build and Push** workflow builds
   `ghcr.io/babo96/tradepost-server:sha-<commit>` on `main`; update the Argo
   manifest tag if the server changed.

## Automatic CurseForge uploads

The **Release** workflow uploads `TradePost-<version>.zip` to CurseForge after
the GitHub release is published. It is skipped until the repository values
below exist, so a release never fails because the project is not set up yet.

1. Create the CurseForge project page and copy its **numeric project ID** from
   the project sidebar.
2. Create an API token at
   <https://www.curseforge.com/account/api-tokens>.
3. In the GitHub repository go to **Settings → Secrets and variables →
   Actions** and add:
   - **Secret** `CURSEFORGE_API_TOKEN` — the token from step 2.
   - **Variable or secret** `CURSEFORGE_PROJECT_ID` — the numeric project ID.
   - **Optional variable** `CURSEFORGE_GAME_VERSIONS` — defaults to
     `88568:1.60.1` (game version type `88568` = Forever, version `1.60.1`).
     Use the numeric type ID; the API does not resolve the display name
     `Forever`.
4. Push a `vX.Y.Z` tag. The upload uses:
   - the addon zip `TradePost-X.Y.Z.zip`,
   - game endpoint `wow`,
   - game version `88568:1.60.1` (the Forever type ID; the TOC interface is
     `16001`),
   - release type `release`,
   - display name `TradePost X.Y.Z`,
   - the generated GitHub release notes as the CurseForge changelog.

Only the addon zip is uploaded to CurseForge. CurseForge rejects third-party
installers and `.exe` files, and its app treats project files as addons; the
Linux and Windows companions stay on GitHub Releases. The CurseForge changelog
automatically appends direct links to both companion assets.

If either the token or the project ID is missing, the workflow prints an
Actions notice and skips the upload. The manual upload flow below still works.
The token must belong to the project owner. For a brand-new project, the first
file can also be uploaded from the CurseForge web UI.

## Project icon

Use an original TradePost asset, not a Blizzard texture:

- **Project icon (CurseForge/WoWUI):** `assets/tradepost-icon-512.png`
- **Source:** `assets/tradepost-icon.svg`
- **In-game:** `addon/TradePost/Media/icon.tga`, referenced by the TOC
  `## IconTexture` and the minimap button.
- **Regenerate:** `make icon` (uses `tools/assets/render_icon.sh`).

## CurseForge / WoWUI listing

- **Game version:** WoW Forever 1.60.1 (`## Interface: 16001`).
- **Category:** Auction/Economy.
- **File:** `TradePost-<version>.zip`; it contains a single `TradePost/` folder
  with the addon core (no `TradePost_Data` or status addon; the companion
  installs those).
- **Description:** use [`addon/TradePost/README.md`](../addon/TradePost/README.md).
  Keep the companion link and the standalone warning.
- **Companion link:** <https://github.com/Babo96/tradepost>
- **Companion files:** point users to
  <https://tradepost.b4b0.de/download/> (or the matching GitHub release assets).
  Do not upload `.exe` or `.tar.gz` files to CurseForge.
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

## TOC project IDs

Once the CurseForge/WoWUI/Wago project pages exist, add their IDs to
`addon/TradePost/TradePost.toc`:

```toc
## X-Curse-Project-ID: <numeric id>
## X-WoWI-ID: <id>
## X-Wago-ID: <id>
```

The addon managers then identify and update the standalone addon. Independently,
the addon warns in chat and Settings when the companion reports a newer release
(`addon_latest`) or when its `## Interface` is older than the client.

## Companion hint

Put the companion link in the project description and in the first screenshot
caption if the platform requires one:

<https://github.com/Babo96/tradepost>

The addon itself prints it on load when the companion is missing.
