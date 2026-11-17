# Phase 0 probe guide

The probe answers the questions that decide the addon's data path. Run it once
on Linux now; the same steps apply on Windows later.

## 0. Safety

The host scripts only touch lines that start with `BFD1`. Every write is
preceded by a timestamped backup of `bindings-cache.wtf`.

### Phase 0 status

- Bindings mailbox: **rejected** (only `/reload` re-reads the file).
- LoadOnDemand ring: **confirmed** (`RINGA123` loaded mid-session).
- New addon folders at runtime: **not discovered** (`MISSING`).
- TOC metadata: **cached** at load/startup.
- Full replicate benchmark: **4,817 ms** for 254,976 auctions, 6,593 unique
  item IDs, no memory growth after GC.

Phase 0 is complete. Sections 2–4 are kept for re-testing.

If anything looks wrong:

```
/bfprobe reset          # remove probe bindings from the client
make probe-restore      # restore the newest bindings backup
make probe-uninstall    # remove the probe addons
```

## 1. Install

```sh
make probe-install
```

Restart WoW completely (new addons are only discovered at startup). On the
character select screen, open **AddOns** and confirm these are enabled:

- `BaboForeverProbe`
- `BFProbeDataA`
- `BFProbeDataB`

Then log in and run:

```
/bfprobe dump
```

### Updating the probe

WoW may keep addon file contents from startup. If new probe commands do not
appear after `/reload`, restart the client. This behaviour is itself relevant
to the LoadOnDemand ring test.

Record build, `GetRealmName()`, `GetCurrentRegion()`, faction, zone and subzone.
Open the auction house and run:

```
/bfprobe ah
```

Record `GetNumReplicateItems()`, throttle readiness, and the commodity status
for linen cloth (2589) and a bag (4496).

Once the count is stable, benchmark iterating the whole list:

```
/bfprobe bench
```

Record total auctions, unique item IDs, usable entries, missing item info,
elapsed milliseconds and Lua memory delta. This decides whether a single full
scan is feasible or whether the addon must shard/target scans.

## 2. Bindings baseline

In game, out of combat:

```
/bfprobe bindwrite click TEST123
/bfprobe bindread
/bfprobe keys
```

On the host:

```sh
make probe-show
```

Expected: one `BFD1` line is visible in the file and `bindread` prints the
action for `CTRL-ALT-SHIFT-F1`. Note the return value of `SetBinding` and the
length of `GetBindingAction`.

## 3. Mid-session inbound (the key test)

Leave the game running. On the host:

```sh
make probe-bind PAYLOAD=MIDSESSION1 KEYS=4
```

Then in game:

```
/bfprobe bindread
```

- If it prints `CLICK BFD1:1:...MIDSESSION1...`, the bindings mailbox works
  without `/reload`.
- If it still shows `TEST123`, `LoadBindings` is cached; the bindings channel
  is dead and we fall back to the ring or reload.

Repeat with a different payload and `SEQ=2` to confirm it keeps updating.

### If `bindread` shows zero actions

First confirm the host actually wrote the file:

```sh
make probe-show
```

If the `BFD1` line is there but the game shows nothing, test whether
`LoadBindings` re-reads the file at all with a **known-valid** command:

```sh
make probe-set KEY=CTRL-ALT-SHIFT-F1 COMMAND=ACTIONBUTTON1
```

In game:

```
/bfprobe bindread
/bfprobe getkey CTRL-ALT-SHIFT-F1
```

- `ACTIONBUTTON1` appears: the file is re-read, but the client drops unknown
  actions. Try `--mode spell`, `--mode item`, `--mode macro` and
  `--mode click` to find a wrapper it keeps.
- Nothing appears: `LoadBindings` is cached; the bindings channel is dead.

Restore the key afterwards with `make probe-clear` or `make probe-restore`.

## 4. Long payloads

Outbound action length:

```
/bfprobe bindlen 255
/bfprobe bindlen 1024
/bfprobe bindlen 4096
```

Inbound chunk length (while the game runs):

```sh
PAYLOAD=$(python3 -c 'print("A"*1024)')
make probe-bind PAYLOAD="$PAYLOAD" KEYS=8 CHUNK_SIZE=200
/bfprobe bindread
```

Record the largest action string the client preserves. This sets the mailbox
chunk size.

## 5. LoadOnDemand ring

In game:

```
/bfprobe ring A
```

Expected: `INITIAL_A`. On the host, while the game runs:

```sh
make probe-ring SLOT=A PAYLOAD=RING123
```

In game:

```
/bfprobe ring A
```

- `INITIAL_A` means an already-loaded addon is not re-executed (expected).
- Now write slot B and load it for the first time:

```sh
make probe-ring SLOT=B PAYLOAD=RINGB123
```

```
/bfprobe ring B
```

- `RINGB123` means a LoadOnDemand addon reads the current file content when it
  is loaded. A ring of slots can therefore deliver mid-session updates.
- `INITIAL_B` means the client cached the file at startup; the ring is dead.

## 6. TOC metadata

```
/bfprobe meta A
```

Host:

```sh
make probe-meta SLOT=A PAYLOAD=META123
```

```
/bfprobe meta A
```

`META123` means `GetAddOnMetadata` reads the TOC live; `INITIAL_META_A` means
it is cached.

## 7. Side effects

- Open **Key Bindings**. Do the reserved `CTRL-ALT-SHIFT-*` entries appear? Do
  they break existing binds?
- Did the action bars flicker or reset after `bindread`?
- Did changing a normal bind in the UI survive the next `bindread`?
- Does `SaveBindings` write the mailbox lines back to disk?

Record anything unexpected in `docs/findings.md`.

## 8. Save the report

```
/bfprobe report
```

Log out or `/reload` to write
`WTF/Account/<account>/SavedVariables/BaboForeverProbe.lua`. Copy the relevant
entries into `docs/findings.md` (do not commit the SavedVariables file).

## 9. Clean up

```
/bfprobe reset
make probe-clear
make probe-uninstall
```

Keep the findings; they decide whether phase 2 builds the mailbox, the ring, or
falls back to `/reload` plus live AH searches.
