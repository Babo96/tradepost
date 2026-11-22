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
  -- Minimap button: shown by default, position stored as degrees around the
  -- minimap (0 = right, 90 = up). Shift-drag moves it, shift-right-click hides.
  minimap = true,
  minimapAngle = 225,
  -- Market ruleset detection. "auto" reads the realm name (with fallbacks);
  -- set pve/pvp/rp/rp-pvp/hc if a custom client hides its realm name.
  rulesetOverride = "auto",
  -- Advanced/admin only. Collects full item tooltip metadata. Off by default
  -- because the SavedVariables payload can be large.
  adminFullSync = false,
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
