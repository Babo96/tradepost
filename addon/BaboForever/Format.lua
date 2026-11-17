local ADDON, BF = ...

local GOLD_ICON = "|TInterface\\MoneyFrame\\UI-GoldIcon:12:12:0:0|t"
local SILVER_ICON = "|TInterface\\MoneyFrame\\UI-SilverIcon:12:12:0:0|t"
local COPPER_ICON = "|TInterface\\MoneyFrame\\UI-CopperIcon:12:12:0:0|t"

function BF:MoneyText(copper)
  copper = math.max(0, math.floor(copper or 0))
  local gold = math.floor(copper / 10000)
  local silver = math.floor((copper % 10000) / 100)
  local rest = copper % 100

  local result = tostring(rest) .. " " .. COPPER_ICON
  if gold ~= 0 or silver ~= 0 then
    if rest < 10 then
      result = "0" .. result
    end
    result = tostring(silver) .. " " .. SILVER_ICON .. " " .. result
  end
  if gold ~= 0 then
    if silver < 10 then
      result = "0" .. result
    end
    result = tostring(gold) .. " " .. GOLD_ICON .. " " .. result
  end
  return result
end

function BF:AgeString(seconds)
  seconds = math.max(0, seconds or 0)
  if seconds < 60 then
    return "<1m"
  elseif seconds < 3600 then
    return string.format("%dm", math.floor(seconds / 60))
  elseif seconds < 86400 then
    return string.format("%dh", math.floor(seconds / 3600))
  end
  return string.format("%dd", math.floor(seconds / 86400))
end

function BF:AgeColor(seconds)
  seconds = math.max(0, seconds or 0)
  if seconds < 900 then
    return "ff22c55e"
  elseif seconds < 3600 then
    return "ffeab308"
  end
  return "ff9ca3af"
end

function BF:SourceLabel(source)
  if source == "live" then
    return "R"
  elseif source == "central" then
    return "C"
  elseif source == "local" then
    return "L"
  end
  return nil
end
