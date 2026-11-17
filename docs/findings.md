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
| `SetBinding` with `CLICK BF...:LeftButton` returns ok | not needed; inbound is dead |
| `GetBindingAction` preserves the full action | not needed |
| `SaveBindings` writes the line to `bindings-cache.wtf` | yes (line 49 observed) |
| `LoadBindings` re-reads a file changed mid-session | **No.** `Y` was `ACTIONBUTTON11` before `/reload` and `ACTIONBUTTON5` after. The file is only re-read at login/reload. |
| Raw `BFD1:...` action preserved | not needed |
| `MACRO BF...` action preserved | not needed |
| Longest preserved action (255 / 1024 / 4096) | not needed |
| `SetBinding` accepts a data-carrying action for uplink | not needed |
| `LoadBindings` resets unsaved keybinds | not observed |
| Action-bar flicker / other side effects | none observed |
| Client rewrites the file during play | not observed |

First `bindread`: `set=1`, `numBindings=320`, `reserved keys with actions: 0`.
That run did not follow a host-side `make probe-bind`, so it only proves the
probe loaded.

Decisive test: changing an existing valid binding (`Y`) in the file was not
visible until `/reload`:

```
before /reload: getkey Y = ACTIONBUTTON11
after  /reload: getkey Y = ACTIONBUTTON5
```

Conclusion: **the bindings mailbox cannot deliver data mid-session.** The
`LoadBindings` idea is rejected. `/reload` does re-read bindings and addon files,
so the data-addon path still works at reload time.

Example action string after a successful write:

```
```

## LoadOnDemand ring

| Test | Result |
|---|---|
| `LoadAddOn` on an unloaded slot reads current file content | **Yes.** `BFProbeDataA` written after startup loaded `RINGA123` mid-session, and `RINGA456` after `/reload`. |
| `LoadAddOn` on an already-loaded slot re-executes | No (expected; slot B stayed `INITIAL_B`). |
| New slot folder created at runtime is discoverable | **No.** `BFProbeDataC`/`D` created while the game ran returned `MISSING`. |
| Slots visible/enabled in the AddOns list | Yes, when present at startup. |

## TOC metadata

| Test | Result |
|---|---|
| `GetAddOnMetadata("BFProbeDataA", "X-BF-Data")` before rewrite | `META123` |
| same after rewriting the TOC to `META789` mid-session | `META123` — cached at load/startup |
| Value truncated or preserved | preserved |

## Decision

- [x] ~~**Bindings mailbox works.**~~ **Rejected:** `LoadBindings` does not
      re-read `bindings-cache.wtf` mid-session; only `/reload`/login picks up
      changes.
- [x] **Ring works.** A pre-created LoadOnDemand slot written after startup
      loads its current file content. New folders are not discovered at runtime
      (`MISSING`), so the ring must be pre-created and fixed-size.
- [ ] **Time-slot ring** to be implemented: N pre-created slots, one per
      5-minute window, producer writes the current window, consumer loads the
      completed window. No metadata/readiness signal is available (TOC metadata
      is cached).
- [ ] **Neither** not applicable; the ring is the live channel.

Chunk size / key count chosen for the mailbox:

```
not applicable; bindings mailbox rejected
```

Ring design chosen:

```
N = 64 slots, 5-minute windows, 64 * 5 min = 5 h 20 min per session.
Slot index = floor(unix_time / 300) mod 64.
Companion: write current window's slot with the cumulative delta since base.
Addon: at each window boundary, load the previous window's slot if not loaded.
After a full wrap, prompt /reload (all slots unload and can be reused).
```

Anything else learned about the client:

```
```
