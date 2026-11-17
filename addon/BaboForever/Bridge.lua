local ADDON, BF = ...

local function mapKey(dbKey)
  if type(dbKey) ~= "string" then
    return nil
  end
  if dbKey:sub(1, 2) == "g:" or dbKey:sub(1, 2) == "p:" then
    return dbKey
  end
  return "i:" .. dbKey
end

local function keysFor(dbKey)
  local key = mapKey(dbKey)
  if not key then
    return {}
  end

  local keys = { key }
  if key:sub(1, 2) == "g:" then
    local itemID = key:match("^g:(%d+):")
    if itemID then
      keys[#keys + 1] = "g:" .. itemID .. ":0"
      keys[#keys + 1] = "i:" .. itemID
    end
  end
  return keys
end

local function ourEntry(dbKey)
  local keys = keysFor(dbKey)
  if #keys == 0 then
    return nil
  end
  local entry, source = BF:GetPrice(keys)
  if entry and entry.p then
    if BF.bridge then
      BF.bridge.lastSource = source
    end
    return entry, source
  end
  return nil
end

function BF:InitBridge()
  if not self.config or not self.config.auctionatorBridge then
    return
  end
  if self.bridge and self.bridge.active then
    self:SetConfig("tooltips", false)
    return
  end

  if not (Auctionator and Auctionator.Database) then
    if not self.bridgeRetry then
      self.bridgeRetry = true
      C_Timer.After(2, function()
        self:InitBridge()
      end)
    end
    return
  end

  local db = Auctionator.Database
  local originals = {
    GetPrice = db.GetPrice,
    GetFirstPrice = db.GetFirstPrice,
    GetPriceAge = db.GetPriceAge,
  }
  if not originals.GetPrice or not originals.GetFirstPrice then
    self:Print("Auctionator bridge: unsupported Database layout")
    return
  end

  self.bridge = {
    active = true,
    originals = originals,
  }

  db.GetPrice = function(_, dbKey)
    local entry = ourEntry(dbKey)
    if entry then
      return entry.p
    end
    if BF.bridge then
      BF.bridge.lastSource = "auctionator"
    end
    return originals.GetPrice(db, dbKey)
  end

  db.GetFirstPrice = function(_, dbKeys)
    for _, key in ipairs(dbKeys or {}) do
      local entry = ourEntry(key)
      if entry then
        return entry.p
      end
    end
    if BF.bridge then
      BF.bridge.lastSource = "auctionator"
    end
    return originals.GetFirstPrice(db, dbKeys)
  end

  db.GetPriceAge = function(_, dbKey)
    local entry = ourEntry(dbKey)
    if entry and entry.t then
      return math.max(0, (time() - entry.t) / 86400)
    end
    if BF.bridge then
      BF.bridge.lastSource = "auctionator"
    end
    if originals.GetPriceAge then
      return originals.GetPriceAge(db, dbKey)
    end
    return nil
  end

  -- Auctionator formats age in whole days. Fractional days (our data) would
  -- show as "0 days", so format minutes/hours for the first day.
  if Auctionator.Tooltip and Auctionator.Tooltip.AddAuctionAgeTip then
    local originalAgeTip = Auctionator.Tooltip.AddAuctionAgeTip
    self.bridge.originals.AddAuctionAgeTip = originalAgeTip
    Auctionator.Tooltip.AddAuctionAgeTip = function(frame, auctionAge, auctionPrice, showUnknown)
      if type(auctionAge) == "number" and auctionAge > 0 and auctionAge < 1 then
        local seconds = auctionAge * 86400
        local text
        if seconds < 3600 then
          text = string.format("%dm", math.max(1, math.floor(seconds / 60)))
        elseif seconds < 86400 then
          text = string.format("%dh", math.floor(seconds / 3600))
        else
          text = string.format("%dd", math.floor(seconds / 86400))
        end
        local label = AUCTIONATOR_L_AUCTION_AGE or "Age"
        local source = BF.bridge and BF.bridge.lastSource
        local marker = nil
        if source == "live" then
          marker = "|cff60a5faR|r"
        elseif source == "central" then
          marker = "|cff60a5faC|r"
        elseif source == "local" then
          marker = "|cff33ff99L|r"
        end
        if marker then
          label = label .. " " .. marker
        end
        local white = WHITE_FONT_COLOR and WHITE_FONT_COLOR:WrapTextInColorCode(text) or text
        frame:AddDoubleLine(label, white)
        return
      end
      return originalAgeTip(frame, auctionAge, auctionPrice, showUnknown)
    end
  end

  self:SetConfig("tooltips", false)
  self:Print("Auctionator display bridge active (our prices replace Auctionator's)")
end

function BF:BridgeOff()
  if not self.bridge or not self.bridge.active then
    self:Print("Auctionator bridge is not active")
    return
  end

  local db = Auctionator and Auctionator.Database
  if db then
    if self.bridge.originals.GetPrice then
      db.GetPrice = self.bridge.originals.GetPrice
    end
    if self.bridge.originals.GetFirstPrice then
      db.GetFirstPrice = self.bridge.originals.GetFirstPrice
    end
    if self.bridge.originals.GetPriceAge then
      db.GetPriceAge = self.bridge.originals.GetPriceAge
    else
      db.GetPriceAge = nil
    end
  end

  if Auctionator and Auctionator.Tooltip and self.bridge.originals.AddAuctionAgeTip then
    Auctionator.Tooltip.AddAuctionAgeTip = self.bridge.originals.AddAuctionAgeTip
  end

  self.bridge = nil
  self:SetConfig("auctionatorBridge", false)
  self:SetConfig("tooltips", true)
  self:Print("Auctionator display bridge off; BF tooltips re-enabled")
end

function BF:BridgeStatus()
  local active = self.bridge and self.bridge.active
  self:Print("Auctionator bridge: " .. (active and "on" or "off") ..
    "  (config=" .. tostring(self.config.auctionatorBridge) ..
    ", BF tooltips=" .. tostring(self.config.tooltips) .. ")")
end

function BF:BridgeTest(itemID)
  itemID = tonumber(itemID) or 2318
  local db = Auctionator and Auctionator.Database
  if not db then
    self:Print("Auctionator is not loaded")
    return
  end

  local dbKey = tostring(itemID)
  local keys = keysFor(dbKey)
  local our, ourSource = ourEntry(dbKey)
  local originals = self.bridge and self.bridge.originals
  local originalPrice = originals and originals.GetPrice and originals.GetPrice(db, dbKey) or nil
  local originalAge = originals and originals.GetPriceAge and originals.GetPriceAge(db, dbKey) or nil
  local currentPrice = db.GetPrice and db.GetPrice(db, dbKey) or nil
  local currentAge = db.GetPriceAge and db.GetPriceAge(db, dbKey) or nil

  self:Print("bridgetest item=" .. itemID .. " keys=" .. table.concat(keys, ","))
  self:Print("  our=" .. tostring(our and our.p) ..
    "  source=" .. tostring(ourSource) ..
    "  ageSec=" .. tostring(our and our.t and (time() - our.t)))
  self:Print("  original=" .. tostring(originalPrice) ..
    "  ageDays=" .. tostring(originalAge))
  self:Print("  current=" .. tostring(currentPrice) ..
    "  ageDays=" .. tostring(currentAge))
  self:Print("  bridge=" .. tostring(self.bridge and self.bridge.active) ..
    "  tooltips=" .. tostring(self.config.tooltips))
end
