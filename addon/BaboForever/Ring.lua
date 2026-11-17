local ADDON, BF = ...

local WINDOW = 300
local DEFAULT_SLOTS = 64

local function slotName(index)
  return string.format("BaboForever_Data_%02d", index)
end

local function isAddOnLoaded(name)
  if C_AddOns and C_AddOns.IsAddOnLoaded then
    local loading, loaded = C_AddOns.IsAddOnLoaded(name)
    return loading or loaded
  end
  return IsAddOnLoaded and IsAddOnLoaded(name) or false
end

function BF:LiveFor(marketKey)
  self.live = self.live or {}
  local live = self.live[marketKey]
  if not live then
    live = { seq = 0, generated = 0, window = nil, lookup = {}, pins = {} }
    self.live[marketKey] = live
  end
  return live
end

function BF:GetLivePrice(keys)
  local live = self.live and self.live[self:MarketKey()]
  if not live then
    return nil
  end
  for _, key in ipairs(keys) do
    local entry = live.lookup[key]
    if entry then
      return entry, key, live
    end
  end
  return nil
end

function BF:ApplyRing(data, targetWindow)
  if type(data) ~= "table" or data.schema ~= 1 then
    return false
  end
  if data.market ~= self:MarketKey() then
    return false
  end
  if data.window ~= targetWindow then
    return false
  end
  if not data.seq or data.seq <= 0 then
    return false
  end

  local live = self:LiveFor(data.market)
  -- Login codes can arrive in a tiny auth-only ring payload. Store it before
  -- the sequence check so a future slot with no new price data still works.
  if data.auth then
    live.auth = data.auth
  end
  if data.seq <= live.seq then
    return false
  end

  if data.base and live.base and data.base ~= live.base then
    -- A new base snapshot: discard old cumulative deltas.
    live.lookup = {}
  end
  if data.base then
    live.base = data.base
  end

  live.seq = data.seq
  live.generated = data.generated or time()
  live.window = data.window

  for _, item in ipairs(data.items or {}) do
    if item.k and item.p then
      live.lookup[item.k] = {
        p = item.p,
        q = item.q,
        n = item.n,
        t = item.t or live.generated,
      }
    end
  end

  for _, key in ipairs(data.removed or {}) do
    live.lookup[key] = nil
  end

  if data.pins then
    live.pins = data.pins
    if data.pins_updated then
      self:ApplyRemoteWatch(data.pins, data.pins_updated)
    end
  end

  self:Debug(string.format(
    "ring applied window=%s seq=%s items=%d",
    tostring(data.window), tostring(data.seq), #(data.items or {})
  ))
  return true
end

function BF:LoadRingWindow(window, force)
  if not self.config.ring then
    return false
  end

  self.ring = self.ring or {
    lastWindow = nil,
    lastSeq = 0,
    wrapWarned = false,
  }

  local count = tonumber(self.config.ringSlots) or DEFAULT_SLOTS
  if count < 1 then
    return false
  end

  local index = window % count
  local name = slotName(index)

  if isAddOnLoaded(name) then
    if force and self.ring and self.ring.lastWindow and window > self.ring.lastWindow + 1 then
      self:RingWarnWrap()
    end
    return false
  end

  local loader = (C_AddOns and C_AddOns.LoadAddOn) or LoadAddOn
  if not loader then
    return false
  end

  BABOFOREVER_RING = nil
  local ok, loaded, reason = pcall(loader, name)
  local data = BABOFOREVER_RING
  BABOFOREVER_RING = nil

  if not ok then
    self:ReportError("ring-load", loaded)
    return false
  end
  if not loaded then
    if reason and reason ~= "MISSING" and reason ~= "DISABLED" then
      self:Debug("ring slot " .. name .. ": " .. tostring(reason))
    end
    return false
  end

  self.ring.lastWindow = window

  if type(data) ~= "table" then
    self:Debug("ring slot " .. name .. " has no payload")
    return false
  end
  if data.window ~= window then
    self:Debug(string.format(
      "ring slot %s stale window %s (wanted %s)",
      name, tostring(data.window), tostring(window)
    ))
    return false
  end

  local applied = self:ApplyRing(data, window)
  if applied then
    self.ring.lastSeq = data.seq
  end
  return applied
end

function BF:RingWarnWrap()
  if self.ring.wrapWarned then
    return
  end
  self.ring.wrapWarned = true
  self:Print("live data ring wrapped; /reload to continue live updates")
end

function BF:RingTick()
  if not self.config.ring then
    return
  end

  local window = tonumber(self.config.ringWindow) or WINDOW
  local now = time()
  local target = math.floor((now - 1) / window)
  self:LoadRingWindow(target, false)

  local delay = window - (now % window) + 1
  self.ring.timer = C_Timer.NewTimer(delay, function()
    self:RingTick()
  end)
end

function BF:InitRing()
  self.ring = self.ring or {
    lastWindow = nil,
    lastSeq = 0,
    wrapWarned = false,
  }
  if not self.config.ring then
    return
  end
  if self.ring.timer then
    self.ring.timer:Cancel()
    self.ring.timer = nil
  end

  local window = tonumber(self.config.ringWindow) or WINDOW
  local now = time()
  local target = math.floor((now - 1) / window)
  self:LoadRingWindow(target, false)

  local delay = window - (now % window) + 1
  self.ring.timer = C_Timer.NewTimer(delay, function()
    self:RingTick()
  end)
end

function BF:RingNow(window)
  window = tonumber(window)
  if not window then
    window = math.floor(time() / (tonumber(self.config.ringWindow) or WINDOW))
  end

  local live = self.live and self.live[self:MarketKey()]
  if live and live.window == window then
    self:Print("ring window " .. window .. " already applied (seq " .. tostring(live.seq) .. ")")
    return
  end

  local applied = self:LoadRingWindow(window, true)
  if applied then
    self:Print("ring loaded window " .. window)
  elseif self.ring and self.ring.lastWindow == window then
    self:Print("ring window " .. window .. " already loaded")
  else
    self:Print("ring window " .. window .. " not applied (missing, stale, or disabled)")
  end
end

function BF:RingStatus()
  local live = self.live and self.live[self:MarketKey()]
  local count = 0
  if live then
    for _ in pairs(live.lookup) do
      count = count + 1
    end
  end
  self:Print("ring: " .. (self.config.ring and "on" or "off") ..
    "  slots=" .. tostring(self.config.ringSlots) ..
    "  window=" .. tostring(self.config.ringWindow) ..
    "  lastWindow=" .. tostring(self.ring and self.ring.lastWindow))
  if live then
    self:Print("live: seq=" .. tostring(live.seq) ..
      "  items=" .. count ..
      "  generated=" .. tostring(live.generated))
  end
end

function BF:RingSetCount(count)
  count = tonumber(count)
  if not count or count < 1 then
    self:Print("usage: /bf ring count <n>")
    return
  end
  self:SetConfig("ringSlots", count)
  self.ring.lastWindow = nil
  self.ring.lastSeq = 0
  self:Print("ring slots set to " .. count .. " (reload to reset loaded slots)")
end
