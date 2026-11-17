local ADDON, BF = ...

local function ageText(timestamp)
  if not timestamp or timestamp <= 0 then
    return "never"
  end
  local seconds = math.max(0, time() - timestamp)
  if seconds < 60 then
    return string.format("%ds ago", seconds)
  elseif seconds < 3600 then
    return string.format("%dm ago", math.floor(seconds / 60))
  elseif seconds < 86400 then
    return string.format("%dh ago", math.floor(seconds / 3600))
  end
  return string.format("%dd ago", math.floor(seconds / 86400))
end

function BF:PrintStatus()
  self:UpdateMarket()
  local db = self:LocalDB()
  local localCount = self:CountLocal()

  self:Print("market: " .. self:MarketLabel() .. "  [" .. self:MarketKey() .. "]")
  self:Print("local prices: " .. localCount .. " items, last scan " .. ageText(db.lastScan))
  if db.lastAuctions then
    self:Print(string.format(
      "last scan: %d auctions, %d items, %d missing info, %d ms",
      db.lastAuctions, db.lastUnique or 0, db.lastMissing or 0, db.lastDuration or 0
    ))
  end
  self:Print("central data: " .. self:CentralLabel())
  self:RingStatus()
  self:BridgeStatus()
  self:Print("next client replicate: " .. math.ceil(self:NextReplicateSeconds() / 60) .. " min")
end

function BF:PrintMarket()
  self:UpdateMarket()
  local mapID = C_Map and C_Map.GetBestMapForUnit and C_Map.GetBestMapForUnit("player") or nil
  self:Print("realm: " .. tostring(self.market.realm))
  self:Print("region: " .. tostring(self.market.region) .. " (" .. tostring(self.market.regionNumber) .. ")")
  self:Print("ruleset: " .. tostring(self.market.ruleset))
  self:Print("faction: " .. tostring(self.market.faction))
  self:Print("house: " .. tostring(self.market.house))
  self:Print("key: " .. tostring(self.market.key))
  self:Print("zone: " .. tostring(GetZoneText and GetZoneText() or "?") ..
    " / " .. tostring(GetSubZoneText and GetSubZoneText() or "?") ..
    " (map " .. tostring(mapID) .. ")")
end

function BF:PrintHelp()
  self:Print("commands:")
  self:Print("  /bf scan [force]     scan the auction house")
  self:Print("  /bf status           local data and throttle state")
  self:Print("  /bf market           market detection details")
  self:Print("  /bf house auto|alliance|horde|neutral")
  self:Print("  /bf tooltips on|off  price tooltips")
  self:Print("  /bf age on|off       show price age")
  self:Print("  /bf supply on|off    show total supply")
  self:Print("  /bf ring [now|on|off|count N]  live data ring")
  self:Print("  /bf browse           open the auction data browser")
  self:Print("  /bf watch [id|key]   toggle a watched item / list the watchlist")
  self:Print("  /bf watch sync       rebuild the Auctionator watchlist")
  self:Print("  /bf favourites on|off mirror Auctionator favourites into the watchlist")
  self:Print("  /bf login            get a short web login code")
  self:Print("  /bf bridge [on|off]  Auctionator display bridge")
  self:Print("  /bf bridgetest [id]  original vs current Auctionator price")
  self:Print("  /bf export [clear|save|local] buffer status / flush / local test")
  self:Print("  /bf clear            clear local prices for this market")
  self:Print("  /bf debug            toggle debug output")
  self:Print("  /bf lasterror        show the last caught error")
end

SLASH_BABOFOREVER1 = "/bf"
SLASH_BABOFOREVER2 = "/babof"
SlashCmdList["BABOFOREVER"] = function(msg)
  local command, rest = string.match(msg or "", "^(%S*)%s*(.*)$")
  command = string.lower(command or "")
  rest = string.lower(rest or "")

  if command == "scan" then
    BF:TryScan(rest == "force")
  elseif command == "status" then
    BF:PrintStatus()
  elseif command == "market" then
    BF:PrintMarket()
  elseif command == "house" then
    if rest == "auto" then
      BF:SetConfig("houseOverride", nil)
    elseif rest == "alliance" or rest == "horde" or rest == "neutral" then
      BF:SetConfig("houseOverride", rest)
    else
      BF:Print("usage: /bf house auto|alliance|horde|neutral")
      return
    end
    BF:UpdateMarket()
    BF:Print("house override: " .. tostring(BF.config.houseOverride or "auto") ..
      " -> " .. BF:MarketKey())
  elseif command == "tooltips" then
    BF:SetConfig("tooltips", rest ~= "off")
    BF:Print("tooltips: " .. tostring(BF.config.tooltips))
  elseif command == "age" then
    BF:SetConfig("showAge", rest ~= "off")
    BF:Print("show age: " .. tostring(BF.config.showAge))
  elseif command == "supply" then
    BF:SetConfig("showSupply", rest ~= "off")
    BF:Print("show supply: " .. tostring(BF.config.showSupply))
  elseif command == "clear" then
    BF:ClearLocal()
  elseif command == "ring" then
    local sub, arg = string.match(rest or "", "^(%S*)%s*(.*)$")
    if sub == "now" then
      BF:RingNow(arg ~= "" and arg or nil)
    elseif sub == "on" then
      BF:SetConfig("ring", true)
      BF:InitRing()
      BF:RingStatus()
    elseif sub == "off" then
      BF:SetConfig("ring", false)
      BF:Print("ring off")
    elseif sub == "count" then
      BF:RingSetCount(arg)
    else
      BF:RingStatus()
    end
  elseif command == "bridge" then
    if rest == "on" then
      BF:SetConfig("auctionatorBridge", true)
      BF:InitBridge()
      BF:BridgeStatus()
    elseif rest == "off" then
      BF:BridgeOff()
    else
      BF:BridgeStatus()
    end
  elseif command == "bridgetest" then
    BF:BridgeTest(rest)
  elseif command == "browse" or command == "browser" then
    BF:ToggleBrowser()
  elseif command == "watch" then
    if rest == "sync" or rest == "auctionator" then
      BF:SyncAuctionatorWatchList(true)
    elseif rest == "" or rest == "list" then
      local keys = BF:WatchKeys()
      if #keys == 0 then
        BF:Print("watchlist is empty. Use /bf watch <itemID> or right-click a browser row.")
      else
        BF:Print("watchlist (" .. #keys .. "):")
        for _, key in ipairs(keys) do
          local itemID = tonumber(key:match("^[igp]:(%d+)"))
          local meta = itemID and BF:GetItemMeta(itemID, false) or nil
          BF:Print("  " .. key .. (meta and ("  " .. meta.n) or ""))
        end
      end
    else
      local key = rest
      local id = tonumber(rest)
      if id then
        key = "i:" .. id
      elseif not key:match("^[igp]:") then
        BF:Print("usage: /bf watch <itemID|i:id|g:id:ilvl>")
        return
      end
      local watched = BF:ToggleWatch(key)
      BF:Print((watched and "watching " or "unwatched ") .. key)
    end
  elseif command == "favourites" or command == "favorites" then
    if rest == "on" then
      BF:SetConfig("auctionatorFavourites", true)
      BF:SyncAuctionatorFavourites(true)
    elseif rest == "off" then
      BF:SetConfig("auctionatorFavourites", false)
      BF:Print("Auctionator favourites sync off")
    else
      BF:Print("Auctionator favourites sync: " ..
        tostring(BF.config.auctionatorFavourites ~= false))
      BF:SyncAuctionatorFavourites(true)
    end
  elseif command == "login" then
    BF:RequestLogin()
  elseif command == "export" then
    if rest == "clear" then
      BF:ClearExport()
    elseif rest == "save" then
      if BF:SaveExport() then
        BF:Print("export saved to disk")
      end
    elseif rest == "local" then
      BF:ExportLocal()
    else
      BF:ExportStatus()
    end
  elseif command == "debug" then
    BF:SetConfig("debug", not BF.config.debug)
    BF:Print("debug: " .. tostring(BF.config.debug))
  elseif command == "lasterror" then
    local lastError = BABOFOREVER_DB and BABOFOREVER_DB.lastError
    if lastError then
      BF:Print(string.format("%s: %s (%s)",
        tostring(lastError.area), tostring(lastError.err), date("%H:%M:%S", lastError.at)))
    else
      BF:Print("no error recorded")
    end
  elseif command == "help" or command == "" then
    BF:PrintHelp()
  else
    BF:PrintHelp()
  end
end
