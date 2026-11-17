local ADDON, BF = ...

BF.central = BF.central or {}

function BF:LoadCentral()
  self.central = {}
  if type(BABOFOREVER_DATA) ~= "table" or BABOFOREVER_DATA.schema ~= 1 then
    return
  end

  for marketKey, market in pairs(BABOFOREVER_DATA.markets or {}) do
    local lookup = {}
    for _, item in ipairs(market.items or {}) do
      if item.k and item.p then
        lookup[item.k] = {
          p = item.p,
          q = item.q,
          n = item.n,
          t = item.t,
        }
      end
    end
    self.central[marketKey] = {
      generated = market.generated,
      version = market.version,
      lookup = lookup,
    }
  end
end

function BF:GetCentralPrice(keys)
  local market = self.central[self:MarketKey()]
  if not market then
    return nil
  end
  for _, key in ipairs(keys) do
    local entry = market.lookup[key]
    if entry then
      return entry, key, market
    end
  end
  return nil
end

function BF:CentralLabel()
  local market = self.central[self:MarketKey()]
  if not market then
    return "none"
  end
  return string.format("v%s (%s)", tostring(market.version), tostring(market.generated))
end
