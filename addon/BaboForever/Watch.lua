local ADDON, BF = ...

local AUTH_SLOTS = 16
local AUTH_FIRST_DELAY = 8
local AUTH_POLL_SECONDS = 2
local AUTH_POLL_TRIES = 10
local AUTH_RING_TRIES = 8

function BF:CharacterKey()
  local name = UnitName("player") or "Unknown"
  local realm = GetRealmName and GetRealmName() or ""
  if realm ~= "" then
    return name .. "-" .. realm
  end
  return name
end

function BF:MarkActive()
  BABOFOREVER_ACTIVE = {
    character = self:CharacterKey(),
    seen = time(),
  }
  return BABOFOREVER_ACTIVE.character
end

function BF:WatchDB()
  BABOFOREVER_WATCH = BABOFOREVER_WATCH or {}
  return BABOFOREVER_WATCH
end

function BF:WatchEntry(character)
  local db = self:WatchDB()
  local entry = db[character]
  if not entry then
    entry = { updated = 0, items = {} }
    db[character] = entry
  end
  entry.items = entry.items or {}
  return entry
end

function BF:CurrentWatch()
  return self:WatchEntry(self:CharacterKey())
end

function BF:IsWatched(key)
  return self:CurrentWatch().items[key] == true
end

function BF:WatchKeys()
  local keys = {}
  for key in pairs(self:CurrentWatch().items) do
    keys[#keys + 1] = key
  end
  table.sort(keys)
  return keys
end

function BF:SetWatched(key, watched)
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
  self:SyncAuctionatorWatchList(false)
  if self.browser and self.browser.frame and self.browser.frame:IsShown() then
    self:RefreshBrowser()
  end
end

function BF:ToggleWatch(key)
  local watched = not self:IsWatched(key)
  self:SetWatched(key, watched)
  return watched
end

-- Called from Ring.lua with the server watch list. The timestamp prevents an
-- older ring slot from overwriting a newer local change.
function BF:SyncAuctionatorWatchList(announce)
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

  local listName = "Babo Forever Watchlist"
  if #keys == 0 then
    -- Do not create an empty list on a fresh login. If a list exists, recreate
    -- it empty to clear the last unwatched item.
    local exists = pcall(Auctionator.API.v1.GetShoppingListItems, "BaboForever", listName)
    if not exists then
      return true
    end
  end

  local ok, err = pcall(Auctionator.API.v1.CreateShoppingList,
    "BaboForever", listName, names)
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

function BF:ApplyRemoteWatch(items, updated)
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
  self:SyncAuctionatorWatchList(false)
  if self.browser and self.browser.frame and self.browser.frame:IsShown() then
    self:RefreshBrowser()
  end
  return true
end

function BF:PrintLoginCode(reply)
  self:Print("login code: |cffffd100" .. tostring(reply.code) .. "|r")
  self:Print("open https://babo-forever.b4b0.de/ and enter it (valid 10 min).")
  if reply.expires then
    self:Print("expires at " .. date("%H:%M", reply.expires) .. ".")
  end
end

function BF:ReadAuthReply(seq, attempt)
  local reply = BABOFOREVER_AUTH_REPLY
  if reply and reply.seq == seq and reply.code then
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
    local name = string.format("BaboForever_Auth_%02d", index)
    local loaded = false
    if C_AddOns and C_AddOns.IsAddOnLoaded then
      local loading, isLoaded = C_AddOns.IsAddOnLoaded(name)
      loaded = loading or isLoaded
    elseif IsAddOnLoaded then
      loaded = IsAddOnLoaded(name)
    end
    if not loaded then
      pcall(loader, name)
      reply = BABOFOREVER_AUTH_REPLY
      if reply and reply.seq == seq and reply.code then
        self:PrintLoginCode(reply)
        return true
      end
      return false
    end
  end
  return false
end

function BF:ReadAuthReplyFromRing(seq, attempt)
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
  if auth and auth.seq == seq and auth.code then
    self:PrintLoginCode(auth)
    return true
  end
  return false
end

function BF:PollAuth(seq, tries)
  if tries >= AUTH_POLL_TRIES then
    self:Print("no login code in game. Open http://127.0.0.1:8787 to read it,")
    self:Print("or restart the game once and try /bf login again.")
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

function BF:RequestLogin()
  BABOFOREVER_AUTH = BABOFOREVER_AUTH or { seq = 0 }
  local auth = BABOFOREVER_AUTH
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
  self:Print("requesting a login code from the companion...")
  self:Print("the code also appears at http://127.0.0.1:8787")
  self:PollAuth(auth.seq, 0)
end
