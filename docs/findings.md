# Phase 0 findings

Fill this in from the in-game probe and the host scripts. Keep it factual; the
next design decision depends on it.

## Client

| Field | Value |
|---|---|
| Build version | 1.60.1 |
| Build number | 70938 |
| Interface | 16001 |
| `WOW_PROJECT_ID` | 18 |
| `WOW_PROJECT_CAMELOT` | 18 |
| `GetRealmName()` | `CAMWOW EU Realm 3 PvE` |
| `GetCurrentRegion()` | 3 |
| Faction | Horde |
| Zone / subzone | Thunder Bluff / (empty) |
| Binding set (`GetCurrentBindingSet`) | 1 |
| Binding count (`GetNumBindings`) | 320 |

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
Region: 3 (EU)
Ruleset / realm name: CAMWOW EU Realm 3 PvE -> ruleset suffix "PvE"
Faction house: Horde (Thunder Bluff)
Neutral house (Booty Bay / Gadgetzan / Everlook): not tested yet
Valanaar: not tested yet
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
