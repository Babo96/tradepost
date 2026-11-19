local ADDON, TP = ...

-- Advanced/admin item metadata backfill.
--
-- This module walks every item id the addon already knows about and stores a
-- compact copy of its full tooltip in TRADEPOST_ITEM_DETAILS. Items are handed
-- to the companion in batches of BATCH_SIZE through TRADEPOST_META_BATCH. The
-- companion acknowledges each batch by loading one of the TradePost_Ack_XX
-- LoadOnDemand addons, which sets TRADEPOST_META_ACK. We never advance to the
-- next batch without that acknowledgement, so a slow/flaky companion cannot
-- silently drop tooltip data.

local BATCH_SIZE = 500
local TICK_INTERVAL = 0.1 -- seconds
local PER_TICK = 2        -- 20 items/sec
local ACK_POLL_SECONDS = 10
local ACK_TIMEOUT_SECONDS = 300
local ACK_SLOTS = 16
local DEFAULT_ICON = 134400 -- inv_misc_questionmark

TRADEPOST_ITEM_DETAILS = TRADEPOST_ITEM_DETAILS or {}
TRADEPOST_META_QUEUE = TRADEPOST_META_QUEUE or {
  ids = {},
  index = 1,
  total = 0,
  nextSeq = 1,
  synced = 0,
}
TRADEPOST_META_QUEUE.ids = TRADEPOST_META_QUEUE.ids or {}
TRADEPOST_META_QUEUE.index = TRADEPOST_META_QUEUE.index or 1
TRADEPOST_META_QUEUE.total = TRADEPOST_META_QUEUE.total or #TRADEPOST_META_QUEUE.ids
TRADEPOST_META_QUEUE.nextSeq = TRADEPOST_META_QUEUE.nextSeq or 1
TRADEPOST_META_QUEUE.synced = TRADEPOST_META_QUEUE.synced or 0
TRADEPOST_META_BATCH = TRADEPOST_META_BATCH or nil

TP.meta = TP.meta or {
  running = false,
  waitingAck = false,
  paused = false,
}

local function validID(id)
  return type(id) == "number" and id > 0 and id == math.floor(id)
end

local function keyID(key)
  if type(key) ~= "string" then
    return nil
  end
  local id = key:match("^i:(%d+)$")
  if not id then
    id = key:match("^g:(%d+):%d+$")
  end
  if not id then
    id = key:match("^p:(%d+)")
  end
  local number = tonumber(id)
  if validID(number) then
    return number
  end
  return nil
end

-- Build the id queue from the item cache and every locally scanned market.
-- Sorted so a reload/resume processes the same order.
local function collectIDs()
  local seen = {}
  for id in pairs(TRADEPOST_ITEMS or {}) do
    local number = tonumber(id)
    if validID(number) then
      seen[number] = true
    end
  end
  if TRADEPOST_DB and TRADEPOST_DB.markets then
    for _, db in pairs(TRADEPOST_DB.markets) do
      for key in pairs(db.prices or {}) do
        local number = keyID(key)
        if number then
          seen[number] = true
        end
      end
    end
  end
  local ids = {}
  for id in pairs(seen) do
    ids[#ids + 1] = id
  end
  table.sort(ids)
  return ids
end

local function numberOrNil(value)
  if type(value) == "number" then
    return value
  end
  return nil
end

local function stringOrNil(value)
  if type(value) == "string" and value ~= "" then
    return value
  end
  return nil
end

local function boolOrNil(value)
  if type(value) == "boolean" then
    return value
  end
  return nil
end

local function readStats(link)
  local stats = {}
  if not link or not C_Item or type(C_Item.GetItemStats) ~= "function" then
    return stats
  end
  local ok, result = pcall(C_Item.GetItemStats, link)
  if not ok or type(result) ~= "table" then
    return stats
  end
  for key, value in pairs(result) do
    -- Tooltip stat tables contain numbers; drop the occasional boolean flag.
    if type(key) == "string" and type(value) == "number" then
      stats[key] = value
    end
  end
  return stats
end

-- C_TooltipInfo is the only reliable source for the full rendered tooltip.
-- Keep plain text only: no textures, fonts, colors or raw tooltip objects.
local function readDetailLines(id)
  local lines = {}
  local fn = C_TooltipInfo and C_TooltipInfo.GetItemByID
  if type(fn) ~= "function" then
    return { lines = lines }
  end
  local ok, data = pcall(fn, id)
  if not ok or type(data) ~= "table" then
    ok, data = pcall(fn, { itemID = id })
  end
  if not ok or type(data) ~= "table" then
    return { lines = lines }
  end
  for _, line in ipairs(data.lines or {}) do
    local left = stringOrNil(line.leftText)
    local right = stringOrNil(line.rightText)
    local lineType = tonumber(line.type)
    if left or right then
      lines[#lines + 1] = {
        left = left,
        right = right,
        type = lineType,
      }
    end
  end
  return { lines = lines }
end

local function readSetID(id, link)
  if not C_Item then
    return nil
  end
  local fn = C_Item.GetItemSetInfo or C_Item.GetItemSetID
  if type(fn) ~= "function" then
    return nil
  end
  local ok, result = pcall(fn, link or id)
  if not ok then
    return nil
  end
  if type(result) == "number" and result > 0 then
    return result
  end
  if type(result) == "table" then
    local setID = tonumber(result.setID or result.id or result[1])
    if setID and setID > 0 then
      return setID
    end
  end
  return nil
end

local function readInventoryType(id)
  if not C_Item then
    return nil
  end
  local fn = C_Item.GetItemInventoryType
  if type(fn) ~= "function" then
    return nil
  end
  local ok, result = pcall(fn, id)
  if ok then
    return numberOrNil(result)
  end
  return nil
end

local function hasUniqueLine(lines)
  local uniqueText = _G.ITEM_UNIQUE or "Unique"
  local uniqueEquipped = _G.ITEM_UNIQUE_EQUIPPABLE or "Unique-Equipped"
  for _, line in ipairs(lines or {}) do
    local left = line.left
    if left then
      if uniqueText and (left == uniqueText or left:find(uniqueText, 1, true)) then
        return true
      end
      if uniqueEquipped and left == uniqueEquipped then
        return true
      end
    end
  end
  return nil
end

-- Build one compact detail record for an item id. Returns nil for ids the
-- client cannot resolve (they are excluded from the batch rather than uploaded
-- with an empty name, which the server rejects).
function TP:MetaCollect(id)
  local cached = TRADEPOST_ITEMS and TRADEPOST_ITEMS[id]
  local name, link, quality, itemLevel, requiredLevel, classID, subclassID
  local maxStack, equipLoc, icon, vendorPrice, bindType

  if C_Item and type(C_Item.GetItemInfo) == "function" then
    -- GetItemInfo: name, link, quality, itemLevel, itemMinLevel, itemType,
    -- itemSubType, stackCount, equipLoc, icon, sellPrice, classID,
    -- subclassID, bindType. Positions 6/7 are localized strings, not the
    -- numeric class/subclass ids.
    local ok
    ok, name, link, quality, itemLevel, requiredLevel, _, _,
      maxStack, equipLoc, icon, vendorPrice, classID, subclassID, bindType =
      pcall(C_Item.GetItemInfo, id)
    if not ok then
      name = nil
    end
  end
  if not link and C_Item and type(C_Item.GetItemLink) == "function" then
    local ok, value = pcall(C_Item.GetItemLink, id)
    if ok then
      link = stringOrNil(value)
    end
  end

  if C_Item and type(C_Item.GetItemInfoInstant) == "function" then
    -- GetItemInfoInstant: itemID, itemType, itemSubType, itemEquipLoc,
    -- icon, classID, subclassID.
    local ok, instantID, instantType, instantSubType, instantEquipLoc,
      instantIcon, instantClass, instantSubclass =
      pcall(C_Item.GetItemInfoInstant, id)
    if ok then
      if not equipLoc then
        equipLoc = stringOrNil(instantEquipLoc)
      end
      if not icon and type(instantIcon) == "number" then
        icon = instantIcon
      end
      if not classID and type(instantClass) == "number" then
        classID = instantClass
      end
      if not subclassID and type(instantSubclass) == "number" then
        subclassID = instantSubclass
      end
    end
  end

  if not name then
    name = cached and cached.n
  end
  if not name then
    return nil
  end

  if not quality and cached then
    quality = cached.q
  end
  if not icon and cached then
    icon = cached.i
  end
  if not classID and cached then
    classID = cached.c
  end
  if not subclassID and cached then
    subclassID = cached.s
  end

  local detailLines = readDetailLines(id)
  local stats = readStats(link)

  local armor = numberOrNil(stats["ITEM_MOD_ARMOR"])
    or numberOrNil(stats["ITEM_MOD_ARMOR_SHORT"])

  return {
    id = id,
    name = name,
    quality = numberOrNil(quality) or 1,
    class_id = numberOrNil(classID),
    subclass_id = numberOrNil(subclassID),
    icon = numberOrNil(icon) or DEFAULT_ICON,
    item_level = numberOrNil(itemLevel),
    required_level = numberOrNil(requiredLevel),
    bind_type = bindType,
    inventory_type = readInventoryType(id),
    equip_loc = stringOrNil(equipLoc),
    armor = armor,
    vendor_price = numberOrNil(vendorPrice),
    max_stack = numberOrNil(maxStack),
    unique_item = hasUniqueLine(detailLines.lines),
    set_id = readSetID(id, link),
    stats = stats,
    details = detailLines,
  }
end

-- Rebuild TRADEPOST_META_QUEUE. Force mode starts a fresh pass over every id.
function TP:MetaQueueBuild(force)
  local queue = TRADEPOST_META_QUEUE
  local ids = collectIDs()
  queue.ids = ids
  queue.index = 1
  queue.total = #ids
  queue.completed = false
  queue.force = force and true or false
  queue.nextSeq = queue.nextSeq or 1
  if force then
    queue.synced = 0
  end
  self.meta.pending = nil
  return #ids
end

function TP:MetaStopProcessTicker()
  if self.meta.processTicker then
    self.meta.processTicker:Cancel()
    self.meta.processTicker = nil
  end
end

function TP:MetaStopAckTicker()
  if self.meta.ackTicker then
    self.meta.ackTicker:Cancel()
    self.meta.ackTicker = nil
  end
end

function TP:MetaEnsureProcessTicker()
  if self.meta.processTicker or not C_Timer then
    return
  end
  self.meta.processTicker = C_Timer.NewTicker(TICK_INTERVAL, function()
    local ok, err = pcall(TP.MetaProcessTick, TP)
    if not ok then
      TP:ReportError("meta-process", err)
    end
  end)
end

function TP:MetaEnsureAckTicker()
  if self.meta.ackTicker or not C_Timer then
    return
  end
  self.meta.ackTicker = C_Timer.NewTicker(1, function()
    local ok, err = pcall(TP.MetaAckTick, TP)
    if not ok then
      TP:ReportError("meta-ack", err)
    end
  end)
end

function TP:MetaFinalizeBatch()
  local pending = self.meta.pending or {}
  self.meta.pending = nil
  if #pending == 0 then
    return false
  end
  local queue = TRADEPOST_META_QUEUE
  local seq = tonumber(queue.nextSeq) or 1
  queue.nextSeq = seq + 1
  TRADEPOST_META_BATCH = {
    seq = seq,
    count = #pending,
    items = pending,
  }
  self.meta.batchSeq = seq
  self.meta.batchCount = #pending
  self.meta.waitingAck = true
  self.meta.ackStarted = time()
  self.meta.ackTimedOut = false
  self.meta.paused = false
  -- Do not burn an ack slot before the companion has had a chance to write
  -- the acknowledgement; the first poll happens ACK_POLL_SECONDS later.
  self.meta.lastAckPoll = time()
  self.meta.ackIndex = 0
  self:MetaStopProcessTicker()
  self:SaveExport()
  -- The Forever client defers/throttles SavedVariables writes. Retry the
  -- flush so the companion can see the batch without a manual /reload.
  if self.ScheduleExportRetry then
    self:ScheduleExportRetry()
  end
  self:MetaEnsureAckTicker()
  self:Print("metadata batch " .. seq .. " ready (" .. #pending ..
    " items). Waiting for the companion before continuing.")
  return true
end

function TP:MetaFinish(reason)
  self:MetaStopProcessTicker()
  self:MetaStopAckTicker()
  self.meta.running = false
  self.meta.waitingAck = false
  self.meta.paused = false
  self.meta.pending = nil
  self.meta.batchSeq = nil
  self.meta.batchCount = nil
  self.meta.ackStarted = nil
  self.meta.ackTimedOut = false
  self.meta.lastAckPoll = nil
  local queue = TRADEPOST_META_QUEUE
  if (queue.index or 1) > (queue.total or 0) then
    queue.completed = true
  end
  self:SaveExport()
  self:Print("metadata sync " .. (reason or "finished") .. " (" ..
    tostring(queue.synced or 0) .. " synced)")
  if self.settings and self.settings.refresh then
    self.settings.refresh()
  end
end

function TP:MetaAckReceived()
  local meta = self.meta
  local count = meta.batchCount
    or (TRADEPOST_META_BATCH and TRADEPOST_META_BATCH.count)
    or 0
  TRADEPOST_META_BATCH = nil
  local queue = TRADEPOST_META_QUEUE
  queue.synced = (queue.synced or 0) + count
  meta.waitingAck = false
  meta.paused = false
  meta.ackTimedOut = false
  meta.ackStarted = nil
  meta.lastAckPoll = nil
  meta.batchSeq = nil
  meta.batchCount = nil
  self:SaveExport()
  self:Print("metadata batch acknowledged (" .. count .. " items, " ..
    tostring(queue.synced) .. " synced)")
  if (queue.index or 1) <= (queue.total or 0) then
    meta.running = true
    self:MetaEnsureProcessTicker()
  else
    self:MetaFinish("complete")
  end
  if self.settings and self.settings.refresh then
    self.settings.refresh()
  end
end

local function isAddonLoaded(name)
  if C_AddOns and type(C_AddOns.IsAddOnLoaded) == "function" then
    return C_AddOns.IsAddOnLoaded(name) and true or false
  end
  if type(IsAddOnLoaded) == "function" then
    local result = IsAddOnLoaded(name)
    if type(result) == "boolean" then
      return result
    end
    return result ~= nil and result ~= false
  end
  return false
end

local function loadAddon(name)
  if C_AddOns and type(C_AddOns.LoadAddOn) == "function" then
    return C_AddOns.LoadAddOn(name)
  end
  if type(LoadAddOn) == "function" then
    return LoadAddOn(name)
  end
  return false
end

-- Find one ack slot that this client has not loaded yet. Loading a slot makes
-- its Data.lua run and defines TRADEPOST_META_ACK. The companion publishes the
-- acknowledgement into every slot, so any unloaded slot works; we prefer the
-- batch-specific slot so a stale slot is never consumed needlessly.
function TP:MetaNextAckSlot()
  local meta = self.meta
  if meta.batchSeq then
    local index = meta.batchSeq % ACK_SLOTS
    local name = string.format("TradePost_Ack_%02d", index)
    if not isAddonLoaded(name) then
      meta.ackIndex = (index + 1) % ACK_SLOTS
      return name
    end
  end
  for offset = 0, ACK_SLOTS - 1 do
    local index = ((meta.ackIndex or 0) + offset) % ACK_SLOTS
    local name = string.format("TradePost_Ack_%02d", index)
    if not isAddonLoaded(name) then
      meta.ackIndex = (index + 1) % ACK_SLOTS
      return name
    end
  end
  return nil
end

function TP:MetaAckTick()
  local meta = self.meta
  if not meta.running or not meta.waitingAck then
    return
  end
  -- Never touch ack slots unless the admin mode is still enabled.
  if not (self.config and self.config.adminFullSync) then
    self:MetaStop()
    return
  end

  local now = time()
  if not meta.lastAckPoll or (now - meta.lastAckPoll) >= ACK_POLL_SECONDS then
    meta.lastAckPoll = now
    local name = self:MetaNextAckSlot()
    if name then
      local ok, err = pcall(loadAddon, name)
      if not ok then
        self:Debug("ack slot " .. name .. ": " .. tostring(err))
      end
    end
  end

  local ack = TRADEPOST_META_ACK
  if type(ack) == "table" and tonumber(ack.batch) == meta.batchSeq and ack.ok ~= false then
    self:MetaAckReceived()
    return
  end

  if not meta.ackTimedOut and meta.ackStarted
    and (now - meta.ackStarted) >= ACK_TIMEOUT_SECONDS then
    meta.ackTimedOut = true
    meta.paused = true
    self:Print("metadata batch " .. tostring(meta.batchSeq) ..
      " was not acknowledged after " .. ACK_TIMEOUT_SECONDS ..
      "s; paused and kept the batch. /tradepost meta status")
    if self.settings and self.settings.refresh then
      self.settings.refresh()
    end
  end
end

function TP:MetaProcessTick()
  local meta = self.meta
  if not meta.running or meta.waitingAck or meta.paused then
    return
  end
  local queue = TRADEPOST_META_QUEUE
  local ids = queue.ids or {}
  local pending = meta.pending or {}
  local processed = 0

  while (queue.index or 1) <= #ids and processed < PER_TICK do
    local id = ids[queue.index]
    queue.index = (queue.index or 1) + 1
    processed = processed + 1
    if validID(id) then
      local detail = self:MetaCollect(id)
      if detail then
        TRADEPOST_ITEM_DETAILS[id] = detail
        pending[#pending + 1] = detail
      end
    end
  end
  meta.pending = pending

  if #pending >= BATCH_SIZE or (queue.index or 1) > #ids then
    if #pending > 0 then
      self:MetaFinalizeBatch()
    else
      self:MetaFinish("complete")
    end
  end
end

-- Start/resume the backfill. Force starts a fresh pass over every known item.
function TP:MetaStart(force)
  if not (self.config and self.config.adminFullSync) then
    self:Print("admin full metadata sync is disabled. Enable it in " ..
      "/tradepost settings under Advanced / Admin first.")
    return
  end

  local meta = self.meta
  if force then
    -- Explicit admin action: discard the pending batch and start over.
    TRADEPOST_META_BATCH = nil
    TRADEPOST_ITEM_DETAILS = {}
    self:MetaQueueBuild(true)
  elseif TRADEPOST_META_BATCH and TRADEPOST_META_BATCH.seq then
    meta.running = true
    meta.paused = false
    meta.waitingAck = true
    meta.batchSeq = tonumber(TRADEPOST_META_BATCH.seq)
    meta.batchCount = TRADEPOST_META_BATCH.count
      or #(TRADEPOST_META_BATCH.items or {})
    meta.ackStarted = time()
    meta.ackTimedOut = false
    meta.lastAckPoll = nil
    self:MetaEnsureAckTicker()
    self:Print("resuming metadata batch " .. tostring(meta.batchSeq) ..
      "; waiting for the companion")
    return
  elseif not (TRADEPOST_META_QUEUE.ids and #TRADEPOST_META_QUEUE.ids > 0)
    or (TRADEPOST_META_QUEUE.index or 1) > (TRADEPOST_META_QUEUE.total or 0) then
    local count = self:MetaQueueBuild(false)
    if count == 0 then
      self:Print("no known items to sync yet; scan the auction house first.")
      return
    end
  end

  meta.running = true
  meta.waitingAck = false
  meta.paused = false
  meta.ackTimedOut = false
  meta.batchSeq = nil
  meta.batchCount = nil
  meta.ackStarted = nil
  meta.lastAckPoll = nil
  meta.pending = meta.pending or {}
  TRADEPOST_META_QUEUE.completed = false
  TRADEPOST_META_QUEUE.nextSeq = TRADEPOST_META_QUEUE.nextSeq or 1
  self:MetaEnsureProcessTicker()
  self:Print("metadata sync started: " .. tostring(TRADEPOST_META_QUEUE.total or 0) ..
    " queued, batch size " .. BATCH_SIZE)
  if self.settings and self.settings.refresh then
    self.settings.refresh()
  end
end

function TP:MetaStop()
  local meta = self.meta
  if meta.pending and #meta.pending > 0 then
    self:MetaFinalizeBatch()
  elseif TRADEPOST_META_BATCH and TRADEPOST_META_BATCH.seq then
    self:SaveExport()
  end
  self:MetaStopProcessTicker()
  self:MetaStopAckTicker()
  meta.running = false
  meta.waitingAck = false
  meta.paused = true
  self:SaveExport()
  self:Print("metadata sync stopped. Pending batch/queue data is kept.")
  if self.settings and self.settings.refresh then
    self.settings.refresh()
  end
end

-- Called on login. Resumes automatically when admin mode was left enabled.
function TP:MetaResume()
  if not (self.config and self.config.adminFullSync) then
    return
  end
  local batch = TRADEPOST_META_BATCH
  if batch and batch.seq then
    self:MetaStart(false)
    return
  end
  local queue = TRADEPOST_META_QUEUE
  if queue and queue.ids and #queue.ids > 0 and not queue.completed
    and (queue.index or 1) <= (queue.total or #queue.ids) then
    self:MetaStart(false)
  end
end

function TP:MetaDetailCount()
  local count = 0
  for _ in pairs(TRADEPOST_ITEM_DETAILS or {}) do
    count = count + 1
  end
  return count
end

function TP:MetaStatusText()
  local meta = self.meta or {}
  local queue = TRADEPOST_META_QUEUE or {}
  local batch = TRADEPOST_META_BATCH
  local total = queue.total or #(queue.ids or {})
  local done = math.max(0, (queue.index or 1) - 1)
  if done > total then
    done = total
  end

  local state = "off"
  if self.config and self.config.adminFullSync then
    if meta.waitingAck then
      state = meta.ackTimedOut and "waiting for ack (timed out)" or "waiting for ack"
    elseif meta.paused then
      state = "paused"
    elseif meta.running then
      state = "running"
    else
      state = "enabled (idle)"
    end
  end

  local text = string.format("Admin full sync: %s   Queued %d/%d   Synced %d   Details %d",
    state, done, total, queue.synced or 0, self:MetaDetailCount())
  if batch and batch.seq then
    text = text .. string.format("   Batch %d (%d items)",
      batch.seq, batch.count or #(batch.items or {}))
  elseif meta.batchSeq then
    text = text .. string.format("   Batch %d", meta.batchSeq)
  end
  return text
end

function TP:MetaStatus()
  self:Print(self:MetaStatusText())
  local queue = TRADEPOST_META_QUEUE
  self:Print("queue persists in TRADEPOST_META_QUEUE; details in TRADEPOST_ITEM_DETAILS")
  self:Print("batches require a companion ack via TradePost_Ack_00..15; " ..
    "newly installed ack addons are only discovered after a client restart")
  if queue and queue.completed then
    self:Print("last full pass completed; use /tradepost meta force to requeue every item")
  end
end

function TP:MetaInit()
  TRADEPOST_ITEM_DETAILS = TRADEPOST_ITEM_DETAILS or {}
  TRADEPOST_META_QUEUE = TRADEPOST_META_QUEUE or {}
  TRADEPOST_META_QUEUE.ids = TRADEPOST_META_QUEUE.ids or {}
  TRADEPOST_META_QUEUE.index = TRADEPOST_META_QUEUE.index or 1
  TRADEPOST_META_QUEUE.total = TRADEPOST_META_QUEUE.total
    or #TRADEPOST_META_QUEUE.ids
  TRADEPOST_META_QUEUE.nextSeq = TRADEPOST_META_QUEUE.nextSeq or 1
  TRADEPOST_META_QUEUE.synced = TRADEPOST_META_QUEUE.synced or 0
  self.meta = self.meta or {}
end

TP:MetaInit()
