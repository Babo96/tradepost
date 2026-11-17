local ADDON, BF = ...

BABOFOREVER_ITEMS = BABOFOREVER_ITEMS or {}

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

function BF:CacheItemMeta(itemID, name, quality, icon)
  if not itemID or not name then
    return nil
  end
  local existing = BABOFOREVER_ITEMS[itemID]
  if existing and existing.n then
    return existing
  end
  local _, _, _, _, _, classID, subClassID = C_Item.GetItemInfoInstant(itemID)
  local meta = {
    n = name,
    i = icon or DEFAULT_ICON,
    q = quality or 1,
    c = classID,
    s = subClassID,
  }
  BABOFOREVER_ITEMS[itemID] = meta
  return meta
end

local function cacheItem(itemID)
  local existing = BABOFOREVER_ITEMS[itemID]
  if existing and existing.n then
    return existing
  end
  local name, _, quality, _, _, _, _, _, _, icon = C_Item.GetItemInfo(itemID)
  if not name then
    return nil
  end
  return BF:CacheItemMeta(itemID, name, quality, icon)
end

local function finishQueue()
  queueTicker:Cancel()
  queueTicker = nil
  queue = {}
  queued = {}
  queueIndex = 0
  if BF.browser and BF.browser.frame and BF.browser.frame:IsShown() then
    BF:FilterBrowser()
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

function BF:QueueItemMeta(itemID)
  if BABOFOREVER_ITEMS[itemID] or queued[itemID] then
    return
  end
  queued[itemID] = true
  queue[#queue + 1] = itemID
  if not queueTicker then
    queueTicker = C_Timer.NewTicker(0.2, processQueue)
  end
end

function BF:GetItemMeta(itemID, request)
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
  if meta and BF.browser and BF.browser.frame and BF.browser.frame:IsShown() then
    if not refreshTimer then
      refreshTimer = C_Timer.NewTimer(0.5, function()
        refreshTimer = nil
        local browser = BF.browser
        if browser and browser.frame and browser.frame:IsShown() then
          if browser.query and browser.query ~= "" then
            BF:FilterBrowser()
          else
            BF:RefreshBrowser()
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

function BF:BuildBrowserData()
  local marketKey = self:MarketKey()
  local merged = {}

  local function add(entries, source)
    for key, entry in pairs(entries or {}) do
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

function BF:FilterBrowser()
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

function BF:RefreshBrowser()
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
      row:Show()
    else
      row.itemID = nil
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
  BF:RefreshBrowser()
end

function BF:CreateBrowser()
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

  local frame = CreateFrame("Frame", "BaboForeverBrowser", UIParent, "BackdropTemplate")
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
  title:SetText("Babo Forever — Auction Data")

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
  headerPrice:SetPoint("TOPRIGHT", -238, -84)
  headerPrice:SetText("PRICE")
  local headerMeta = frame:CreateFontString(nil, "OVERLAY", "GameFontNormalSmall")
  headerMeta:SetPoint("TOPRIGHT", -96, -84)
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
    row.price:SetPoint("RIGHT", row, "RIGHT", -150, 0)
    row.price:SetJustifyH("RIGHT")

    row.meta = row:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
    row.meta:SetPoint("RIGHT", row, "RIGHT", -8, 0)
    row.meta:SetJustifyH("RIGHT")

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

  table.insert(UISpecialFrames, "BaboForeverBrowser")
  self.browser.frame:Hide()
  return frame
end

function BF:ToggleBrowser()
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

function BF:CreateMinimapButton()
  if self.minimapButton or not Minimap then
    return
  end
  local button = CreateFrame("Button", "BaboForeverMinimapButton", Minimap)
  button:SetSize(28, 28)
  button:SetFrameStrata("MEDIUM")
  button:SetFrameLevel(Minimap:GetFrameLevel() + 5)
  button:SetPoint("CENTER", Minimap, "CENTER", -58, -58)
  button:SetNormalTexture("Interface\\Icons\\inv_misc_coin_01")
  button:SetHighlightTexture("Interface\\Minimap\\UI-Minimap-ZoomButton-Highlight", "ADD")
  local icon = button:GetNormalTexture()
  if icon then
    icon:SetTexCoord(0.08, 0.92, 0.08, 0.92)
  end
  button:SetScript("OnClick", function()
    BF:ToggleBrowser()
  end)
  button:SetScript("OnEnter", function(btn)
    GameTooltip:SetOwner(btn, "ANCHOR_LEFT")
    GameTooltip:AddLine("Babo Forever")
    GameTooltip:AddLine("Browse auction prices", 0.8, 0.8, 0.8)
    GameTooltip:Show()
  end)
  button:SetScript("OnLeave", function()
    GameTooltip:Hide()
  end)
  self.minimapButton = button
end

local init = CreateFrame("Frame")
init:RegisterEvent("PLAYER_LOGIN")
init:SetScript("OnEvent", function()
  BF:CreateMinimapButton()
end)
