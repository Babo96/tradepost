# In-game price browser design notes

> The browser is implemented in `addon/TradePost/Browser.lua`; this page keeps
the original design notes and virtual-list rationale. Current features are in
[`addon/TradePost/README.md`](../addon/TradePost/README.md).

A minimal icon, similar in spirit to the Dungeon Journal button, opens a
searchable auction-data window fed by the same local/live/central sources as
the tooltips.

## Original implementation (0.1.0)

- Minimap coin button; left click toggles the window, `/tradepost browse` also works.
- Search by item name or item ID, live filtering while typing.
- Columns: item (quality colored), price, age, quantity, and source marker
  (`R` live ring, `C` central snapshot, `L` local scan).
- Sort cycle: name → price → age. The choice is stored as `browserSort`.
- Mouse wheel and scrollbar over a fixed pool of 16 rows.
- Hover shows the normal item tooltip (including the TP price line); shift
  click inserts the item link into chat.
- Market is the current character market (`eu:<ruleset>:<house>`), refreshed
  whenever the window opens.
- Item names/icons/quality come from the client item cache and are persisted
  in `TRADEPOST_ITEMS`; uncached items show `Item #<id>` and are resolved in
  the background. No extra server round trip or data-addon size is needed.

Known limits:

- Rows are merged from local/live/central at open time; a scan while the window
  is open is visible after closing and reopening (or `/reload`).
- Gear rows show the item ID and price of the specific `g:<id>:<ilvl>` key, but
  the tooltip uses the base item link, not the exact bonus/ilvl variant.
- No market switch button yet; the window follows the character's market.

## Entry point

Two options, in order of preference:

1. **Addon compartment icon** (`AddonCompartmentFunc`): the official small icon
   next to the minimap. No custom minimap math; the icon can use
   `Interface\Icons\inv_misc_coin_01` or a custom 32×32 TGA/BLP later.
2. **Minimap button** fallback with saved position, for clients/addon managers
   where the compartment is not available.

The icon opens `TradePostBrowser`; a second click or Escape closes it.

## Window

Minimal, not a full AH replacement:

- title + market selector (`eu:pve:horde`, `eu:pve:alliance`,
  `eu:pve:neutral`)
- search box (item name or item ID)
- list: icon, name, item level for gear, unit price, age, supply, source marker
- sort by price/name/age; filter equipment/commodities
- click a row to show the normal item tooltip with the TP/Auctionator price
- optional “watch” button (pins, phase 2 web UI)
- status line: snapshot version, live seq, last update

## Data needs

The addon can already iterate the local/live/central price keys. To search by
**name**, it needs item metadata:

- during a scan, collect `itemID`, `name`, `icon`, `classID`, `subClassID`,
  `quality` for each unique item
- upload metadata once (new `items` batch in the ingest payload or a separate
  `/v1/items` endpoint)
- server stores `items` and includes metadata in the base snapshot (or the
  companion writes a compact `TradePost_Data/Items.lua`)
- addon builds a search index on login from the snapshot; unknown items fall
  back to `C_Item.GetItemInfo` + `RequestLoadItemDataByID`

For 9–10k keys, a plain Lua array of `{id, name, icon, class, quality}` is
~500 KB; acceptable for the data addon. Lazy name loading is the fallback if the
snapshot grows.

## Virtual list

Do not create one row per item. Build a simple virtual list:

- keep a filtered array of keys
- render ~20 visible rows in a scroll frame
- update rows on scroll/search

This keeps the window cheap with 10k+ entries.

## Phases

1. Addon-side browser over local + central/live data, icon + search by item ID.
2. Item metadata in the snapshot → search by name.
3. Pins/watchlist from the web UI.
4. Price history sparkline (daily snapshots).
