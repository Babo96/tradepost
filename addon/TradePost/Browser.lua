local ADDON, TP = ...

TRADEPOST_ITEMS = TRADEPOST_ITEMS or {}

local ROW_HEIGHT = 26
local VISIBLE_ROWS = 16
local BROWSER_WIDTH = 640
local BROWSER_HEIGHT = 540
local LIST_WIDTH = 528
local DEFAULT_ICON = 134400 -- inv_misc_questionmark

local queue = {}
local queued = {}
local queueIndex = 0
local queueTicker
local refreshTimer

function TP:CacheItemMeta(itemID, name, quality, icon, link)
  if not itemID or not name or name == "" then
    return nil
  end
  local existing = self.DecodeItem(TRADEPOST_ITEMS[itemID])
  if existing and existing.n and existing.n ~= "" then
    return existing
  end
  local _, _, _, _, _, classID, subClassID = C_Item.GetItemInfoInstant(link or itemID)
  local q = tonumber(quality)
  if not q or q < 0 then
    q = 1
  end
  if not icon or icon == 0 then
    icon = DEFAULT_ICON
  end
  local meta = {
    n = name,
    i = icon,
    q = q,
    c = classID,
    s = subClassID,
  }
  TRADEPOST_ITEMS[itemID] = self.EncodeItem(meta)
  return meta
end

local function cacheItem(itemID)
  local existing = TP.DecodeItem(TRADEPOST_ITEMS[itemID])
  if existing and existing.n and existing.n ~= "" then
    return existing
  end
  local name, _, quality, _, _, _, _, _, _, icon = C_Item.GetItemInfo(itemID)
  if not name or name == "" then
    return nil
  end
  return TP:CacheItemMeta(itemID, name, quality, icon)
end

local function finishQueue()
  queueTicker:Cancel()
  queueTicker = nil
  queue = {}
  queued = {}
  queueIndex = 0
  if TP.WatchKeys and #TP:WatchKeys() > 0 then
    TP:SyncAuctionatorWatchList(false)
  end
  if TP.browser and TP.browser.frame and TP.browser.frame:IsShown() then
    TP:FilterBrowser()
  end
end

local function processQueue()
  local processed = 0
  while queueIndex < #queue and processed < 120 do
    queueIndex = queueIndex + 1
    local id = queue[queueIndex]
    if C_Item.GetItemInfo(id) then
      cacheItem(id)
      queued[id] = nil
    else
      C_Item.RequestLoadItemDataByID(id)
      -- Keep it flagged; ITEM_DATA_LOAD_RESULT caches it when it arrives.
    end
    processed = processed + 1
  end
  if queueIndex >= #queue then
    finishQueue()
  end
end

function TP:QueueItemMeta(itemID)
  local existing = TP.DecodeItem(TRADEPOST_ITEMS[itemID])
  if (existing and existing.n and existing.n ~= "") or queued[itemID] then
    return
  end
  queued[itemID] = true
  queue[#queue + 1] = itemID
  if not queueTicker then
    queueTicker = C_Timer.NewTicker(0.2, processQueue)
  end
end

function TP:GetItemMeta(itemID, request)
  local meta = cacheItem(itemID)
  if not meta and request then
    self:QueueItemMeta(itemID)
  end
  return meta
end

local loader = CreateFrame("Frame")
loader:RegisterEvent("ITEM_DATA_LOAD_RESULT")
loader:SetScript("OnEvent", function(_, _, itemID)
  if not itemID then
    return
  end
  queued[itemID] = nil
  local meta = cacheItem(itemID)
  if meta and TP.browser and TP.browser.frame and TP.browser.frame:IsShown() then
    if not refreshTimer then
      refreshTimer = C_Timer.NewTimer(0.5, function()
        refreshTimer = nil
        local browser = TP.browser
        if browser and browser.frame and browser.frame:IsShown() then
          if browser.query and browser.query ~= "" then
            TP:FilterBrowser()
          else
            TP:RefreshBrowser()
          end
        end
      end)
    end
  end
end)

local function itemInfoFromKey(key)
  local itemID, itemLevel = key:match("^i:(%d+)$")
  if itemID then
    return tonumber(itemID), 0
  end
  itemID, itemLevel = key:match("^g:(%d+):(%d+)$")
  if itemID then
    return tonumber(itemID), tonumber(itemLevel) or 0
  end
  return nil, nil
end

local function sourceRank(source)
  if source == "live" then
    return 3
  elseif source == "central" then
    return 2
  end
  return 1
end

function TP:BuildBrowserData()
  local marketKey = self:MarketKey()
  local merged = {}

  local function add(entries, source)
    for key, raw in pairs(entries or {}) do
      local entry = TP.DecodePrice(raw) or raw
      if entry and entry.p then
        local current = merged[key]
        if not current
          or (entry.t or 0) > (current.entry.t or 0)
          or ((entry.t or 0) == (current.entry.t or 0) and sourceRank(source) > sourceRank(current.source))
        then
          merged[key] = { key = key, entry = entry, source = source }
        end
      end
    end
  end

  add(self:LocalDB().prices, "local")
  local live = self.live and self.live[marketKey]
  if live then
    add(live.lookup, "live")
  end
  local central = self.central[marketKey]
  if central then
    add(central.lookup, "central")
  end

  local items = {}
  for _, record in pairs(merged) do
    local itemID, itemLevel = itemInfoFromKey(record.key)
    if itemID then
      record.itemID = itemID
      record.ilvl = itemLevel or 0
      local meta = self:GetItemMeta(itemID, true)
      record.name = meta and meta.n or ("Item #" .. itemID)
      record.icon = meta and meta.i or DEFAULT_ICON
      record.quality = meta and meta.q or 1
      items[#items + 1] = record
    end
  end
  self.browser.all = items
  self:FilterBrowser()
end

local function browserSort(a, b, mode)
  if mode == "price" then
    if a.entry.p == b.entry.p then
      return (a.name or "") < (b.name or "")
    end
    return a.entry.p < b.entry.p
  elseif mode == "age" then
    if (a.entry.t or 0) == (b.entry.t or 0) then
      return (a.name or "") < (b.name or "")
    end
    return (a.entry.t or 0) > (b.entry.t or 0)
  end
  return (a.name or ""):lower() < (b.name or ""):lower()
end

function TP:FilterBrowser()
  local browser = self.browser
  if not browser or not browser.all then
    return
  end
  local query = (browser.query or ""):lower()
  local numeric = tonumber(query)
  local filtered = {}

  for _, item in ipairs(browser.all) do
    local matches = false
    if query == "" then
      matches = true
    elseif numeric then
      matches = item.itemID == numeric or item.key:find(query, 1, true) ~= nil
    else
      matches = item.name and item.name:lower():find(query, 1, true) ~= nil
    end
    if matches then
      filtered[#filtered + 1] = item
    end
  end

  table.sort(filtered, function(a, b)
    return browserSort(a, b, browser.sort)
  end)
  browser.filtered = filtered
  browser.offset = 0
  if browser.slider then
    browser.slider:SetValue(0)
  end
  self:RefreshBrowser()
end

local function qualityColor(quality)
  local color = ITEM_QUALITY_COLORS and ITEM_QUALITY_COLORS[quality or 1]
  if color and color.r then
    return color.r, color.g, color.b
  end
  return 1, 1, 1
end

local function itemLink(itemID)
  local _, link = C_Item.GetItemInfo(itemID)
  return link or ("item:" .. itemID)
end

local function sortLabel(mode)
  if mode == "price" then
    return "Sort: price"
  elseif mode == "age" then
    return "Sort: age"
  end
  return "Sort: name"
end

function TP:RefreshBrowser()
  local browser = self.browser
  if not browser or not browser.frame or not browser.frame:IsShown() then
    return
  end
  local filtered = browser.filtered or {}
  local maxOffset = math.max(0, #filtered - VISIBLE_ROWS)
  if browser.offset > maxOffset then
    browser.offset = maxOffset
  end

  if browser.slider then
    browser.updatingSlider = true
    if maxOffset > 0 then
      browser.slider:SetMinMaxValues(0, maxOffset)
      browser.slider:SetValue(browser.offset)
      browser.slider:Show()
    else
      browser.slider:SetMinMaxValues(0, 1)
      browser.slider:SetValue(0)
      browser.slider:Hide()
    end
    browser.updatingSlider = false
  end

  local offset = browser.offset or 0
  for index = 1, VISIBLE_ROWS do
    local row = browser.rows[index]
    local item = filtered[offset + index]
    if item then
      local entry = item.entry
      local age = math.max(0, time() - (entry.t or 0))
      row.icon:SetTexture(item.icon or DEFAULT_ICON)
      row.name:SetText(item.name or ("Item #" .. tostring(item.itemID)))
      row.name:SetTextColor(qualityColor(item.quality or 1))
      row.price:SetText(self:MoneyText(entry.p))
      row.meta:SetText(string.format(
        "%s  |cff9ca3af%d|r%s",
        self:AgeString(age),
        entry.q or 0,
        self:SourceLabel(item.source) and ("  |cff60a5fa" .. self:SourceLabel(item.source) .. "|r") or ""
      ))
      row.itemID = item.itemID
      row.itemLevel = item.ilvl
      row.itemKey = item.key
      local watched = self:IsWatched(item.key)
      row.star.text:SetText(watched and "★" or "☆")
      if watched then
        row.star.text:SetTextColor(1, 0.82, 0.25)
      else
        row.star.text:SetTextColor(0.45, 0.5, 0.58)
      end
      row:Show()
    else
      row.itemID = nil
      row.itemKey = nil
      row:Hide()
    end
  end

  browser.status:SetText(string.format(
    "%d / %d items  ·  %s  ·  %s",
    #filtered, #(browser.all or {}), self:MarketKey(), self:CentralLabel()
  ))
end

local function scrollRows(browser, delta)
  local maxOffset = math.max(0, #(browser.filtered or {}) - VISIBLE_ROWS)
  local offset = math.max(0, math.min(maxOffset, (browser.offset or 0) + delta))
  if offset == browser.offset then
    return
  end
  browser.offset = offset
  if browser.slider then
    browser.updatingSlider = true
    browser.slider:SetValue(offset)
    browser.updatingSlider = false
  end
  TP:RefreshBrowser()
end

function TP:CreateBrowser()
  if self.browser and self.browser.frame then
    return self.browser.frame
  end

  self.browser = {
    all = {},
    filtered = {},
    query = "",
    offset = 0,
    sort = (self.config and self.config.browserSort) or "name",
  }

  local frame = CreateFrame("Frame", "TradePostBrowser", UIParent, "BackdropTemplate")
  frame:SetSize(BROWSER_WIDTH, BROWSER_HEIGHT)
  frame:SetPoint("CENTER")
  frame:SetFrameStrata("DIALOG")
  frame:SetClampedToScreen(true)
  frame:SetMovable(true)
  frame:EnableMouse(true)
  frame:RegisterForDrag("LeftButton")
  frame:SetScript("OnDragStart", frame.StartMoving)
  frame:SetScript("OnDragStop", frame.StopMovingOrSizing)
  frame:SetBackdrop({
    bgFile = "Interface\\DialogFrame\\UI-DialogBox-Background",
    edgeFile = "Interface\\DialogFrame\\UI-DialogBox-Border",
    tile = true,
    tileSize = 32,
    edgeSize = 32,
    insets = { left = 11, right = 12, top = 12, bottom = 11 },
  })
  self.browser.frame = frame

  local title = frame:CreateFontString(nil, "OVERLAY", "GameFontNormalLarge")
  title:SetPoint("TOPLEFT", 20, -18)
  title:SetText("TradePost — Auction Data")

  local close = CreateFrame("Button", nil, frame, "UIPanelCloseButton")
  close:SetPoint("TOPRIGHT", -6, -6)

  local search = CreateFrame("EditBox", nil, frame, "SearchBoxTemplate")
  search:SetSize(390, 26)
  search:SetPoint("TOPLEFT", 20, -48)
  search:SetAutoFocus(false)
  -- Hook instead of SetScript so SearchBoxTemplate keeps its own placeholder
  -- handling and hides the "Search" hint while typing.
  search:HookScript("OnTextChanged", function(edit)
    self.browser.query = edit:GetText() or ""
    self:FilterBrowser()
  end)
  local syncElapsed = 0
  search:HookScript("OnUpdate", function(edit, elapsed)
    syncElapsed = syncElapsed + elapsed
    if syncElapsed < 0.2 then
      return
    end
    syncElapsed = 0
    local text = edit:GetText() or ""
    if text ~= self.browser.query then
      self.browser.query = text
      self:FilterBrowser()
    end
  end)
  search:HookScript("OnEscapePressed", function(edit)
    edit:ClearFocus()
  end)
  search:HookScript("OnEnterPressed", function(edit)
    edit:ClearFocus()
  end)
  frame.search = search

  local sortButton = CreateFrame("Button", nil, frame, "UIPanelButtonTemplate")
  sortButton:SetSize(120, 24)
  sortButton:SetPoint("TOPRIGHT", -20, -49)
  sortButton:SetText(sortLabel(self.browser.sort))
  sortButton:SetScript("OnClick", function(button)
    local order = { "name", "price", "age" }
    local nextSort = "name"
    for index, value in ipairs(order) do
      if value == self.browser.sort then
        nextSort = order[index % #order + 1]
        break
      end
    end
    self.browser.sort = nextSort
    self:SetConfig("browserSort", nextSort)
    button:SetText(sortLabel(nextSort))
    self:FilterBrowser()
  end)
  frame.sortButton = sortButton

  local headerName = frame:CreateFontString(nil, "OVERLAY", "GameFontNormalSmall")
  headerName:SetPoint("TOPLEFT", 52, -84)
  headerName:SetText("ITEM")
  local headerPrice = frame:CreateFontString(nil, "OVERLAY", "GameFontNormalSmall")
  headerPrice:SetPoint("TOPRIGHT", -256, -84)
  headerPrice:SetText("PRICE")
  local headerMeta = frame:CreateFontString(nil, "OVERLAY", "GameFontNormalSmall")
  headerMeta:SetPoint("TOPRIGHT", -114, -84)
  headerMeta:SetText("AGE / QTY")

  local list = CreateFrame("Frame", nil, frame)
  list:SetPoint("TOPLEFT", 24, -100)
  list:SetSize(LIST_WIDTH, VISIBLE_ROWS * ROW_HEIGHT)
  list:EnableMouseWheel(true)
  list:SetScript("OnMouseWheel", function(_, delta)
    scrollRows(self.browser, -delta * 3)
  end)
  self.browser.list = list

  local slider = CreateFrame("Slider", nil, frame)
  slider:SetOrientation("VERTICAL")
  slider:SetPoint("TOPLEFT", list, "TOPRIGHT", 10, 0)
  slider:SetPoint("BOTTOMLEFT", list, "BOTTOMRIGHT", 10, 0)
  slider:SetWidth(16)
  slider:SetThumbTexture("Interface\\Buttons\\UI-ScrollBar-Knob")
  slider:SetMinMaxValues(0, 1)
  slider:SetValueStep(1)
  slider:SetObeyStepOnDrag(true)
  slider:SetScript("OnValueChanged", function(_, value)
    if self.browser.updatingSlider then
      return
    end
    scrollRows(self.browser, math.floor(value + 0.5) - (self.browser.offset or 0))
  end)
  slider:Hide()
  self.browser.slider = slider

  local rows = {}
  for index = 1, VISIBLE_ROWS do
    local row = CreateFrame("Button", nil, list)
    row:SetSize(LIST_WIDTH, ROW_HEIGHT - 2)
    row:SetPoint("TOPLEFT", 0, -((index - 1) * ROW_HEIGHT))
    row:EnableMouseWheel(true)
    row:SetScript("OnMouseWheel", function(_, delta)
      scrollRows(self.browser, -delta * 3)
    end)

    row.icon = row:CreateTexture(nil, "ARTWORK")
    row.icon:SetSize(20, 20)
    row.icon:SetPoint("LEFT", 0, 0)
    row.icon:SetTexCoord(0.08, 0.92, 0.08, 0.92)

    row.name = row:CreateFontString(nil, "OVERLAY", "GameFontHighlight")
    row.name:SetPoint("LEFT", row.icon, "RIGHT", 6, 0)
    row.name:SetWidth(250)
    row.name:SetJustifyH("LEFT")

    row.price = row:CreateFontString(nil, "OVERLAY", "GameFontHighlight")
    row.price:SetPoint("RIGHT", row, "RIGHT", -168, 0)
    row.price:SetJustifyH("RIGHT")

    row.meta = row:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
    row.meta:SetPoint("RIGHT", row, "RIGHT", -26, 0)
    row.meta:SetJustifyH("RIGHT")

    row.star = CreateFrame("Button", nil, row)
    row.star:SetSize(18, 18)
    row.star:SetPoint("RIGHT", row, "RIGHT", -2, 0)
    row.star.text = row.star:CreateFontString(nil, "OVERLAY", "GameFontHighlight")
    row.star.text:SetPoint("CENTER")
    row.star.text:SetText("☆")
    row.star:SetScript("OnClick", function(button)
      local parent = button:GetParent()
      if parent.itemKey then
        TP:ToggleWatch(parent.itemKey)
      end
    end)
    row.star:SetScript("OnEnter", function(button)
      GameTooltip:SetOwner(button, "ANCHOR_RIGHT")
      GameTooltip:AddLine(button:GetParent().itemKey and "Toggle watchlist" or "Watchlist")
      GameTooltip:AddLine("Click to watch or unwatch this item", 0.8, 0.8, 0.8)
      GameTooltip:Show()
    end)
    row.star:SetScript("OnLeave", function()
      GameTooltip:Hide()
    end)

    row:SetHighlightTexture("Interface\\QuestFrame\\UI-QuestTitleHighlight", "ADD")
    row:SetScript("OnEnter", function(button)
      if not button.itemID then
        return
      end
      GameTooltip:SetOwner(button, "ANCHOR_RIGHT")
      GameTooltip:SetHyperlink(itemLink(button.itemID))
      GameTooltip:Show()
    end)
    row:SetScript("OnLeave", function()
      GameTooltip:Hide()
    end)
    row:SetScript("OnClick", function(button)
      if not button.itemID or not IsShiftKeyDown() then
        return
      end
      if ChatEdit_InsertLink then
        ChatEdit_InsertLink(itemLink(button.itemID))
      end
    end)
    rows[index] = row
  end
  self.browser.rows = rows

  local status = frame:CreateFontString(nil, "OVERLAY", "GameFontDisableSmall")
  status:SetPoint("BOTTOMLEFT", 22, 16)
  status:SetPoint("BOTTOMRIGHT", -22, 16)
  status:SetJustifyH("LEFT")
  self.browser.status = status

  table.insert(UISpecialFrames, "TradePostBrowser")
  self.browser.frame:Hide()
  return frame
end

function TP:ToggleBrowser()
  local frame = self:CreateBrowser()
  if frame:IsShown() then
    frame:Hide()
    return
  end
  self:UpdateMarket()
  frame:Show()
  self:BuildBrowserData()
  self:RefreshBrowser()
end

local MINIMAP_DEFAULT_ANGLE = 225

-- Cursor angle around the minimap centre. Only the angle is used, so the
-- result is safe across minimap/UI scale differences.
local function minimapAngleFromCursor()
  if not Minimap or not Minimap.GetCenter then
    return nil
  end
  local mx, my = Minimap:GetCenter()
  local scale = Minimap:GetEffectiveScale()
  if not mx or not my or not scale or scale == 0 then
    return nil
  end
  local x, y = GetCursorPosition()
  if not x or not y then
    return nil
  end
  x, y = x / scale, y / scale
  local atan2 = math.atan2
  if not atan2 then
    atan2 = function(py, px)
      if px > 0 then
        return math.atan(py / px)
      elseif px < 0 and py >= 0 then
        return math.atan(py / px) + math.pi
      elseif px < 0 then
        return math.atan(py / px) - math.pi
      elseif py > 0 then
        return math.pi / 2
      elseif py < 0 then
        return -math.pi / 2
      end
      return 0
    end
  end
  return math.deg(atan2(y - my, x - mx))
end

function TP:MinimapRadius()
  local width = Minimap and Minimap.GetWidth and Minimap:GetWidth() or 140
  return width / 2 + 10
end

function TP:PlaceMinimapButton(angle)
  local button = self.minimapButton
  if not button or not Minimap then
    return
  end
  if angle == nil then
    angle = tonumber(self.config and self.config.minimapAngle) or MINIMAP_DEFAULT_ANGLE
  end
  local radians = math.rad(angle)
  local radius = self:MinimapRadius()
  button:ClearAllPoints()
  button:SetPoint("CENTER", Minimap, "CENTER",
    math.cos(radians) * radius, math.sin(radians) * radius)
end

function TP:SetMinimapVisible(show)
  show = show and true or false
  self:SetConfig("minimap", show)
  local button = self.minimapButton
  if button then
    if show then
      self:PlaceMinimapButton()
      button:Show()
    else
      button:Hide()
      if GameTooltip then
        GameTooltip:Hide()
      end
    end
  end
  if self.settings and self.settings.refresh then
    self.settings.refresh()
  end
end

function TP:ToggleMinimap()
  local show = not (self.config and self.config.minimap)
  self:SetMinimapVisible(show)
  if show then
    self:Print("minimap button shown")
  else
    self:Print("minimap button hidden; /tradepost minimap shows it again")
  end
end

function TP:CreateMinimapButton()
  if self.minimapButton or not Minimap then
    return
  end
  local button = CreateFrame("Button", "TradePostMinimapButton", Minimap)
  button:SetSize(28, 28)
  button:SetFrameStrata("MEDIUM")
  button:SetFrameLevel(Minimap:GetFrameLevel() + 5)
  button:SetNormalTexture("Interface\\AddOns\\TradePost\\Media\\icon")
  button:SetHighlightTexture("Interface\\AddOns\\TradePost\\Media\\highlight", "ADD")
  local icon = button:GetNormalTexture()
  if icon then
    icon:SetTexCoord(0, 1, 0, 1)
  end

  self.minimapButton = button
  self:PlaceMinimapButton()

  local dragging = false
  local suppressUntil = 0

  local function updateDrag()
    local angle = minimapAngleFromCursor()
    if angle ~= nil then
      TP:PlaceMinimapButton(angle)
    end
  end

  button:RegisterForClicks("LeftButtonUp", "RightButtonUp")
  button:RegisterForDrag("LeftButton")

  button:SetScript("OnDragStart", function()
    if not IsShiftKeyDown() then
      return
    end
    dragging = true
    -- A drag ends with a button release; swallow the click that follows it.
    suppressUntil = GetTime() + 0.25
    if GameTooltip then
      GameTooltip:Hide()
    end
    button:SetScript("OnUpdate", updateDrag)
    updateDrag()
  end)

  button:SetScript("OnDragStop", function()
    if not dragging then
      return
    end
    dragging = false
    button:SetScript("OnUpdate", nil)
    local angle = minimapAngleFromCursor()
    if angle ~= nil then
      angle = angle % 360
      TP:SetConfig("minimapAngle", math.floor(angle * 10 + 0.5) / 10)
    end
    TP:PlaceMinimapButton()
  end)

  button:SetScript("OnClick", function(_, mouseButton)
    if GetTime() < suppressUntil then
      suppressUntil = 0
      return
    end
    if IsShiftKeyDown() then
      if mouseButton == "RightButton" then
        TP:SetMinimapVisible(false)
        TP:Print("minimap button hidden; /tradepost minimap or the addon settings bring it back")
      end
      return
    end
    if mouseButton == "RightButton" then
      TP:OpenSettings()
    else
      TP:ToggleBrowser()
    end
  end)

  button:SetScript("OnEnter", function(btn)
    GameTooltip:SetOwner(btn, "ANCHOR_LEFT")
    GameTooltip:AddLine("TradePost")
    GameTooltip:AddLine("Left click: browse auction prices", 0.8, 0.8, 0.8)
    GameTooltip:AddLine("Right click: open settings", 0.8, 0.8, 0.8)
    GameTooltip:AddLine("Shift + drag: move around the minimap", 0.8, 0.8, 0.8)
    GameTooltip:AddLine("Shift + right click: hide the button", 0.8, 0.8, 0.8)
    GameTooltip:Show()
  end)
  button:SetScript("OnLeave", function()
    GameTooltip:Hide()
  end)

  if TP.config and TP.config.minimap == false then
    button:Hide()
  end
end

local init = CreateFrame("Frame")
init:RegisterEvent("PLAYER_LOGIN")
init:SetScript("OnEvent", function()
  TP:CreateMinimapButton()
end)
