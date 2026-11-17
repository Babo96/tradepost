local ADDON, BF = ...

local MAX_AGE = 21 * 24 * 60 * 60

function BF:LocalDBFor(marketKey)
  BABOFOREVER_DB = BABOFOREVER_DB or {}
  BABOFOREVER_DB.markets = BABOFOREVER_DB.markets or {}
  local db = BABOFOREVER_DB.markets[marketKey]
  if not db then
    db = { prices = {}, lastScan = 0, lastScanMarket = nil }
    BABOFOREVER_DB.markets[marketKey] = db
  end
  db.prices = db.prices or {}
  return db
end

function BF:LocalDB()
  return self:LocalDBFor(self:MarketKey())
end

function BF:SetLocalPriceFor(marketKey, key, price, quantity, listings, timestamp)
  local db = self:LocalDBFor(marketKey)
  local entry = db.prices[key]
  if not entry then
    entry = {}
    db.prices[key] = entry
  end
  entry.p = price
  entry.q = quantity
  entry.n = listings
  entry.t = timestamp
end

function BF:SetLocalPrice(key, price, quantity, listings, timestamp)
  self:SetLocalPriceFor(self:MarketKey(), key, price, quantity, listings, timestamp)
end

function BF:GetLocalPrice(keys)
  local db = self:LocalDB()
  for _, key in ipairs(keys) do
    local entry = db.prices[key]
    if entry then
      return entry, key
    end
  end
  return nil
end

function BF:ClearLocal()
  local db = self:LocalDB()
  db.prices = {}
  db.lastScan = 0
  self:Print("local prices cleared for " .. self:MarketKey())
end

function BF:PruneLocal()
  if not BABOFOREVER_DB or not BABOFOREVER_DB.markets then
    return
  end
  local now = time()
  for _, db in pairs(BABOFOREVER_DB.markets) do
    for key, entry in pairs(db.prices or {}) do
      if not entry.t or (now - entry.t) > MAX_AGE then
        db.prices[key] = nil
      end
    end
  end
end
