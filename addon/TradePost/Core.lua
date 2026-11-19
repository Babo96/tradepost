local ADDON, TP = ...
TP.name = ADDON
TP.version = "0.2.0"
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
  TRADEPOST_DB.markets = TRADEPOST_DB.markets or {}

  self:UpdateMarket()
  self:PruneLocal()
  self:LoadCentral()
end

function TP:OnLogin()
  self:UpdateMarket()
  self:MarkActive()
  self:PruneLocal()
  self:InitRing()
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
  if not self.favouritesTimer then
    self.favouritesTimer = C_Timer.NewTicker(60, function()
      pcall(function()
        TP:SyncAuctionatorFavourites(false)
      end)
    end)
  end
  self:Print("v" .. self.version .. " ready. Market: " .. self:MarketKey() .. ". /tradepost help")
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
  end)
  self:TryScan(false)
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
  elseif event == "REPLICATE_ITEM_LIST_UPDATE" then
    TP:OnReplicateUpdate()
  end
end

local frame = CreateFrame("Frame")
frame:RegisterEvent("ADDON_LOADED")
frame:RegisterEvent("PLAYER_LOGIN")
frame:RegisterEvent("AUCTION_HOUSE_SHOW")
frame:RegisterEvent("AUCTION_HOUSE_CLOSED")
frame:RegisterEvent("REPLICATE_ITEM_LIST_UPDATE")
frame:SetScript("OnEvent", function(_, event, arg1)
  local ok, err = pcall(TP.DispatchEvent, TP, event, arg1)
  if not ok then
    TP:ReportError(event, err)
  end
end)
