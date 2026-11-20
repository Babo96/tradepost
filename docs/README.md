# TradePost documentation

New here? Start with the **[project README](../README.md)**. It covers what
TradePost is, how the crowd data works, and the Windows/Linux install steps.

## For players

- [Downloads](https://tradepost.b4b0.de/download/) — addon, Windows companion, Linux companion
- [Windows companion guide](windows.md) — wizard, game folder, dashboard, updates
- [Linux companion guide](linux.md) — tar install, Proton, systemd, updates
- [Website and API](web.md) — search, filters, stats, history, hover cards
- [Watchlist and login](watch.md) — one-time codes, in-game/website sync

## How it works

- [Architecture](architecture.md) — components, markets, data flow, history, security
- [Companion ↔ game protocol](protocol.md) — data addon, live ring deltas, status addon, SavedVariables

## Distribution

- [Publishing the addon](publishing.md) — CurseForge/WoWUI package, companion hint

## Server and self-hosting

- [Server API](../server/README.md) — endpoints, ingest, schema, aggregation, deployment
- [Deployment notes](../deploy/README.md) — GitHub Actions, GHCR, Argo CD

## Components

- [Companion](../companion/README.md) — setup wizard, dashboard, sync loop, updates
- [Addon](../addon/TradePost/README.md) — in-game features, commands, companion warning

## Development history

These describe the client probes and earlier design phases. They are kept for
context; the current architecture and protocol are the two documents above.

- [Phase 0 probe guide](probe.md)
- [Phase 0 findings](findings.md)
- [Phase 3 local test checklist](phase3-test.md)
- [In-game browser design notes](browser.md)
