local ADDON, TP = ...

-- The companion installs a tiny TradePost_Companion status addon containing:
--   TRADEPOST_COMPANION = { online = true/false, updated = <unix time>,
--                           addon_latest = "0.2.7", addon_url = "..." }
-- It is written when the companion comes online, on shutdown, on a reload
-- request, and when its update check finds a new addon release. No heartbeat.

local function versionParts(value)
  local text = tostring(value or ""):gsub("^v", ""):gsub("[^%d.].*$", "")
  local major, minor, patch = text:match("^(%d+)%.?(%d*)%.?(%d*)")
  return tonumber(major) or 0, tonumber(minor) or 0, tonumber(patch) or 0
end

local function isNewer(a, b)
  local am, an, ap = versionParts(a)
  local bm, bn, bp = versionParts(b)
  if am ~= bm then
    return am > bm
  end
  if an ~= bn then
    return an > bn
  end
  return ap > bp
end

function TP:LoadCompanion()
  local status = TRADEPOST_COMPANION
  local present = type(status) == "table"
  self.companion = {
    present = present,
    running = present and status.online == true,
    version = present and status.version or nil,
    addon_latest = present and status.addon_latest or nil,
    addon_url = present and status.addon_url or nil,
  }
end

function TP:CheckCompanion()
  self:LoadCompanion()
  return self:CompanionReady()
end

function TP:CompanionReady()
  return self.companion ~= nil and self.companion.running == true
end

function TP:AddonLatest()
  return self.companion and self.companion.addon_latest
end

function TP:AddonUpdateAvailable()
  local latest = self:AddonLatest()
  if not latest or latest == "" then
    return false
  end
  return isNewer(latest, self.version)
end
