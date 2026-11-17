local ADDON, BF = ...

local function ageString(seconds)
  if seconds < 60 then
    return "<1m"
  elseif seconds < 3600 then
    return string.format("%dm", math.floor(seconds / 60))
  elseif seconds < 86400 then
    return string.format("%dh", math.floor(seconds / 3600))
  end
  return string.format("%dd", math.floor(seconds / 86400))
end

local function ageColor(seconds)
  if seconds < 900 then
    return "ff22c55e"
  elseif seconds < 3600 then
    return "ffeab308"
  end
  return "ff9ca3af"
end

local function moneyText(copper)
  copper = math.max(0, math.floor(copper or 0))
  if GetCoinTextureString then
    return GetCoinTextureString(copper)
  end
  local gold = math.floor(copper / 10000)
  local silver = math.floor((copper % 10000) / 100)
  local rest = copper % 100
  return string.format("%dg %ds %dc", gold, silver, rest)
end

function BF:GetPrice(keys)
  local localEntry = self:GetLocalPrice(keys)
  local centralEntry, _, centralMarket = self:GetCentralPrice(keys)

  if localEntry and centralEntry then
    if (centralEntry.t or 0) > (localEntry.t or 0) then
      return centralEntry, "central", centralMarket
    end
    return localEntry, "local"
  end
  if localEntry then
    return localEntry, "local"
  end
  if centralEntry then
    return centralEntry, "central", centralMarket
  end
  return nil
end

function BF:AddPriceLines(tooltip, link)
  if not self.config or not self.config.tooltips then
    return
  end
  if not link then
    return
  end

  local keys = self:KeysFromLink(link)
  if #keys == 0 then
    return
  end

  local entry, source = self:GetPrice(keys)
  if not entry or not entry.p then
    return
  end

  local age = math.max(0, time() - (entry.t or 0))
  local text = moneyText(entry.p)

  if self.config.showAge then
    text = text .. "  |c" .. ageColor(age) .. "(" .. ageString(age) .. ")|r"
  end

  if self.config.showSupply and entry.q then
    text = text .. "  |cff9ca3af" .. tostring(entry.q) .. "x|r"
  end

  if source == "central" then
    text = text .. "  |cff60a5faC|r"
  end

  tooltip:AddDoubleLine("|cff33ff99BF|r", text)
end

if TooltipDataProcessor and Enum and Enum.TooltipDataType then
  TooltipDataProcessor.AddTooltipPostCall(Enum.TooltipDataType.Item, function(tooltip)
    local _, link = tooltip:GetItem()
    BF:AddPriceLines(tooltip, link)
  end)
else
  local function hook(tooltip)
    if not tooltip then
      return
    end
    tooltip:HookScript("OnTooltipSetItem", function(frame)
      local _, link = frame:GetItem()
      BF:AddPriceLines(frame, link)
    end)
  end

  hook(GameTooltip)
  hook(ItemRefTooltip)
  hook(ShoppingTooltip1)
  hook(ShoppingTooltip2)
end
