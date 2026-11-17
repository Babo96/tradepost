local ADDON, BF = ...
BF.name = ADDON
BF.version = "0.1.0"
BF.atAuctionHouse = false

function BF:Print(...)
  print("|cff33ff99BaboForever|r", ...)
end

function BF:Debug(...)
  if self.config and self.config.debug then
    self:Print(...)
  end
end

function BF:ReportError(area, err)
  local message = tostring(err)
  if self.lastError == message then
    return
  end
  self.lastError = message
  if BABOFOREVER_DB then
    BABOFOREVER_DB.lastError = {
      area = area,
      err = message,
      at = time(),
    }
  end
  self:Print("|cffff5555" .. tostring(area) .. " error:|r " .. message)
end

function BF:OnLoad()
  self:InitConfig()

  BABOFOREVER_DB = BABOFOREVER_DB or {}
  BABOFOREVER_DB.version = 1
  BABOFOREVER_ITEMS = BABOFOREVER_ITEMS or {}
  BABOFOREVER_DB.markets = BABOFOREVER_DB.markets or {}

  self:UpdateMarket()
  self:PruneLocal()
  self:LoadCentral()
end

function BF:OnLogin()
  self:UpdateMarket()
  self:PruneLocal()
  self:InitRing()
  self:InitBridge()
  self:Print("v" .. self.version .. " ready. Market: " .. self:MarketKey() .. ". /bf help")
end

function BF:OnAuctionHouseShow()
  self.atAuctionHouse = true
  self:UpdateMarket()
  self:Debug("AH opened. Market: " .. self:MarketKey())
  self:TryScan(false)
end

function BF:OnAuctionHouseClosed()
  self.atAuctionHouse = false
  self:UpdateMarket()
  if self.scan then
    self.scan.waiting = false
  end
end

function BF:OnReplicateUpdate()
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

  if BABOFOREVER_DB then
    BABOFOREVER_DB.lastReplicate = time()
  end
  if self.scan then
    self.scan.waiting = false
    self.scan.lastEventCount = count
  end
  self:ScheduleQuietCheck(count)
end

function BF:CountLocal()
  local db = self:LocalDB()
  local count = 0
  for _ in pairs(db.prices or {}) do
    count = count + 1
  end
  return count
end

function BF:DispatchEvent(event, arg1)
  if event == "ADDON_LOADED" then
    if arg1 == ADDON then
      BF:OnLoad()
    elseif arg1 == "BaboForever_Data" then
      BF:LoadCentral()
    end
  elseif event == "PLAYER_LOGIN" then
    BF:OnLogin()
  elseif event == "AUCTION_HOUSE_SHOW" then
    BF:OnAuctionHouseShow()
  elseif event == "AUCTION_HOUSE_CLOSED" then
    BF:OnAuctionHouseClosed()
  elseif event == "REPLICATE_ITEM_LIST_UPDATE" then
    BF:OnReplicateUpdate()
  end
end

local frame = CreateFrame("Frame")
frame:RegisterEvent("ADDON_LOADED")
frame:RegisterEvent("PLAYER_LOGIN")
frame:RegisterEvent("AUCTION_HOUSE_SHOW")
frame:RegisterEvent("AUCTION_HOUSE_CLOSED")
frame:RegisterEvent("REPLICATE_ITEM_LIST_UPDATE")
frame:SetScript("OnEvent", function(_, event, arg1)
  local ok, err = pcall(BF.DispatchEvent, BF, event, arg1)
  if not ok then
    BF:ReportError(event, err)
  end
end)
