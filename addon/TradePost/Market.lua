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

TP.RULESET_KEYS = {
  pve = true,
  pvp = true,
  rp = true,
  ["rp-pvp"] = true,
  hc = true,
}

local function cleanName(name)
  if type(name) ~= "string" then
    return ""
  end
  return (name:gsub("^%s+", ""):gsub("%s+$", ""))
end

-- Suffix match against every ruleset, case-insensitively. RP-PvP is checked
-- before PvP so it wins on names ending in both.
local function rulesetFromName(name)
  name = string.lower(cleanName(name))
  if name == "" then
    return nil
  end
  for _, candidate in ipairs(RULESETS) do
    local suffix = string.lower(candidate.name)
    if #name >= #suffix and name:sub(-#suffix) == suffix then
      return candidate.key
    end
  end
  return nil
end

-- The Forever client does not always populate GetRealmName(): it can return
-- an empty string while the normalized name or the realm list still carries
-- the "... PvE" suffix. Collect every source instead of trusting one.
local function realmCandidates()
  local names = {}
  local seen = {}
  local function add(name)
    name = cleanName(name)
    if name ~= "" and not seen[name] then
      seen[name] = true
      names[#names + 1] = name
    end
  end
  if GetRealmName then
    add(GetRealmName())
  end
  if GetNormalizedRealmName then
    add(GetNormalizedRealmName())
  end
  if GetAutoCompleteRealms then
    local ok, realms = pcall(GetAutoCompleteRealms)
    if ok and type(realms) == "table" then
      for _, name in ipairs(realms) do
        add(name)
      end
    end
  end
  return names
end

-- Last resort: infer the ruleset from markets this install has data for
-- (local scans and the central data addon). Only a single unambiguous ruleset
-- is accepted, so a real multi-ruleset server cannot be guessed wrong.
local function rulesetFromKnownMarkets(region, house)
  local found
  local ambiguous = false
  local function consider(key)
    if type(key) ~= "string" then
      return
    end
    local r, ruleset, h = key:match("^([^:]+):([^:]+):([^:]+)$")
    if not r or r ~= region or ruleset == "unknown" then
      return
    end
    if h ~= house and h ~= "neutral" and house ~= "neutral" then
      return
    end
    if found and found ~= ruleset then
      ambiguous = true
      return
    end
    found = ruleset
  end
  for key in pairs((TRADEPOST_DB and TRADEPOST_DB.markets) or {}) do
    consider(key)
  end
  for key in pairs((TRADEPOST_DATA and TRADEPOST_DATA.markets) or {}) do
    consider(key)
  end
  if ambiguous then
    return nil
  end
  return found
end

function TP:DetectRuleset(region, house)
  local override = self.config and self.config.rulesetOverride
  if override and override ~= "" and override ~= "auto" then
    return override, "override"
  end
  for _, name in ipairs(realmCandidates()) do
    local ruleset = rulesetFromName(name)
    if ruleset then
      return ruleset, "auto"
    end
  end
  local known = rulesetFromKnownMarkets(region, house)
  if known then
    return known, "known market"
  end
  return "unknown", "auto"
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
  local names = realmCandidates()
  local realm = names[1] or "unknown"
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

  local ruleset, rulesetSource = self:DetectRuleset(region, house)

  local previous = self.market and self.market.key
  self.market = {
    realm = realm,
    realmRaw = cleanName(GetRealmName and GetRealmName() or ""),
    realmCandidates = names,
    ruleset = ruleset,
    rulesetSource = rulesetSource,
    region = region,
    regionNumber = regionNumber,
    faction = faction,
    house = house,
    key = region .. ":" .. ruleset .. ":" .. house,
  }
  -- Character/faction/ruleset changes move the live ring to another market.
  -- Tell the companion immediately so it selects the new snapshot.
  if previous and previous ~= self.market.key and self.MarkActive then
    self:MarkActive()
  end
  return self.market
end

function TP:SetRulesetOverride(value)
  value = string.lower(cleanName(value or ""))
  if value == "" or value == "auto" then
    self:SetConfig("rulesetOverride", "auto")
    self:UpdateMarket()
    return true
  end
  if not TP.RULESET_KEYS[value] then
    return false
  end
  self:SetConfig("rulesetOverride", value)
  self:UpdateMarket()
  return true
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
