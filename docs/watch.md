# Watchlist sync

The same watchlist is shared by the in-game browser and the website.

## Login (code only)

1. In game: `/bf login`
2. The addon asks the companion for a one-time code. The companion calls
   `POST /v1/auth/code` and writes the reply into a load-on-demand sync addon.
3. The code appears in chat, for example:
   ```
   login code: ABCD2345
   open https://babo-forever.b4b0.de/ and enter it (valid 10 min).
   ```
4. On the website click **Sync watch**, enter the code, **Log in**.

No username is typed. The code is bound server-side to the active character
(`Name-Realm`), expires after 10 minutes, and can be used once. The browser
gets an HttpOnly session cookie valid for 30 days.

## In game

- Use the **star** at the right of a browser row, or
  `/bf watch <itemID>` / `/bf watch i:2589` / `/bf watch g:12345:70`.
- `/bf watch` lists the current character's watchlist.
- The watchlist is mirrored into the Auctionator shopping list
  **“Babo Forever Watchlist”** via `Auctionator.API.v1.CreateShoppingList`
  (public API, no direct SavedVariables writes). Use `/bf watch sync` to rebuild
  it manually. Item names are required; they come from the scan/browser cache,
  so an item that has never resolved appears after its data loads.
- Changes are written to `BABOFOREVER_WATCH`, flushed with `SaveAddOns`, and
  uploaded by the companion on its next 5-second SavedVariables check.

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
is in `BF:ApplyRemoteWatch`.

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
- The website session is per character. Run `/bf login` again to switch.
- The addon has 16 load-on-demand sync slots. After many logins without a
  client restart, run `/reload` if a new code does not appear.
