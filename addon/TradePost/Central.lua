local ADDON, TP = ...

TP.central = TP.central or {}

function TP:LoadCentral()
  self.central = {}
  self.companion = { present = false, running = false, generated = 0 }
  if type(TRADEPOST_DATA) ~= "table" or TRADEPOST_DATA.schema ~= 1 then
    return
  end
  if TRADEPOST_DATA.companion == 1 then
    local generated = tonumber(TRADEPOST_DATA.generated) or 0
    self.companion.present = true
    self.companion.generated = generated
    self.companion.running = generated > 0 and (time() - generated) <= 600
  end

  for marketKey, market in pairs(TRADEPOST_DATA.markets or {}) do
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

function TP:CheckCompanion()
  local companion = self.companion or {}
  local generated = tonumber(companion.generated) or 0
  companion.running = companion.present == true and
    generated > 0 and (time() - generated) <= 600
  self.companion = companion
  return companion.running
end

function TP:CompanionReady()
  return self.companion ~= nil and self.companion.running == true
end

function TP:GetCentralPrice(keys)
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

function TP:CentralLabel()
  local market = self.central[self:MarketKey()]
  local base = market and ("base v" .. tostring(market.version)) or "base none"
  local live = self.live and self.live[self:MarketKey()]
  if live and live.seq > 0 then
    return "live seq " .. tostring(live.seq) .. " + " .. base
  end
  return base
end
