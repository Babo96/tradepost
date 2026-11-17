# bfc — Babo Forever Companion

Phase 4. Linux first; Windows is a later port.

## Shape

Single Go binary, no CGO. Cross-compiles to `linux/amd64`, `linux/arm64` now and
`windows/amd64` later.

```
bfc run          start the daemon and the local dashboard
bfc status       print sync state
bfc sync         one upload/download pass
bfc doctor       check paths, permissions, addon versions, mailbox
bfc install      install/update the addon in a detected WoW install
bfc service      install/remove the systemd user unit (Linux)
bfc ui           open the dashboard
```

Dashboard: `http://127.0.0.1:8787` — last upload/download, market, snapshot
age, mailbox state, addon version, logs, “open WoW folder”, “reload needed”.

## Discovery

Linux:

- `/games/World of Warcraft`
- `~/.wine/drive_c/Program Files (x86)/World of Warcraft`
- Steam Proton `compatdata` prefixes
- Lutris prefixes
- manual path in the dashboard

Windows (later):

- registry `HKLM\SOFTWARE\WOW6432Node\Blizzard Entertainment\World of Warcraft`
- `%ProgramFiles(x86)%\World of Warcraft`
- manual path

## Files

Reads:

- `_forever_/WTF/Account/<id>/SavedVariables/BaboForever.lua`
- `_forever_/WTF/Account/<id>/bindings-cache.wtf`

Writes:

- `_forever_/Interface/AddOns/BaboForever_Data/Data.lua`
- mailbox lines in `bindings-cache.wtf` (phase 0 decides whether this is viable)

All writes are atomic and backed up. The companion never writes to Auctionator.
