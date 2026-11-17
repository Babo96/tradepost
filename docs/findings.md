# Phase 0 findings

Fill this in from the in-game probe and the host scripts. Keep it factual; the
next design decision depends on it.

## Client

| Field | Value |
|---|---|
| Build version | |
| Build number | |
| Interface | |
| `WOW_PROJECT_ID` | |
| `WOW_PROJECT_CAMELOT` | |
| `GetRealmName()` | |
| `GetCurrentRegion()` | |
| Faction | |
| Zone / subzone | |
| Binding set (`GetCurrentBindingSet`) | |
| Binding count (`GetNumBindings`) | |

## Auction house

| Field | Value |
|---|---|
| `GetNumReplicateItems()` when opened | |
| Same after an Auctionator scan | |
| `IsThrottledMessageSystemReady()` | |
| `GetItemCommodityStatus(2589)` | |
| `GetItemCommodityStatus(4496)` | |
| Scan wall-clock time (if measured) | |
| Memory before/after (`collectgarbage("count")`) | |

Notes on market structure observed in game:

```
Region:
Ruleset / realm name:
Faction house:
Neutral house (Booty Bay / Gadgetzan / Everlook):
Valanaar:
```

## Bindings mailbox

| Test | Result |
|---|---|
| `SetBinding` with `CLICK BF...:LeftButton` returns ok | |
| `GetBindingAction` preserves the full action | |
| `SaveBindings` writes the line to `bindings-cache.wtf` | |
| `LoadBindings` re-reads a file changed mid-session | |
| Raw `BFD1:...` action preserved | |
| `MACRO BF...` action preserved | |
| Longest preserved action (255 / 1024 / 4096) | |
| `SetBinding` accepts a data-carrying action for uplink | |
| `LoadBindings` resets unsaved keybinds | |
| Action-bar flicker / other side effects | |
| Client rewrites the file during play | |

Example action string after a successful write:

```
```

## LoadOnDemand ring

| Test | Result |
|---|---|
| `LoadAddOn` on an unloaded slot reads current file content | |
| `LoadAddOn` on an already-loaded slot re-executes | |
| New slot folder created at runtime is discoverable | |
| Slots visible/enabled in the AddOns list | |

## TOC metadata

| Test | Result |
|---|---|
| `GetAddOnMetadata("BFProbeDataA", "X-BF-Data")` before rewrite | |
| same after rewriting the TOC mid-session | |
| Value truncated or preserved | |

## Decision

Pick one:

- [ ] **Bindings mailbox works.** Build downlink deltas via `LoadBindings`;
      use `BFU1` for uplink if `SetBinding` accepts data.
- [ ] **Ring works.** Use N LoadOnDemand slots for full/delta snapshots;
      mailbox only if it also works.
- [ ] **Neither works.** Central data at login/`/reload` only; keep live AH
      targeted queries and search-result capture for real-time prices.

Chunk size / key count chosen for the mailbox:

```
```

Anything else learned about the client:

```
```
