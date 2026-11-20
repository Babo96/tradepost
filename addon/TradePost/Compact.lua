local ADDON, TP = ...

-- Compact SavedVariables encoding for the local price DB and item cache.
-- The tables keep their keys, but each value is a string:
--   prices[key] = "price,qty,listings,scanned"
--   TRADEPOST_ITEMS[id] = "name\ticon\tclass\tsubclass\tquality"
-- This removes the per-entry table syntax that dominates the file size.
-- Decoders accept both strings and legacy tables, and CompactMigrate converts
-- old tables once at load.

function TP.EncodePrice(entry)
  if type(entry) ~= "table" then
    return entry
  end
  local price = tonumber(entry.p) or 0
  local quantity = tonumber(entry.q) or 0
  local listings = tonumber(entry.n) or 0
  local scanned = tonumber(entry.t) or 0
  return string.format("%d,%d,%d,%d",
    math.floor(price + 0.5), quantity, listings, scanned)
end

function TP.DecodePrice(value)
  if type(value) == "table" then
    return value
  end
  if type(value) ~= "string" then
    return nil
  end
  local p, q, n, t = value:match("^(%-?%d+),(%-?%d+),(%-?%d+),(%-?%d+)$")
  if not p then
    return nil
  end
  return {
    p = tonumber(p),
    q = tonumber(q),
    n = tonumber(n),
    t = tonumber(t),
  }
end

function TP.EncodeItem(meta)
  if type(meta) ~= "table" then
    return meta
  end
  local name = tostring(meta.n or ""):gsub("[\t\r\n]+", " ")
  return string.format("%s\t%d\t%d\t%d\t%d",
    name,
    tonumber(meta.i) or 0,
    tonumber(meta.c) or 0,
    tonumber(meta.s) or 0,
    tonumber(meta.q) or 1)
end

function TP.DecodeItem(value)
  if type(value) == "table" then
    return value
  end
  if type(value) ~= "string" then
    return nil
  end
  local name, icon, classID, subclassID, quality =
    value:match("^([^\t]*)\t(%-?%d+)\t(%-?%d+)\t(%-?%d+)\t(%-?%d+)$")
  if not name then
    return nil
  end
  return {
    n = name,
    i = tonumber(icon),
    c = tonumber(classID),
    s = tonumber(subclassID),
    q = tonumber(quality),
  }
end

-- One-time conversion of legacy table entries. Safe to call on every load.
function TP:CompactMigrate()
  if TRADEPOST_DB and TRADEPOST_DB.markets then
    for _, db in pairs(TRADEPOST_DB.markets) do
      for key, entry in pairs(db.prices or {}) do
        if type(entry) == "table" then
          db.prices[key] = self.EncodePrice(entry)
        end
      end
    end
  end
  for id, meta in pairs(TRADEPOST_ITEMS or {}) do
    local decoded = self.DecodeItem(meta)
    if not decoded or not decoded.n or decoded.n == "" then
      -- Empty names came from old scans where the client had no item data.
      -- Drop them so the browser requests a real item cache entry.
      TRADEPOST_ITEMS[id] = nil
    elseif type(meta) == "table" then
      TRADEPOST_ITEMS[id] = self.EncodeItem(decoded)
    end
  end
end
