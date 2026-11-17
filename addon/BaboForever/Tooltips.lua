local ADDON, BF = ...

function BF:GetPrice(keys)
  local localEntry = self:GetLocalPrice(keys)
  local liveEntry, _, live = self:GetLivePrice(keys)
  local centralEntry, _, centralMarket = self:GetCentralPrice(keys)

  local best, source, market
  local function consider(entry, entrySource, entryMarket)
    if entry and entry.p then
      if not best or (entry.t or 0) > (best.t or 0) then
        best = entry
        source = entrySource
        market = entryMarket
      end
    end
  end

  consider(liveEntry, "live", live)
  consider(localEntry, "local")
  consider(centralEntry, "central", centralMarket)

  return best, source, market
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
  local price = self:MoneyText(entry.p)
  if WHITE_FONT_COLOR then
    price = WHITE_FONT_COLOR:WrapTextInColorCode(price)
  else
    price = "|cffffffff" .. price .. "|r"
  end

  -- Extra information stays on the left; the price goes in the right column so
  -- it lines up with Vendor/Auction and the other right-aligned values.
  local left = "|cff33ff99BF|r"
  if self.config.showAge then
    left = left .. "  |c" .. BF:AgeColor(age) .. "(" .. BF:AgeString(age) .. ")|r"
  end
  if self.config.showSupply and entry.q then
    left = left .. "  |cff9ca3af" .. tostring(entry.q) .. "x|r"
  end
  if source == "central" then
    left = left .. "  |cff60a5faC|r"
  elseif source == "live" then
    left = left .. "  |cff60a5faR|r"
  end

  if type(tooltip.AddDoubleLine) ~= "function" then
    return
  end
  tooltip:AddDoubleLine(left, price)
end

local function linkFromTooltip(tooltip, data)
  if data then
    local link = data.hyperlink or data.itemLink
    if link then
      return link
    end
  end
  if tooltip and type(tooltip.GetItem) == "function" then
    local ok, _, link = pcall(tooltip.GetItem, tooltip)
    if ok and link then
      return link
    end
  end
  return nil
end

local function safeTooltip(tooltip, data)
  if not BF.config or not BF.config.tooltips then
    return
  end

  local ok, err = pcall(function()
    local link = linkFromTooltip(tooltip, data)
    if link then
      BF:AddPriceLines(tooltip, link)
    end
  end)
  if not ok then
    BF:ReportError("tooltip", err)
    BF:SetConfig("tooltips", false)
    if not BF.tooltipErrorShown then
      BF.tooltipErrorShown = true
      BF:Print("tooltips disabled after an error; /bf tooltips on to retry")
    end
  end
end

if TooltipDataProcessor and Enum and Enum.TooltipDataType then
  TooltipDataProcessor.AddTooltipPostCall(Enum.TooltipDataType.Item, function(tooltip, data)
    safeTooltip(tooltip, data)
  end)
else
  local function hook(tooltip)
    if not tooltip then
      return
    end
    tooltip:HookScript("OnTooltipSetItem", function(frame)
      safeTooltip(frame)
    end)
  end

  hook(GameTooltip)
  hook(ItemRefTooltip)
  hook(ShoppingTooltip1)
  hook(ShoppingTooltip2)
end
