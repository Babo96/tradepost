# Live mailbox protocol (draft v0)

Status: **rejected for mid-session delivery (phase 0).** A valid binding change
in `bindings-cache.wtf` was not visible until `/reload`; `LoadBindings` does not
re-read the file while the client is running. Retained here for the uplink
idea and as a record of the experiment. The LoadOnDemand ring is the remaining
mid-session candidate.

## Idea

`LoadBindings(bindingSet)` is documented to read
`WTF/Account/<account>/bindings-cache.wtf`. `GetBindingAction(key)` returns the
command stored for a key. The companion can therefore write data into binding
commands while the game runs, and the addon can poll for it without `/reload`.

Bindings are valid `CLICK <frame>:<button>` actions so pressing a reserved key
does nothing harmful (the frame does not exist).

## Reserved keys

```
CTRL-ALT-SHIFT-F1 .. F12
CTRL-ALT-SHIFT-0 .. 9
CTRL-ALT-SHIFT-<letter>   (letters except F, I, O, T)
```

47 keys. The companion checks for collisions before using a key.

## Line format

Downlink (companion → addon):

```
bind CTRL-ALT-SHIFT-F1 CLICK BFD1:<seq>:<index>:<total>:<chunk>:LeftButton
bind CTRL-ALT-SHIFT-F2 CLICK BFD1:<seq>:<index>:<total>:<chunk>:LeftButton
```

Uplink (addon → companion), if `SetBinding` accepts the action:

```
bind CTRL-ALT-SHIFT-F12 CLICK BFU1:<seq>:<index>:<total>:<chunk>:LeftButton
```

- `BFD1` / `BFU1` — downlink/uplink, protocol version 1
- `seq` — monotonically increasing payload sequence
- `index` / `total` — chunk position and count
- `chunk` — payload characters; the probe uses `[A-Za-z0-9._-]`, real payloads
  will use digits and `,;:` only after verifying the parser preserves them

The addon concatenates chunks in order, checks `total`, verifies a CRC
(future: decimal CRC32 in the header), then decodes.

## Payload

Numeric first, to survive case folding and trimming:

```
v=1;base=<snapshot-version>;seq=<mailbox-seq>;t=<unix>;
add=12345,1299,20;67890,450,3;
del=11111,22222;
pins=12345,67890
```

- `add` — item key/price/qty triples (gear key encoded as `id.ilvl`)
- `del` — removed item keys
- `pins` — watchlist item IDs (v2)

Full snapshots are written to `BaboForever_Data` at login/reload. The mailbox
carries deltas; if the addon is too far behind, the companion marks
`full=1` and sends the whole snapshot over several polls.

## Rules

1. The companion preserves every line that is not a `BFD1`/`BFU1` mailbox line.
2. A timestamped backup is made before every write.
3. Writes are atomic: temp file in the same directory, fsync, replace.
4. The addon polls out of combat only (`LoadBindings` is no-combat).
5. Poll frequency: on AH open, zone change, and every 60 s while idle.
6. `/bf live off` stops polling and removes mailbox lines.
7. If the client rejects unknown binding actions, fall back to the
   LoadOnDemand ring, then to `/reload`.

## Open questions for phase 0

- Does `LoadBindings` re-read the file mid-session?
- Does it preserve `CLICK BF...` actions exactly, including long strings?
- What is the maximum action length before truncation?
- Does `LoadBindings` reset unsaved keybind changes or flicker action bars?
- Does `SetBinding` accept a data-carrying `CLICK` action for uplink?
- Does the client rewrite the bindings file during play, dropping mailbox lines?
- Does a LoadOnDemand addon load changed file contents mid-session?
- Is `GetAddOnMetadata` live or cached?
