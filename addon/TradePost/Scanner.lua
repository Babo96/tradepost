local ADDON, TP = ...

local THROTTLE = 15 * 60
local QUIET_SECONDS = 3
local CHUNK = 2000

TP.scan = TP.scan or {
  running = false,
  waiting = false,
}

local function remaining(seconds)
  if seconds <= 0 then
    return "now"
  end
  local minutes = math.ceil(seconds / 60)
  return string.format("%d min", minutes)
end

function TP:NextReplicateSeconds()
  local last = (TRADEPOST_DB and TRADEPOST_DB.lastReplicate) or 0
  return math.max(0, THROTTLE - (time() - last))
end

function TP:TryScan(force)
  if self.scan.running then
    self:Print("a scan is already running")
    return
  end
  if self.scan.waiting then
    self:Print("already waiting for auction data")
    return
  end
  if self.scan.suppressUntil and time() < self.scan.suppressUntil then
    self:Print("Auction House error; scan retry in " ..
      remaining(self.scan.suppressUntil - time()))
    return
  end

  local count = (C_AuctionHouse and C_AuctionHouse.GetNumReplicateItems()) or 0
  local now = time()
  local db = self:LocalDB()
  local marketKey = self:MarketKey()
  local sameMarket = db.lastScanMarket == marketKey
  if db.lastScanMarket == nil and (TRADEPOST_DB.lastReplicate or 0) > 0 then
    -- First run: a populated list just arrived; assume it belongs to the AH we
    -- are standing at and harvest it. Later scans record the exact market.
    sameMarket = true
  end
  local lastReplicate = (TRADEPOST_DB and TRADEPOST_DB.lastReplicate) or 0
  local replicateAge = now - lastReplicate
  local lastScanAge = (db.lastScan and db.lastScan > 0) and (now - db.lastScan) or math.huge
  local recentScan = lastScanAge < 60

  -- /tradepost scan force: reprocess the list already in the client. This does not
  -- hit the 15-minute replicate throttle and is the way to force a fresh
  -- export for the companion.
  if force and count and count > 0 and sameMarket then
    self:Print("reprocessing cached list (" .. count .. " auctions)")
    self:StartProcessing(count)
    return
  end

  if force and (not count or count == 0) and replicateAge < THROTTLE then
    self:Print("cached list is empty after restart; client throttle: " ..
      remaining(THROTTLE - replicateAge))
    return
  end

  -- Harvest a fresh list for this market that we have not processed yet (first
  -- run, or another addon refreshed it). Do not reprocess our own recent scan.
  if not force and count and count > 0 and sameMarket and replicateAge < THROTTLE then
    if recentScan then
      self:Print("recent scan; next scan in " .. remaining(THROTTLE - lastScanAge) ..
        " (/tradepost status)")
      return
    end
    self:StartProcessing(count)
    return
  end

  -- The list is stale or from another market. Do not store it under this
  -- market; request a fresh replicate if the throttle allows.
  if not force and replicateAge < THROTTLE then
    if count and count > 0 and not sameMarket then
      self:Print("cached auction list is from another market; client throttle: " ..
        remaining(THROTTLE - replicateAge))
    elseif count and count > 0 then
      self:Print("cached auction list is stale; client throttle: " ..
        remaining(THROTTLE - replicateAge))
    else
      self:Print("client throttle active; next replicate in " ..
        remaining(THROTTLE - replicateAge))
    end
    return
  end

  self.scan.lastEventCount = nil
  self.scan.suppressUntil = nil
  -- Record the request, not only a successful list update. A failed or
  -- throttled request must not make us hammer the AH on every open.
  if TRADEPOST_DB then
    TRADEPOST_DB.lastReplicate = now
  end
  local ok, err = pcall(C_AuctionHouse.ReplicateItems)
  if not ok then
    self:Print("could not request a scan: " .. tostring(err))
    return
  end

  self.scan.waiting = true
  self:Print("scan requested; waiting for auction data...")

  C_Timer.After(30, function()
    if self.scan.waiting then
      self.scan.waiting = false
      self:Print("no auction data after 30 s. The full-scan API is still throttled or the AH returned an error; Auctionator browse uses a different API.")
    end
  end)
end

function TP:ScheduleQuietCheck(count)
  if self.scan.running then
    return
  end
  self.scan.quietCount = count

  if self.scan.quietTimer then
    return
  end

  self.scan.quietTimer = C_Timer.NewTimer(QUIET_SECONDS, function()
    self.scan.quietTimer = nil
    local current = (C_AuctionHouse and C_AuctionHouse.GetNumReplicateItems()) or 0
    if current > (self.scan.quietCount or 0) then
      self:ScheduleQuietCheck(current)
    else
      local ok, err = pcall(self.StartProcessing, self, current)
      if not ok then
        self:ReportError("scan-start", err)
      end
    end
  end)
end

function TP:StartProcessing(total)
  if self.scan.running or not total or total <= 0 then
    return
  end

  self.scan.running = true
  self.scan.waiting = false
  self.scan.index = 0
  self.scan.total = total
  self.scan.acc = {}
  self.scan.metaSeen = {}
  self.scan.auctions = 0
  self.scan.missing = 0
  self.scan.started = debugprofilestop()
  self.scan.marketKey = self:MarketKey()
  self.scan.lastEventCount = total
  self.scan.suppressUntil = nil

  self:Print("scanning " .. total .. " auctions...")

  self.scan.ticker = C_Timer.NewTicker(0, function()
    local index = self.scan.index
    local stop = math.min(index + CHUNK, total)

    local ok, err = pcall(function()
      for i = index, stop - 1 do
        local info = { C_AuctionHouse.GetReplicateItemInfo(i) }
        local quantity = info[3]
        local buyout = info[10]
        local itemID = info[17]
        local allInfo = info[18]

        if not allInfo then
          self.scan.missing = self.scan.missing + 1
        end

        if itemID and not self.scan.metaSeen[itemID] then
          self.scan.metaSeen[itemID] = true
          local name, _, quality, _, _, _, _, _, _, icon = C_Item.GetItemInfo(itemID)
          if not name then
            name = type(info[1]) == "string" and info[1] or nil
            quality = quality or (type(info[4]) == "number" and info[4] or nil)
            icon = icon or info[2]
          end
          if name then
            self:CacheItemMeta(itemID, name, quality, icon)
          end
        end

        if itemID and quantity and quantity > 0 and buyout and buyout > 0 then
          local key = self:KeyFromAuction(i, info)
          if key then
            local unitPrice = buyout / quantity
            local entry = self.scan.acc[key]
            if not entry then
              entry = { p = unitPrice, q = 0, n = 0 }
              self.scan.acc[key] = entry
            end
            if unitPrice < entry.p then
              entry.p = unitPrice
            end
            entry.q = entry.q + quantity
            entry.n = entry.n + 1
            self.scan.auctions = self.scan.auctions + 1
          end
        end
      end
    end)

    if not ok then
      if self.scan.ticker then
        self.scan.ticker:Cancel()
      end
      self.scan.running = false
      self.scan.acc = nil
      self:ReportError("scan", err)
      return
    end

    self.scan.index = stop
    if index > 0 and index % 50000 == 0 then
      self:Debug("scan progress " .. index .. "/" .. total)
    end

    if stop >= total then
      self.scan.ticker:Cancel()
      local ok, err = pcall(self.FinishScan, self)
      if not ok then
        self.scan.running = false
        self.scan.acc = nil
        self:ReportError("scan-finish", err)
      end
    end
  end)
end

function TP:FinishScan()
  local elapsed = debugprofilestop() - self.scan.started
  local timestamp = time()
  local marketKey = self.scan.marketKey or self:MarketKey()
  local db = self:LocalDBFor(marketKey)

  local unique = 0
  for key, entry in pairs(self.scan.acc) do
    self:SetLocalPriceFor(marketKey, key, math.floor(entry.p + 0.5), entry.q, entry.n, timestamp)
    unique = unique + 1
  end

  pcall(function()
    TP:DiscoverBlizzardFavorites(self.scan.acc)
  end)
  self:RecordExport(marketKey, timestamp, self.scan.acc)
  pcall(function()
    TP:SyncAuctionatorFavourites(false)
    TP:SyncAuctionatorWatchList(false)
  end)
  C_Timer.After(2, function()
    pcall(function()
      TP:SaveExport()
    end)
  end)
  pcall(function()
    TP:ScheduleExportRetry()
  end)

  db.lastScan = timestamp
  db.lastScanMarket = marketKey
  db.lastAuctions = self.scan.auctions
  db.lastUnique = unique
  db.lastMissing = self.scan.missing
  db.lastDuration = math.floor(elapsed)

  local auctions = self.scan.auctions
  local missing = self.scan.missing

  self.scan.running = false
  self.scan.acc = nil
  self.scan.suppressUntil = time() + 60
  self.scan.lastEventCount = (C_AuctionHouse and C_AuctionHouse.GetNumReplicateItems()) or self.scan.total

  self:Print(string.format(
    "scanned %d auctions, %d items in %.1f s (%d missing item info)",
    auctions, unique, elapsed / 1000, missing
  ))
end
