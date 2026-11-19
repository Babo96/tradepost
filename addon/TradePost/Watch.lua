local ADDON, TP = ...

local AUTH_SLOTS = 16
local AUTH_FIRST_DELAY = 60
local AUTH_POLL_SECONDS = 30
local AUTH_POLL_TRIES = 12
local AUTH_RING_TRIES = 8

function TP:CharacterKey()
  local name = UnitName("player") or "Unknown"
  local realm = GetRealmName and GetRealmName() or ""
  if realm ~= "" then
    return name .. "-" .. realm
  end
  return name
end

function TP:MarkActive()
  TRADEPOST_ACTIVE = {
    character = self:CharacterKey(),
    seen = time(),
  }
  return TRADEPOST_ACTIVE.character
end

function TP:WatchDB()
  TRADEPOST_WATCH = TRADEPOST_WATCH or {}
  return TRADEPOST_WATCH
end

function TP:WatchEntry(character)
  local db = self:WatchDB()
  local entry = db[character]
  if not entry then
    entry = { updated = 0, items = {} }
    db[character] = entry
  end
  entry.items = entry.items or {}
  return entry
end

function TP:CurrentWatch()
  return self:WatchEntry(self:CharacterKey())
end

function TP:IsWatched(key)
  return self:CurrentWatch().items[key] == true
end

function TP:WatchKeys()
  local keys = {}
  for key in pairs(self:CurrentWatch().items) do
    keys[#keys + 1] = key
  end
  table.sort(keys)
  return keys
end

function TP:SetWatched(key, watched)
  if not key or key == "" then
    return
  end
  local entry = self:CurrentWatch()
  if watched then
    entry.items[key] = true
  else
    entry.items[key] = nil
  end
  entry.updated = time()
  self:MarkActive()
  self:SaveExport()
  self:SyncBlizzardFavorites(false)
  self:SyncAuctionatorWatchList(false)
  if self.browser and self.browser.frame and self.browser.frame:IsShown() then
    self:RefreshBrowser()
  end
end

function TP:ToggleWatch(key)
  local watched = not self:IsWatched(key)
  self:SetWatched(key, watched)
  return watched
end

-- Called from Ring.lua with the server watch list. The timestamp prevents an
-- older ring slot from overwriting a newer local change.
function TP:MakeAuctionItemKey(key)
  if not C_AuctionHouse or type(C_AuctionHouse.MakeItemKey) ~= "function" then
    return nil
  end
  local prefix, itemID, itemLevel = key:match("^([igp]):(%d+):?(%d*)")
  itemID = tonumber(itemID)
  if not itemID then
    return nil
  end
  if prefix == "g" then
    return C_AuctionHouse.MakeItemKey(itemID, tonumber(itemLevel) or 0)
  end
  if prefix == "i" then
    return C_AuctionHouse.MakeItemKey(itemID)
  end
  return nil
end

function TP:KeyFromAuctionItemKey(itemKey)
  if type(itemKey) ~= "table" or not itemKey.itemID then
    return nil
  end
  local itemID = itemKey.itemID
  local classID = self:ClassIDFor(itemID)
  if classID == 2 or classID == 4 then
    local itemLevel = tonumber(itemKey.itemLevel) or 0
    return "g:" .. itemID .. ":" .. itemLevel
  end
  return "i:" .. itemID
end

function TP:InitBlizzardFavorites()
  if self.blizzardFavoritesHooked then
    return
  end
  if not C_AuctionHouse or type(C_AuctionHouse.SetFavoriteItem) ~= "function" then
    return
  end
  self.blizzardFavoritesHooked = true
  hooksecurefunc(C_AuctionHouse, "SetFavoriteItem", function(itemKey, setFavorite)
    if TP.blizzardFavoriteBusy then
      return
    end
    TP:OnBlizzardFavorite(itemKey, setFavorite)
  end)
end

function TP:OnBlizzardFavorite(itemKey, setFavorite)
  local key = self:KeyFromAuctionItemKey(itemKey)
  if not key then
    return
  end
  self.blizzardFavoriteBusy = true
  local entry = self:CurrentWatch()
  entry.blizzardFavorites = entry.blizzardFavorites or {}
  if setFavorite then
    entry.blizzardFavorites[key] = true
    pcall(self.SetWatched, self, key, true)
  else
    entry.blizzardFavorites[key] = nil
    pcall(self.SetWatched, self, key, false)
  end
  self.blizzardFavoriteBusy = false
end

-- Push watchlist entries into the default Auction House favorites. Run only
-- while at the AH; outside it the next AH open performs the sync.
function TP:SyncBlizzardFavorites(announce)
  if self.blizzardFavoriteBusy or not self.atAuctionHouse then
    return false
  end
  if not C_AuctionHouse or type(C_AuctionHouse.SetFavoriteItem) ~= "function" then
    return false
  end
  if type(C_AuctionHouse.MakeItemKey) ~= "function" then
    return false
  end
  if type(C_AuctionHouse.FavoritesAreAvailable) == "function" and not C_AuctionHouse.FavoritesAreAvailable() then
    return false
  end

  local entry = self:CurrentWatch()
  local previous = entry.blizzardFavorites or {}
  local current = {}
  local changed = false
  self.blizzardFavoriteBusy = true
  for _, key in ipairs(self:WatchKeys()) do
    local itemKey = self:MakeAuctionItemKey(key)
    if itemKey then
      current[key] = true
      if not previous[key] then
        pcall(C_AuctionHouse.SetFavoriteItem, itemKey, true)
        changed = true
      end
    end
  end
  for key in pairs(previous) do
    if not current[key] then
      local itemKey = self:MakeAuctionItemKey(key)
      if itemKey then
        pcall(C_AuctionHouse.SetFavoriteItem, itemKey, false)
        changed = true
      end
    end
  end
  entry.blizzardFavorites = current
  self.blizzardFavoriteBusy = false
  if changed then
    self:SaveExport()
  end
  if announce then
    self:Print("Auction House favorites synced: " .. #self:WatchKeys() .. " item(s)")
  end
  return true
end

-- Discover favorites that existed before this session. Only keys present in
-- the scan can be checked; the SetFavoriteItem hook handles later changes.
function TP:DiscoverBlizzardFavorites(keys)
  if self.favoritesScanned or not self.atAuctionHouse then
    return false
  end
  if not C_AuctionHouse or type(C_AuctionHouse.IsFavoriteItem) ~= "function" then
    return false
  end
  self.favoritesScanned = true
  local entry = self:CurrentWatch()
  entry.blizzardFavorites = entry.blizzardFavorites or {}
  local changed = false
  for key in pairs(keys or {}) do
    local itemKey = self:MakeAuctionItemKey(key)
    if itemKey then
      local ok, favorite = pcall(C_AuctionHouse.IsFavoriteItem, itemKey)
      if ok then
        if favorite then
          if not entry.items[key] then
            entry.items[key] = true
            changed = true
          end
          entry.blizzardFavorites[key] = true
        elseif entry.blizzardFavorites[key] then
          entry.items[key] = nil
          entry.blizzardFavorites[key] = nil
          changed = true
        end
      end
    end
  end
  if changed then
    entry.updated = time()
    self:MarkActive()
    self:SaveExport()
    if self.browser and self.browser.frame and self.browser.frame:IsShown() then
      self:RefreshBrowser()
    end
  end
  self:SyncBlizzardFavorites(false)
  return true
end

function TP:AuctionatorFavourites()
  if not Auctionator or type(Auctionator.Groups) ~= "table" then
    return nil
  end
  if type(Auctionator.Groups.GetGroupList) ~= "function" then
    return nil
  end
  local ok, list = pcall(Auctionator.Groups.GetGroupList, "FAVOURITES")
  if not ok or type(list) ~= "table" then
    return nil
  end
  return list
end

-- Mirror Auctionator's Selling -> Favourites star into the watchlist. This is
-- additive/differential: items removed from Favourites are removed only if
-- they were added by this sync, so website/browser watches are not lost.
function TP:SyncAuctionatorFavourites(announce)
  if self.config and (self.config.auctionatorBridge == false or self.config.auctionatorFavourites == false) then
    return false
  end
  local list = self:AuctionatorFavourites()
  if not list then
    return false
  end

  local entry = self:CurrentWatch()
  local previous = entry.auctionator or {}
  local current = {}
  local changed = false

  for _, link in ipairs(list) do
    local keys = self:KeysFromLink(link)
    local key = keys and keys[1]
    if key and not current[key] then
      current[key] = true
      if not entry.items[key] then
        entry.items[key] = true
        changed = true
      end
    end
  end

  for key in pairs(previous) do
    if not current[key] and entry.items[key] then
      entry.items[key] = nil
      changed = true
    end
  end
  entry.auctionator = current

  if changed then
    entry.updated = time()
    self:MarkActive()
    self:SaveExport()
    self:SyncAuctionatorWatchList(false)
    if self.browser and self.browser.frame and self.browser.frame:IsShown() then
      self:RefreshBrowser()
    end
  end
  if announce then
    self:Print("Auctionator favourites: " .. #list .. " item(s), watchlist now " ..
      #self:WatchKeys() .. " item(s)")
  end
  return true
end

function TP:SyncAuctionatorWatchList(announce)
  if self.config and self.config.auctionatorBridge == false then
    return false
  end
  if not Auctionator or type(Auctionator.API) ~= "table" or type(Auctionator.API.v1) ~= "table" then
    if announce then
      self:Print("Auctionator API is not available")
    end
    return false
  end
  if type(Auctionator.API.v1.CreateShoppingList) ~= "function" then
    if announce then
      self:Print("Auctionator shopping list API is not available")
    end
    return false
  end

  local keys = self:WatchKeys()
  local names = {}
  local seen = {}
  local missing = 0
  for _, key in ipairs(keys) do
    local itemID = tonumber(key:match("^[igp]:(%d+)"))
    local name = nil
    if itemID then
      local meta = self:GetItemMeta(itemID, false)
      name = meta and meta.n or nil
      if not name then
        name = C_Item.GetItemInfo(itemID)
        if not name then
          self:GetItemMeta(itemID, true)
          missing = missing + 1
        end
      end
    end
    if name then
      name = name:gsub("[;^\"]", ""):gsub("^%s+", ""):gsub("%s+$", "")
      if name ~= "" and not seen[name] then
        seen[name] = true
        names[#names + 1] = '"' .. name .. '"'
      end
    end
  end
  table.sort(names)

  if #names == 0 and #keys > 0 then
    if announce then
      self:Print("no item names cached yet; hover the items or reopen the browser first")
    end
    return false
  end

  local listName = "TradePost Watchlist"
  if #keys == 0 then
    -- Do not create an empty list on a fresh login. If a list exists, recreate
    -- it empty to clear the last unwatched item.
    local exists = pcall(Auctionator.API.v1.GetShoppingListItems, "TradePost", listName)
    if not exists then
      return true
    end
  end

  local ok, err = pcall(Auctionator.API.v1.CreateShoppingList,
    "TradePost", listName, names)
  if not ok then
    self:Debug("auctionator watchlist: " .. tostring(err))
    if announce then
      self:Print("could not update Auctionator: " .. tostring(err))
    end
    return false
  end
  if announce then
    self:Print("Auctionator watchlist updated: " .. #names .. " item(s)" ..
      (missing > 0 and (" (" .. missing .. " waiting for item data)") or ""))
  end
  return true
end

function TP:ApplyRemoteWatch(items, updated)
  updated = tonumber(updated) or 0
  local entry = self:CurrentWatch()
  if updated <= (entry.updated or 0) + 1 then
    return false
  end
  entry.items = {}
  for _, key in ipairs(items or {}) do
    if type(key) == "string" then
      entry.items[key] = true
    end
  end
  entry.updated = updated
  self:MarkActive()
  self:SaveExport()
  self:SyncBlizzardFavorites(false)
  self:SyncAuctionatorWatchList(false)
  if self.browser and self.browser.frame and self.browser.frame:IsShown() then
    self:RefreshBrowser()
  end
  return true
end

function TP:PrintLoginCode(reply)
  self:Print("login code: |cffffd100" .. tostring(reply.code) .. "|r")
  self:Print("open https://tradepost.b4b0.de/ and enter it (valid 10 min).")
  if reply.expires then
    self:Print("expires at " .. date("%H:%M", reply.expires) .. ".")
  end
end

local function freshLoginCode(reply, character)
  if type(reply) ~= "table" or not reply.code then
    return false
  end
  if reply.expires and reply.expires <= time() then
    return false
  end
  if reply.character and character and reply.character ~= character then
    return false
  end
  return true
end

function TP:ReadAuthReply(seq, attempt)
  local character = self:CharacterKey()
  local reply = TRADEPOST_AUTH_REPLY
  if freshLoginCode(reply, character) then
    self:PrintLoginCode(reply)
    return true
  end

  local loader = (C_AddOns and C_AddOns.LoadAddOn) or LoadAddOn
  if not loader then
    return false
  end
  -- Load at most one new slot per poll. The companion publishes the code to
  -- every slot, so we still find it after several early polls.
  for offset = 0, AUTH_SLOTS - 1 do
    local index = ((attempt or 0) + offset) % AUTH_SLOTS
    local name = string.format("TradePost_Auth_%02d", index)
    local loaded = false
    if C_AddOns and C_AddOns.IsAddOnLoaded then
      local loading, isLoaded = C_AddOns.IsAddOnLoaded(name)
      loaded = loading or isLoaded
    elseif IsAddOnLoaded then
      loaded = IsAddOnLoaded(name)
    end
    if not loaded then
      pcall(loader, name)
      reply = TRADEPOST_AUTH_REPLY
      if freshLoginCode(reply, character) then
        self:PrintLoginCode(reply)
        return true
      end
      return false
    end
  end
  return false
end

function TP:ReadAuthReplyFromRing(seq, attempt)
  if not self.config or not self.config.ring then
    return false
  end
  if (attempt or 0) >= AUTH_RING_TRIES then
    return false
  end
  local count = tonumber(self.config.ringSlots) or 64
  if count <= 8 then
    return false
  end
  local priceSlots = count - 8
  local syncCount = 8
  local index = priceSlots + ((attempt or 0) % syncCount)
  local data = self:LoadSyncSlot(index)
  local auth = data and data.auth
  if freshLoginCode(auth, self:CharacterKey()) then
    self:PrintLoginCode(auth)
    return true
  end
  return false
end

function TP:PollAuth(seq, tries)
  if tries >= AUTH_POLL_TRIES then
    self:Print("no login code in game. Open http://127.0.0.1:8787 to read it,")
    self:Print("or restart the game once and try /tradepost login again.")
    return
  end
  local delay = tries == 0 and AUTH_FIRST_DELAY or AUTH_POLL_SECONDS
  C_Timer.After(delay, function()
    if self:ReadAuthReply(seq, tries) or self:ReadAuthReplyFromRing(seq, tries) then
      return
    end
    self:PollAuth(seq, tries + 1)
  end)
end

function TP:RequestLogin()
  -- The companion publishes a fresh code every 8 minutes, so most logins can
  -- use the cached one immediately instead of waiting for a throttled
  -- SavedVariables flush round-trip.
  if self:ReadAuthReply(0, 0) or self:ReadAuthReplyFromRing(0, 0) then
    return
  end
  TRADEPOST_AUTH = TRADEPOST_AUTH or { seq = 0 }
  local auth = TRADEPOST_AUTH
  auth.seq = (auth.seq or 0) + 1
  auth.request = {
    seq = auth.seq,
    action = "login",
    character = self:CharacterKey(),
    market = self:MarketKey(),
    at = time(),
  }
  self:MarkActive()
  self:SaveExport()
  -- The Forever client defers SavedVariables writes by several minutes; retry
  -- the flush so the companion receives the request without a manual /reload.
  if self.ScheduleExportRetry then
    self:ScheduleExportRetry()
  end
  self:Print("requesting a login code from the companion (may take a few minutes)...")
  self:Print("the code also appears at http://127.0.0.1:8787")
  self:PollAuth(auth.seq, 0)
end
