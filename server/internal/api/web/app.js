"use strict";

const $ = (selector) => document.querySelector(selector);

const QUALITY = [
  "#9d9d9d", "#ffffff", "#1eff00", "#0070dd",
  "#a335ee", "#ff8000", "#e6cc80", "#e6cc80",
];

const state = {
  market: localStorage.getItem("bf-market") || "",
  results: [],
  selected: null,
  watch: new Set(JSON.parse(localStorage.getItem("bf-watch") || "[]")),
};

function saveWatch() {
  localStorage.setItem("bf-watch", JSON.stringify([...state.watch]));
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

function itemLevel(key) {
  const parts = key.split(":");
  if (parts[0] === "g" && parts.length > 2) {
    const level = Number(parts[2]);
    return level > 0 ? `ilvl ${level}` : "";
  }
  return "";
}

async function api(path, params) {
  const url = new URL(path, window.location.origin);
  for (const [key, value] of Object.entries(params || {})) {
    if (value !== undefined && value !== null && value !== "") {
      url.searchParams.set(key, value);
    }
  }
  const response = await fetch(url, { headers: { Accept: "application/json" } });
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
    if (state.watch.has(item.key)) {
      state.watch.delete(item.key);
    } else {
      state.watch.add(item.key);
    }
    saveWatch();
    renderResults();
    if (state.selected && state.selected.key === item.key) {
      renderDetail(state.selected);
    }
  });
  return button;
}

function renderResults() {
  const container = $("#results");
  const watchOnly = $("#watch-only").checked;
  const rows = state.results.filter((item) => !watchOnly || state.watch.has(item.key));
  $("#status").textContent =
    `${rows.length} shown · ${state.results.length} result${state.results.length === 1 ? "" : "s"}`;

  if (!rows.length) {
    container.innerHTML = "";
    const empty = document.createElement("p");
    empty.className = "empty";
    empty.textContent = watchOnly ? "Watchlist is empty for these results." : "No items found.";
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
    row.addEventListener("click", () => selectItem(item));

    const starCell = document.createElement("td");
    starCell.append(starButton(item));
    row.append(starCell);

    const itemCell = document.createElement("td");
    const name = document.createElement("div");
    name.className = "item-name";
    name.textContent = itemName(item);
    name.style.color = QUALITY[item.quality] || "#ffffff";
    const sub = document.createElement("div");
    sub.className = "item-sub";
    sub.textContent = [itemLevel(item.key), `#${item.item_id}`].filter(Boolean).join(" · ");
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

function selectItem(item) {
  state.selected = item;
  renderResults();
  renderDetail(item);
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
  watch.addEventListener("click", () => {
    if (state.watch.has(item.key)) {
      state.watch.delete(item.key);
    } else {
      state.watch.add(item.key);
    }
    saveWatch();
    renderResults();
    renderDetail(item);
  });

  detail.append(heading, key, current, watch);

  const chartTitle = document.createElement("h3");
  chartTitle.textContent = "Price history";
  const chart = document.createElement("div");
  chart.className = "chart-wrap";
  chart.textContent = "Loading history…";
  const marketsTitle = document.createElement("h3");
  marketsTitle.textContent = "Markets";
  const markets = document.createElement("div");
  markets.textContent = "Loading markets…";
  detail.append(chartTitle, chart, marketsTitle, markets);

  const requested = item.key;
  Promise.all([
    api("/v1/item/history", { market: state.market, key: item.key, limit: 168 }),
    api("/v1/item/markets", { key: item.key }),
  ]).then(([history, comparison]) => {
    if (!state.selected || state.selected.key !== requested) return;
    const points = history.points || [];
    chart.innerHTML = renderChart(points);
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

function renderChart(points) {
  if (!points.length) {
    return `<p class="empty">No hourly history yet. New scans will build this chart.</p>`;
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
  const dots = points.map((point, index) => `
      <circle cx="${x(index)}" cy="${y(point.max)}" r="2.5" fill="#f0c040" />
      <circle cx="${x(index)}" cy="${y(point.min)}" r="2.5" fill="#4ade80" />`).join("");
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
  const label = (date) => date.toLocaleString([], { month: "short", day: "numeric", hour: "2-digit" });

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

async function search() {
  if (!state.market) return;
  $("#status").textContent = "Searching…";
  try {
    const data = await api("/v1/items", {
      market: state.market,
      q: $("#query").value.trim(),
      limit: 100,
    });
    state.results = data.items || [];
    renderResults();
  } catch (error) {
    $("#status").textContent = error.message;
  }
}

let searchTimer;
$("#query").addEventListener("input", () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(search, 350);
});

$("#search-form").addEventListener("submit", (event) => {
  event.preventDefault();
  clearTimeout(searchTimer);
  search();
});

$("#watch-only").addEventListener("change", renderResults);

$("#market").addEventListener("change", (event) => {
  state.market = event.target.value;
  localStorage.setItem("bf-market", state.market);
  state.selected = null;
  loadMeta().then(search).catch((error) => {
    $("#status").textContent = error.message;
  });
});

loadMeta()
  .then(search)
  .catch((error) => {
    $("#status").textContent = error.message;
  });
