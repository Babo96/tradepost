local ADDON, BF = ...

local DEFAULTS = {
  tooltips = true,
  showAge = true,
  showSupply = true,
  debug = false,
  houseOverride = nil,
}

function BF:InitConfig()
  BABOFOREVER_CONFIG = BABOFOREVER_CONFIG or {}
  for key, value in pairs(DEFAULTS) do
    if BABOFOREVER_CONFIG[key] == nil then
      BABOFOREVER_CONFIG[key] = value
    end
  end
  self.config = BABOFOREVER_CONFIG
end

function BF:SetConfig(key, value)
  self.config[key] = value
end
