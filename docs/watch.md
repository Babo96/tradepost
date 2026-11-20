# Watchlist and login

The same watchlist is shared by the in-game browser and the website. The
website login uses a one-time code, never a password. The companion is required
for the cross-device part; the in-game watchlist works locally without it.

## Login (code only)

1. In game: `/tradepost login`.
2. The addon first tries a **cached** code for the current character. The
   companion publishes a fresh code every 8 minutes, so this is normally
   instant.
3. If no fresh code is cached, the addon writes a request to
   `TRADEPOST_AUTH` and polls. The companion calls `POST /v1/auth/code` and
   writes the reply into all 16 `TradePost_Auth_XX` slots and the 8 reserved
   ring sync slots.
4. The code appears in chat, for example:

   ```
   login code: ABCD2345
   open https://tradepost.b4b0.de/ and enter it (valid 10 min).
   ```

5. On the website click **Sync watch**, enter the code and **Log in**.

No username is typed. The server binds the code to the active character
(`Name-Realm`), expires it after 10 minutes, and allows one use. The browser
gets an HttpOnly session cookie valid for 30 days. The character name is stored
only for the watchlist session.

If the companion is not running, the addon warns and `/tradepost login` does not
request a code. Install/run the companion first.

The 16 auth slots and 8 ring sync slots are one-shot per UI session. After many
logins without a restart, `/reload` resets them; the companion publishes a new
code on its next 8-minute refresh.

## In game

- Use the **star** on a browser row, or
  `/tradepost watch <itemID>` / `/tradepost watch i:2589` /
  `/tradepost watch g:12345:70`.
- `/tradepost watch` lists the current character's watchlist.
- The default Auction House **buy star** is the primary source. Clicking it
  adds the item, un-starring removes it, and the watchlist is pushed back as AH
  favorites when at the AH. Existing favorites are discovered on the first scan
  of the session; later changes are caught by a secure hook on
  `C_AuctionHouse.SetFavoriteItem`.
- Auctionator's Selling → Favourites star is mirrored when the Auctionator
  bridge is enabled (`auctionatorBridge`, default on). Disable the bridge with
  `/tradepost bridge off`; `/tradepost favourites off` disables only the
  Auctionator favourites mirror.
- `/tradepost favorites sync` pushes the watchlist into the default AH stars.
- The watchlist is mirrored into the Auctionator shopping list
  **“TradePost Watchlist”** through `Auctionator.API.v1.CreateShoppingList` (a
  public API; no direct SavedVariables writes). `/tradepost watch sync` rebuilds
  it manually.
- Changes are written to `TRADEPOST_WATCH`, flushed with `SaveAddOns`, and
  uploaded by the companion on its next SavedVariables check. A completed scan
  flushes the export immediately and retries the client's throttled write.

The local watchlist is not disabled when the companion is missing; only the
server/session side is unavailable.

## On the website

- Stars in search results and the item card toggle the watchlist.
- Changes use `PUT /v1/watch` and are shown immediately.
- The session is per character. Run `/tradepost login` with another character's
  code to switch.
- Logging out returns to a browser-local watchlist.

## Website → game

The companion fetches the character's server watchlist and writes it into the
live ring payload as `pins` / `pins_updated`. The addon applies it when it loads
a ring slot:

- immediately after `/reload` (the current slot is loaded at login), or
- on the next 5-minute ring window while playing.

An older ring slot cannot overwrite a newer local change; the timestamp check
is in `TP:ApplyRemoteWatch`.

## API

```
POST /v1/auth/code     bearer client token, {"character":"Name-Realm"}
POST /v1/auth/redeem   {"code":"ABCD2345"} -> session cookie
GET  /v1/auth/me       session character
POST /v1/auth/logout
GET  /v1/watch         session cookie, or bearer + ?character=
PUT  /v1/watch         {"items":["i:1251", ...]}
```

The companion uses the bearer token and `?character=` for its own watch sync.
The website uses the session cookie.

## Limits

- Codes expire after 10 minutes and are single-use. The alphabet excludes
  `I/O/0/1` for easier typing.
- The website session is per character; the watchlist is per character.
- The addon has 16 load-on-demand auth slots plus 8 ring sync slots. After many
  logins without a client restart, `/reload` is needed.
- The companion publishes a code every 8 minutes; a code is also generated on
  demand when a request reaches it.
- If the client was already running when the auth/sync addons were first
  installed, restart the client once so they are discovered. Later codes work
  after `/reload`.
