local ADDON, TP = ...

local PANEL_WIDTH = 620
local PANEL_HEIGHT = 540
local HOUSE_LABELS = {
  auto = "Auto",
  alliance = "Alliance",
  horde = "Horde",
  neutral = "Neutral",
}

local function ageText(timestamp)
  if not timestamp or timestamp <= 0 then
    return "never"
  end
  local seconds = math.max(0, time() - timestamp)
  if seconds < 60 then
    return string.format("%ds ago", seconds)
  elseif seconds < 3600 then
    return string.format("%dm ago", math.floor(seconds / 60))
  elseif seconds < 86400 then
    return string.format("%dh ago", math.floor(seconds / 3600))
  end
  return string.format("%dd ago", math.floor(seconds / 86400))
end

local function addTooltip(frame, title, text)
  frame:SetScript("OnEnter", function(owner)
    GameTooltip:SetOwner(owner, "ANCHOR_RIGHT")
    GameTooltip:AddLine(title or "", 1, 1, 1)
    if text then
      GameTooltip:AddLine(text, 0.8, 0.8, 0.8, true)
    end
    GameTooltip:Show()
  end)
  frame:SetScript("OnLeave", function()
    GameTooltip:Hide()
  end)
end

local function createCheckbox(parent, x, y, label, key, tooltip, onChange)
  local checkbox = CreateFrame("CheckButton", nil, parent, "InterfaceOptionsCheckButtonTemplate")
  checkbox:SetPoint("TOPLEFT", x, y)
  checkbox.Text:SetText(label)
  if tooltip then
    addTooltip(checkbox, label, tooltip)
  end
  checkbox:SetScript("OnClick", function(button)
    local value = button:GetChecked() and true or false
    TP:SetConfig(key, value)
    if onChange then
      onChange(value)
    end
    if TP.settings then
      TP.settings.refresh()
    end
  end)
  return checkbox
end

local function createButton(parent, x, y, width, label, tooltip, onClick)
  local button = CreateFrame("Button", nil, parent, "UIPanelButtonTemplate")
  button:SetSize(width, 24)
  button:SetPoint("TOPLEFT", x, y)
  button:SetText(label)
  button:SetScript("OnClick", onClick)
  if tooltip then
    addTooltip(button, label, tooltip)
  end
  return button
end

local function createPanel()
  local panel = CreateFrame("Frame", "TradePostSettingsPanel", UIParent)
  panel.name = "TradePost"
  panel:SetSize(PANEL_WIDTH, PANEL_HEIGHT)

  local settings = {
    panel = panel,
    controls = {},
    houseButtons = {},
    refreshing = false,
  }

  local refresh -- forward declaration

  local y = -16
  settings.controls.tooltips = createCheckbox(panel, 20, y, "Price tooltips", "tooltips",
    "Show TradePost prices in item tooltips.")
  settings.controls.showAge = createCheckbox(panel, 320, y, "Show price age", "showAge",
    "Append how old the price data is.")

  y = y - 28
  settings.controls.showSupply = createCheckbox(panel, 20, y, "Show supply", "showSupply",
    "Append the total quantity listed on the auction house.")
  settings.controls.debug = createCheckbox(panel, 320, y, "Debug output", "debug",
    "Print extra diagnostics to the chat frame.")

  y = y - 28
  settings.controls.ring = createCheckbox(panel, 20, y, "Live data ring", "ring",
    "Load crowd-sourced live price snapshots from the TradePost_Data slots.",
    function(value)
      if value then
        TP:InitRing()
      end
    end)
  settings.controls.auctionatorBridge = createCheckbox(panel, 320, y, "Auctionator bridge", "auctionatorBridge",
    "Replace Auctionator's displayed prices with TradePost prices.",
    function(value)
      if value then
        TP:InitBridge()
      elseif TP.bridge then
        TP:BridgeOff()
      end
    end)

  y = y - 28
  settings.controls.auctionatorFavourites = createCheckbox(panel, 20, y, "Auctionator favourites sync",
    "auctionatorFavourites",
    "Mirror Auctionator favourites into the TradePost watchlist.",
    function(value)
      if value then
        TP:SyncAuctionatorFavourites(true)
      end
    end)

  y = y - 34
  local sliderLabel = panel:CreateFontString(nil, "OVERLAY", "GameFontNormal")
  sliderLabel:SetPoint("TOPLEFT", 20, y)
  sliderLabel:SetText("Ring slots")
  settings.sliderLabel = sliderLabel

  y = y - 22
  local slider = CreateFrame("Slider", nil, panel)
  slider:SetOrientation("HORIZONTAL")
  slider:SetPoint("TOPLEFT", 20, y)
  slider:SetSize(300, 16)
  slider:SetThumbTexture("Interface\\Buttons\\UI-SliderBar-Button-Horizontal")
  slider:SetMinMaxValues(8, 128)
  slider:SetValueStep(1)
  slider:SetObeyStepOnDrag(true)
  slider:SetScript("OnValueChanged", function(_, value)
    value = math.floor(value + 0.5)
    if settings.refreshing or settings.updatingSlider then
      return
    end
    TP:SetConfig("ringSlots", value)
    sliderLabel:SetText("Ring slots: " .. value)
    if TP.ring then
      TP.ring.lastWindow = nil
      TP.ring.lastSeq = 0
    end
  end)
  settings.slider = slider

  y = y - 42
  local houseLabel = panel:CreateFontString(nil, "OVERLAY", "GameFontNormal")
  houseLabel:SetPoint("TOPLEFT", 20, y)
  houseLabel:SetText("Auction house override")
  settings.houseLabel = houseLabel

  y = y - 24
  local houseOrder = { "auto", "alliance", "horde", "neutral" }
  for index, value in ipairs(houseOrder) do
    local button = CreateFrame("Button", nil, panel, "UIPanelButtonTemplate")
    button:SetSize(96, 22)
    button:SetPoint("TOPLEFT", 20 + (index - 1) * 104, y)
    button:SetText(HOUSE_LABELS[value])
    button.houseValue = value
    button:SetScript("OnClick", function(clicked)
      local override
      if clicked.houseValue ~= "auto" then
        override = clicked.houseValue
      end
      TP:SetConfig("houseOverride", override)
      TP:UpdateMarket()
      if settings.refresh then
        settings.refresh()
      end
    end)
    addTooltip(button, HOUSE_LABELS[value],
      "Force this auction house market instead of detecting it from your faction and zone.")
    settings.houseButtons[value] = button
  end

  y = y - 38
  local statusHeading = panel:CreateFontString(nil, "OVERLAY", "GameFontNormal")
  statusHeading:SetPoint("TOPLEFT", 20, y)
  statusHeading:SetText("Status")

  y = y - 20
  local status = panel:CreateFontString(nil, "OVERLAY", "GameFontHighlightSmall")
  status:SetPoint("TOPLEFT", 20, y)
  status:SetWidth(PANEL_WIDTH - 40)
  status:SetHeight(80)
  status:SetJustifyH("LEFT")
  status:SetJustifyV("TOP")
  settings.status = status

  y = y - 84
  local actionHeading = panel:CreateFontString(nil, "OVERLAY", "GameFontNormal")
  actionHeading:SetPoint("TOPLEFT", 20, y)
  actionHeading:SetText("Actions")

  y = y - 26
  createButton(panel, 20, y, 186, "Scan now", "Request an auction house scan now (the client throttle still applies).",
    function()
      TP:TryScan(true)
    end)
  createButton(panel, 220, y, 186, "Browse", "Open the auction data browser.",
    function()
      TP:ToggleBrowser()
    end)
  createButton(panel, 420, y, 186, "Auctionator list sync", "Rebuild the Auctionator shopping/wish list from the TradePost watchlist.",
    function()
      TP:SyncAuctionatorWatchList(true)
    end)

  y = y - 30
  createButton(panel, 20, y, 186, "AH favorites sync", "Push the TradePost watchlist into the default Auction House favorites (requires the AH).",
    function()
      TP:SyncBlizzardFavorites(true)
    end)
  createButton(panel, 220, y, 186, "Login code", "Request a short login code for the TradePost companion site.",
    function()
      TP:RequestLogin()
    end)
  createButton(panel, 420, y, 186, "Export save", "Flush the export buffer to SavedVariables so the companion can pick it up.",
    function()
      if TP:SaveExport() then
        TP:Print("export saved to disk")
      end
    end)

  y = y - 30
  createButton(panel, 20, y, 186, "Clear local data", "Delete all locally scanned prices for the current market.",
    function()
      if not StaticPopupDialogs or not StaticPopup_Show then
        TP:ClearLocal()
        if settings.refresh then
          settings.refresh()
        end
        return
      end
      StaticPopup_Show("TRADEPOST_CONFIRM_CLEAR", TP:MarketKey())
    end)

  refresh = function()
    if not panel:IsShown() then
      return
    end
    settings.refreshing = true
    TP:UpdateMarket()

    local config = TP.config or {}
    for key, checkbox in pairs(settings.controls) do
      checkbox:SetChecked(config[key] and true or false)
    end

    local slots = tonumber(config.ringSlots) or 64
    slots = math.max(8, math.min(128, math.floor(slots + 0.5)))
    settings.slider:SetValue(slots)
    settings.sliderLabel:SetText("Ring slots: " .. slots)

    for value, button in pairs(settings.houseButtons) do
      local selected = value == "auto" and not config.houseOverride
        or config.houseOverride == value
      if selected then
        button:SetText("|cff33ff99" .. HOUSE_LABELS[value] .. "|r")
      else
        button:SetText(HOUSE_LABELS[value])
      end
    end

    local db = TP:LocalDB()
    local live = TP.live and TP.live[TP:MarketKey()]
    local liveCount = 0
    if live then
      for _ in pairs(live.lookup or {}) do
        liveCount = liveCount + 1
      end
    end

    settings.status:SetText(table.concat({
      "Version: v" .. tostring(TP.version),
      "Market: " .. TP:MarketLabel() .. "  [" .. TP:MarketKey() .. "]",
      "Local: " .. TP:CountLocal() .. " items, last scan " .. ageText(db.lastScan),
      "Central: " .. TP:CentralLabel(),
      "Live ring: seq " .. tostring(live and live.seq or 0) .. " (" .. liveCount .. " items)",
    }, "\n"))

    settings.refreshing = false
  end
  settings.refresh = refresh

  panel:SetScript("OnShow", function()
    refresh()
  end)

  local elapsed = 0
  panel:SetScript("OnUpdate", function(_, delta)
    if settings.refreshing then
      return
    end
    elapsed = elapsed + delta
    if elapsed < 1 then
      return
    end
    elapsed = 0
    refresh()
  end)

  return settings
end

function TP:OpenSettings()
  local settings = self.settings
  if not settings then
    return
  end

  if settings.category and Settings and Settings.OpenToCategory then
    local ok = pcall(Settings.OpenToCategory, settings.category:GetID())
    if ok then
      return
    end
  end

  if InterfaceOptionsFrame_OpenToCategory then
    pcall(InterfaceOptionsFrame_OpenToCategory, settings.panel)
    pcall(InterfaceOptionsFrame_OpenToCategory, settings.panel)
  end
end

if StaticPopupDialogs then
  StaticPopupDialogs["TRADEPOST_CONFIRM_CLEAR"] = {
    text = "Clear all locally scanned TradePost prices for %s?",
    button1 = YES,
    button2 = NO,
    OnAccept = function()
      TP:ClearLocal()
      if TP.settings and TP.settings.refresh then
        TP.settings.refresh()
      end
    end,
    timeout = 0,
    whileDead = true,
    hideOnEscape = true,
    preferredIndex = 3,
  }
end

local settings = createPanel()
TP.settings = settings

if Settings and Settings.RegisterCanvasLayoutCategory and Settings.RegisterAddOnCategory then
  local category = Settings.RegisterCanvasLayoutCategory(settings.panel, "TradePost")
  Settings.RegisterAddOnCategory(category)
  settings.category = category
elseif InterfaceOptions_AddCategory then
  InterfaceOptions_AddCategory(settings.panel)
end
