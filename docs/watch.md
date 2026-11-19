# Watchlist sync

The same watchlist is shared by the in-game browser and the website.

## Login (code only)

1. In game: `/tradepost login`
2. The addon asks the companion for a one-time code. The companion calls
   `POST /v1/auth/code` and writes the reply into a load-on-demand sync addon.
3. The code appears in chat, for example:
   ```
   login code: ABCD2345
   open https://tradepost.b4b0.de/ and enter it (valid 10 min).
   ```
4. On the website click **Sync watch**, enter the code, **Log in**.

No username is typed. The code is bound server-side to the active character
(`Name-Realm`), expires after 10 minutes, and can be used once. The browser
gets an HttpOnly session cookie valid for 30 days.

## In game

- Use the **star** at the right of a browser row, or
  `/tradepost watch <itemID>` / `/tradepost watch i:2589` / `/tradepost watch g:12345:70`.
- `/tradepost watch` lists the current character's watchlist.
- The default Auction House **buy star** is the primary source. Clicking it
  adds the item to the watchlist, un-starring removes it, and the watchlist is
  pushed back as AH favorites when at the AH. Existing favorites are discovered
  on the first scan of the session; later changes are caught by a secure hook on
  `C_AuctionHouse.SetFavoriteItem`.
- Auctionator's Selling → Favourites star is mirrored too, but only when the
  Auctionator bridge is enabled (`auctionatorBridge`, default on). Disable the
  bridge with `/tradepost bridge off`; `/tradepost favourites off` disables only the
  Auctionator favourites mirror.
- `/tradepost favorites sync` pushes the watchlist into the default AH stars.
- The watchlist is mirrored into the Auctionator shopping list
  **“TradePost Watchlist”** via `Auctionator.API.v1.CreateShoppingList`
  (public API, no direct SavedVariables writes). Use `/tradepost watch sync` to rebuild
  it manually. Item names are required; they come from the scan/browser cache,
  so an item that has never resolved appears after its data loads.
- Changes are written to `TRADEPOST_WATCH`, flushed with `SaveAddOns`, and
  uploaded by the companion on its next 5-second SavedVariables check.
- A completed auction scan writes the export immediately (and again after two
  seconds), so the companion uploads the new scan, item metadata, and any
  watch/favourite changes automatically.

## On the website

- Stars in the search results and item detail toggle the watchlist.
- Changes are written with `PUT /v1/watch` and shown immediately.
- Logging out returns to a browser-local watchlist.

## Website -> game

The companion fetches the character's server watchlist and writes it into the
live ring data as `pins` / `pins_updated`. The addon applies it when it loads a
ring slot:

- immediately after `/reload` (the current slot is loaded at login), or
- on the next 5-minute ring window while playing.

An older ring slot cannot overwrite a newer local change; the timestamp check
is in `TP:ApplyRemoteWatch`.

## API

```
POST /v1/auth/code        bearer client token, {"character":"Name-Realm"}
POST /v1/auth/redeem      {"code":"ABCD2345"} -> session cookie
GET  /v1/auth/me          session character
POST /v1/auth/logout
GET  /v1/watch            session cookie, or bearer + ?character=
PUT  /v1/watch            {"items":["i:1259", ...]}
```

## Limits

- Codes have no rate limit beyond the 10-minute expiry; the alphabet excludes
  `I/O/0/1` to make typing easier.
- The website session is per character. Run `/tradepost login` again to switch.
- The addon has 16 load-on-demand sync slots. After many logins without a
  client restart, run `/reload` if a new code does not appear.
- If the client was already running when the sync addons were first installed,
  the companion also publishes the code into upcoming live-ring slots
  (`auth` + `seq`). This makes `/tradepost login` work after `/reload` without a full
  client restart. A restart is still needed once for the dedicated sync addons
  to be discovered.
