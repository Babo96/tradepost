local ADDON, TP = ...
TP.name = ADDON
TP.version = "0.2.20"
TP.atAuctionHouse = false

function TP:Print(...)
  print("|cff33ff99TradePost|r", ...)
end

function TP:Debug(...)
  if self.config and self.config.debug then
    self:Print(...)
  end
end

function TP:ReportError(area, err)
  local message = tostring(err)
  if self.lastError == message then
    return
  end
  self.lastError = message
  if TRADEPOST_DB then
    TRADEPOST_DB.lastError = {
      area = area,
      err = message,
      at = time(),
    }
  end
  self:Print("|cffff5555" .. tostring(area) .. " error:|r " .. message)
end

function TP:OnLoad()
  self:InitConfig()

  TRADEPOST_DB = TRADEPOST_DB or {}
  TRADEPOST_DB.version = 1
  TRADEPOST_ITEMS = TRADEPOST_ITEMS or {}
  if self.ExportDB then
    pcall(self.ExportDB, self)
  end
  TRADEPOST_DB.markets = TRADEPOST_DB.markets or {}
  if self.CompactMigrate then
    pcall(self.CompactMigrate, self)
  end

  self:UpdateMarket()
  self:PruneLocal()
  self.companion = self.companion or { present = false, running = false, generated = 0 }
  self:LoadCentral()
end

function TP:OnLogin()
  self:UpdateMarket()
  self:LoadCentral()
  self:CheckCompanion()
  self:MarkActive()
  self:PruneLocal()
  if self:CompanionReady() then
    self:InitRing()
  end
  self:InitBridge()
  self:InitBlizzardFavorites()
  C_Timer.After(3, function()
    pcall(function()
      TP:SaveExport()
    end)
  end)
  C_Timer.After(6, function()
    pcall(function()
      TP:SyncAuctionatorFavourites(false)
      TP:SyncAuctionatorWatchList(false)
    end)
  end)
  if self.MetaResume then
    pcall(self.MetaResume, self)
  end
  if not self.favouritesTimer then
    self.favouritesTimer = C_Timer.NewTicker(60, function()
      pcall(function()
        TP:SyncAuctionatorFavourites(false)
      end)
    end)
  end
  if not self:CompanionReady() then
    -- One small SavedVariables request per reload; the companion answers by
    -- setting the status addon's online flag once. No heartbeat.
    TRADEPOST_EXPORT = TRADEPOST_EXPORT or {}
    TRADEPOST_EXPORT.companionRequest = time()
    self:WarnNoCompanion()
  end
  self:CheckAddonVersion()
  self:Print("v" .. self.version .. " ready. Market: " .. self:MarketKey() .. ". /tradepost help")
end

function TP:CheckAddonVersion()
  local getMetadata
  if C_AddOns and type(C_AddOns.GetAddOnMetadata) == "function" then
    getMetadata = function(field)
      return C_AddOns.GetAddOnMetadata(ADDON, field)
    end
  elseif type(GetAddOnMetadata) == "function" then
    getMetadata = function(field)
      return GetAddOnMetadata(ADDON, field)
    end
  end
  local tocInterface = getMetadata and tonumber(getMetadata("Interface")) or 0
  local _, _, _, clientInterface = GetBuildInfo()
  self.interfaceOutdated = tocInterface > 0 and clientInterface ~= nil
    and tocInterface < clientInterface
  if self.interfaceOutdated then
    self:Print("|cffff5555out of date for this client|r (addon interface " ..
      tostring(tocInterface) .. ", client " .. tostring(clientInterface) ..
      "). Update: https://github.com/Babo96/tradepost/releases")
  end
  if self:AddonUpdateAvailable() then
    local url = self.companion and self.companion.addon_url
      or "https://github.com/Babo96/tradepost/releases"
    self:Print("|cff33ff99v" .. tostring(self:AddonLatest()) ..
      " is available|r (installed " .. tostring(self.version) ..
      "). Update via the companion or " .. tostring(url))
  end
end

function TP:WarnNoCompanion()
  if self.companionWarned then
    return
  end
  self.companionWarned = true
  self:Print("|cffff5555companion not detected.|r Live prices, web login, watch sync and full metadata are disabled; local data still works.")
  self:Print("Install/run the companion for full functionality: https://github.com/Babo96/tradepost")
end

function TP:OnAuctionHouseShow()
  self.atAuctionHouse = true
  self:UpdateMarket()
  self:InitBlizzardFavorites()
  self:Debug("AH opened. Market: " .. self:MarketKey())
  C_Timer.After(2, function()
    pcall(function()
      TP:SyncBlizzardFavorites(false)
    end)
    -- Let the Blizzard AH frames initialize before asking for a full snapshot.
    if TP.atAuctionHouse then
      TP:TryScan(false)
    end
  end)
end

function TP:OnAuctionHouseFailure()
  if self.scan then
    self.scan.waiting = false
    self.scan.suppressUntil = time() + 60
  end
  local now = time()
  if not self.lastAhError or now - self.lastAhError >= 60 then
    self.lastAhError = now
    self:Print("Auction House returned an internal error; TradePost will retry its scan in 60 s.")
  end
end

function TP:OnAuctionHouseClosed()
  self.atAuctionHouse = false
  self:UpdateMarket()
  if self.scan then
    self.scan.waiting = false
  end
end

function TP:OnReplicateUpdate()
  -- GetReplicateItemInfo() calls during our own scan also fire this event.
  if self.scan and self.scan.running then
    return
  end
  if self.scan and self.scan.suppressUntil and time() < self.scan.suppressUntil then
    return
  end

  local count = C_AuctionHouse and C_AuctionHouse.GetNumReplicateItems() or 0
  if not count or count <= 0 then
    return
  end

  if self.scan and self.scan.lastEventCount == count and not self.scan.waiting then
    return
  end

  local db = self:LocalDB()
  local marketKey = self:MarketKey()
  if db.lastScanMarket and db.lastScanMarket ~= marketKey then
    return
  end
  local lastScan = db.lastScan or 0
  if not self.scan.waiting and lastScan > 0 and (time() - lastScan) < 300 then
    return
  end

  if TRADEPOST_DB then
    TRADEPOST_DB.lastReplicate = time()
  end
  if self.scan then
    self.scan.waiting = false
    self.scan.lastEventCount = count
  end
  self:ScheduleQuietCheck(count)
end

function TP:CountLocal()
  local db = self:LocalDB()
  local count = 0
  for _ in pairs(db.prices or {}) do
    count = count + 1
  end
  return count
end

function TP:DispatchEvent(event, arg1)
  if event == "ADDON_LOADED" then
    if arg1 == ADDON then
      TP:OnLoad()
    elseif arg1 == "TradePost_Data" then
      TP:LoadCentral()
    end
  elseif event == "PLAYER_LOGIN" then
    TP:OnLogin()
  elseif event == "AUCTION_HOUSE_SHOW" then
    TP:OnAuctionHouseShow()
  elseif event == "AUCTION_HOUSE_CLOSED" then
    TP:OnAuctionHouseClosed()
  elseif event == "AUCTION_HOUSE_BROWSE_FAILURE" then
    TP:OnAuctionHouseFailure()
  elseif event == "REPLICATE_ITEM_LIST_UPDATE" then
    TP:OnReplicateUpdate()
  end
end

local frame = CreateFrame("Frame")
frame:RegisterEvent("ADDON_LOADED")
frame:RegisterEvent("PLAYER_LOGIN")
frame:RegisterEvent("AUCTION_HOUSE_SHOW")
frame:RegisterEvent("AUCTION_HOUSE_CLOSED")
frame:RegisterEvent("AUCTION_HOUSE_BROWSE_FAILURE")
frame:RegisterEvent("REPLICATE_ITEM_LIST_UPDATE")
frame:SetScript("OnEvent", function(_, event, arg1)
  local ok, err = pcall(TP.DispatchEvent, TP, event, arg1)
  if not ok then
    TP:ReportError(event, err)
  end
end)
