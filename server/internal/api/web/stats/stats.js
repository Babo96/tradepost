"use strict";

/* TradePost market stats. Reads /v1/stats and paints a stock-market style
   summary of the auction house. All money values arrive as copper integers. */

const $ = (selector, root = document) => root.querySelector(selector);

const QUALITY = [
  "#9d9d9d", "#ffffff", "#1eff00", "#0070dd",
  "#a335ee", "#ff8000", "#e6cc80", "#e6cc80",
];
const QUALITY_NAMES = [
  "Poor", "Common", "Uncommon", "Rare",
  "Epic", "Legendary", "Artifact", "Heirloom",
];
const CLASS_NAMES = {
  0: "Consumable", 1: "Container", 2: "Weapon", 3: "Gem",
  4: "Armor", 5: "Reagent", 6: "Projectile", 7: "Trade Goods",
  8: "Generic", 9: "Recipe", 11: "Quiver", 12: "Quest",
  13: "Key", 15: "Miscellaneous", 16: "Glyph", 17: "Battle Pet",
  18: "WoW Token",
};
const MONTHS = [
  "Jan", "Feb", "Mar", "Apr", "May", "Jun",
  "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
];
const REFRESH_MS = 60_000;

const state = {
  market: localStorage.getItem("tradepost-market") || "",
  data: null,
  meta: [],
  loading: false,
  lastError: "",
  lastLoaded: 0,
  chartSeen: false,
  chartPoints: [],
  timer: 0,
  requestId: 0,
};

/* ---------- Generic helpers ---------- */

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined && text !== null) node.textContent = text;
  return node;
}

function svgEl(tag, attrs) {
  const node = document.createElementNS("http://www.w3.org/2000/svg", tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    node.setAttribute(key, String(value));
  }
  return node;
}

function toNum(value) {
  if (value === null || value === undefined || value === "") return null;
  const number = typeof value === "number" ? value : Number(value);
  return Number.isFinite(number) ? number : null;
}

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function trimFixed(number, digits) {
  const fixed = Math.abs(number).toFixed(digits);
  const trimmed = fixed.includes(".") ? fixed.replace(/\.?0+$/, "") : fixed;
  return (number < 0 ? "−" : "") + trimmed;
}

/* Same g/s/c style used by the prices page. */
function money(copper) {
  const value = Math.max(0, Math.floor(toNum(copper) ?? 0));
  const gold = Math.floor(value / 10000);
  const silver = Math.floor((value % 10000) / 100);
  const rest = value % 100;
  const parts = [];
  if (gold) parts.push(`${gold.toLocaleString("en-GB")}g`);
  if (silver) parts.push(`${silver}s`);
  if (rest || parts.length === 0) parts.push(`${rest}c`);
  return parts.join(" ");
}

/* Compact gold for big figures: 387.9k g, 3.88M g. */
function moneyShort(copper) {
  const value = Math.max(0, toNum(copper) ?? 0);
  if (value === 0) return "0 g";
  const gold = value / 10000;
  if (gold >= 1e9) return `${trimFixed(gold / 1e9, 2)}B g`;
  if (gold >= 1e6) return `${trimFixed(gold / 1e6, 2)}M g`;
  if (gold >= 1e4) return `${trimFixed(gold / 1e3, 1)}k g`;
  if (gold >= 1000) return `${Math.round(gold).toLocaleString("en-GB")} g`;
  return money(value);
}

function numShort(value) {
  const number = toNum(value) ?? 0;
  const abs = Math.abs(number);
  if (abs >= 1e9) return `${trimFixed(number / 1e9, 2)}B`;
  if (abs >= 1e6) return `${trimFixed(number / 1e6, 2)}M`;
  if (abs >= 1e5) return `${trimFixed(number / 1e3, 1)}k`;
  return number.toLocaleString("en-GB");
}

function signed(value, formatter) {
  const number = toNum(value);
  if (number === null) return "—";
  if (number === 0) return formatter(0);
  return `${number > 0 ? "+" : "−"}${formatter(Math.abs(number))}`;
}

function pctText(value) {
  const number = toNum(value);
  if (number === null) return "—";
  const digits = Math.abs(number) < 10 ? 2 : 1;
  const sign = number > 0 ? "+" : number < 0 ? "−" : "";
  return `${sign}${Math.abs(number).toFixed(digits)}%`;
}

function formatIndex(value) {
  const number = toNum(value);
  if (number === null) return "—";
  if (Math.abs(number) >= 1000) {
    return number.toLocaleString("en-GB", { maximumFractionDigits: 1 });
  }
  return number.toFixed(1);
}

function shareText(value) {
  const number = Math.max(0, toNum(value) ?? 0);
  if (number > 0 && number < 0.1) return "<0.1%";
  return `${number.toFixed(1)}%`;
}

function pad2(number) {
  return String(number).padStart(2, "0");
}

function fmtBucket(seconds) {
  const stamp = toNum(seconds);
  if (!stamp) return "—";
  const date = new Date(stamp * 1000);
  if (Number.isNaN(date.getTime())) return "—";
  return `${date.getDate()} ${MONTHS[date.getMonth()]} ${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
}

function fmtClock(seconds) {
  const stamp = toNum(seconds);
  if (!stamp) return "—";
  const date = new Date(stamp * 1000);
  if (Number.isNaN(date.getTime())) return "—";
  return `${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
}

function ageFromMinutes(minutes) {
  const total = Math.max(0, Math.round(toNum(minutes) ?? 0));
  if (total < 1) return "just now";
  if (total < 60) return `${total}m ago`;
  if (total < 60 * 48) {
    const hours = Math.floor(total / 60);
    const rest = total % 60;
    return rest ? `${hours}h ${rest}m ago` : `${hours}h ago`;
  }
  return `${Math.floor(total / 1440)}d ago`;
}

function itemLabel(item) {
  if (!item || typeof item !== "object") return "Unknown item";
  if (item.name) return String(item.name);
  if (item.item_id !== undefined && item.item_id !== null) return `Item #${item.item_id}`;
  return "Unknown item";
}

function qualityColor(quality) {
  const index = toNum(quality);
  return index !== null && QUALITY[index] ? QUALITY[index] : "#ffffff";
}

function qualityName(quality) {
  const index = toNum(quality);
  return index !== null && QUALITY_NAMES[index] ? QUALITY_NAMES[index] : "Unknown";
}

function className(classID) {
  const id = toNum(classID);
  if (id === null) return "Other";
  return CLASS_NAMES[String(id)] || `Class ${id}`;
}

/* app.js does not read query params, so point at the search page the safest way. */
function searchHref(name) {
  const query = String(name || "").trim();
  return query ? `/?q=${encodeURIComponent(query)}` : "/";
}

function seriesPoints(data) {
  const raw = data && data.index && Array.isArray(data.index.points) ? data.index.points : [];
  return raw
    .map((point) => ({
      t: toNum(point && point.t),
      value: toNum(point && point.value),
    }))
    .filter((point) => point.t !== null && point.t > 0 && point.value !== null)
    .sort((a, b) => a.t - b.t);
}

function referenceInfo(windowData) {
  const win = windowData || {};
  const bucket = toNum(win.reference_bucket);
  const hours = toNum(win.reference_hours);
  if (!bucket || bucket <= 0 || !hours || hours <= 0) return null;
  return { bucket, hours };
}

function isEmptyMarket(data) {
  if (!data) return false;
  const totals = data.totals || {};
  const items = toNum(totals.items) ?? 0;
  const offers = toNum(totals.offers) ?? 0;
  const points = seriesPoints(data);
  const heavy = Array.isArray(data.heavyweights) ? data.heavyweights.length : 0;
  return items === 0 && offers === 0 && points.length === 0 && heavy === 0;
}

/* ---------- Notice + status ---------- */

function showNotice(text, isError) {
  $("#notice-text").textContent = text;
  $("#notice").hidden = false;
  $("#notice").classList.toggle("is-error", Boolean(isError));
}

function hideNotice() {
  $("#notice").hidden = true;
  $("#notice").classList.remove("is-error");
}

function updateNotice() {
  if (state.lastError) {
    const suffix = state.data ? " Showing the last good data." : "";
    showNotice(`${state.lastError}${suffix}`, true);
    return;
  }
  if (state.data && isEmptyMarket(state.data)) {
    showNotice("No data for this market yet. Stats appear after the first companion scan.", false);
    return;
  }
  hideNotice();
}

function setStatus(text, isError) {
  const node = $("#refresh-state");
  node.textContent = text;
  node.classList.toggle("is-error", Boolean(isError));
}

function updateRefreshUI() {
  const pill = $("#sync-pill");
  const pillText = $("#sync-text");

  if (state.lastError) {
    pill.className = "sync-pill is-error";
    pillText.textContent = "Refresh failed";
    setStatus(state.data ? "Showing earlier data" : "Couldn't load stats", true);
    return;
  }
  if (!state.data) {
    pill.className = "sync-pill";
    pillText.textContent = state.loading ? "Connecting" : "Waiting for data";
    if (!state.loading) setStatus("No data yet", false);
    return;
  }

  pill.className = "sync-pill is-live";
  pillText.textContent = "Live";
  const seconds = Math.max(0, Math.round((Date.now() - state.lastLoaded) / 1000));
  if (state.loading) {
    setStatus("Refreshing…", false);
  } else if (seconds < 5) {
    setStatus("Updated just now", false);
  } else if (seconds < 60) {
    setStatus(`Updated ${seconds}s ago`, false);
  } else {
    setStatus(`Updated ${Math.floor(seconds / 60)}m ago`, false);
  }
}

/* ---------- KPI row ---------- */

function chip(kind, arrow, text, title) {
  const node = el("span", `chip ${kind}`);
  if (arrow) {
    const mark = el("span", "arrow", arrow);
    mark.setAttribute("aria-hidden", "true");
    node.append(mark);
  }
  node.append(el("span", "", text));
  if (title) node.title = title;
  return node;
}

function renderDelta(slot, delta, formatter, ref, emptyMarket) {
  slot.textContent = "";
  if (emptyMarket) {
    slot.append(chip("quiet", "", "no data"));
    return;
  }
  if (!ref) {
    slot.append(chip("quiet", "", "no comparison", "No earlier scan to compare against"));
    return;
  }
  const number = toNum(delta);
  if (number === null || number === 0) {
    const kind = number === 0 ? "flat" : "quiet";
    slot.append(chip(kind, number === 0 ? "–" : "", number === 0 ? "no change" : "no change data",
      `Compared with ${fmtBucket(ref.bucket)}`));
    return;
  }
  const kind = number > 0 ? "up" : "down";
  const arrow = number > 0 ? "▲" : "▼";
  slot.append(chip(kind, arrow, signed(number, formatter),
    `Compared with ${fmtBucket(ref.bucket)} (${Math.round(ref.hours)}h ago)`));
}

function setKpiValue(id, text) {
  const node = $(id);
  node.textContent = text;
}

function renderKpis(data) {
  const index = data.index || {};
  const totals = data.totals || {};
  const win = data.window || {};
  const ref = referenceInfo(win);
  const points = seriesPoints(data);
  const empty = isEmptyMarket(data);
  const lastValue = points.length ? points[points.length - 1].value : null;

  /* Market index */
  setKpiValue("#kpi-index", lastValue === null ? "—" : formatIndex(lastValue));
  const indexChip = $("#kpi-index-chip");
  indexChip.textContent = "";
  const changePct = toNum(index.change_pct);
  if (lastValue === null && !points.length) {
    indexChip.append(chip("quiet", "", "no data", "No index points yet"));
  } else if (empty) {
    indexChip.append(chip("quiet", "", "no data", "No index points yet"));
  } else if (!ref) {
    indexChip.append(chip("quiet", "", "no comparison", "No earlier scan to compare against"));
  } else if (points.length < 2) {
    indexChip.append(chip("quiet", "", "not enough history", "The index needs at least two scans"));
  } else if (changePct === null) {
    indexChip.append(chip("quiet", "", "no change data"));
  } else {
    const kind = changePct > 0 ? "up" : changePct < 0 ? "down" : "flat";
    const arrow = changePct > 0 ? "▲" : changePct < 0 ? "▼" : "–";
    indexChip.append(chip(kind, arrow, pctText(changePct),
      `Compared with ${fmtBucket(ref.bucket)} (${Math.round(ref.hours)}h ago)`));
  }
  drawSpark(points, changePct);

  /* Items listed */
  const items = toNum(totals.items);
  setKpiValue("#kpi-items", items === null ? "—" : numShort(items));
  const itemsPrev = toNum(totals.items_prev);
  renderDelta($("#kpi-items-chip"),
    items !== null && itemsPrev !== null ? items - itemsPrev : null, numShort, ref, empty);

  /* Offers */
  const offers = toNum(totals.offers);
  setKpiValue("#kpi-offers", offers === null ? "—" : numShort(offers));
  const offersPrev = toNum(totals.offers_prev);
  renderDelta($("#kpi-offers-chip"),
    offers !== null && offersPrev !== null ? offers - offersPrev : null, numShort, ref, empty);

  /* Total stock quantity */
  const quantity = toNum(totals.quantity);
  setKpiValue("#kpi-qty", quantity === null ? "—" : numShort(quantity));
  const quantityPrev = toNum(totals.quantity_prev);
  renderDelta($("#kpi-qty-chip"),
    quantity !== null && quantityPrev !== null ? quantity - quantityPrev : null, numShort, ref, empty);

  /* Market value */
  const value = toNum(totals.value);
  setKpiValue("#kpi-value", value === null ? "—" : moneyShort(value));
  const valuePrev = toNum(totals.value_prev);
  renderDelta($("#kpi-value-chip"),
    value !== null && valuePrev !== null ? value - valuePrev : null, moneyShort, ref, empty);

  /* New items */
  const newItems = data.new_items || {};
  const newCount = toNum(newItems.count) ??
    (Array.isArray(newItems.items) ? newItems.items.length : 0);
  setKpiValue("#kpi-new", numShort(newCount));
  const newChip = $("#kpi-new-chip");
  newChip.textContent = "";
  newChip.append(newCount > 0
    ? chip("window", "", "in window", "Items first seen in this window")
    : chip("quiet", "", "none in window", "No new items in this window"));

  /* Delisted */
  const delisted = toNum(data.delisted);
  setKpiValue("#kpi-delisted", delisted === null ? "—" : numShort(delisted));
  const delistedChip = $("#kpi-delisted-chip");
  delistedChip.textContent = "";
  if (delisted === null) {
    delistedChip.append(chip("quiet", "", "no data"));
  } else {
    delistedChip.append(delisted > 0
      ? chip("window", "", "in window", "Items no longer listed in this window")
      : chip("quiet", "", "none in window", "No items were delisted in this window"));
  }
}

function setSparkVisible(visible) {
  /* SVG elements do not reflect the HTML hidden property, so use the attribute. */
  const svg = $("#kpi-spark");
  if (visible) svg.removeAttribute("hidden");
  else svg.setAttribute("hidden", "");
}

function drawSpark(points, changePct) {
  const svg = $("#kpi-spark");
  if (points.length < 2) {
    setSparkVisible(false);
    return;
  }
  setSparkVisible(true);
  svg.textContent = "";
  const values = points.map((point) => point.value);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;
  const coords = points.map((point, index) => {
    const x = 2 + (index / (points.length - 1)) * 68;
    const y = 23 - ((point.value - min) / span) * 19;
    return `${x.toFixed(2)},${y.toFixed(2)}`;
  });
  const rising = changePct === null
    ? points[points.length - 1].value >= points[0].value
    : changePct >= 0;
  svg.append(svgEl("polyline", {
    points: coords.join(" "),
    fill: "none",
    stroke: rising ? "#4ade80" : "#f87171",
    "stroke-width": "1.6",
    "stroke-linecap": "round",
    "stroke-linejoin": "round",
  }));
}

/* ---------- Window ---------- */

function renderWindow(data) {
  const win = data.window || {};
  const points = seriesPoints(data);
  const first = toNum(win.first_bucket) ?? (points.length ? points[0].t : null);
  const last = toNum(win.current_bucket) ?? (points.length ? points[points.length - 1].t : null);
  const line = $("#window-line");
  line.textContent = "";

  if (!first || !last) {
    line.textContent = "No scan window yet. Stats appear after the first companion scan.";
  } else {
    line.append(document.createTextNode("Scan window "));
    line.append(el("strong", "", `${fmtBucket(first)} → ${fmtBucket(last)}`));
    const ref = referenceInfo(win);
    if (ref) {
      line.append(document.createTextNode(" · compared with "));
      line.append(el("strong", "", fmtBucket(ref.bucket)));
      line.append(document.createTextNode(` (${Math.round(ref.hours)}h ago)`));
    } else {
      line.append(document.createTextNode(" · no earlier scan to compare"));
    }
  }

  const badge = $("#market-badge");
  const market = data.market || state.market;
  badge.textContent = market || "";
  badge.hidden = !market;
}

/* ---------- Index chart ---------- */

function niceStep(raw) {
  if (!Number.isFinite(raw) || raw <= 0) return 1;
  const power = Math.pow(10, Math.floor(Math.log10(raw)));
  const scaled = raw / power;
  const step = scaled <= 1 ? 1 : scaled <= 2 ? 2 : scaled <= 2.5 ? 2.5 : scaled <= 5 ? 5 : 10;
  return step * power;
}

let chartResizeTimer = 0;

function renderChart(data) {
  const plot = $("#chart-plot");
  const points = seriesPoints(data);
  const panel = plot.closest(".chart-panel");
  state.chartPoints = points;
  const note = $("#chart-note");
  const caption = $("#chart-caption");
  plot.textContent = "";

  if (points.length < 2) {
    if (panel) panel.style.setProperty("--line-color", "#7d8b9c");
    const placeholder = el("div", "chart-placeholder");
    placeholder.append(el("strong", "", "Collecting history"));
    placeholder.append(el("span", "", isEmptyMarket(data)
      ? "No scans for this market yet. The index appears after the first two companion scans."
      : "The index needs at least two scans before it can draw a line."));
    plot.append(placeholder);
    note.textContent = points.length ? "1 scan so far" : "Waiting for the first scan";
    caption.textContent = "";
    return;
  }

  const width = Math.max(272, Math.round(plot.clientWidth || 720));
  const height = width < 560 ? 186 : 252;
  const small = width < 470;
  const pad = { top: 16, right: small ? 44 : 56, bottom: 26, left: 44 };
  const innerW = Math.max(80, width - pad.left - pad.right);
  const innerH = height - pad.top - pad.bottom;

  const values = points.map((point) => point.value);
  let min = Math.min(100, ...values);
  let max = Math.max(100, ...values);
  if (max - min < 1.5) {
    const mid = (min + max) / 2;
    min = mid - 0.75;
    max = mid + 0.75;
  }
  const share = (max - min) * 0.12;
  const domainMin = min - share;
  const domainMax = max + share;
  const step = niceStep((domainMax - domainMin) / 5);
  const ticks = [];
  for (let tick = Math.ceil(domainMin / step) * step;
       tick <= domainMax + 1e-9 && ticks.length < 20;
       tick += step) {
    ticks.push(Number(tick.toFixed(6)));
  }

  const firstT = points[0].t;
  const lastT = points[points.length - 1].t;
  const timeSpan = lastT - firstT || 1;
  const x = (time) => pad.left + ((time - firstT) / timeSpan) * innerW;
  const y = (value) => pad.top + ((domainMax - value) / (domainMax - domainMin)) * innerH;

  const changePct = toNum(data.index && data.index.change_pct);
  const rising = changePct === null
    ? points[points.length - 1].value >= points[0].value
    : changePct >= 0;
  const lineColor = rising ? "#4ade80" : "#f87171";
  if (panel) panel.style.setProperty("--line-color", lineColor);

  const svg = svgEl("svg", {
    width,
    height,
    viewBox: `0 0 ${width} ${height}`,
    role: "img",
    "aria-label": `Market index from ${fmtBucket(firstT)} to ${fmtBucket(lastT)}, last value ${formatIndex(points[points.length - 1].value)}.`,
  });

  const defs = svgEl("defs", {});
  const gradient = svgEl("linearGradient", {
    id: "chart-fill", x1: "0", y1: "0", x2: "0", y2: "1",
  });
  gradient.append(svgEl("stop", { offset: "0", "stop-color": lineColor, "stop-opacity": "0.3" }));
  gradient.append(svgEl("stop", { offset: "1", "stop-color": lineColor, "stop-opacity": "0" }));
  defs.append(gradient);
  svg.append(defs);

  for (const tick of ticks) {
    const gy = y(tick);
    svg.append(svgEl("line", {
      x1: pad.left, y1: gy, x2: width - pad.right, y2: gy, class: "chart-grid-line",
    }));
    const diff = tick - 100;
    const decimals = Number.isInteger(diff) ? 0 : 1;
    const label = `${diff > 0 ? "+" : diff < 0 ? "−" : ""}${Math.abs(diff).toFixed(decimals)}%`;
    const text = svgEl("text", {
      x: pad.left - 8, y: gy + 3.5, "text-anchor": "end", class: "chart-axis-text",
    });
    text.textContent = label;
    svg.append(text);
  }

  const baseY = y(100);
  svg.append(svgEl("line", {
    x1: pad.left, y1: baseY, x2: width - pad.right, y2: baseY, class: "chart-base",
  }));
  const baseLabel = svgEl("text", {
    x: width - pad.right + 6, y: baseY + 3.5, class: "chart-axis-text",
  });
  baseLabel.textContent = "100";
  svg.append(baseLabel);

  const coords = points.map((point) => [x(point.t), y(point.value)]);
  const linePath = coords
    .map((point, index) => `${index ? "L" : "M"}${point[0].toFixed(2)} ${point[1].toFixed(2)}`)
    .join(" ");
  const bottom = height - pad.bottom;
  const areaPath = `${linePath} L${coords[coords.length - 1][0].toFixed(2)} ${bottom} ` +
    `L${coords[0][0].toFixed(2)} ${bottom} Z`;

  svg.append(svgEl("path", { d: areaPath, class: "chart-area" }));
  const line = svgEl("path", { d: linePath, class: "chart-line", pathLength: "1" });
  if (!state.chartSeen) {
    line.classList.add("is-drawing");
    state.chartSeen = true;
  }
  svg.append(line);

  const last = coords[coords.length - 1];
  svg.append(svgEl("circle", {
    cx: last[0], cy: last[1], r: 9, class: "chart-cursor-halo", opacity: "0.16",
  }));
  svg.append(svgEl("circle", {
    cx: last[0], cy: last[1], r: 3.6, class: "chart-last-dot",
  }));

  const lastValue = points[points.length - 1].value;
  const valueLabel = svgEl("text", {
    x: last[0] > width - pad.right - 34 ? last[0] - 10 : last[0] + 10,
    y: Math.max(12, Math.min(height - pad.bottom - 6, last[1] + 4)),
    "text-anchor": last[0] > width - pad.right - 34 ? "end" : "start",
    class: "chart-value-text",
  });
  valueLabel.textContent = formatIndex(lastValue);
  svg.append(valueLabel);

  const firstLabel = svgEl("text", {
    x: pad.left, y: height - 7, class: "chart-axis-text",
  });
  firstLabel.textContent = fmtBucket(firstT);
  svg.append(firstLabel);

  const lastLabel = svgEl("text", {
    x: width - pad.right, y: height - 7, "text-anchor": "end", class: "chart-axis-text",
  });
  lastLabel.textContent = fmtBucket(lastT);
  svg.append(lastLabel);

  const guide = svgEl("line", {
    x1: 0, x2: 0, y1: pad.top, y2: bottom, class: "chart-guide", opacity: "0",
  });
  const halo = svgEl("circle", {
    cx: 0, cy: 0, r: 8, class: "chart-cursor-halo", opacity: "0",
  });
  const cursor = svgEl("circle", {
    cx: 0, cy: 0, r: 3.6, class: "chart-cursor", opacity: "0",
  });
  const hit = svgEl("rect", {
    x: pad.left, y: pad.top, width: innerW, height: innerH,
    fill: "transparent", style: "cursor: crosshair",
  });
  svg.append(guide, halo, cursor, hit);
  plot.append(svg);

  const tip = el("div", "chart-tip");
  tip.hidden = true;
  plot.append(tip);

  function hideHover() {
    guide.setAttribute("opacity", "0");
    halo.setAttribute("opacity", "0");
    cursor.setAttribute("opacity", "0");
    tip.hidden = true;
  }

  hit.addEventListener("pointermove", (event) => {
    const rect = svg.getBoundingClientRect();
    if (!rect.width) return;
    const px = ((event.clientX - rect.left) / rect.width) * width;
    let bestIndex = 0;
    let bestDistance = Infinity;
    coords.forEach((point, index) => {
      const distance = Math.abs(point[0] - px);
      if (distance < bestDistance) {
        bestDistance = distance;
        bestIndex = index;
      }
    });
    const point = coords[bestIndex];
    const item = points[bestIndex];
    guide.setAttribute("x1", point[0]);
    guide.setAttribute("x2", point[0]);
    halo.setAttribute("cx", point[0]);
    halo.setAttribute("cy", point[1]);
    cursor.setAttribute("cx", point[0]);
    cursor.setAttribute("cy", point[1]);
    guide.setAttribute("opacity", "1");
    halo.setAttribute("opacity", "1");
    cursor.setAttribute("opacity", "1");

    const diff = item.value - 100;
    tip.textContent = "";
    tip.append(el("span", "tip-time", fmtBucket(item.t)));
    tip.append(el("span", "tip-value", formatIndex(item.value)));
    tip.append(el("span", `tip-change change ${diff >= 0 ? "up" : "down"}`,
      `${diff >= 0 ? "+" : "−"}${Math.abs(diff).toFixed(1)}% since base`));
    tip.hidden = false;
    tip.style.left = `${clamp(point[0], 64, width - 64)}px`;
    tip.style.top = "6px";
  });
  hit.addEventListener("pointerleave", hideHover);
  hit.addEventListener("pointercancel", hideHover);

  const scanCount = points.length;
  note.textContent = `${scanCount} scans · base 100`;
  const ref = referenceInfo(data.window || {});
  if (changePct !== null && ref) {
    caption.textContent =
      `Last ${formatIndex(lastValue)} · ${pctText(changePct)} vs ${Math.round(ref.hours)}h ago`;
    caption.title = `Compared with ${fmtBucket(ref.bucket)}`;
  } else {
    caption.textContent = `Last ${formatIndex(lastValue)} · base 100 at window start`;
  }
}

/* ---------- Breadth ---------- */

function renderBreadth(data) {
  const breadth = data.breadth || {};
  const up = toNum(breadth.Up ?? breadth.up) ?? 0;
  const down = toNum(breadth.Down ?? breadth.down) ?? 0;
  const flat = toNum(breadth.Flat ?? breadth.flat) ?? 0;
  const matched = toNum(breadth.Matched ?? breadth.matched);
  const total = up + down + flat;
  const bar = $("#breadth-bar");
  const legend = $("#breadth-legend");
  const matchedNode = $("#breadth-matched");

  bar.textContent = "";
  bar.setAttribute("aria-label", total > 0
    ? `${up} items up, ${down} items down, ${flat} flat, out of ${total} matched items.`
    : "No matched items to compare.");

  if (total === 0) {
    bar.append(el("span", "breadth-empty", "No matched items to compare"));
  } else {
    const parts = [
      { key: "up", count: up, className: "up" },
      { key: "down", count: down, className: "down" },
      { key: "flat", count: flat, className: "flat" },
    ];
    for (const part of parts) {
      if (!part.count) continue;
      const pct = (part.count / total) * 100;
      const segment = el("span", `breadth-seg ${part.className}`);
      segment.style.flex = `0 0 ${pct}%`;
      if (pct >= 12) segment.textContent = `${Math.round(pct)}%`;
      segment.title = `${part.count.toLocaleString("en-GB")} items (${pct.toFixed(1)}%)`;
      bar.append(segment);
    }
  }

  legend.textContent = "";
  const legendRows = [
    { key: "up", label: "▲ Up", count: up },
    { key: "down", label: "▼ Down", count: down },
    { key: "flat", label: "– Flat", count: flat },
  ];
  for (const row of legendRows) {
    const pct = total > 0 ? (row.count / total) * 100 : null;
    const item = el("span", `breadth-item ${row.key}`);
    const dot = el("i", "b-dot");
    dot.setAttribute("aria-hidden", "true");
    item.append(dot);
    item.append(el("span", "b-label", row.label));
    item.append(el("strong", "", row.count.toLocaleString("en-GB")));
    item.append(el("span", "b-pct", pct === null ? "—" : `${pct.toFixed(1)}%`));
    legend.append(item);
  }

  if (total === 0) {
    matchedNode.textContent = "No matched items";
    matchedNode.title = "";
  } else {
    const count = matched ?? total;
    matchedNode.textContent = `${count.toLocaleString("en-GB")} matched items`;
    const ref = referenceInfo(data.window || {});
    matchedNode.title = ref
      ? `Prices compared with the scan at ${fmtBucket(ref.bucket)}`
      : "No earlier scan to compare against";
  }
}

/* ---------- Movers ---------- */

function emptyTableRow(text) {
  const row = el("tr");
  const cell = el("td", "empty-row", text);
  cell.colSpan = 5;
  row.append(cell);
  return row;
}

function moverRow(item) {
  const row = el("tr");
  const href = searchHref(item.item_id ?? item.name);
  if (item.name) row.dataset.href = href;

  const nameCell = el("td");
  const link = el("a", "item-link", itemLabel(item));
  link.href = href;
  link.style.color = qualityColor(item.quality);
  link.title = `${itemLabel(item)} · ${qualityName(item.quality)}`;
  nameCell.append(link);
  row.append(nameCell);

  const price = toNum(item.price);
  row.append(el("td", "num price-cell", price === null ? "—" : money(price)));

  const previous = toNum(item.price_prev);
  row.append(el("td", "num was-cell", previous === null ? "—" : money(previous)));

  const changeCell = el("td", "num");
  const change = toNum(item.change_pct);
  if (change === null) {
    changeCell.append(el("span", "change flat", "—"));
  } else {
    const kind = change > 0 ? "up" : change < 0 ? "down" : "flat";
    const mark = el("span", "change " + kind);
    const arrow = el("span", "arrow", change > 0 ? "▲" : change < 0 ? "▼" : "–");
    arrow.setAttribute("aria-hidden", "true");
    mark.append(arrow, document.createTextNode(pctText(change)));
    changeCell.append(mark);
  }
  row.append(changeCell);

  const quantity = toNum(item.quantity);
  const stock = el("td", "num stock-cell", quantity === null ? "—" : numShort(quantity));
  if (quantity !== null) stock.title = `${quantity.toLocaleString("en-GB")} listed`;
  row.append(stock);

  return row;
}

function renderMoverTable(tbody, rows, emptyText) {
  tbody.textContent = "";
  if (!rows.length) {
    tbody.append(emptyTableRow(emptyText));
    return;
  }
  for (const item of rows) {
    if (item && typeof item === "object") tbody.append(moverRow(item));
  }
  if (!tbody.children.length) tbody.append(emptyTableRow(emptyText));
}

function renderMovers(data) {
  const movers = data.movers || {};
  const gainers = Array.isArray(movers.gainers) ? movers.gainers : [];
  const losers = Array.isArray(movers.losers) ? movers.losers : [];
  const ref = referenceInfo(data.window || {});
  const empty = isEmptyMarket(data);
  const comparable = Boolean(ref) && seriesPoints(data).length >= 2;
  const emptyText = empty
    ? "No data for this market yet."
    : !comparable
      ? "Not enough history to compare."
      : "No price changes in this window.";
  renderMoverTable($("#gainers-body"), gainers, emptyText);
  renderMoverTable($("#losers-body"), losers, emptyText);
}

/* ---------- Stock movers ---------- */

function stockRow(item) {
  const row = el("li", "stock-row");

  const main = el("div", "stock-main");
  const link = el("a", "stock-name", itemLabel(item));
  link.href = searchHref(item.item_id ?? item.name);
  link.style.color = qualityColor(item.quality);
  main.append(link);
  const previous = toNum(item.quantity_prev);
  const current = toNum(item.quantity);
  const subText = previous !== null && current !== null
    ? `${previous.toLocaleString("en-GB")} → ${current.toLocaleString("en-GB")} listed`
    : current !== null
      ? `${current.toLocaleString("en-GB")} listed`
      : "Quantity unknown";
  main.append(el("span", "stock-sub", subText));
  row.append(main);

  const price = toNum(item.price);
  row.append(el("span", "stock-price", price === null ? "—" : money(price)));

  let delta = toNum(item.delta);
  if (delta === null && current !== null && previous !== null) delta = current - previous;
  if (delta === null) {
    row.append(el("span", "chip quiet", "—"));
  } else {
    const kind = delta > 0 ? "up" : delta < 0 ? "down" : "flat";
    const arrow = delta > 0 ? "▲" : delta < 0 ? "▼" : "–";
    row.append(chip(kind, arrow, signed(delta, numShort), "Listed quantity change"));
  }

  return row;
}

function renderStockList(list, rows, emptyText) {
  list.textContent = "";
  if (!rows.length) {
    list.append(el("li", "empty-row", emptyText));
    return;
  }
  for (const item of rows) {
    if (item && typeof item === "object") list.append(stockRow(item));
  }
  if (!list.children.length) list.append(el("li", "empty-row", emptyText));
}

function renderStock(data) {
  const stock = data.stock || {};
  const added = Array.isArray(stock.added) ? stock.added : [];
  const removed = Array.isArray(stock.removed) ? stock.removed : [];
  const ref = referenceInfo(data.window || {});
  const empty = isEmptyMarket(data);
  const comparable = Boolean(ref) && seriesPoints(data).length >= 2;
  const base = empty
    ? "No data for this market yet."
    : !comparable
      ? "Not enough history to compare."
      : "";
  renderStockList($("#stock-added"), added, base || "No stock added in this window.");
  renderStockList($("#stock-removed"), removed, base || "No stock removed in this window.");
}

/* ---------- Composition ---------- */

function renderClasses(data) {
  const list = $("#class-list");
  list.textContent = "";
  const rows = (Array.isArray(data.classes) ? data.classes : [])
    .filter((row) => row && typeof row === "object");

  if (!rows.length) {
    list.append(el("li", "empty-row", isEmptyMarket(data)
      ? "No data for this market yet."
      : "No class breakdown in this window."));
    return;
  }

  const totalValue = rows.reduce((sum, row) => sum + Math.max(0, toNum(row.value) ?? 0), 0);
  const ref = referenceInfo(data.window || {});

  for (const row of rows.slice(0, 6)) {
    const value = Math.max(0, toNum(row.value) ?? 0);
    const percent = totalValue > 0 ? (value / totalValue) * 100 : 0;
    const item = el("li", "class-row");
    item.append(el("span", "class-name", className(row.class_id)));
    item.append(el("span", "class-value", moneyShort(value)));

    const change = toNum(row.change_pct);
    const changeNode = el("span", "class-change");
    if (change === null) {
      changeNode.textContent = "—";
    } else if (!ref) {
      changeNode.textContent = "—";
    } else {
      changeNode.classList.add(change > 0 ? "up" : change < 0 ? "down" : "flat");
      changeNode.textContent =
        `${change > 0 ? "▲" : change < 0 ? "▼" : "–"} ${change > 0 ? "+" : change < 0 ? "−" : ""}${Math.abs(change).toFixed(1)}%`;
      changeNode.title = `Compared with ${fmtBucket(ref.bucket)} (${Math.round(ref.hours)}h ago)`;
    }
    item.append(changeNode);

    const bar = el("span", "class-bar");
    const fill = el("i");
    fill.style.width = percent > 0 ? `${Math.max(percent, 0.8)}%` : "0%";
    bar.append(fill);
    bar.title = `${percent.toFixed(1)}% of listed value`;
    item.append(bar);

    list.append(item);
  }
}

function renderQuality(data) {
  const body = $("#quality-body");
  body.textContent = "";
  const rows = (Array.isArray(data.quality) ? data.quality : [])
    .filter((row) => row && typeof row === "object");

  if (!rows.length) {
    const row = el("tr");
    const cell = el("td", "empty-row", isEmptyMarket(data)
      ? "No data for this market yet."
      : "No quality breakdown in this window.");
    cell.colSpan = 3;
    row.append(cell);
    body.append(row);
    return;
  }

  for (const item of rows) {
    const row = el("tr");
    const nameCell = el("td");
    const wrap = el("span", "quality-name");
    const dot = el("i", "quality-dot");
    dot.style.background = qualityColor(item.quality);
    dot.style.color = qualityColor(item.quality);
    dot.setAttribute("aria-hidden", "true");
    wrap.append(dot, document.createTextNode(qualityName(item.quality)));
    nameCell.append(wrap);
    row.append(nameCell);

    const items = toNum(item.items);
    row.append(el("td", "num stock-cell", items === null ? "—" : numShort(items)));
    const value = toNum(item.value);
    row.append(el("td", "num price-cell", value === null ? "—" : moneyShort(value)));
    body.append(row);
  }
}

function renderBands(data) {
  const body = $("#bands-body");
  body.textContent = "";
  const rows = (Array.isArray(data.bands) ? data.bands : [])
    .filter((row) => row && typeof row === "object");

  if (!rows.length) {
    const row = el("tr");
    const cell = el("td", "empty-row", isEmptyMarket(data)
      ? "No data for this market yet."
      : "No price bands in this window.");
    cell.colSpan = 3;
    row.append(cell);
    body.append(row);
    return;
  }

  for (const item of rows) {
    const row = el("tr");
    row.append(el("td", "band-cell", item.label ? String(item.label) : "—"));
    const items = toNum(item.items);
    row.append(el("td", "num stock-cell", items === null ? "—" : numShort(items)));
    const value = toNum(item.value);
    row.append(el("td", "num price-cell", value === null ? "—" : moneyShort(value)));
    body.append(row);
  }
}

/* ---------- Heavyweights ---------- */

function renderHeavyweights(data) {
  const list = $("#heavy-list");
  list.textContent = "";
  const rows = (Array.isArray(data.heavyweights) ? data.heavyweights : [])
    .filter((row) => row && typeof row === "object");

  if (!rows.length) {
    list.append(el("li", "empty-row", isEmptyMarket(data)
      ? "No data for this market yet."
      : "No items with a listed value yet."));
    return;
  }

  const maxShare = rows.reduce((max, row) => Math.max(max, toNum(row.share_pct) ?? 0), 0);

  rows.slice(0, 10).forEach((item, index) => {
    const row = el("li", "heavy-row");
    row.append(el("span", "heavy-rank", String(index + 1).padStart(2, "0")));

    const link = el("a", "heavy-name", itemLabel(item));
    link.href = searchHref(item.item_id ?? item.name);
    link.title = itemLabel(item);
    row.append(link);

    const shareValue = Math.max(0, toNum(item.share_pct) ?? 0);
    const share = el("span", "heavy-share");
    const track = el("span", "heavy-track");
    const fill = el("i");
    const relative = maxShare > 0 ? (shareValue / maxShare) * 100 : 0;
    fill.style.width = `${clamp(relative, shareValue > 0 ? 2 : 0, 100)}%`;
    track.append(fill);
    track.title = `${shareText(shareValue)} of the whole market`;
    share.append(track);
    share.append(el("span", "heavy-pct", shareText(shareValue)));
    row.append(share);

    const value = toNum(item.value);
    row.append(el("span", "heavy-value", value === null ? "—" : moneyShort(value)));

    list.append(row);
  });
}

/* ---------- Freshness ---------- */

function renderFreshness(data) {
  const freshness = data.freshness || {};
  const scanAge = toNum(freshness.last_scan_age_min);
  const scanAt = toNum(freshness.last_scan);

  const scanNode = $("#fresh-scan");
  if (!scanAt) {
    scanNode.textContent = "—";
    scanNode.title = "";
  } else {
    scanNode.textContent = scanAge === null ? "—" : ageFromMinutes(scanAge);
    scanNode.title = `Last scan at ${fmtBucket(scanAt)}`;
  }

  const scans = toNum(freshness.scans_24h);
  $("#fresh-scans").textContent = scans === null ? "—" : `${scans.toLocaleString("en-GB")}`;

  const clients = toNum(freshness.clients_24h);
  $("#fresh-clients").textContent = clients === null
    ? "—"
    : `${clients.toLocaleString("en-GB")} ${clients === 1 ? "contributor" : "contributors"}`;

  const generatedAt = toNum(data.generated_at);
  $("#fresh-generated").textContent = generatedAt ? fmtBucket(generatedAt) : "—";

  const freshTime = $("#fresh-time");
  freshTime.textContent = generatedAt ? `Data as of ${fmtBucket(generatedAt)}` : "";
}

/* ---------- Render pipeline ---------- */

function render(data) {
  renderWindow(data);
  renderKpis(data);
  renderChart(data);
  renderBreadth(data);
  renderMovers(data);
  renderStock(data);
  renderClasses(data);
  renderQuality(data);
  renderBands(data);
  renderHeavyweights(data);
  renderFreshness(data);
  updateNotice();
}

function renderLoading() {
  state.chartPoints = [];
  for (const id of [
    "#kpi-index", "#kpi-items", "#kpi-offers", "#kpi-qty",
    "#kpi-value", "#kpi-new", "#kpi-delisted",
  ]) {
    const node = $(id);
    node.textContent = "";
    node.append(el("span", "skel skel-value"));
  }
  for (const id of [
    "#kpi-index-chip", "#kpi-items-chip", "#kpi-offers-chip", "#kpi-qty-chip",
    "#kpi-value-chip", "#kpi-new-chip", "#kpi-delisted-chip",
  ]) {
    $(id).textContent = "";
  }
  setSparkVisible(false);
  $("#window-line").textContent = "Loading the latest scan window…";
  $("#market-badge").hidden = true;

  $("#chart-plot").textContent = "";
  $("#chart-plot").append(el("p", "chart-placeholder", "Loading market history…"));
  $("#chart-note").textContent = "Base 100 at the first scan";
  $("#chart-caption").textContent = "";
  const loadingPanel = $("#chart-plot").closest(".chart-panel");
  if (loadingPanel) loadingPanel.style.setProperty("--line-color", "#7d8b9c");

  $("#breadth-matched").textContent = "Loading…";
  const bar = $("#breadth-bar");
  bar.textContent = "";
  bar.append(el("span", "breadth-empty", "Loading…"));
  $("#breadth-legend").textContent = "";

  for (const id of ["#gainers-body", "#losers-body"]) {
    const body = $(id);
    body.textContent = "";
    body.append(emptyTableRow("Loading…"));
  }
  for (const id of ["#stock-added", "#stock-removed"]) {
    const list = $(id);
    list.textContent = "";
    list.append(el("li", "empty-row", "Loading…"));
  }
  for (const id of ["#class-list", "#heavy-list"]) {
    const list = $(id);
    list.textContent = "";
    list.append(el("li", "empty-row", "Loading…"));
  }
  for (const id of ["#quality-body", "#bands-body"]) {
    const body = $(id);
    body.textContent = "";
    const row = el("tr");
    const cell = el("td", "empty-row", "Loading…");
    cell.colSpan = 3;
    row.append(cell);
    body.append(row);
  }
  for (const id of ["#fresh-scan", "#fresh-scans", "#fresh-clients", "#fresh-generated"]) {
    $(id).textContent = "—";
  }
  $("#fresh-time").textContent = "";
}

function renderUnavailable() {
  for (const id of [
    "#kpi-index", "#kpi-items", "#kpi-offers", "#kpi-qty",
    "#kpi-value", "#kpi-new", "#kpi-delisted",
  ]) {
    $(id).textContent = "—";
  }
  for (const id of [
    "#kpi-index-chip", "#kpi-items-chip", "#kpi-offers-chip", "#kpi-qty-chip",
    "#kpi-value-chip", "#kpi-new-chip", "#kpi-delisted-chip",
  ]) {
    $(id).textContent = "";
  }
  setSparkVisible(false);
  $("#window-line").textContent = "Latest scan window unavailable.";

  const plot = $("#chart-plot");
  plot.textContent = "";
  const panel = plot.closest(".chart-panel");
  if (panel) panel.style.setProperty("--line-color", "#7d8b9c");
  const placeholder = el("div", "chart-placeholder");
  placeholder.append(el("strong", "", "Couldn't load market history"));
  placeholder.append(el("span", "", "Try again when the connection is back."));
  plot.append(placeholder);
  $("#chart-caption").textContent = "";
  $("#chart-note").textContent = "—";

  $("#breadth-matched").textContent = "—";
  const bar = $("#breadth-bar");
  bar.textContent = "";
  bar.append(el("span", "breadth-empty", "No data"));
  $("#breadth-legend").textContent = "";

  for (const id of ["#gainers-body", "#losers-body"]) {
    $(id).textContent = "";
    $(id).append(emptyTableRow("Couldn't load market stats."));
  }
  for (const id of ["#stock-added", "#stock-removed", "#class-list", "#heavy-list"]) {
    $(id).textContent = "";
    $(id).append(el("li", "empty-row", "Couldn't load market stats."));
  }
  for (const id of ["#quality-body", "#bands-body"]) {
    const body = $(id);
    body.textContent = "";
    const row = el("tr");
    const cell = el("td", "empty-row", "Couldn't load market stats.");
    cell.colSpan = 3;
    row.append(cell);
    body.append(row);
  }
  for (const id of ["#fresh-scan", "#fresh-scans", "#fresh-clients", "#fresh-generated"]) {
    $(id).textContent = "—";
  }
  $("#fresh-time").textContent = "";
}

/* ---------- Loading ---------- */

async function loadMeta() {
  try {
    const response = await fetch("/v1/meta", { headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error(`Market list unavailable (${response.status})`);
    const markets = await response.json();
    if (!Array.isArray(markets) || !markets.length) throw new Error("No markets available yet.");
    state.meta = markets.filter((entry) => entry && typeof entry.market === "string" && entry.market);

    const select = $("#market");
    select.textContent = "";
    for (const entry of state.meta) {
      const option = document.createElement("option");
      option.value = entry.market;
      option.textContent = entry.market;
      select.append(option);
    }

    if (!state.meta.some((entry) => entry.market === state.market)) {
      const preferred = state.meta.find((entry) => entry.market === "eu:pve:horde")
        || state.meta[0];
      state.market = preferred.market;
      localStorage.setItem("tradepost-market", state.market);
    }
    select.value = state.market;
  } catch (_) {
    const select = $("#market");
    if (!state.market) state.market = "eu:pve:horde";
    if (!select.options.length) {
      const option = document.createElement("option");
      option.value = state.market;
      option.textContent = state.market;
      select.append(option);
    }
    select.value = state.market;
  }
}

async function loadStats({ silent = false } = {}) {
  /* A new market switch may interrupt an older request; requestId makes the
     newest response the only one that paints. */
  state.loading = true;
  const request = ++state.requestId;

  if (!silent) setStatus("Loading…", false);
  else if (state.data) setStatus("Refreshing…", false);

  const params = new URLSearchParams({ hours: "24" });
  if (state.market) params.set("market", state.market);

  try {
    const response = await fetch(`/v1/stats?${params.toString()}`, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      let message = `Couldn't load market stats (${response.status}).`;
      try {
        const body = await response.json();
        if (body && body.error) message = String(body.error);
      } catch (_) { /* keep the status message */ }
      throw new Error(message);
    }
    const data = await response.json();
    if (request !== state.requestId) return;
    if (!data || typeof data !== "object") throw new Error("The stats response was empty.");

    state.data = data;
    state.lastError = "";
    state.lastLoaded = Date.now();
    render(data);
  } catch (error) {
    if (request !== state.requestId) return;
    state.lastError = error && error.message ? error.message : "Couldn't load market stats.";
    if (state.data) {
      updateNotice();
    } else {
      renderUnavailable();
      updateNotice();
    }
  } finally {
    if (request === state.requestId) {
      state.loading = false;
      updateRefreshUI();
    }
  }
}

/* ---------- Boot ---------- */

function changeMarket(market) {
  state.market = market;
  localStorage.setItem("tradepost-market", market);
  state.data = null;
  state.lastError = "";
  state.chartSeen = false;
  renderLoading();
  hideNotice();
  $("#market-badge").textContent = market;
  $("#market-badge").hidden = !market;
  updateRefreshUI();
  loadStats();
}

function boot() {
  const select = $("#market");
  if (state.market) {
    const option = document.createElement("option");
    option.value = state.market;
    option.textContent = state.market;
    select.append(option);
    select.value = state.market;
  }

  select.addEventListener("change", (event) => {
    const market = event.target.value;
    if (!market || market === state.market) return;
    changeMarket(market);
  });

  $("#refresh").addEventListener("click", () => {
    if (!state.loading) loadStats({ silent: Boolean(state.data) });
  });

  $("#notice-retry").addEventListener("click", () => {
    if (state.loading) return;
    state.lastError = "";
    updateNotice();
    loadMeta().finally(() => loadStats({ silent: Boolean(state.data) }));
  });

  /* The item name is the real link; this makes the rest of the row follow it. */
  for (const id of ["#gainers-body", "#losers-body"]) {
    $(id).addEventListener("click", (event) => {
      if (event.target.closest("a")) return;
      const row = event.target.closest("tr[data-href]");
      if (row) window.location.assign(row.dataset.href);
    });
  }

  window.addEventListener("resize", () => {
    clearTimeout(chartResizeTimer);
    chartResizeTimer = setTimeout(() => {
      if (state.data) renderChart(state.data);
    }, 160);
  });

  document.addEventListener("visibilitychange", () => {
    if (document.hidden || state.loading) return;
    if (!state.lastLoaded || Date.now() - state.lastLoaded >= REFRESH_MS) {
      loadStats({ silent: Boolean(state.data) });
    }
  });

  state.timer = setInterval(() => {
    if (document.hidden || state.loading) return;
    loadStats({ silent: true });
  }, REFRESH_MS);

  setInterval(() => {
    if (!state.loading) updateRefreshUI();
  }, 10_000);

  loadMeta().finally(() => loadStats());
}

boot();
