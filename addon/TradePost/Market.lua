local ADDON, TP = ...

local REGIONS = {
  [1] = "us",
  [2] = "kr",
  [3] = "eu",
  [4] = "tw",
  [5] = "cn",
}

local RULESETS = {
  { name = "RP-PvP", key = "rp-pvp" },
  { name = "PvE", key = "pve" },
  { name = "PvP", key = "pvp" },
  { name = "RP", key = "rp" },
  { name = "Hardcore", key = "hc" },
}

-- Locale-independent neutral-house zones (Blackwater / Valanaar).
local NEUTRAL_MAPS = {
  [440] = true,   -- Tanaris (Gadgetzan)
  [441] = true,   -- Gadgetzan subzone
  [33] = true,    -- Stranglethorn Vale (Booty Bay)
  [35] = true,    -- Booty Bay subzone
  [618] = true,   -- Winterspring (Everlook)
  [619] = true,   -- Everlook subzone
  [16593] = true, -- Zephras Isle (Valanaar)
}

local NEUTRAL_ZONES = {
  ["Tanaris"] = true,
  ["Stranglethorn Vale"] = true,
  ["Winterspring"] = true,
  ["Zephras Isle"] = true,
}

local function rulesetFromRealm(realm)
  for _, candidate in ipairs(RULESETS) do
    if realm:sub(-#candidate.name) == candidate.name then
      return candidate.key
    end
  end
  return "unknown"
end

local function isNeutralZone()
  if C_Map and C_Map.GetBestMapForUnit then
    local ok, mapID = pcall(C_Map.GetBestMapForUnit, "player")
    if ok and mapID and NEUTRAL_MAPS[mapID] then
      return true
    end
  end
  local zone = GetZoneText and GetZoneText() or ""
  return NEUTRAL_ZONES[zone] == true
end

function TP:UpdateMarket()
  local realm = (GetRealmName and GetRealmName()) or "unknown"
  local ruleset = rulesetFromRealm(realm)
  local regionNumber = (GetCurrentRegion and GetCurrentRegion()) or 0
  local region = REGIONS[regionNumber] or ("region" .. tostring(regionNumber))

  local faction = UnitFactionGroup and UnitFactionGroup("player")
  local house = "unknown"
  if faction == "Alliance" then
    house = "alliance"
  elseif faction == "Horde" then
    house = "horde"
  end

  if self.atAuctionHouse and isNeutralZone() then
    house = "neutral"
  end

  if self.config and self.config.houseOverride then
    house = self.config.houseOverride
  end

  self.market = {
    realm = realm,
    ruleset = ruleset,
    region = region,
    regionNumber = regionNumber,
    faction = faction,
    house = house,
    key = region .. ":" .. ruleset .. ":" .. house,
  }
  return self.market
end

function TP:MarketKey()
  return self.market and self.market.key or "unknown"
end

function TP:MarketLabel()
  local market = self.market
  if not market then
    return "unknown"
  end
  return string.format("%s / %s / %s", market.region, market.ruleset, market.house)
end
