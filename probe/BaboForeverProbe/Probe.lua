-- Babo Forever Probe
--
-- Phase 0 diagnostics. Nothing here is part of the shipping addon.
--
-- The probe answers four questions:
--   1. What does the client report for build, realm/market, faction and AH state?
--   2. Can the companion write a file that the running client can read through
--      LoadBindings, without /reload?
--   3. can a LoadOnDemand addon be updated on disk and loaded mid-session?
--   4. Is TOC metadata (GetAddOnMetadata) cached at startup or read live?
--
-- Results are kept in BABOFOREVER_PROBE (SavedVariables) and printed to chat.

local ADDON_NAME = ...

BABOFOREVER_PROBE = BABOFOREVER_PROBE or {}
local DB = BABOFOREVER_PROBE

local PREFIX = "|cff33ff99BaboProbe|r "

local function out(...)
  local parts = {}
  for i = 1, select("#", ...) do
    parts[i] = tostring(select(i, ...))
  end
  print(PREFIX .. table.concat(parts, " "))
end

local function currentSet()
  if GetCurrentBindingSet then
    return GetCurrentBindingSet()
  end
  return 1
end

local function safe(fn, ...)
  if type(fn) ~= "function" then
    return nil
  end
  local ok, value = pcall(fn, ...)
  if ok then
    return value
  end
  return nil
end

-- GetItemCommodityStatus takes an ItemLocation, not an itemID.
local function commodityStatus(itemID)
  if not (C_AuctionHouse and C_AuctionHouse.GetItemCommodityStatus and ItemLocation) then
    return nil
  end
  local ok, status = pcall(function()
    local location = ItemLocation:CreateFromItemID(itemID)
    return C_AuctionHouse.GetItemCommodityStatus(location)
  end)
  if ok then
    return status
  end
  return nil
end

local function valueString(value)
  if type(value) == "table" then
    local parts = {}
    for k, v in pairs(value) do
      parts[#parts + 1] = tostring(k) .. "=" .. tostring(v)
    end
    table.sort(parts)
    return "{" .. table.concat(parts, ", ") .. "}"
  end
  return tostring(value)
end

local function record(label, data)
  DB.results = DB.results or {}
  DB.results[#DB.results + 1] = {
    at = time(),
    when = date("%Y-%m-%d %H:%M:%S"),
    label = label,
    data = data,
  }
  while #DB.results > 250 do
    table.remove(DB.results, 1)
  end
end

-- Reserved keys for the bindings mailbox. F1..F12 and digits are separate
-- entries; letters F, I, O and T are intentionally left out to make accidental
-- collisions with common user binds less likely.
local KEYS = {}
do
  local letters = {
    "A", "B", "C", "D", "E", "G", "H", "J", "K", "L", "M",
    "N", "P", "Q", "R", "S", "U", "V", "W", "X", "Y", "Z",
  }
  for i = 1, 12 do
    KEYS[#KEYS + 1] = "CTRL-ALT-SHIFT-F" .. i
  end
  for i = 0, 9 do
    KEYS[#KEYS + 1] = "CTRL-ALT-SHIFT-" .. i
  end
  for _, letter in ipairs(letters) do
    KEYS[#KEYS + 1] = "CTRL-ALT-SHIFT-" .. letter
  end
end

local function environment()
  local buildVersion, buildNumber, buildDate, interfaceVersion = GetBuildInfo()
  return {
    addon = ADDON_NAME,
    buildVersion = buildVersion,
    buildNumber = buildNumber,
    buildDate = buildDate,
    interfaceVersion = interfaceVersion,
    projectID = WOW_PROJECT_ID,
    camelot = WOW_PROJECT_CAMELOT,
    realm = GetRealmName and GetRealmName() or nil,
    region = GetCurrentRegion and GetCurrentRegion() or nil,
    faction = UnitFactionGroup and UnitFactionGroup("player") or nil,
    zone = GetZoneText and GetZoneText() or nil,
    subzone = GetSubZoneText and GetSubZoneText() or nil,
    bindingSet = currentSet(),
    numBindings = GetNumBindings and GetNumBindings() or nil,
    ahReplicateItems = C_AuctionHouse and safe(C_AuctionHouse.GetNumReplicateItems) or nil,
    ahThrottleReady = C_AuctionHouse and safe(C_AuctionHouse.IsThrottledMessageSystemReady) or nil,
    commodityLinen = commodityStatus(2589),
  }
end

local function printTable(label, tbl)
  out(label)
  local keys = {}
  for k in pairs(tbl) do
    keys[#keys + 1] = k
  end
  table.sort(keys, function(a, b) return tostring(a) < tostring(b) end)
  for _, k in ipairs(keys) do
    out("  " .. tostring(k) .. " = " .. valueString(tbl[k]))
  end
end

local function cmdDump()
  local env = environment()
  printTable("environment", env)
  record("dump", env)
end

local function cmdBindWrite(mode, payload)
  mode = mode or "click"
  payload = payload or "TEST123"

  local key = KEYS[1]
  local command
  if mode == "raw" then
    command = "BFD1:" .. payload
  elseif mode == "macro" then
    command = "MACRO BF" .. payload
  elseif mode == "spell" then
    command = "SPELL " .. payload
  else
    command = "CLICK BF" .. payload .. ":LeftButton"
  end

  local before = GetBindingAction(key)
  local ok = SetBinding(key, command)
  local after = GetBindingAction(key)
  local set = currentSet()
  SaveBindings(set)

  out("bindwrite", "key=" .. key, "mode=" .. mode, "ok=" .. tostring(ok))
  out("  before=" .. tostring(before))
  out("  after=" .. tostring(after), "len=" .. tostring(after and #after or 0))
  out("  set=" .. tostring(set))

  record("bindwrite", {
    key = key,
    mode = mode,
    command = command,
    ok = ok,
    before = before,
    after = after,
    set = set,
  })
end

local function cmdBindRead()
  local set = currentSet()
  local ok, err = pcall(LoadBindings, set)
  out("bindread", "set=" .. tostring(set), "ok=" .. tostring(ok), "err=" .. tostring(err))
  out("  numBindings=" .. tostring(GetNumBindings and GetNumBindings() or nil))

  local found = 0
  for _, key in ipairs(KEYS) do
    local action = GetBindingAction(key)
    if action and action ~= "" then
      found = found + 1
      out("  " .. key .. " = " .. action)
    end
  end
  out("  reserved keys with actions: " .. found)
  record("bindread", {
    set = set,
    ok = ok,
    err = err,
    found = found,
  })
end

local function cmdBindLen(n)
  n = tonumber(n) or 1024
  local key = KEYS[2]
  local command = "CLICK BF" .. string.rep("A", n) .. ":LeftButton"
  local ok = SetBinding(key, command)
  local after = GetBindingAction(key)
  SaveBindings(currentSet())
  out("bindlen", "requested=" .. n, "ok=" .. tostring(ok),
    "stored=" .. tostring(after and #after or 0))
  record("bindlen", {
    requested = n,
    ok = ok,
    stored = after and #after or 0,
  })
end

local function cmdKeys()
  local set = currentSet()
  out("keys set=" .. tostring(set))
  for _, key in ipairs(KEYS) do
    local action = GetBindingAction(key)
    out("  " .. key .. " = " .. tostring(action or "-"))
  end
end

local function cmdReset()
  local removed = 0
  for _, key in ipairs(KEYS) do
    local action = GetBindingAction(key)
    if action and (
      action:find("^BF") or
      action:find("^CLICK BF") or
      action:find("^MACRO BF") or
      action:find("^SPELL BF")
    ) then
      SetBinding(key, nil)
      removed = removed + 1
    end
  end
  SaveBindings(currentSet())
  out("reset removed=" .. removed .. " (run /bfprobe keys to verify)")
  record("reset", { removed = removed })
end

local function slotName(slot)
  if slot and slot:upper() == "B" then
    return "BFProbeDataB"
  end
  return "BFProbeDataA"
end

local function cmdRing(slot)
  local name = slotName(slot)
  local loader = (C_AddOns and C_AddOns.LoadAddOn) or LoadAddOn
  local loaded, reason = loader(name)
  local payload = _G[name .. "_Payload"]
  local stamp = _G[name .. "_Stamp"]
  out("ring", name, "loaded=" .. tostring(loaded), "reason=" .. tostring(reason))
  out("  payload=" .. tostring(payload), "stamp=" .. tostring(stamp))
  record("ring", {
    addon = name,
    loaded = loaded,
    reason = reason,
    payload = payload,
    stamp = stamp,
  })
end

local function cmdMeta(slot)
  local name = slotName(slot)
  local getter = (C_AddOns and C_AddOns.GetAddOnMetadata) or GetAddOnMetadata
  local ok, value = pcall(getter, name, "X-BF-Data")
  local okTitle, title = pcall(getter, name, "Title")
  out("meta", name, "x-bf-data=" .. tostring(value), "ok=" .. tostring(ok))
  out("  title=" .. tostring(title), "titleOk=" .. tostring(okTitle))
  record("meta", {
    addon = name,
    ok = ok,
    value = value,
    title = title,
  })
end

local function cmdAh()
  local data = {
    replicateItems = C_AuctionHouse and safe(C_AuctionHouse.GetNumReplicateItems) or nil,
    throttleReady = C_AuctionHouse and safe(C_AuctionHouse.IsThrottledMessageSystemReady) or nil,
    commodityLinen = commodityStatus(2589),
    itemBag = commodityStatus(4496),
    zone = GetZoneText and GetZoneText() or nil,
    subzone = GetSubZoneText and GetSubZoneText() or nil,
  }
  printTable("auction-house", data)
  record("ah", data)
end

local function cmdReport()
  DB.results = DB.results or {}
  out("report entries=" .. #DB.results)
  for i = math.max(1, #DB.results - 30), #DB.results do
    local entry = DB.results[i]
    out("  [" .. tostring(entry.when) .. "] " .. tostring(entry.label) ..
      " " .. valueString(entry.data))
  end
  out("SavedVariables: BABOFOREVER_PROBE (written on /reload or logout)")
end

local function cmdClear()
  DB.results = {}
  out("results cleared")
end

local function cmdHelp()
  out("commands:")
  out("  /bfprobe dump                 -- build, realm, faction, zone, AH state")
  out("  /bfprobe ah                   -- auction-house state only")
  out("  /bfprobe bindwrite [mode] [payload] -- click|raw|macro|spell")
  out("  /bfprobe bindread             -- LoadBindings + read reserved keys")
  out("  /bfprobe bindlen [n]          -- write a long CLICK action")
  out("  /bfprobe keys                 -- list reserved keys and actions")
  out("  /bfprobe ring [A|B]           -- LoadAddOn a probe data slot")
  out("  /bfprobe meta [A|B]           -- read X-BF-Data from the TOC")
  out("  /bfprobe reset                -- remove probe bindings")
  out("  /bfprobe report               -- print recorded results")
  out("  /bfprobe clear                -- clear recorded results")
  out("  /bfprobe help")
end

SLASH_BABOFOREVERPROBE1 = "/bfprobe"
SlashCmdList["BABOFOREVERPROBE"] = function(msg)
  local command, rest = string.match(msg or "", "^(%S*)%s*(.*)$")
  command = string.lower(command or "")

  local args = {}
  for word in string.gmatch(rest or "", "%S+") do
    args[#args + 1] = word
  end

  local handlers = {
    dump = function() cmdDump() end,
    env = function() cmdDump() end,
    ah = function() cmdAh() end,
    bindwrite = function() cmdBindWrite(args[1], args[2]) end,
    bindread = function() cmdBindRead() end,
    bindlen = function() cmdBindLen(args[1]) end,
    keys = function() cmdKeys() end,
    ring = function() cmdRing(args[1]) end,
    meta = function() cmdMeta(args[1]) end,
    reset = function() cmdReset() end,
    report = function() cmdReport() end,
    clear = function() cmdClear() end,
    help = function() cmdHelp() end,
  }

  local handler = handlers[command]
  if not handler then
    cmdHelp()
    return
  end

  local ok, err = pcall(handler)
  if not ok then
    out("error: " .. tostring(err))
  end
end

local frame = CreateFrame("Frame")
frame:RegisterEvent("ADDON_LOADED")
frame:RegisterEvent("PLAYER_LOGIN")
frame:SetScript("OnEvent", function(_, event, arg1)
  if event == "ADDON_LOADED" and arg1 == ADDON_NAME then
    BABOFOREVER_PROBE = BABOFOREVER_PROBE or {}
    DB = BABOFOREVER_PROBE
    DB.version = 1
    DB.results = DB.results or {}
    out("loaded. /bfprobe help")
  elseif event == "PLAYER_LOGIN" then
    out("ready. start with /bfprobe dump")
  end
end)
