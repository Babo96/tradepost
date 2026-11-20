local ADDON, TP = ...

local MAX_AGE = 21 * 24 * 60 * 60

function TP:LocalDBFor(marketKey)
  TRADEPOST_DB = TRADEPOST_DB or {}
  TRADEPOST_DB.markets = TRADEPOST_DB.markets or {}
  local db = TRADEPOST_DB.markets[marketKey]
  if not db then
    db = { prices = {}, lastScan = 0, lastScanMarket = nil }
    TRADEPOST_DB.markets[marketKey] = db
  end
  db.prices = db.prices or {}
  return db
end

function TP:LocalDB()
  return self:LocalDBFor(self:MarketKey())
end

function TP:SetLocalPriceFor(marketKey, key, price, quantity, listings, timestamp)
  local db = self:LocalDBFor(marketKey)
  db.prices[key] = self.EncodePrice({
    p = price,
    q = quantity,
    n = listings,
    t = timestamp,
  })
end

function TP:SetLocalPrice(key, price, quantity, listings, timestamp)
  self:SetLocalPriceFor(self:MarketKey(), key, price, quantity, listings, timestamp)
end

function TP:GetLocalPrice(keys)
  local db = self:LocalDB()
  for _, key in ipairs(keys) do
    local entry = self.DecodePrice(db.prices[key])
    if entry then
      return entry, key
    end
  end
  return nil
end

function TP:ClearLocal()
  local db = self:LocalDB()
  db.prices = {}
  db.lastScan = 0
  self:Print("local prices cleared for " .. self:MarketKey())
end

function TP:PruneLocal()
  if not TRADEPOST_DB or not TRADEPOST_DB.markets then
    return
  end
  local now = time()
  for _, db in pairs(TRADEPOST_DB.markets) do
    for key, raw in pairs(db.prices or {}) do
      local entry = self.DecodePrice(raw)
      if not entry or not entry.t or (now - entry.t) > MAX_AGE then
        db.prices[key] = nil
      end
    end
  end
end
