local ADDON, TP = ...

-- The companion installs a tiny TradePost_Companion status addon containing:
--   TRADEPOST_COMPANION = { online = true/false, updated = <unix time> }
-- It is written once when the companion comes online, once on shutdown, and
-- once when a reload request asks for it. There is deliberately no heartbeat.

function TP:LoadCompanion()
  local status = TRADEPOST_COMPANION
  local present = type(status) == "table"
  self.companion = {
    present = present,
    running = present and status.online == true,
    version = present and status.version or nil,
  }
end

function TP:CheckCompanion()
  self:LoadCompanion()
  return self:CompanionReady()
end

function TP:CompanionReady()
  return self.companion ~= nil and self.companion.running == true
end
