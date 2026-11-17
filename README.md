# babo-forever

Crowd-sourced auction house prices for **World of Warcraft: Forever**, with
in-game tooltips, an Auctionator display bridge, and a self-hosted backend.

The addon scans the auction house, shows market-aware prices, and uses a
pre-created **LoadOnDemand ring** to receive updates while the game is running.
The Go companion uploads scans and writes snapshots into the game. A Go +
Postgres server aggregates scans and serves snapshots.

## Why

Forever has no realms. The auction house is split into a small number of markets:

```
region × ruleset (suffix of GetRealmName) × house (alliance / horde / neutral)
```

Existing price addons keep one database per realm and therefore mix faction and
neutral-house prices. `babo-forever` keeps them separate and fills gaps with a
central database fed by many players' scans.

Each player still scans at most every 15 minutes (the client's account-wide
replicate throttle), but the pool refreshes much faster than any single player
could.

## Repository

```
addon/BaboForever/       Lua addon (scan, local DB, tooltips, ring, bridge)
data-addon/              generated central snapshot addon
ring/                    generated live slots (via tools/ring)
tools/ring/              slot generator and test writer
tools/probe/             phase 0 diagnostics
companion/               Go companion (Linux first, Windows later)
server/                  Go API + Postgres
deploy/                  k8s / compose manifests (later)
docs/                    architecture, protocol, probe guide, findings
```

## Quick start

### 1. Install the addon and ring slots

```sh
make addon-install
make ring-install COUNT=64
```

Restart WoW. The 64 `BaboForever_Data_XX` slots are LoadOnDemand and must be
enabled in the AddOns list.

In game:

```
/bf status
/bf market
```

### 2. Run the server

```sh
make db-up
make server-run
```

Server on `:8080`, Postgres on `localhost:5432` (`babo`/`babo`).

### 3. Enroll and run the companion

```sh
make companion-build
dist/bfc enroll --server http://127.0.0.1:8080 --wow "/games/World of Warcraft"
dist/bfc install --source "$PWD" --wow "/games/World of Warcraft" --count 64
dist/bfc run
```

Dashboard: `http://127.0.0.1:8787`

The companion reads `BABOFOREVER_EXPORT` from SavedVariables (written on
`/reload` or logout), uploads scans, downloads snapshots, rewrites
`BaboForever_Data/Data.lua`, and writes the current ring slot.

### 4. Test

```sh
make check
```

In game, `/bf scan`, hover items, and `/bf status`. Auctionator shows our prices
through the read-only bridge; `/bf bridge off` restores Auctionator and enables
the BF tooltip line.

## Phase status

- [x] Phase 0: client/API probes, market model, 280k scan benchmark
- [x] Addon core: scan, local DB, market-aware tooltips
- [x] Live LoadOnDemand ring
- [x] Auctionator display bridge
- [x] Server: enroll, ingest, meta, snapshot, data-addon zip
- [x] Linux companion: SavedVariables export, upload, snapshot/ring writer,
      local dashboard
- [x] Deployment on `babo-forever.b4b0.de` (Argo app `babo-forever`)
- [x] In-game price browser (icon + search window)
- [x] Windows companion build + setup zip (cross-compiled; needs a real Windows
      client test)
- [ ] v1 release packaging (signed/tagged Linux + Windows artifacts)
- [x] Item metadata ingest (client item cache -> server)
- [x] v2 web browser: search, hourly history, market comparison, watchlist
- [x] Two-way watchlist sync with `/bf login` one-time codes
- [ ] Server-side item metadata in the data addon (offline name search)

## Design decisions

- Lua addon, `Interface: 16001`, no external libraries.
- Companion in Go, no CGO, local web dashboard on `127.0.0.1:8787`.
- Linux companion first; Windows is the same Go program, cross-compiled with
  `CGO_ENABLED=0` and packaged with the addon sources.
- Server: Go + Postgres 17/18, EU markets only at launch.
- Live data: LoadOnDemand ring; the `LoadBindings` mailbox was rejected in
  phase 0. See [`docs/protocol.md`](docs/protocol.md).
- Auctionator integration: read-only display bridge. We never write its
  database or SavedVariables.

## Documentation

## Deployed

- API: `https://babo-forever.b4b0.de`
- Image: `git.babocraft.de/babo/babo-forever-server`
- Namespace: `babo-forever`
- Manifests: `homelab-argo/babo-forever`
- Argo app: `homelab-argo/argocd/apps/babo-forever.yaml`

## Documentation

- [`docs/architecture.md`](docs/architecture.md)
- [`docs/protocol.md`](docs/protocol.md)
- [`docs/probe.md`](docs/probe.md)
- [`docs/phase3-test.md`](docs/phase3-test.md)
- [`docs/browser.md`](docs/browser.md)
- [`docs/windows.md`](docs/windows.md)
- [`docs/web.md`](docs/web.md)
- [`docs/watch.md`](docs/watch.md)
- [`docs/findings.md`](docs/findings.md)

## License

MIT. See [`LICENSE`](LICENSE).

World of Warcraft and Blizzard Entertainment are trademarks of Blizzard
Entertainment, Inc. This project is unofficial and not affiliated with
Blizzard.
