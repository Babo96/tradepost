local ADDON, TP = ...

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

function TP:PrintStatus()
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

function TP:PrintMarket()
  self:UpdateMarket()
  local mapID = C_Map and C_Map.GetBestMapForUnit and C_Map.GetBestMapForUnit("player") or nil
  self:Print("realm: '" .. tostring(self.market.realm) .. "'")
  self:Print("region: " .. tostring(self.market.region) .. " (" .. tostring(self.market.regionNumber) .. ")")
  self:Print("ruleset: " .. tostring(self.market.ruleset) ..
    (self.market.rulesetSource and (" (" .. self.market.rulesetSource .. ")") or ""))
  if self.market.ruleset == "unknown" then
    local names = self.market.realmCandidates or {}
    self:Print("realm sources: " .. (#names > 0 and table.concat(names, " | ") or "none"))
    self:Print("set it manually: /tradepost ruleset pve")
  end
  self:Print("faction: " .. tostring(self.market.faction))
  self:Print("house: " .. tostring(self.market.house))
  self:Print("key: " .. tostring(self.market.key))
  self:Print("zone: " .. tostring(GetZoneText and GetZoneText() or "?") ..
    " / " .. tostring(GetSubZoneText and GetSubZoneText() or "?") ..
    " (map " .. tostring(mapID) .. ")")
end

function TP:PrintHelp()
  self:Print("commands:")
  self:Print("  /tradepost                  open the settings panel")
  self:Print("  /tradepost scan [force]     scan the auction house")
  self:Print("  /tradepost status           local data and throttle state")
  self:Print("  /tradepost market           market detection details")
  self:Print("  /tradepost house auto/alliance/horde/neutral")
  self:Print("  /tradepost ruleset auto/pve/pvp/rp/rp-pvp/hc")
  self:Print("  /tradepost tooltips on/off  price tooltips")
  self:Print("  /tradepost age on/off       show price age")
  self:Print("  /tradepost supply on/off    show total supply")
  self:Print("  /tradepost ring [now/on/off/count N]  live data ring")
  self:Print("  /tradepost browse           open the auction data browser")
  self:Print("  /tradepost minimap [on/off] show or hide the minimap button")
  self:Print("  /tradepost watch [id/key]   toggle a watched item / list the watchlist")
  self:Print("  /tradepost watch sync       rebuild the Auctionator watchlist")
  self:Print("  /tradepost favourites on/off mirror Auctionator favourites into the watchlist")
  self:Print("  /tradepost favorites sync   push the watchlist into the default AH stars")
  self:Print("  /tradepost login            get a short web login code")
  self:Print("  /tradepost bridge [on/off]  Auctionator display bridge")
  self:Print("  /tradepost bridgetest [id]  original vs current Auctionator price")
  self:Print("  /tradepost export [clear/save/local] buffer status / flush / local test")
  self:Print("  /tradepost meta status|start|stop|force|enable|disable  full item metadata backfill (admin)")
  self:Print("  /tradepost clear            clear local prices for this market")
  self:Print("  /tradepost debug            toggle debug output")
  self:Print("  /tradepost lasterror        show the last caught error")
end

SLASH_TRADEPOST1 = "/tradepost"
SlashCmdList["TRADEPOST"] = function(msg)
  local command, rest = string.match(msg or "", "^(%S*)%s*(.*)$")
  command = string.lower(command or "")
  rest = string.lower(rest or "")

  if command == "scan" then
    TP:TryScan(rest == "force")
  elseif command == "status" then
    TP:PrintStatus()
  elseif command == "market" then
    TP:PrintMarket()
  elseif command == "house" then
    if rest == "auto" then
      TP:SetConfig("houseOverride", nil)
    elseif rest == "alliance" or rest == "horde" or rest == "neutral" then
      TP:SetConfig("houseOverride", rest)
    else
      TP:Print("usage: /tradepost house auto/alliance/horde/neutral")
      return
    end
    TP:UpdateMarket()
    TP:Print("house override: " .. tostring(TP.config.houseOverride or "auto") ..
      " -> " .. TP:MarketKey())
  elseif command == "ruleset" then
    local value = rest ~= "" and rest or "auto"
    if not TP:SetRulesetOverride(value) then
      TP:Print("usage: /tradepost ruleset auto/pve/pvp/rp/rp-pvp/hc")
      return
    end
    TP:Print("ruleset: " .. TP:MarketKey() .. " (" .. tostring(TP.config.rulesetOverride) .. ")")
  elseif command == "tooltips" then
    TP:SetConfig("tooltips", rest ~= "off")
    TP:Print("tooltips: " .. tostring(TP.config.tooltips))
  elseif command == "age" then
    TP:SetConfig("showAge", rest ~= "off")
    TP:Print("show age: " .. tostring(TP.config.showAge))
  elseif command == "supply" then
    TP:SetConfig("showSupply", rest ~= "off")
    TP:Print("show supply: " .. tostring(TP.config.showSupply))
  elseif command == "clear" then
    TP:ClearLocal()
  elseif command == "ring" then
    local sub, arg = string.match(rest or "", "^(%S*)%s*(.*)$")
    if sub == "now" then
      TP:RingNow(arg ~= "" and arg or nil)
    elseif sub == "on" then
      TP:SetConfig("ring", true)
      TP:InitRing()
      TP:RingStatus()
    elseif sub == "off" then
      TP:SetConfig("ring", false)
      TP:Print("ring off")
    elseif sub == "count" then
      TP:RingSetCount(arg)
    else
      TP:RingStatus()
    end
  elseif command == "bridge" then
    if rest == "on" then
      TP:SetConfig("auctionatorBridge", true)
      TP:InitBridge()
      TP:BridgeStatus()
    elseif rest == "off" then
      TP:BridgeOff()
    else
      TP:BridgeStatus()
    end
  elseif command == "bridgetest" then
    TP:BridgeTest(rest)
  elseif command == "settings" or command == "options" then
    TP:OpenSettings()
  elseif command == "browse" or command == "browser" then
    TP:ToggleBrowser()
  elseif command == "minimap" then
    if rest == "on" then
      TP:SetMinimapVisible(true)
      TP:Print("minimap button shown")
    elseif rest == "off" then
      TP:SetMinimapVisible(false)
      TP:Print("minimap button hidden")
    else
      TP:ToggleMinimap()
    end
  elseif command == "watch" then
    if rest == "sync" or rest == "auctionator" then
      TP:SyncAuctionatorWatchList(true)
    elseif rest == "" or rest == "list" then
      local keys = TP:WatchKeys()
      if #keys == 0 then
        TP:Print("watchlist is empty. Use /tradepost watch <itemID> or right-click a browser row.")
      else
        TP:Print("watchlist (" .. #keys .. "):")
        for _, key in ipairs(keys) do
          local itemID = tonumber(key:match("^[igp]:(%d+)"))
          local meta = itemID and TP:GetItemMeta(itemID, false) or nil
          TP:Print("  " .. key .. (meta and ("  " .. meta.n) or ""))
        end
      end
    else
      local key = rest
      local id = tonumber(rest)
      if id then
        key = "i:" .. id
      elseif not key:match("^[igp]:") then
        TP:Print("usage: /tradepost watch <itemID / i:id / g:id:ilvl>")
        return
      end
      local watched = TP:ToggleWatch(key)
      TP:Print((watched and "watching " or "unwatched ") .. key)
    end
  elseif command == "favourites" or command == "favorites" then
    if rest == "sync" then
      TP:SyncBlizzardFavorites(true)
    elseif rest == "on" then
      TP:SetConfig("auctionatorFavourites", true)
      TP:SyncAuctionatorFavourites(true)
    elseif rest == "off" then
      TP:SetConfig("auctionatorFavourites", false)
      TP:Print("Auctionator favourites sync off")
    else
      TP:Print("Auctionator favourites sync: " ..
        tostring(TP.config.auctionatorFavourites ~= false))
      TP:SyncAuctionatorFavourites(true)
    end
  elseif command == "login" then
    TP:RequestLogin()
  elseif command == "export" then
    if rest == "clear" then
      TP:ClearExport()
    elseif rest == "save" then
      if TP:SaveExport() then
        TP:Print("export saved to disk")
      end
    elseif rest == "local" then
      TP:ExportLocal()
    else
      TP:ExportStatus()
    end
  elseif command == "meta" then
    local sub = string.match(rest or "", "^(%S*)")
    if sub == "start" then
      TP:MetaStart(false, true)
    elseif sub == "enable" then
      TP:SetConfig("adminFullSync", true)
      TP:MetaStart(false)
    elseif sub == "disable" then
      TP:SetConfig("adminFullSync", false)
      TP:MetaStop()
    elseif sub == "stop" then
      TP:MetaStop()
    elseif sub == "force" then
      TP:MetaStart(true)
    else
      TP:MetaStatus()
    end
  elseif command == "debug" then
    TP:SetConfig("debug", not TP.config.debug)
    TP:Print("debug: " .. tostring(TP.config.debug))
  elseif command == "lasterror" then
    local lastError = TRADEPOST_DB and TRADEPOST_DB.lastError
    if lastError then
      TP:Print(string.format("%s: %s (%s)",
        tostring(lastError.area), tostring(lastError.err), date("%H:%M:%S", lastError.at)))
    else
      TP:Print("no error recorded")
    end
  elseif command == "help" then
    TP:PrintHelp()
  elseif command == "" then
    TP:OpenSettings()
  else
    TP:PrintHelp()
  end
end
