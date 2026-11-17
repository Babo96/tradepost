# Live ring protocol (v1)

Status: **validated** in phase 0. A LoadOnDemand addon written on disk while the
game is running loads its **current** file content when first loaded. That is
the live channel.

The bindings mailbox was rejected: `LoadBindings` does not re-read
`bindings-cache.wtf` mid-session. TOC metadata is cached at load/startup, and
new addon folders created at runtime are not discovered. The ring must
therefore be **pre-created, fixed-size, and time-indexed**.

## Slots

```
Interface/AddOns/BaboForever_Data_00/
  BaboForever_Data_00.toc   ## LoadOnDemand: 1
  Data.lua
...
BaboForever_Data_63/
```

64 slots = 64 windows of 5 minutes = **5 h 20 min** per session. After a full
wrap the addon asks for `/reload`; all LoadOnDemand slots unload and can be
reused.

## Time windows

```
window = floor(unix_time / 300)
slot   = window mod 64
```

- In game, `time()` is the host clock; the companion uses the same clock.
- The **companion** writes slot `slot` during the window. It may rewrite it
  several times as new data arrives; the latest write wins.
- The **addon** loads the slot for the **previous completed window** at each
  boundary:
  `load_window = floor((now - 1) / 300)`.
- If the slot is already loaded, skip. This happens after a wrap; the addon
  then prompts for `/reload`.
- If the companion was not running, the slot is empty/stale; the addon loads
  it once, ignores it, and tries the next window. One window is lost; the next
  cumulative delta covers the change.

At login/reload, load the previous window's slot immediately, then align to
the next 5-minute boundary.

## Slot payload

`Data.lua` defines one global and overwrites the previous one:

```lua
BABOFOREVER_RING = {
  schema = 1,
  market = "eu:pve:horde",
  window = 12345,             -- floor(unix_time / 300)
  generated = 1794944447,     -- unix time of the snapshot
  base = 41,                  -- base snapshot version loaded at login
  seq = 42,                   -- monotonic delta sequence
  items = {                   -- cumulative delta since base
    { k = "i:2589", p = 125, q = 20, t = 1794944400 },
    { k = "g:12345:70", p = 4200, q = 1, t = 1794944380 },
  },
  removed = { "i:6789" },
  pins = { 2589, 12345 },     -- v2 watchlist
}
```

- `items` is **cumulative since the base**, not a per-window delta. A missed
  window therefore loses nothing; the next slot contains all changes.
- The companion keeps the per-item aggregate for the market and diffs it
  against the base loaded at login.
- Slot size grows with session changes; typical 5–50 KB, worst case ~200 KB.
  The addon replaces the global, so old tables are garbage-collected.

## Producer (companion)

1. On session start (data addon written), capture the base snapshot version.
2. Every 60 s, fetch the latest market snapshot, diff against the base.
3. Write the current window's slot atomically (temp + rename + backup).
4. If nothing changed, write a heartbeat with the same `items` (or skip; the
   addon will load the last written slot).
5. Stop after `64` windows; print “reload WoW to continue live data”.
6. Detect a new session by the SavedVariables mtime (written on `/reload` or
   logout) and reset.

## Consumer (addon)

1. At login, load `BaboForever_Data` (full base).
2. `C_Timer.After(seconds_to_next_boundary, tick)`, then every 300 s.
3. `tick`: compute the previous window, slot name; if not loaded, `LoadAddOn`.
4. If the global matches the current market and `seq > last_seq`, apply:
   - upsert `items`
   - remove `removed`
   - update `pins`
5. If `seq <= last_seq`, ignore (stale slot).
6. If the slot is already loaded and the window is new (wrap), show
   “live data exhausted; /reload to continue”.

## Why not the alternatives

- **Bindings mailbox:** `LoadBindings` did not re-read the file mid-session;
  a valid `Y` binding change only appeared after `/reload`. The client also
  dropped unknown `BFD1` actions on save/load.
- **TOC metadata:** `GetAddOnMetadata` returned the old value after the TOC was
  rewritten, so it cannot be used as a “slot ready” flag.
- **New folders at runtime:** `LoadAddOn("BFProbeDataC")` returned `MISSING`
  for a folder created after startup. The ring is fixed-size.
- **Addon messages:** 255 bytes, server-relayed, spam-policy risk.

## Open items

- Scan benchmark done: 254,976 auctions in 4,817 ms, 6,593 unique item IDs,
  -568 KB memory after GC. The real pass adds equipment-link/item-level work;
  measure that in the addon prototype.
- Choose 48 vs 64 vs 96 slots after measuring typical session length and slot
  size.
- Delta vs full snapshot per window; cumulative deltas are the current default.

## Live upload (outbound)

The inbound ring has an outbound counterpart: the addon calls
`C_AddOns.SaveAddOns()` after a scan. That writes `BABOFOREVER_EXPORT` to
`SavedVariables/BaboForever.lua` immediately, without `/reload`.

The companion watches the SavedVariables mtime every 5 seconds, parses
`BABOFOREVER_EXPORT`, uploads new scan hashes, and records them in
`~/.config/babo-forever/state.json`.

Fallbacks if `SaveAddOns` is unavailable or restricted:

- `/bf export save` to force a flush (for testing)
- normal `/reload` or logout

`/bf export` shows the last flush time and the export size.
