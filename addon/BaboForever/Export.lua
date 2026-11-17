local ADDON, BF = ...

local MAX_SCANS = 3

function BF:ExportDB()
  BABOFOREVER_EXPORT = BABOFOREVER_EXPORT or {}
  BABOFOREVER_EXPORT.version = 1
  BABOFOREVER_EXPORT.scans = BABOFOREVER_EXPORT.scans or {}
  return BABOFOREVER_EXPORT
end

function BF:SaveExport()
  local saver = (C_AddOns and C_AddOns.SaveAddOns) or SaveAddOns
  if type(saver) ~= "function" then
    self:Print("SaveAddOns is not available; export will be written on /reload")
    return false
  end
  local ok, err = pcall(saver)
  if not ok then
    self:ReportError("export-save", err)
    return false
  end
  self.lastSave = time()
  return true
end

-- One ASCII record per scan:
--   market|scanned_at|key,price,quantity,listings;key,price,...
-- The companion parses this without needing a Lua interpreter.
function BF:RecordExport(marketKey, timestamp, accumulator)
  local parts = {}
  for key, entry in pairs(accumulator) do
    if entry.p and entry.q and entry.n then
      parts[#parts + 1] = string.format(
        "%s,%d,%d,%d",
        key,
        math.floor(entry.p + 0.5),
        entry.q,
        entry.n
      )
    end
  end

  local payload = table.concat({
    marketKey,
    tostring(timestamp),
    table.concat(parts, ";"),
  }, "|")

  local db = self:ExportDB()
  table.insert(db.scans, payload)
  while #db.scans > MAX_SCANS do
    table.remove(db.scans, 1)
  end
  db.lastRecord = timestamp
  db.lastSize = #payload
  self:SaveExport()
end

function BF:ClearExport()
  local db = self:ExportDB()
  db.scans = {}
  db.lastRecord = nil
  db.lastSize = nil
  self:Print("export buffer cleared")
end

-- Testing helper: turn the current local DB into an export record without
-- scanning again. The record keeps the last scan timestamp, so it does not
-- pretend the data is newer than it is.
function BF:ExportLocal()
  local db = self:LocalDB()
  local count = 0
  for _ in pairs(db.prices or {}) do
    count = count + 1
  end
  if count == 0 then
    self:Print("local DB is empty; run /bf scan after the client throttle")
    return
  end
  local timestamp = db.lastScan or time()
  self:RecordExport(self:MarketKey(), timestamp, db.prices)
  self:Print("exported " .. count .. " local prices for " .. self:MarketKey() ..
    " (scan " .. date("%H:%M", timestamp) .. ")")
end

function BF:ExportStatus()
  local db = self:ExportDB()
  self:Print("export: " .. #db.scans .. " scan(s), last " ..
    (db.lastRecord and date("%Y-%m-%d %H:%M", db.lastRecord) or "never") ..
    (db.lastSize and (" (" .. db.lastSize .. " bytes)") or ""))
  self:Print("live flush: " .. (self.lastSave and ("last " .. date("%H:%M:%S", self.lastSave)) or "not yet") ..
    " (C_AddOns.SaveAddOns)")
  self:Print("SavedVariables: BABOFOREVER_EXPORT (also written on /reload or logout)")
end
