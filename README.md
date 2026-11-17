# babo-forever

Crowd-sourced auction house prices for **World of Warcraft: Forever**, with in-game
tooltips and a self-hosted backend.

The project is built in phases. This repository currently contains **phase 0**:
the diagnostic probe used to verify the client APIs, the auction-house data
shape, and the live file-mailbox idea before the real addon and companion are
written.

## Why

Forever has no realms. The auction house is split into a small number of markets:

```
region × ruleset (GetRealmName) × house (alliance / horde / neutral)
```

Existing price addons keep one database per realm and therefore mix faction and
neutral-house prices. `babo-forever` keeps them separate and fills gaps with a
central database fed by many players' scans.

Each player still scans at most every 15 minutes (the client's account-wide
replicate throttle), but the pool refreshes much faster than any single player
could. The companion app uploads scans and writes a central snapshot back into
the game.

## Repository layout

```
addon/              the real addon (phase 1)
data-addon/         generated central snapshot addon
probe/              phase 0 diagnostics (this phase)
tools/probe/        host-side scripts that rewrite files while WoW runs
companion/          host-side sync daemon (Linux first, Windows later)
server/             API + Postgres + snapshot builder
deploy/             k8s / docker-compose manifests
docs/               architecture, probe guide, protocol, findings
```

## Phase 0: run the probe

Requirements:

- WoW Forever installed (default `/games/World of Warcraft`)
- Python 3.11+
- Linux host (the Windows companion comes later)

```sh
make probe-install
# restart WoW (new addons are only noticed at startup), then /reload
```

In game:

```
/bfprobe dump
/bfprobe help
```

While the game is running, rewrite the bindings mailbox from the host:

```sh
make probe-bind PAYLOAD=TEST123 KEYS=4
/bfprobe bindread
```

Then test the LoadOnDemand ring:

```sh
make probe-ring SLOT=A PAYLOAD=RING123
/bfprobe ring A
```

And the TOC metadata cache:

```sh
make probe-meta PAYLOAD=META123
/bfprobe meta
```

Record the results in [`docs/findings.md`](docs/findings.md). The full test
sequence is in [`docs/probe.md`](docs/probe.md).

**Safety:** the host scripts back up `bindings-cache.wtf` before writing and
only touch lines with the `BFD1` prefix. `/bfprobe reset` removes test bindings;
`make probe-restore` restores the latest backup.

## Design decisions

- Lua addon, `Interface: 16001`, no external libraries.
- Companion in Go, no CGO, local web dashboard on `127.0.0.1:8787`.
- Linux companion first; Windows is a port behind `internal/platform`, not a
  rewrite.
- Server: Go + Postgres 17, EU markets only at launch.
- Live data: the addon cannot make HTTP requests. Scans and central data move
  through the game's files; the probe verifies which file-read primitive works
  mid-session (`LoadBindings`, LoadOnDemand ring, or reload only).
- Auctionator integration: read-only display bridge. We never write its
  database or SavedVariables.

See [`docs/architecture.md`](docs/architecture.md) and
[`docs/protocol.md`](docs/protocol.md).

## Status

- [x] Phase 0 scaffold
- [ ] Phase 0 probe results
- [ ] Addon core
- [ ] Live mailbox + Auctionator bridge
- [ ] Server
- [ ] Linux companion
- [ ] Deployment
- [ ] v1 release

## License

MIT. See [`LICENSE`](LICENSE).

World of Warcraft and Blizzard Entertainment are trademarks of Blizzard
Entertainment, Inc. This project is unofficial and not affiliated with
Blizzard.
