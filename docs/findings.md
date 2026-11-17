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
| `GetNumReplicateItems()` when opened | 0 outside AH |
| Same after `/bfprobe scan` | 105,472 then 280,707 |
| `IsThrottledMessageSystemReady()` | true |
| `GetItemCommodityStatus(2589)` | not captured yet |
| `GetItemCommodityStatus(4496)` | not captured yet |
| Scan wall-clock time (if measured) | pending `/bfprobe bench` |
| Memory before/after (`collectgarbage("count")`) | pending `/bfprobe bench` |

The live list reached **280,707** auctions after a full replicate. That is much
larger than the ~80k example in the API docs, so the addon must aggregate in a
streaming pass and must not keep the raw list or per-auction tables.

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
| `SetBinding` with `CLICK BF...:LeftButton` returns ok | not run yet |
| `GetBindingAction` preserves the full action | not run yet |
| `SaveBindings` writes the line to `bindings-cache.wtf` | not run yet |
| `LoadBindings` re-reads a file changed mid-session | pending; first `bindread` had no host-written lines |
| Raw `BFD1:...` action preserved | not run yet |
| `MACRO BF...` action preserved | not run yet |
| Longest preserved action (255 / 1024 / 4096) | not run yet |
| `SetBinding` accepts a data-carrying action for uplink | not run yet |
| `LoadBindings` resets unsaved keybinds | not run yet |
| Action-bar flicker / other side effects | not run yet |
| Client rewrites the file during play | not run yet |

First `bindread`: `set=1`, `numBindings=320`, `reserved keys with actions: 0`.
That run did not follow a host-side `make probe-bind`, so it only proves the
probe loaded; it is not a failure of the channel.

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
