"use strict";

const $ = (selector) => document.querySelector(selector);

const QUALITY = [
  "#9d9d9d", "#ffffff", "#1eff00", "#0070dd",
  "#a335ee", "#ff8000", "#e6cc80", "#e6cc80",
];
const QUALITY_NAMES = [
  "Poor", "Common", "Uncommon", "Rare",
  "Epic", "Legendary", "Artifact", "Heirloom",
];

const PAGE_SIZE = 50;

// Starting direction for each sort key. The user can flip it afterwards.
const DEFAULT_SORT_DIR = {
  price: "asc",
  name: "asc",
  ilvl: "desc",
  quality: "desc",
  scanned: "desc",
};

const CLASS_NAMES = {
  0: "Consumable", 1: "Container", 2: "Weapon", 3: "Gem",
  4: "Armor", 5: "Reagent", 6: "Projectile", 7: "Trade Goods",
  8: "Generic", 9: "Recipe", 11: "Quiver", 12: "Quest",
  13: "Key", 15: "Miscellaneous", 16: "Glyph", 17: "Battle Pet",
  18: "WoW Token",
};

const SUBCLASSES = {
  2: {
    0: "One-Hand Axe", 1: "Two-Hand Axe", 2: "Bow", 3: "Gun",
    4: "One-Hand Mace", 5: "Two-Hand Mace", 6: "Polearm",
    7: "One-Hand Sword", 8: "Two-Hand Sword", 10: "Staff",
    13: "Fist Weapon", 14: "Miscellaneous", 15: "Dagger",
    16: "Thrown", 17: "Spear", 18: "Crossbow", 19: "Wand",
    20: "Fishing Pole",
  },
  4: {
    0: "Miscellaneous", 1: "Cloth", 2: "Leather", 3: "Mail",
    4: "Plate", 5: "Buckler", 6: "Shield", 7: "Libram",
    8: "Idol", 9: "Totem", 10: "Sigil",
  },
  6: { 2: "Arrow", 3: "Bullet" },
  7: {
    0: "Trade Goods", 1: "Parts", 2: "Explosives", 3: "Devices",
    4: "Jewelcrafting", 5: "Cloth", 6: "Leather", 7: "Metal & Stone",
    8: "Cooking", 9: "Herb", 10: "Elemental", 11: "Other",
    12: "Enchanting", 16: "Inscription",
  },
};

const INVENTORY_NAMES = {
  0: "Non-equippable", 1: "Head", 2: "Neck", 3: "Shoulder",
  4: "Shirt", 5: "Chest", 6: "Waist", 7: "Legs", 8: "Feet",
  9: "Wrist", 10: "Hands", 11: "Finger", 12: "Trinket",
  13: "One-Hand", 14: "Shield", 15: "Ranged", 16: "Back",
  17: "Two-Hand", 18: "Bag", 19: "Tabard", 20: "Chest",
  21: "Main Hand", 22: "Off Hand", 23: "Held in Off-hand",
  24: "Ammo", 25: "Thrown", 26: "Ranged", 27: "Relic",
};

const EQUIP_LOCATIONS = {
  INVTYPE_HEAD: "Head", INVTYPE_NECK: "Neck", INVTYPE_SHOULDER: "Shoulder",
  INVTYPE_BODY: "Shirt", INVTYPE_CHEST: "Chest", INVTYPE_ROBE: "Chest",
  INVTYPE_WAIST: "Waist", INVTYPE_LEGS: "Legs", INVTYPE_FEET: "Feet",
  INVTYPE_WRIST: "Wrist", INVTYPE_HAND: "Hands", INVTYPE_FINGER: "Finger",
  INVTYPE_TRINKET: "Trinket", INVTYPE_WEAPON: "One-Hand",
  INVTYPE_SHIELD: "Shield", INVTYPE_RANGED: "Ranged", INVTYPE_CLOAK: "Back",
  INVTYPE_2HWEAPON: "Two-Hand", INVTYPE_BAG: "Bag", INVTYPE_TABARD: "Tabard",
  INVTYPE_WEAPONMAINHAND: "Main Hand", INVTYPE_WEAPONOFFHAND: "Off Hand",
  INVTYPE_HOLDABLE: "Held in Off-hand", INVTYPE_AMMO: "Ammo",
  INVTYPE_THROWN: "Thrown", INVTYPE_RANGEDRIGHT: "Ranged",
  INVTYPE_RELIC: "Relic", INVTYPE_PROFESSION_TOOL: "Profession Tool",
  INVTYPE_PROFESSION_GEAR: "Profession Gear",
};

const BIND_TYPES = {
  1: "Binds when picked up",
  2: "Binds when equipped",
  3: "Binds when used",
  4: "Quest item",
  5: "Binds to account",
  6: "Binds to account until equipped",
  7: "Binds to account",
  8: "Binds to account until equipped",
  9: "Binds to account",
};

const STAT_LABELS = {
  stamina: "Stamina", strength: "Strength", agility: "Agility",
  intellect: "Intellect", spirit: "Spirit", armor: "Armor",
  attack_power: "Attack Power", ranged_attack_power: "Ranged Attack Power",
  spell_power: "Spell Power", crit_rating: "Critical Strike",
  haste_rating: "Haste", mastery_rating: "Mastery",
  versatility: "Versatility", hit_rating: "Hit", defense_rating: "Defense",
  mana: "Mana", health: "Health", block_rating: "Block",
  dodge_rating: "Dodge", parry_rating: "Parry",
  expertise_rating: "Expertise", resilience_rating: "Resilience",
  spell_penetration: "Spell Penetration", mp5: "Mana per 5 sec",
  hp5: "Health per 5 sec", pvp_power: "PvP Power",
  pvp_resilience: "PvP Resilience",
};

const DETAIL_KNOWN_KEYS = new Set([
  "description", "flavor", "flavor_text", "lines", "tooltip", "text",
  "equip", "equip_text", "use", "use_text", "spells", "set", "set_bonus",
  "requirements", "note", "stats",
]);

const state = {
  market: localStorage.getItem("tradepost-market") || "",
  results: [],
  selected: null,
  session: null,
  range: localStorage.getItem("tradepost-range") || "all",
  watch: new Set(JSON.parse(localStorage.getItem("tradepost-watch") || "[]")),
  page: 1,
  total: 0,
};

const detailCache = new Map();
const variantCache = new Map();
const cardState = {
  el: null, timer: 0, pinned: false, key: "", token: 0, x: 0, y: 0,
  restoreScroll: null,
};
// Keeps keyboard focus on the variant list that was just used, even though
// selecting a variant rebuilds the card and the detail panel.
const variantFocus = { surface: "", level: -1, itemID: 0, expires: 0 };

function saveWatch() {
  localStorage.setItem("tradepost-watch", JSON.stringify([...state.watch]));
}

function money(copper) {
  copper = Math.max(0, Math.floor(copper || 0));
  const gold = Math.floor(copper / 10000);
  const silver = Math.floor((copper % 10000) / 100);
  const rest = copper % 100;
  const parts = [];
  if (gold) parts.push(`${gold.toLocaleString()}g`);
  if (silver) parts.push(`${silver}s`);
  if (rest || parts.length === 0) parts.push(`${rest}c`);
  return parts.join(" ");
}

function ageText(seconds) {
  const diff = Math.max(0, Math.floor(Date.now() / 1000 - (seconds || 0)));
  if (diff < 60) return "just now";
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

function itemName(item) {
  return item.name || `Item #${item.item_id}`;
}

function itemLevel(item) {
  const level = Number(item && item.item_level) || levelFromKey(item && item.key);
  return level > 0 ? `ilvl ${level}` : "";
}

function levelFromKey(key) {
  const parts = String(key || "").split(":");
  if (parts[0] === "g" && parts.length > 2) {
    const level = Number(parts[2]);
    return level > 0 ? level : 0;
  }
  return 0;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined && text !== null) node.textContent = text;
  return node;
}

function qualityColor(quality) {
  return QUALITY[Number(quality)] || "#ffffff";
}

function subclassName(classID, subclassID) {
  const map = SUBCLASSES[String(classID)];
  return (map && map[String(subclassID)]) || "";
}

function slotName(info) {
  const inventoryType = Number(info.inventory_type);
  if (inventoryType > 0) return INVENTORY_NAMES[inventoryType] || "";
  const location = info.equip_loc;
  if (typeof location === "string" && location && location !== "INVTYPE_NON_EQUIP") {
    return EQUIP_LOCATIONS[location] || "";
  }
  return "";
}

function typeLine(info) {
  const classID = info.class_id;
  const parts = [];
  const className = CLASS_NAMES[String(classID)];
  if (className) parts.push(className);
  const subclass = subclassName(classID, info.subclass_id);
  if (subclass && !parts.includes(subclass)) parts.push(subclass);
  // Weapon subclasses already describe the hand/slot; avoid "One-Hand Sword · One-Hand".
  if (Number(classID) !== 2) {
    const slot = slotName(info);
    if (slot && !parts.includes(slot)) parts.push(slot);
  }
  return parts.join(" · ");
}

function statLabel(key) {
  const normalized = String(key)
    .replace(/^ITEM_MOD_/i, "")
    .toLowerCase()
    .replace(/_short$/, "");
  if (normalized === "resistance0_name" || normalized === "resistance0") return "Armor";
  if (STAT_LABELS[normalized]) return STAT_LABELS[normalized];
  return normalized
    .replace(/[_-]+/g, " ")
    .replace(/\b[a-z]/g, (letter) => letter.toUpperCase());
}

function statValueText(label, value) {
  if (typeof value === "number" && Number.isFinite(value)) {
    const suffix = /(pct|percent|%)/i.test(label) ? "%" : "";
    const prefix = value > 0 && !suffix ? "+" : "";
    return `${prefix}${value.toLocaleString()}${suffix}`;
  }
  if (typeof value === "string") return value.trim() || null;
  if (typeof value === "boolean") return value ? "Yes" : null;
  if (value === null || value === undefined) return null;
  return String(value);
}

function normalizeStats(raw) {
  if (raw === null || raw === undefined || raw === "") return [];
  let data = raw;
  if (typeof data === "string") {
    try {
      data = JSON.parse(data);
    } catch (_) {
      return [{ label: "", value: data }];
    }
  }
  const lines = [];
  const push = (key, value) => {
    const text = statValueText(String(key), value);
    if (text !== null) lines.push({ label: statLabel(key), value: text });
  };
  if (Array.isArray(data)) {
    for (const entry of data) {
      if (typeof entry === "string") {
        if (entry.trim()) lines.push({ label: "", value: entry.trim() });
      } else if (entry && typeof entry === "object") {
        const key = entry.key ?? entry.stat ?? entry.name ?? entry.label;
        if (key !== undefined && key !== null && key !== "") {
          push(key, entry.value ?? entry.amount ?? entry.rating ?? "");
        } else if (typeof entry.text === "string" && entry.text.trim()) {
          lines.push({ label: "", value: entry.text.trim() });
        }
      }
    }
    return lines;
  }
  if (data && typeof data === "object") {
    for (const [key, value] of Object.entries(data)) {
      if (value === null || value === undefined || value === "") continue;
      if (typeof value === "object" && !Array.isArray(value)) {
        push(key, value.value ?? value.amount ?? value.rating ?? "");
      } else {
        push(key, value);
      }
    }
  }
  return lines;
}

function safeColor(value) {
  if (typeof value !== "string") return "";
  const color = value.trim();
  if (/^#[0-9a-f]{3,8}$/i.test(color)) return color;
  if (/^[a-z]{3,20}$/i.test(color)) return color;
  return "";
}

function detailEntries(details) {
  const lines = [];
  if (!details) return lines;

  if (typeof details === "string") {
    const trimmed = details.trim();
    if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
      try {
        return detailEntries(JSON.parse(trimmed));
      } catch (_) { /* plain text below */ }
    }
    if (trimmed) lines.push({ text: details });
    return lines;
  }
  if (typeof details === "number" || typeof details === "boolean") {
    lines.push({ text: String(details) });
    return lines;
  }

  let list = null;
  if (Array.isArray(details)) {
    list = details;
  } else if (typeof details === "object") {
    if (Array.isArray(details.lines)) list = details.lines;
    else if (typeof details.lines === "string") list = [details.lines];
    else if (Array.isArray(details.tooltip)) list = details.tooltip;
    else if (typeof details.tooltip === "string") list = [details.tooltip];
    else if (Array.isArray(details.text)) list = details.text;
    else if (typeof details.text === "string") list = [details.text];
  }

  for (const entry of list || []) {
    if (typeof entry === "string") {
      const text = entry.trim();
      if (text && !/retrieving item information/i.test(text)) lines.push({ text });
      continue;
    }
    if (typeof entry === "number" || typeof entry === "boolean") {
      lines.push({ text: String(entry) });
      continue;
    }
    if (!entry || typeof entry !== "object") continue;
    if (Number(entry.type) === 41) continue;
    if (typeof entry.left === "string" && /retrieving item information/i.test(entry.left)) continue;
    if (typeof entry.text === "string" && /retrieving item information/i.test(entry.text)) continue;
    if (typeof entry.text === "string" || typeof entry.color === "string") {
      const label = entry.label ?? entry.left ?? entry.name;
      const value = entry.value ?? entry.right ?? entry.amount;
      if (label !== undefined && label !== null && label !== "") {
        lines.push({
          label: String(label),
          value: value === undefined || value === null ? "" : String(value),
          color: entry.color,
        });
      } else if (typeof entry.text === "string") {
        lines.push({ text: entry.text, color: entry.color });
      } else if (value !== undefined && value !== null) {
        lines.push({ text: String(value), color: entry.color });
      }
      continue;
    }
    const label = entry.label ?? entry.left ?? entry.name ?? entry.key;
    const value = entry.value ?? entry.right ?? entry.amount ?? entry.rating;
    if (label === undefined || label === null || label === "") {
      if (value !== undefined && value !== null) {
        lines.push({ text: String(value), color: entry.color });
      }
      continue;
    }
    lines.push({
      label: String(label),
      value: value === undefined || value === null ? "" : String(value),
      color: entry.color,
    });
  }

  if (details && typeof details === "object" && !Array.isArray(details)) {
    const extras = [
      "equip", "equip_text", "use", "use_text", "spells", "set",
      "set_bonus", "requirements", "note",
    ];
    for (const key of extras) {
      const value = details[key];
      const values = Array.isArray(value) ? value : [value];
      for (const entry of values) {
        if (typeof entry === "string" && entry.trim()) {
          lines.push({ text: entry.trim() });
        }
      }
    }
  }

  return lines;
}

function unknownDetailJSON(details) {
  if (!details || typeof details !== "object" || Array.isArray(details)) return "";
  const rest = {};
  for (const [key, value] of Object.entries(details)) {
    if (!DETAIL_KNOWN_KEYS.has(key) && value !== null && value !== undefined) {
      rest[key] = value;
    }
  }
  if (!Object.keys(rest).length) return "";
  try {
    return JSON.stringify(rest, null, 2);
  } catch (_) {
    return "";
  }
}

function detailDescription(details) {
  if (!details || typeof details !== "object" || Array.isArray(details)) return "";
  for (const key of ["description", "flavor", "flavor_text"]) {
    if (typeof details[key] === "string" && details[key].trim()) {
      return details[key].trim();
    }
  }
  return "";
}

async function api(path, params, options) {
  const url = new URL(path, window.location.origin);
  for (const [key, value] of Object.entries(params || {})) {
    if (value !== undefined && value !== null && value !== "") {
      url.searchParams.set(key, value);
    }
  }
  const init = { ...(options || {}) };
  init.headers = { Accept: "application/json", ...(init.headers || {}) };
  if (init.body && !init.headers["Content-Type"]) {
    init.headers["Content-Type"] = "application/json";
  }
  const response = await fetch(url, init);
  if (response.status === 204) {
    return null;
  }
  if (!response.ok) {
    let message = response.statusText;
    try {
      const body = await response.json();
      if (body.error) message = body.error;
    } catch (_) { /* keep status text */ }
    throw new Error(message);
  }
  return response.json();
}

async function loadLive() {
  try {
    const data = await api("/v1/live");
    const count = Number(data.live_clients) || 0;
    const el = $("#live");
    el.textContent = `${count} live contributor${count === 1 ? "" : "s"}`;
    el.hidden = false;
  } catch (_) {
    $("#live").hidden = true;
  }
}

async function loadMeta() {
  const markets = await api("/v1/meta");
  const select = $("#market");
  select.innerHTML = "";
  if (!markets.length) {
    $("#meta").textContent = "No snapshots yet";
    return;
  }
  for (const market of markets) {
    const option = document.createElement("option");
    option.value = market.market;
    option.textContent = market.market;
    select.append(option);
  }
  if (!markets.some((market) => market.market === state.market)) {
    const preferred = markets.find((market) => market.market === "eu:pve:horde");
    state.market = (preferred || markets[0]).market;
  }
  select.value = state.market;

  const selected = markets.find((market) => market.market === state.market);
  if (selected) {
    $("#meta").textContent =
      `${selected.items.toLocaleString()} items · snapshot v${selected.version} · ${ageText(selected.last_scan)}`;
  }
}

function starButton(item) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "star" + (state.watch.has(item.key) ? " on" : "");
  button.textContent = state.watch.has(item.key) ? "★" : "☆";
  button.title = state.watch.has(item.key) ? "Remove from watchlist" : "Add to watchlist";
  button.addEventListener("click", (event) => {
    event.stopPropagation();
    toggleWatch(item.key);
  });
  return button;
}

function renderPager() {
  const pager = $("#pager");
  const total = state.total;
  if (!total) {
    pager.hidden = true;
    return;
  }
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const page = Math.min(state.page, pages);
  const start = (page - 1) * PAGE_SIZE + 1;
  const end = Math.min(total, page * PAGE_SIZE);
  $("#page-label").textContent =
    `${start.toLocaleString()}–${end.toLocaleString()} of ${total.toLocaleString()}`;
  $("#page-of").textContent = `page ${page} / ${pages}`;
  $("#prev-page").disabled = page <= 1;
  $("#next-page").disabled = page >= pages;
  pager.hidden = false;
}

function renderResults() {
  const container = $("#results");
  if (!cardState.pinned) hideCard();
  const watchOnly = $("#watch-only").checked;
  const rows = state.results.filter((item) => !watchOnly || state.watch.has(item.key));
  const totalLabel = state.total.toLocaleString();
  $("#status").textContent = watchOnly
    ? `${rows.length} watched on this page · ${totalLabel} match${state.total === 1 ? "" : "es"}`
    : `${totalLabel} match${state.total === 1 ? "" : "es"} · showing ${rows.length}`;

  renderPager();

  if (!rows.length) {
    container.innerHTML = "";
    const empty = document.createElement("p");
    empty.className = "empty";
    empty.textContent = watchOnly
      ? "No watched items on this page."
      : "No items found. Try loosening the filters.";
    container.append(empty);
    return;
  }

  const table = document.createElement("table");
  const head = document.createElement("thead");
  head.innerHTML = "<tr><th></th><th>Item</th><th class='num'>Price</th><th class='num'>Qty</th><th class='num'>Age</th></tr>";
  table.append(head);

  const body = document.createElement("tbody");
  for (const item of rows) {
    const row = document.createElement("tr");
    row.className = "row" + (state.selected && state.selected.key === item.key ? " active" : "");
    row.tabIndex = 0;
    row.addEventListener("click", (event) => selectItem(item, event.clientX, event.clientY));
    row.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      const rect = row.getBoundingClientRect();
      selectItem(item, rect.left + 28, Math.min(rect.top + rect.height, window.innerHeight - 24));
    });
    row.addEventListener("mouseenter", (event) => {
      scheduleHover(item, event.clientX, event.clientY);
    });
    row.addEventListener("mouseleave", cancelHover);
    row.addEventListener("focus", () => {
      if (cardState.pinned) return;
      const rect = row.getBoundingClientRect();
      scheduleHover(item, rect.left + 28, Math.min(rect.bottom, window.innerHeight - 24), 0);
    });
    row.addEventListener("blur", () => {
      if (!cardState.pinned) hideCard();
    });

    const starCell = document.createElement("td");
    starCell.append(starButton(item));
    row.append(starCell);

    const itemCell = document.createElement("td");
    const name = document.createElement("div");
    name.className = "item-name";
    name.textContent = itemName(item);
    name.style.color = qualityColor(item.quality);
    const sub = document.createElement("div");
    sub.className = "item-sub";
    sub.textContent = [itemLevel(item), `#${item.item_id}`].filter(Boolean).join(" · ");
    itemCell.append(name, sub);
    row.append(itemCell);

    const price = document.createElement("td");
    price.className = "price num";
    price.textContent = money(item.price);
    row.append(price);

    const qty = document.createElement("td");
    qty.className = "qty num";
    qty.textContent = (item.quantity || 0).toLocaleString();
    row.append(qty);

    const scanned = document.createElement("td");
    scanned.className = "age num";
    scanned.textContent = ageText(item.scanned);
    row.append(scanned);

    body.append(row);
  }
  table.append(body);
  container.innerHTML = "";
  container.append(table);
}

function getDetail(item) {
  const cacheKey = `${state.market}|${item.key}`;
  const cached = detailCache.get(cacheKey);
  if (cached) return cached;
  const request = api("/v1/item/detail", { market: state.market, key: item.key })
    .catch((error) => {
      detailCache.delete(cacheKey);
      throw error;
    });
  detailCache.set(cacheKey, request);
  return request;
}

// ---------- Variant selector ----------

function keyItemID(key) {
  const parts = String(key || "").split(":");
  if (parts[0] !== "i" && parts[0] !== "g") return 0;
  const id = Number(parts[1]);
  return Number.isFinite(id) && id > 0 ? id : 0;
}

// One item ID can have several price keys (i:<id>, g:<id>:0, g:<id>:<level>).
// They are grouped by effective level: an explicit level in the key wins,
// then the item's metadata level, and 0 means the base variant.
function effectiveLevel(item, fallbackLevel) {
  const fromKey = levelFromKey(item && item.key);
  if (fromKey > 0) return fromKey;
  const meta = Number(item && item.item_level) || Number(fallbackLevel) || 0;
  return meta > 0 ? Math.floor(meta) : 0;
}

function variantLabel(level) {
  return level > 0 ? `ilvl ${level}` : "Base";
}

// When two keys resolve to the same level, prefer the explicit level key,
// then the level-0 key, then a plain item key.
function variantRank(key) {
  const parts = String(key || "").split(":");
  if (parts[0] === "g") return Number(parts[2]) > 0 ? 0 : 1;
  return 2;
}

function preferredVariant(candidate, current) {
  if (candidate.rank !== current.rank) return candidate.rank < current.rank;
  if (candidate.price !== current.price) return candidate.price < current.price;
  return candidate.quantity > current.quantity;
}

function buildVariants(items, itemID) {
  const byLevel = new Map();
  for (const raw of items || []) {
    if (!raw || !raw.key) continue;
    if (itemID && keyItemID(raw.key) !== itemID) continue;
    const level = effectiveLevel(raw);
    const entry = {
      ...raw,
      level,
      label: variantLabel(level),
      rank: variantRank(raw.key),
      price: Number(raw.price) || 0,
      quantity: Number(raw.quantity) || 0,
    };
    const current = byLevel.get(level);
    if (!current || preferredVariant(entry, current)) byLevel.set(level, entry);
  }
  return [...byLevel.values()].sort((a, b) => a.level - b.level);
}

function getVariants(item) {
  const itemID = Number(item && item.item_id) || keyItemID(item && item.key);
  if (!itemID) return Promise.resolve([]);
  const cacheKey = `${state.market}|${itemID}`;
  const cached = variantCache.get(cacheKey);
  if (cached) return cached;
  const request = api("/v1/items", { market: state.market, q: itemID, limit: 200 })
    .then((data) => buildVariants(data && data.items, itemID))
    .catch((error) => {
      variantCache.delete(cacheKey);
      throw error;
    });
  variantCache.set(cacheKey, request);
  return request;
}

function selectVariant(item, surface) {
  if (!item || !item.key) return;
  if (state.selected && state.selected.key === item.key) return;
  const card = cardElement();
  if (card && !card.hidden) cardState.restoreScroll = card.scrollTop;
  variantFocus.surface = surface || "detail";
  variantFocus.level = effectiveLevel(item);
  variantFocus.itemID = Number(item.item_id) || keyItemID(item.key);
  variantFocus.expires = Date.now() + 2500;
  selectItem(item, cardState.x, cardState.y);
}

function variantSection(options) {
  const interactive = Boolean(options.interactive);
  const surface = options.surface;
  const activeLevel = Number(options.activeLevel) || 0;
  const itemID = Number(options.item && options.item.item_id) ||
    keyItemID(options.item && options.item.key);

  const box = el("div", "variants");
  box.hidden = true;
  const head = el("div", "variants-head");
  const count = el("span", "variants-count");
  head.append(el("span", "", "Variants"), count);
  const list = el("div", "variant-list");
  if (interactive) {
    list.setAttribute("role", "group");
    list.setAttribute("aria-label", "Item variants");
  }
  box.append(head, list);

  getVariants(options.item).then((variants) => {
    if (!variants.length || variants.length < 2 || !box.isConnected) return;
    count.textContent = `${variants.length} levels`;
    let activeEntry = null;
    for (const variant of variants) {
      const active = variant.level === activeLevel;
      const entry = document.createElement(interactive ? "button" : "div");
      entry.className = "variant" + (active ? " is-active" : "");
      const quantity = (variant.quantity || 0).toLocaleString();
      entry.append(
        el("span", "variant-level", variant.label),
        el("span", "variant-price", money(variant.price)),
        el("span", "variant-qty", quantity),
      );
      entry.title = `${variant.label} · ${money(variant.price)} · ${quantity} available`;
      if (interactive) {
        entry.type = "button";
        entry.setAttribute("aria-pressed", String(active));
        entry.setAttribute("aria-label",
          `${variant.label}, ${money(variant.price)}, ${quantity} in stock`);
        entry.addEventListener("click", (event) => {
          // Keep the document click handler from closing the pinned card when
          // the click target is replaced by the re-render.
          event.preventDefault();
          event.stopPropagation();
          selectVariant(variant, surface);
        });
      }
      if (active) activeEntry = entry;
      list.append(entry);
    }
    box.hidden = false;
    if (activeEntry && list.scrollHeight > list.clientHeight) {
      list.scrollTop = Math.max(0,
        activeEntry.offsetTop - (list.clientHeight - activeEntry.offsetHeight) / 2);
    }
    if (list.scrollHeight > list.clientHeight) {
      const syncMore = () => {
        const remaining = list.scrollHeight - list.clientHeight - list.scrollTop;
        list.classList.toggle("has-more", remaining > 2);
      };
      list.addEventListener("scroll", syncMore, { passive: true });
      syncMore();
    }
    if (interactive && variantFocus.surface === surface &&
        variantFocus.itemID === itemID && variantFocus.level === activeLevel &&
        Date.now() < variantFocus.expires) {
      variantFocus.surface = "";
      const target = activeEntry || list.querySelector(".variant");
      if (target) target.focus({ preventScroll: true });
    }
    if (typeof options.onGrow === "function") options.onGrow();
  }).catch(() => { /* variants are a bonus; leave the rest of the card intact */ });

  return box;
}

function cardElement() {
  if (!cardState.el) cardState.el = $("#item-card");
  return cardState.el;
}

function scheduleHover(item, x, y, delay = 140) {
  clearTimeout(cardState.timer);
  if (cardState.pinned) return;
  cardState.timer = setTimeout(() => {
    if (cardState.pinned) return;
    showCard(item, x, y, false);
  }, delay);
}

function cancelHover() {
  clearTimeout(cardState.timer);
  if (cardState.pinned) return;
  hideCard();
}

function hideCard() {
  clearTimeout(cardState.timer);
  cardState.token++;
  cardState.pinned = false;
  cardState.restoreScroll = null;
  const card = cardElement();
  if (!card) return;
  card.hidden = true;
  card.classList.remove("pinned");
}

function showCard(item, x, y, pinned) {
  const card = cardElement();
  if (!card) return;
  clearTimeout(cardState.timer);
  cardState.pinned = Boolean(pinned);
  cardState.key = item.key;
  cardState.x = Number(x) || window.innerWidth / 2;
  cardState.y = Number(y) || window.innerHeight / 3;
  const token = ++cardState.token;

  card.hidden = false;
  card.classList.toggle("pinned", cardState.pinned);
  card.textContent = "";

  const head = el("div", "card-head");
  const name = el("div", "card-name", itemName(item));
  name.style.color = qualityColor(item.quality);
  head.append(name);
  if (cardState.pinned) {
    const close = el("button", "card-close", "×");
    close.type = "button";
    close.setAttribute("aria-label", "Close item details");
    close.addEventListener("click", hideCard);
    head.append(close);
  }
  card.append(head, el("div", "card-loading", "Loading item details…"));
  placeCard(cardState.x, cardState.y);

  getDetail(item).then((detail) => {
    if (token !== cardState.token) return;
    paintCard(detail, item);
    placeCard(cardState.x, cardState.y);
  }).catch((error) => {
    if (token !== cardState.token) return;
    card.textContent = "";
    card.append(el("p", "error", error.message || "Item details unavailable."));
    placeCard(cardState.x, cardState.y);
  });
}

function placeCard(x, y) {
  const card = cardElement();
  if (!card || card.hidden) return;
  const pad = 12;
  const gap = 16;
  const width = card.offsetWidth;
  const height = card.offsetHeight;
  let left = x + gap;
  let top = y + gap;
  if (left + width > window.innerWidth - pad) left = x - width - gap;
  if (top + height > window.innerHeight - pad) top = y - height - gap;
  left = Math.min(Math.max(pad, left), Math.max(pad, window.innerWidth - width - pad));
  top = Math.min(Math.max(pad, top), Math.max(pad, window.innerHeight - height - pad));
  card.style.left = `${left}px`;
  card.style.top = `${top}px`;
}

function paintCard(detail, fallbackItem) {
  const card = cardElement();
  if (!card) return;
  const info = detail || {};
  const name = info.name || itemName(fallbackItem);
  const quality = typeof info.quality === "number" ? info.quality : Number(fallbackItem.quality) || 0;
  const itemLevelValue = Number(info.item_level) || levelFromKey(fallbackItem.key);

  card.textContent = "";

  const head = el("div", "card-head");
  const nameEl = el("div", "card-name", name);
  nameEl.style.color = qualityColor(quality);
  head.append(nameEl);
  if (cardState.pinned) {
    const close = el("button", "card-close", "×");
    close.type = "button";
    close.setAttribute("aria-label", "Close item details");
    close.addEventListener("click", hideCard);
    head.append(close);
  }
  card.append(head);

  const subParts = [];
  if (QUALITY_NAMES[quality]) subParts.push(QUALITY_NAMES[quality]);
  const type = typeLine({
    class_id: info.class_id,
    subclass_id: info.subclass_id,
    inventory_type: info.inventory_type,
    equip_loc: info.equip_loc,
  });
  if (type) subParts.push(type);
  if (subParts.length) card.append(el("div", "card-sub", subParts.join(" · ")));

  if (itemLevelValue > 0) {
    card.append(el("div", "card-line gold", `Item Level ${itemLevelValue}`));
  }

  const bindType = info.bind_type;
  if (bindType !== undefined && bindType !== null && Number(bindType) > 0) {
    card.append(el("div", "card-line muted", BIND_TYPES[bindType] || `Bind type ${bindType}`));
  }
  if (info.unique_item) card.append(el("div", "card-line muted", "Unique"));

  const armor = Number(info.armor) || 0;
  if (armor > 0) {
    card.append(el("div", "card-line", `${armor.toLocaleString()} Armor`));
  }

  const damageMin = Number(info.damage_min) || 0;
  const damageMax = Number(info.damage_max) || 0;
  if (damageMin || damageMax) {
    card.append(el("div", "card-line", `${damageMin.toLocaleString()}–${damageMax.toLocaleString()} Damage`));
    const delay = Number(info.delay) || 0;
    if (delay > 0) {
      const speed = delay / 1000;
      const dps = ((damageMin + damageMax) / 2) / speed;
      card.append(el("div", "card-line muted",
        `Speed ${speed.toFixed(2)} · (${dps.toFixed(1)} damage per second)`));
    }
  }

  const stats = normalizeStats(info.stats);
  if (stats.length) {
    const block = el("div", "card-stats");
    for (const stat of stats) {
      const row = el("div", "card-stat");
      row.append(el("span", "stat-label", stat.label), el("span", "stat-value", stat.value));
      block.append(row);
    }
    card.append(block);
  }

  const entries = detailEntries(info.details);
  if (entries.length) {
    const block = el("div", "card-details");
    for (const entry of entries) {
      const line = el("div", "card-detail-line");
      const color = safeColor(entry.color);
      if (color) line.style.color = color;
      if (entry.label) {
        line.append(el("span", "detail-left", entry.label), el("span", "detail-right", entry.value));
      } else {
        line.textContent = entry.text || "";
      }
      block.append(line);
    }
    card.append(block);
  }

  const description = detailDescription(info.details);
  if (description) card.append(el("div", "card-description", description));

  const fallbackJSON = unknownDetailJSON(info.details);
  if (fallbackJSON) {
    const pre = el("pre", "card-pre");
    pre.textContent = fallbackJSON;
    card.append(pre);
  }

  const requiredLevel = Number(info.required_level) || 0;
  if (requiredLevel > 0) {
    card.append(el("div", "card-line muted", `Requires level ${requiredLevel}`));
  }
  const vendorPrice = Number(info.vendor_price) || 0;
  if (vendorPrice > 0) {
    card.append(el("div", "card-line muted", `Sells for ${money(vendorPrice)}`));
  }
  const maxStack = Number(info.max_stack) || 0;
  if (maxStack > 1) {
    card.append(el("div", "card-line muted", `Max stack: ${maxStack.toLocaleString()}`));
  }

  const price = Number(info.price ?? fallbackItem.price) || 0;
  const quantity = Number(info.quantity ?? fallbackItem.quantity) || 0;
  const listings = Number(info.listings ?? fallbackItem.listings) || 0;
  const scanned = Number(info.scanned ?? fallbackItem.scanned) || 0;
  card.append(variantSection({
    item: fallbackItem,
    activeLevel: effectiveLevel({ key: fallbackItem.key, item_level: itemLevelValue }),
    interactive: cardState.pinned,
    surface: "card",
    onGrow: () => {
      if (cardState.el && !cardState.el.hidden) placeCard(cardState.x, cardState.y);
    },
  }));
  const grid = el("div", "card-price");
  const cell = (label, value, main) => {
    const wrap = el("div", main ? "card-price-main" : "");
    wrap.append(el("span", "", label), el("strong", "", value));
    return wrap;
  };
  grid.append(
    cell("Price", money(price), true),
    cell("Quantity", quantity.toLocaleString()),
    cell("Listings", listings.toLocaleString()),
    cell("Scanned", ageText(scanned)),
  );
  card.append(grid);

  if (cardState.restoreScroll != null) {
    card.scrollTop = cardState.restoreScroll;
    cardState.restoreScroll = null;
  }
}

function selectItem(item, x, y) {
  state.selected = item;
  renderResults();
  renderDetail(item);
  const anchorX = typeof x === "number" ? x : Math.max(24, Math.min(window.innerWidth - 40, window.innerWidth / 2));
  const anchorY = typeof y === "number" ? y : 120;
  showCard(item, anchorX, anchorY, true);
}

function renderDetail(item) {
  const detail = $("#detail");
  detail.innerHTML = "";

  const heading = document.createElement("h2");
  heading.textContent = itemName(item);
  heading.style.color = QUALITY[item.quality] || "#ffffff";

  const key = document.createElement("div");
  key.className = "key";
  key.textContent = item.key;

  const current = document.createElement("div");
  current.className = "current";
  current.innerHTML = `
    <div><strong></strong><span>unit price</span></div>
    <div><strong></strong><span>quantity</span></div>
    <div><strong></strong><span>listings</span></div>
    <div><strong></strong><span>scanned</span></div>`;
  const strongs = current.querySelectorAll("strong");
  strongs[0].textContent = money(item.price);
  strongs[1].textContent = (item.quantity || 0).toLocaleString();
  strongs[2].textContent = (item.listings || 0).toLocaleString();
  strongs[3].textContent = ageText(item.scanned);

  const watch = document.createElement("button");
  watch.type = "button";
  watch.textContent = state.watch.has(item.key) ? "★ Watching" : "☆ Watch";
  watch.addEventListener("click", () => toggleWatch(item.key));

  detail.append(heading, key, current);
  detail.append(variantSection({
    item,
    activeLevel: effectiveLevel(item),
    interactive: true,
    surface: "detail",
  }));
  detail.append(watch);

  const chartTitle = document.createElement("h3");
  const rangeLabel = state.range === "12h" ? "12h · 1-minute buckets"
    : state.range === "7d" ? "7d · 15-minute buckets"
    : "all · hourly buckets";
  chartTitle.textContent = "Price history · " + rangeLabel;
  const ranges = document.createElement("div");
  ranges.className = "range-tabs";
  for (const [value, label] of [["12h", "12h"], ["7d", "7d"], ["all", "All"]]) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = label;
    if (state.range === value) button.classList.add("active");
    button.addEventListener("click", () => {
      state.range = value;
      localStorage.setItem("tradepost-range", value);
      if (state.selected) renderDetail(state.selected);
    });
    ranges.append(button);
  }
  const chart = document.createElement("div");
  chart.className = "chart-wrap";
  chart.textContent = "Loading history…";
  const marketsTitle = document.createElement("h3");
  marketsTitle.textContent = "Markets";
  const markets = document.createElement("div");
  markets.textContent = "Loading markets…";
  detail.append(chartTitle, ranges, chart, marketsTitle, markets);

  const requested = item.key;
  Promise.all([
    api("/v1/item/history", { market: state.market, key: item.key, range: state.range }),
    api("/v1/item/markets", { key: item.key }),
  ]).then(([history, comparison]) => {
    if (!state.selected || state.selected.key !== requested) return;
    const points = history.points || [];
    chart.innerHTML = renderChart(points, history);
    wireChart(chart, points);
    const marketView = renderMarkets(comparison.markets || []);
    markets.textContent = "";
    if (typeof marketView === "string") {
      markets.innerHTML = marketView;
    } else {
      markets.append(marketView);
    }
  }).catch((error) => {
    chart.innerHTML = `<p class="error">${escapeHTML(error.message)}</p>`;
    markets.innerHTML = "";
  });
}

function escapeHTML(value) {
  return String(value).replace(/[&<>"']/g, (char) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[char]));
}

function renderChart(points, meta) {
  if (!points.length) {
    return `<p class="empty">No history in this range yet. New scans will build it.</p>`;
  }
  const width = 620;
  const height = 180;
  const pad = 16;
  const values = points.flatMap((point) => [point.min, point.max]);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = Math.max(1, max - min);
  const x = (index) => points.length === 1
    ? width / 2
    : pad + index * (width - 2 * pad) / (points.length - 1);
  const y = (value) => height - pad - (value - min) / span * (height - 2 * pad);

  const maxLine = points.map((point, index) => `${x(index)},${y(point.max)}`).join(" ");
  const minLine = points.map((point, index) => `${x(index)},${y(point.min)}`).join(" ");
  const dots = points.length <= 300 ? points.map((point, index) => `
      <circle cx="${x(index)}" cy="${y(point.max)}" r="2.5" fill="#f0c040" />
      <circle cx="${x(index)}" cy="${y(point.min)}" r="2.5" fill="#4ade80" />`).join("") : "";
  const step = points.length > 1 ? (width - 2 * pad) / (points.length - 1) : (width - 2 * pad);
  const hits = points.map((_, index) => {
    const left = points.length === 1 ? pad : Math.max(pad, x(index) - step / 2);
    const right = points.length === 1 ? width - pad : Math.min(width - pad, x(index) + step / 2);
    return `<rect class="chart-hit" data-index="${index}" data-x="${x(index)}"
      data-max-y="${y(points[index].max)}" data-min-y="${y(points[index].min)}"
      x="${left}" y="${pad}" width="${Math.max(1, right - left)}" height="${height - 2 * pad}" />`;
  }).join("");
  const first = new Date(points[0].t * 1000);
  const last = new Date(points[points.length - 1].t * 1000);
  const range = meta && meta.range;
  const label = (date) => range === "12h"
    ? date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : date.toLocaleDateString([], { month: "short", day: "numeric" });

  return `
    <div class="legend">
      <span><i style="background:#f0c040"></i>max ${money(max)}</span>
      <span><i style="background:#4ade80"></i>min ${money(min)}</span>
      <span class="chart-hint">hover for values</span>
    </div>
    <svg class="chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="Price history">
      <line x1="${pad}" y1="${height - pad}" x2="${width - pad}" y2="${height - pad}" stroke="#2a3441" />
      <polyline fill="none" stroke="#f0c040" stroke-width="2" points="${maxLine}" />
      <polyline fill="none" stroke="#4ade80" stroke-width="2" points="${minLine}" />
      ${dots}
      <line class="chart-guide" x1="0" y1="${pad}" x2="0" y2="${height - pad}" />
      <circle class="chart-cursor-max" cx="0" cy="0" r="4" />
      <circle class="chart-cursor-min" cx="0" cy="0" r="4" />
      ${hits}
      <text x="${pad}" y="${height - 2}" fill="#8b98a8" font-size="11">${label(first)}</text>
      <text x="${width - pad}" y="${height - 2}" fill="#8b98a8" font-size="11" text-anchor="end">${label(last)}</text>
    </svg>
    <div class="chart-tip" hidden></div>`;
}

function wireChart(container, points) {
  const svg = container.querySelector("svg.chart");
  const tip = container.querySelector(".chart-tip");
  const guide = container.querySelector(".chart-guide");
  const maxDot = container.querySelector(".chart-cursor-max");
  const minDot = container.querySelector(".chart-cursor-min");
  if (!svg || !tip || !guide) return;

  const hide = () => {
    tip.hidden = true;
    guide.style.opacity = "0";
    maxDot.style.opacity = "0";
    minDot.style.opacity = "0";
  };

  svg.querySelectorAll(".chart-hit").forEach((hit) => {
    hit.addEventListener("mouseenter", (event) => {
      const index = Number(hit.getAttribute("data-index"));
      const point = points[index];
      if (!point) return;
      const time = new Date(point.t * 1000).toLocaleString([], {
        weekday: "short", month: "short", day: "numeric",
        hour: "2-digit", minute: "2-digit",
      });
      const samples = point.samples || 0;
      tip.innerHTML = `
        <div class="tip-time">${escapeHTML(time)}</div>
        <div class="tip-row"><span>Max</span><strong>${money(point.max)}</strong></div>
        <div class="tip-row"><span>Min</span><strong>${money(point.min)}</strong></div>
        <div class="tip-row"><span>Qty</span><strong>${(point.quantity || 0).toLocaleString()}</strong></div>
        <div class="tip-samples">${samples} sample${samples === 1 ? "" : "s"}</div>`;
      tip.hidden = false;
      const x = hit.getAttribute("data-x");
      guide.setAttribute("x1", x);
      guide.setAttribute("x2", x);
      guide.style.opacity = "1";
      maxDot.setAttribute("cx", x);
      maxDot.setAttribute("cy", hit.getAttribute("data-max-y"));
      maxDot.style.opacity = "1";
      minDot.setAttribute("cx", x);
      minDot.setAttribute("cy", hit.getAttribute("data-min-y"));
      minDot.style.opacity = "1";
      positionChartTip(container, tip, event);
    });
    hit.addEventListener("mousemove", (event) => positionChartTip(container, tip, event));
  });
  svg.addEventListener("mouseleave", hide);
}

function positionChartTip(container, tip, event) {
  if (tip.hidden) return;
  const rect = container.getBoundingClientRect();
  const width = tip.offsetWidth;
  const height = tip.offsetHeight;
  let left = event.clientX - rect.left + 14;
  if (left + width > rect.width - 8) {
    left = event.clientX - rect.left - width - 14;
  }
  left = Math.max(8, Math.min(left, rect.width - width - 8));
  let top = event.clientY - rect.top - height - 14;
  if (top < 8) {
    top = event.clientY - rect.top + 18;
  }
  top = Math.max(8, Math.min(top, rect.height - height - 8));
  tip.style.left = `${left}px`;
  tip.style.top = `${top}px`;
}

function renderMarkets(markets) {
  if (!markets.length) {
    return `<p class="empty">No market prices.</p>`;
  }
  const table = document.createElement("table");
  table.className = "market-table";
  const head = document.createElement("thead");
  head.innerHTML = "<tr><th>Market</th><th class='num'>Price</th><th class='num'>Qty</th><th class='num'>Age</th></tr>";
  table.append(head);
  const body = document.createElement("tbody");
  markets.forEach((market, index) => {
    const row = document.createElement("tr");
    const name = document.createElement("td");
    name.textContent = market.market;
    const price = document.createElement("td");
    price.className = "price num" + (index === 0 ? " best" : "");
    price.textContent = money(market.price);
    const qty = document.createElement("td");
    qty.className = "qty num";
    qty.textContent = (market.quantity || 0).toLocaleString();
    const scanned = document.createElement("td");
    scanned.className = "age num";
    scanned.textContent = ageText(market.scanned);
    row.append(name, price, qty, scanned);
    body.append(row);
  });
  table.append(body);
  return table;
}

function setSyncState(text, ok) {
  const element = $("#sync-state");
  element.textContent = text || "";
  element.classList.toggle("on", Boolean(ok));
}

function toggleWatch(key) {
  if (state.watch.has(key)) {
    state.watch.delete(key);
  } else {
    state.watch.add(key);
  }
  renderResults();
  if (state.selected) {
    renderDetail(state.selected);
  }
  persistWatch();
}

let watchTimer;
function persistWatch() {
  if (!state.session) {
    saveWatch();
    setSyncState("Local only — /tradepost login to sync");
    return;
  }
  setSyncState("Saving…");
  clearTimeout(watchTimer);
  watchTimer = setTimeout(async () => {
    try {
      const list = await api("/v1/watch", {}, {
        method: "PUT",
        body: JSON.stringify({ items: [...state.watch] }),
      });
      state.watch = new Set(list.items || []);
      setSyncState("Watch synced", true);
      renderResults();
      if (state.selected) {
        renderDetail(state.selected);
      }
    } catch (error) {
      setSyncState(error.message);
    }
  }, 300);
}

async function loadWatch() {
  if (state.session) {
    try {
      const list = await api("/v1/watch");
      state.watch = new Set(list.items || []);
      setSyncState("Watch synced", true);
    } catch (error) {
      setSyncState(error.message);
    }
  } else {
    state.watch = new Set(JSON.parse(localStorage.getItem("tradepost-watch") || "[]"));
    setSyncState("Local only — /tradepost login to sync");
  }
  renderResults();
  if (state.selected) {
    renderDetail(state.selected);
  }
}

function updateAuthUI() {
  const area = $("#auth-area");
  area.innerHTML = "";
  if (state.session) {
    const who = document.createElement("span");
    who.className = "who";
    who.textContent = state.session.character;
    const logout = document.createElement("button");
    logout.type = "button";
    logout.className = "ghost";
    logout.textContent = "Log out";
    logout.addEventListener("click", logoutWeb);
    area.append(who, logout);
  } else {
    const open = document.createElement("button");
    open.type = "button";
    open.className = "ghost";
    open.textContent = "Sync watch";
    open.addEventListener("click", openAuthDialog);
    area.append(open);
  }
}

function openAuthDialog() {
  $("#auth-error").textContent = "";
  $("#auth-code").value = "";
  $("#auth-dialog").showModal();
  $("#auth-code").focus();
}

async function mergeWatchAfterLogin() {
  const local = new Set(JSON.parse(localStorage.getItem("tradepost-watch") || "[]"));
  const list = await api("/v1/watch");
  const merged = new Set([...local, ...(list.items || [])]);
  state.watch = merged;
  if (merged.size !== (list.items || []).length) {
    const saved = await api("/v1/watch", {}, {
      method: "PUT",
      body: JSON.stringify({ items: [...merged] }),
    });
    state.watch = new Set(saved.items || []);
  }
  setSyncState("Watch synced", true);
  renderResults();
  if (state.selected) {
    renderDetail(state.selected);
  }
}

async function logoutWeb() {
  try {
    await api("/v1/auth/logout", {}, { method: "POST" });
  } catch (_) { /* session is local anyway */ }
  state.session = null;
  updateAuthUI();
  await loadWatch();
}

async function initAuth() {
  try {
    state.session = await api("/v1/auth/me");
  } catch (_) {
    state.session = null;
  }
  updateAuthUI();
  await loadWatch();
}

function filterNumber(selector, scale) {
  const raw = $(selector).value.trim();
  if (!raw) return "";
  const value = Number(raw);
  if (!Number.isFinite(value) || value < 0) return "";
  return String(Math.round(value * (scale || 1)));
}

function selectedSubclass() {
  if ($("#subclass-select-field").hidden) {
    const raw = $("#subclass-id").value.trim();
    if (!raw) return "";
    const value = Number(raw);
    return Number.isFinite(value) && value >= 0 ? String(Math.floor(value)) : "";
  }
  return $("#subclass-select").value;
}

function setSortDir(dir) {
  const next = dir === "desc" ? "desc" : "asc";
  for (const button of document.querySelectorAll("#sort-dir button[data-dir]")) {
    button.setAttribute("aria-pressed", String(button.dataset.dir === next));
  }
}

function sortDir() {
  const active = document.querySelector('#sort-dir button[data-dir][aria-pressed="true"]');
  return active ? active.dataset.dir : "asc";
}

function currentParams() {
  const params = {
    market: state.market,
    q: $("#query").value.trim(),
    limit: PAGE_SIZE,
    page: state.page,
    sort: $("#sort").value,
    dir: sortDir(),
  };
  const qualities = [...document.querySelectorAll("#quality-list input:checked")]
    .map((input) => input.value);
  if (qualities.length) params.quality = qualities.join(",");
  if ($("#class-id").value !== "") params.class_id = $("#class-id").value;
  const subclass = selectedSubclass();
  if (subclass !== "") params.subclass_id = subclass;
  const numeric = {
    min_ilvl: filterNumber("#min-ilvl"),
    max_ilvl: filterNumber("#max-ilvl"),
    min_req_level: filterNumber("#min-req-level"),
    max_req_level: filterNumber("#max-req-level"),
    min_price: filterNumber("#min-price-gold", 10000),
    max_price: filterNumber("#max-price-gold", 10000),
  };
  for (const [key, value] of Object.entries(numeric)) {
    if (value !== "") params[key] = value;
  }
  if ($("#in-stock").checked) params.in_stock = "1";
  const stats = statsParam();
  if (stats !== "") params.stats = stats;
  if ($("#has-stats").checked) params.has_stats = "1";
  return params;
}

const MAX_STAT_ROWS = 6;

function statRows() {
  return [...document.querySelectorAll("#stats-rows .stat-row")];
}

function statBound(row, selector) {
  const raw = row.querySelector(selector).value.trim();
  if (!raw) return "";
  const value = Number(raw);
  if (!Number.isFinite(value) || value < 0) return "";
  return String(Math.round(value));
}

// Every listed stat must match (AND). Omit empty bounds:
// KEY, KEY:MIN, KEY::MAX, KEY:MIN:MAX.
function statsParam() {
  const parts = [];
  for (const row of statRows()) {
    const key = row.querySelector(".stat-key").value;
    if (!key) continue;
    const min = statBound(row, ".stat-min");
    const max = statBound(row, ".stat-max");
    if (min === "" && max === "") parts.push(key);
    else if (min === "") parts.push(`${key}::${max}`);
    else if (max === "") parts.push(`${key}:${min}`);
    else parts.push(`${key}:${min}:${max}`);
  }
  return parts.join(",");
}

function syncStatRow(row) {
  const select = row.querySelector(".stat-key");
  const min = row.querySelector(".stat-min");
  const max = row.querySelector(".stat-max");
  const active = select.value !== "";
  if (!active) {
    min.value = "";
    max.value = "";
  }
  select.classList.toggle("is-active", active);
  row.classList.toggle("is-active", active);
  min.disabled = !active;
  max.disabled = !active;
}

function updateAddStatButton() {
  const button = $("#add-stat");
  const full = statRows().length >= MAX_STAT_ROWS;
  button.disabled = full;
  button.title = full ? `Up to ${MAX_STAT_ROWS} stats` : "Add another stat";
}

function relabelStatRows() {
  const rows = statRows();
  rows.forEach((row, index) => {
    const number = index + 1;
    row.querySelector(".stat-row-key .stat-row-name").textContent = `Stat ${number}`;
    row.querySelector(".stat-key").setAttribute("aria-label", `Stat ${number}`);
    row.querySelector(".stat-min").setAttribute("aria-label", `Stat ${number} minimum value`);
    row.querySelector(".stat-max").setAttribute("aria-label", `Stat ${number} maximum value`);
    row.querySelector(".stat-remove").setAttribute("aria-label",
      rows.length === 1 ? `Clear stat ${number}` : `Remove stat ${number}`);
    row.querySelector(".stat-remove").title =
      rows.length === 1 ? "Clear this stat" : "Remove this stat";
  });
}

function updateStatsHint() {
  const active = statRows()
    .filter((row) => row.querySelector(".stat-key").value !== "").length;
  $("#stats-block").classList.toggle("is-active", active > 0 || $("#has-stats").checked);
  $("#stats-hint").textContent = active
    ? "All selected stats must match. Leave min or max empty for an open bound."
    : "Add a stat to filter by its value. All selected stats must match.";
}

function addStatRow() {
  const template = $("#stat-row-template");
  const rows = $("#stats-rows");
  if (!template || rows.children.length >= MAX_STAT_ROWS) return null;
  const row = template.content.firstElementChild.cloneNode(true);
  rows.append(row);
  relabelStatRows();
  updateAddStatButton();
  updateStatsHint();
  return row;
}

function removeStatRow(row) {
  const rows = statRows();
  if (!rows.length) return;
  if (rows.length === 1) {
    row.querySelector(".stat-key").value = "";
    syncStatRow(row);
  } else {
    row.remove();
    relabelStatRows();
  }
  updateAddStatButton();
  updateStatsHint();
  updateFilterCount();
}

function resetStatRows() {
  $("#stats-rows").textContent = "";
  addStatRow();
}

function updateFilterCount() {
  let count = 0;
  if (document.querySelector("#quality-list input:checked")) count++;
  if ($("#class-id").value !== "") count++;
  if (selectedSubclass() !== "") count++;
  if ($("#min-ilvl").value.trim() || $("#max-ilvl").value.trim()) count++;
  if ($("#min-req-level").value.trim() || $("#max-req-level").value.trim()) count++;
  if ($("#min-price-gold").value.trim() || $("#max-price-gold").value.trim()) count++;
  if ($("#in-stock").checked) count++;
  count += statRows().filter((row) => row.querySelector(".stat-key").value !== "").length;
  if ($("#has-stats").checked) count++;
  const badge = $("#filter-count");
  badge.textContent = count ? `${count} active` : "";
  badge.hidden = count === 0;
}

function buildQualityFilters() {
  const wrap = $("#quality-list");
  wrap.textContent = "";
  QUALITY_NAMES.forEach((name, index) => {
    const label = el("label", "quality-chip");
    label.style.color = QUALITY[index] || "#ffffff";
    const input = document.createElement("input");
    input.type = "checkbox";
    input.value = String(index);
    input.addEventListener("change", applyFilters);
    const dot = el("i", "quality-dot");
    dot.style.background = QUALITY[index] || "#ffffff";
    label.append(input, dot, el("span", "", name));
    wrap.append(label);
  });
}

function buildClassFilter() {
  const select = $("#class-id");
  select.textContent = "";
  const any = document.createElement("option");
  any.value = "";
  any.textContent = "Any class";
  select.append(any);
  for (const [id, name] of Object.entries(CLASS_NAMES)) {
    const option = document.createElement("option");
    option.value = id;
    option.textContent = name;
    select.append(option);
  }
}

function syncSubclassFilter() {
  const classID = $("#class-id").value;
  const subclasses = SUBCLASSES[classID];
  const selectField = $("#subclass-select-field");
  const inputField = $("#subclass-input-field");
  const select = $("#subclass-select");
  if (subclasses) {
    select.textContent = "";
    const any = document.createElement("option");
    any.value = "";
    any.textContent = "Any subclass";
    select.append(any);
    for (const [id, name] of Object.entries(subclasses)) {
      const option = document.createElement("option");
      option.value = id;
      option.textContent = name;
      select.append(option);
    }
    $("#subclass-id").value = "";
    selectField.hidden = false;
    inputField.hidden = true;
  } else {
    select.textContent = "";
    select.value = "";
    $("#subclass-id").value = "";
    selectField.hidden = true;
    inputField.hidden = false;
  }
  updateFilterCount();
}

function resetFilters() {
  document.querySelectorAll("#quality-list input:checked").forEach((input) => {
    input.checked = false;
  });
  $("#query").value = "";
  $("#class-id").value = "";
  $("#subclass-id").value = "";
  $("#subclass-select").value = "";
  for (const selector of [
    "#min-ilvl", "#max-ilvl", "#min-req-level", "#max-req-level",
    "#min-price-gold", "#max-price-gold",
  ]) {
    $(selector).value = "";
  }
  $("#in-stock").checked = false;
  $("#has-stats").checked = false;
  resetStatRows();
  $("#sort").value = "price";
  setSortDir(DEFAULT_SORT_DIR.price);
  syncSubclassFilter();
  clearTimeout(searchTimer);
  clearTimeout(statTimer);
  applyFilters();
}

let searchSeq = 0;

async function search() {
  if (!state.market) return;
  hideCard();
  const sequence = ++searchSeq;
  $("#status").textContent = "Searching…";
  try {
    const data = await api("/v1/items", currentParams());
    if (sequence !== searchSeq) return;
    state.page = data.page || 1;
    state.total = Number(data.total) || 0;
    state.results = data.items || [];
    renderResults();
    $("#results").scrollTop = 0;
  } catch (error) {
    if (sequence !== searchSeq) return;
    $("#status").textContent = error.message;
  }
}

function applyFilters() {
  state.page = 1;
  updateFilterCount();
  search();
}

let searchTimer;
$("#query").addEventListener("input", () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    state.page = 1;
    search();
  }, 350);
});

$("#search-form").addEventListener("submit", (event) => {
  event.preventDefault();
  clearTimeout(searchTimer);
  state.page = 1;
  search();
});

for (const selector of [
  "#min-ilvl", "#max-ilvl", "#min-req-level", "#max-req-level",
  "#min-price-gold", "#max-price-gold", "#subclass-id",
]) {
  const input = $(selector);
  let timer;
  input.addEventListener("input", () => {
    updateFilterCount();
    clearTimeout(timer);
    timer = setTimeout(applyFilters, 300);
  });
  input.addEventListener("keydown", (event) => {
    if (event.key !== "Enter") return;
    event.preventDefault();
    clearTimeout(timer);
    applyFilters();
  });
}

let statTimer;
$("#stats-rows").addEventListener("change", (event) => {
  const select = event.target.closest(".stat-key");
  if (!select) return;
  syncStatRow(select.closest(".stat-row"));
  updateStatsHint();
  applyFilters();
});

$("#stats-rows").addEventListener("input", (event) => {
  if (!event.target.closest(".stat-min, .stat-max")) return;
  updateFilterCount();
  clearTimeout(statTimer);
  statTimer = setTimeout(applyFilters, 300);
});

$("#stats-rows").addEventListener("keydown", (event) => {
  if (event.key !== "Enter") return;
  if (!event.target.closest(".stat-min, .stat-max")) return;
  event.preventDefault();
  clearTimeout(statTimer);
  applyFilters();
});

$("#stats-rows").addEventListener("click", (event) => {
  const button = event.target.closest(".stat-remove");
  if (!button) return;
  removeStatRow(button.closest(".stat-row"));
  applyFilters();
});

$("#add-stat").addEventListener("click", () => {
  const row = addStatRow();
  if (!row) return;
  row.querySelector(".stat-key").focus();
});

$("#class-id").addEventListener("change", () => {
  syncSubclassFilter();
  applyFilters();
});
$("#subclass-select").addEventListener("change", applyFilters);
$("#sort").addEventListener("change", () => {
  setSortDir(DEFAULT_SORT_DIR[$("#sort").value] || "asc");
  applyFilters();
});
$("#sort-dir").addEventListener("click", (event) => {
  const button = event.target.closest("button[data-dir]");
  if (!button || button.getAttribute("aria-pressed") === "true") return;
  setSortDir(button.dataset.dir);
  applyFilters();
});
$("#in-stock").addEventListener("change", applyFilters);
$("#has-stats").addEventListener("change", () => {
  updateStatsHint();
  applyFilters();
});
$("#filters-reset").addEventListener("click", resetFilters);

$("#prev-page").addEventListener("click", () => {
  if (state.page <= 1) return;
  state.page -= 1;
  search();
});

$("#next-page").addEventListener("click", () => {
  if (state.page * PAGE_SIZE >= state.total) return;
  state.page += 1;
  search();
});

$("#watch-only").addEventListener("change", renderResults);

$("#auth-cancel").addEventListener("click", () => $("#auth-dialog").close());
$("#auth-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const code = $("#auth-code").value.toUpperCase().replace(/[^A-Z0-9]/g, "");
  if (code.length < 6) {
    $("#auth-error").textContent = "Enter the code from /tradepost login.";
    return;
  }
  try {
    const result = await api("/v1/auth/redeem", {}, {
      method: "POST",
      body: JSON.stringify({ code }),
    });
    state.session = { character: result.character, expires_at: result.expires_at };
    $("#auth-dialog").close();
    updateAuthUI();
    await mergeWatchAfterLogin();
  } catch (error) {
    $("#auth-error").textContent = error.message;
  }
});

$("#market").addEventListener("change", (event) => {
  state.market = event.target.value;
  localStorage.setItem("tradepost-market", state.market);
  state.selected = null;
  state.page = 1;
  hideCard();
  loadMeta().then(search).catch((error) => {
    $("#status").textContent = error.message;
  });
});

document.addEventListener("click", (event) => {
  if (!cardState.pinned) return;
  const card = cardElement();
  if (!card || card.hidden) return;
  if (card.contains(event.target)) return;
  if (event.target.closest && event.target.closest("tr.row")) return;
  hideCard();
});

document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape") return;
  if (cardState.el && !cardState.el.hidden) hideCard();
});

window.addEventListener("resize", () => {
  if (!cardState.el || cardState.el.hidden) return;
  if (cardState.pinned) placeCard(cardState.x, cardState.y);
  else hideCard();
});

$("#results").addEventListener("scroll", () => {
  if (cardState.el && !cardState.el.hidden) hideCard();
});

$("#filters").open = window.innerWidth > 900;
setSortDir(DEFAULT_SORT_DIR[$("#sort").value] || "asc");
buildQualityFilters();
buildClassFilter();
syncSubclassFilter();
addStatRow();
updateFilterCount();
loadLive();
setInterval(loadLive, 60_000);

// Deep links from the stats page: /?q=<item id or name>&market=<market>.
const bootParams = new URLSearchParams(window.location.search);
const bootQuery = (bootParams.get("q") || "").trim();
if (bootQuery) $("#query").value = bootQuery;
const bootMarket = (bootParams.get("market") || "").trim();
if (bootMarket) state.market = bootMarket;

loadMeta()
  .then(initAuth)
  .then(search)
  .catch((error) => {
    $("#status").textContent = error.message;
  });
