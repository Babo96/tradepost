local ADDON, TP = ...

local WEAPON = (Enum and Enum.ItemClass and Enum.ItemClass.Weapon) or 2
local ARMOR = (Enum and Enum.ItemClass and Enum.ItemClass.Armor) or 4

local classCache = {}
local ilvlCache = {}
local ilvlCacheCount = 0
local ILVL_CACHE_LIMIT = 20000

local function isEquipmentClass(classID)
  return classID == WEAPON or classID == ARMOR
end

function TP:ClassIDFor(itemID)
  local cached = classCache[itemID]
  if cached ~= nil then
    return cached or nil
  end
  local ok, classID = pcall(function()
    local _, _, _, _, _, id = C_Item.GetItemInfoInstant(itemID)
    return id
  end)
  if not ok then
    classID = nil
  end
  classCache[itemID] = classID or false
  return classID
end

function TP:ILvlFromLink(link)
  if not link then
    return nil
  end
  local cached = ilvlCache[link]
  if cached ~= nil then
    return cached or nil
  end
  local ok, ilvl = pcall(C_Item.GetDetailedItemLevelInfo, link)
  if not ok then
    ilvl = nil
  end
  if ilvlCacheCount < ILVL_CACHE_LIMIT then
    ilvlCache[link] = ilvl or false
    ilvlCacheCount = ilvlCacheCount + 1
  end
  return ilvl
end

local function itemIDFromLink(link)
  if not link then
    return nil
  end
  local itemID = C_Item.GetItemInfoInstant(link)
  if itemID then
    return itemID
  end
  local parsed = string.match(link, "item:(%d+)")
  return parsed and tonumber(parsed) or nil
end

-- Returns the primary DB key, itemID and itemLevel for one auction entry.
function TP:KeyFromAuction(index, info)
  local itemID = info[17]
  if not itemID then
    return nil
  end

  local classID = self:ClassIDFor(itemID)
  if not isEquipmentClass(classID) then
    return "i:" .. itemID, itemID, 0
  end

  local link = C_AuctionHouse.GetReplicateItemLink(index)
  local itemLevel = self:ILvlFromLink(link)
  if itemLevel and itemLevel > 0 then
    return "g:" .. itemID .. ":" .. itemLevel, itemID, itemLevel
  end
  return "g:" .. itemID .. ":0", itemID, 0
end

-- Returns keys to try for a tooltip link, most specific first.
function TP:KeysFromLink(link)
  local itemID = itemIDFromLink(link)
  if not itemID then
    return {}
  end

  local _, _, _, _, _, classID = C_Item.GetItemInfoInstant(link)
  if not classID then
    classID = self:ClassIDFor(itemID)
  end

  if isEquipmentClass(classID) then
    local itemLevel = self:ILvlFromLink(link)
    if itemLevel and itemLevel > 0 then
      return {
        "g:" .. itemID .. ":" .. itemLevel,
        "g:" .. itemID .. ":0",
        "i:" .. itemID,
      }
    end
    return { "g:" .. itemID .. ":0", "i:" .. itemID }
  end

  return { "i:" .. itemID }
end
