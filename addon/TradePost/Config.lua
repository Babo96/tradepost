local ADDON, TP = ...

local DEFAULTS = {
  tooltips = true,
  showAge = true,
  showSupply = true,
  debug = false,
  houseOverride = nil,
  ring = true,
  ringSlots = 64,
  ringWindow = 300,
  auctionatorBridge = true,
  auctionatorFavourites = true,
}

function TP:InitConfig()
  TRADEPOST_CONFIG = TRADEPOST_CONFIG or {}
  for key, value in pairs(DEFAULTS) do
    if TRADEPOST_CONFIG[key] == nil then
      TRADEPOST_CONFIG[key] = value
    end
  end
  self.config = TRADEPOST_CONFIG
end

function TP:SetConfig(key, value)
  self.config[key] = value
end
