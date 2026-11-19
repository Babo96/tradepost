package dashboard

import "html/template"

const baseCSS = `
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body { margin: 0; font: 14px/1.5 system-ui, sans-serif; background: #111827; color: #e5e7eb; }
  main { max-width: 900px; margin: 40px auto; padding: 0 20px 60px; }
  h1 { font-size: 22px; margin: 0 0 4px; color: #86efac; }
  h2 { font-size: 17px; margin: 0 0 14px; }
  .sub { color: #9ca3af; margin-bottom: 24px; }
  .nav { display: flex; gap: 16px; margin-bottom: 20px; font-size: 13px; }
  .nav a { color: #93c5fd; text-decoration: none; }
  .nav a:hover { text-decoration: underline; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; }
  .card { background: #1f2937; border: 1px solid #374151; border-radius: 10px; padding: 14px 16px; }
  .label { color: #9ca3af; font-size: 12px; text-transform: uppercase; letter-spacing: .05em; }
  .value { font-size: 18px; margin-top: 4px; word-break: break-word; }
  .ok { color: #86efac; } .bad { color: #fca5a5; } .warn { color: #fcd34d; }
  .notice { border-radius: 8px; padding: 12px 14px; margin: 0 0 20px; border: 1px solid #374151; background: #1f2937; }
  .notice.bad { border-color: #7f1d1d; background: #2a1215; color: #fca5a5; }
  .notice.ok { border-color: #14532d; background: #10241a; color: #86efac; }
  button { background: #16a34a; color: white; border: 0; border-radius: 8px; padding: 10px 18px; font-weight: 600; cursor: pointer; font: inherit; }
  button.secondary { background: #374151; }
  button:disabled { opacity: .5; cursor: default; }
  input, select { background: #0b1220; color: #e5e7eb; border: 1px solid #374151; border-radius: 8px; padding: 9px 11px; font: inherit; width: 100%; }
  label { display: block; margin: 14px 0 6px; color: #9ca3af; font-size: 13px; }
  .row { display: flex; gap: 10px; align-items: end; flex-wrap: wrap; }
  .row > div { flex: 1 1 280px; }
  .actions { display: flex; gap: 10px; flex-wrap: wrap; margin-top: 20px; }
  .steps { display: flex; gap: 10px; margin: 0 0 22px; }
  .step { flex: 1; border-top: 3px solid #374151; padding-top: 8px; color: #6b7280; font-size: 12px; }
  .step.active { border-color: #22c55e; color: #86efac; }
  .step.done { border-color: #166534; color: #9ca3af; }
  .panel { background: #1f2937; border: 1px solid #374151; border-radius: 10px; padding: 20px; }
  pre { background: #0b1220; border: 1px solid #374151; border-radius: 8px; padding: 12px; overflow: auto; }
  code { font-family: ui-monospace, monospace; }
  footer { color: #6b7280; margin-top: 28px; font-size: 12px; }
  .muted { color: #9ca3af; font-size: 12px; }
  ul.summary { margin: 0; padding-left: 20px; }
  ul.summary li { margin: 4px 0; }
`

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>TradePost Companion</title>
<style>` + baseCSS + `</style>
</head>
<body>
<main>
  <div class="nav"><a href="/">Dashboard</a><a href="/settings">Settings</a></div>
  <h1>TradePost Companion</h1>
  <div class="sub">Crowd-sourced auction data for WoW Forever</div>
  {{if .Setup}}<div class="notice bad">Setup required: {{.Reason}} — <a href="/setup" style="color:#fca5a5">open the setup wizard</a>.</div>{{end}}

  <div class="grid">
    <div class="card"><div class="label">Server</div><div class="value" id="server">–</div></div>
    <div class="card"><div class="label">Market</div><div class="value" id="market">–</div></div>
    <div class="card"><div class="label">Last sync</div><div class="value" id="lastSync">–</div></div>
    <div class="card"><div class="label">Snapshot</div><div class="value" id="snapshot">–</div></div>
    <div class="card"><div class="label">Last upload</div><div class="value" id="lastUpload">–</div></div>
    <div class="card"><div class="label">Last download</div><div class="value" id="lastDownload">–</div></div>
    <div class="card" id="loginCard" style="display:none">
      <div class="label">Login code</div>
      <div class="value" id="loginCode" style="font-family: ui-monospace, monospace; letter-spacing: 2px; user-select: all;">–</div>
      <div class="sub" id="loginMeta"></div>
    </div>
  </div>

  <p class="actions">
    <button id="sync">Sync now</button>
    <a href="/settings" style="align-self:center">Settings</a>
    <span id="message" class="sub" style="align-self:center"></span>
  </p>

  <pre id="json">loading…</pre>
  <footer>Local dashboard. No data leaves this machine except scans sent to your server.</footer>
</main>
<script>
const TOKEN = "{{.Token}}";
const fmt = (ts) => ts ? new Date(ts * 1000).toLocaleString() : 'never';
async function refresh() {
  const res = await fetch('/api/status');
  const data = await res.json();
  const c = data.config || {};
  const s = data.status || {};
  document.getElementById('server').textContent = c.server || '–';
  document.getElementById('market').textContent = c.market || '–';
  document.getElementById('lastSync').textContent = fmt(s.last_sync);
  document.getElementById('snapshot').textContent = s.version ? ('v' + s.version + ' · ' + (s.items || 0) + ' items') : '–';
  document.getElementById('lastUpload').textContent = fmt(s.last_upload);
  document.getElementById('lastDownload').textContent = fmt(s.last_download);
  const loginCard = document.getElementById('loginCard');
  if (s.login_code && (!s.login_expires || s.login_expires * 1000 > Date.now())) {
    loginCard.style.display = '';
    document.getElementById('loginCode').textContent = s.login_code;
    document.getElementById('loginMeta').textContent =
      (s.login_character || '') + (s.login_expires ? ' · expires ' + fmt(s.login_expires) : '');
  } else {
    loginCard.style.display = 'none';
  }
  if (data.setup_required) {
    location.href = '/setup';
    return;
  }
  const msg = document.getElementById('message');
  msg.textContent = s.error ? s.error : (s.running ? 'syncing…' : '');
  msg.className = s.error ? 'bad' : 'sub';
  document.getElementById('json').textContent = JSON.stringify(data, null, 2);
}
document.getElementById('sync').addEventListener('click', async (e) => {
  e.target.disabled = true;
  await fetch('/api/sync', {method: 'POST', headers: {'X-TradePost-Token': TOKEN}});
  e.target.disabled = false;
  refresh();
});
refresh();
setInterval(refresh, 5000);
</script>
</body>
</html>
`))

const setupScript = `
const TOKEN = "{{.Token}}";
let validated = false;
let validationInfo = null;
const $ = (id) => document.getElementById(id);
function showStep(n) {
  for (let i = 1; i <= 3; i++) {
    $('step' + i).hidden = i !== n;
    const chip = $('chip' + i);
    chip.className = 'step' + (i === n ? ' active' : (i < n ? ' done' : ''));
  }
  $('stepNum').textContent = n;
  window.scrollTo(0, 0);
}
async function post(path, body) {
  const res = await fetch(path, {
    method: 'POST',
    headers: {'Content-Type': 'application/json', 'X-TradePost-Token': TOKEN},
    body: JSON.stringify(body || {})
  });
  let data = {};
  try { data = await res.json(); } catch (e) { data = {error: 'unexpected response'}; }
  return {status: res.status, data};
}
function setAlert(message, kind) {
  const el = $('alert');
  if (!message) { el.hidden = true; return; }
  el.hidden = false;
  el.className = 'notice ' + (kind || 'bad');
  el.textContent = message;
}
async function loadCandidates() {
  const res = await fetch('/api/detect');
  const data = await res.json();
  const select = $('candidateSelect');
  select.innerHTML = '';
  const list = data.detected && data.detected.length ? data.detected : [];
  if (!list.length) {
    const option = document.createElement('option');
    option.value = '';
    option.textContent = 'No WoW install detected automatically';
    select.appendChild(option);
    return;
  }
  for (const path of list) {
    const option = document.createElement('option');
    option.value = path;
    option.textContent = path;
    select.appendChild(option);
  }
  if (!$('wowDir').value) { $('wowDir').value = list[0]; validateWow(); }
}
async function validateWow() {
  const path = $('wowDir').value.trim();
  validated = false;
  validationInfo = null;
  $('next1').disabled = true;
  $('validation').hidden = false;
  $('validation').className = 'notice';
  $('validation').textContent = 'Checking…';
  const {data} = await post('/api/wow/validate', {path});
  if (!data.valid) {
    $('validation').className = 'notice bad';
    $('validation').textContent = data.error || 'Invalid game folder';
    return;
  }
  validated = true;
  validationInfo = data.info;
  $('validation').className = 'notice ok';
  $('validation').textContent = 'Valid install: ' + data.info.root + ' · ' + data.info.accounts.length + ' account folder(s) · writable';
  $('next1').disabled = false;
}
$('browse').addEventListener('click', async () => {
  setAlert('');
  const {data} = await post('/api/wow/pick', {title: 'Select the World of Warcraft folder'});
  if (!data.ok) {
    setAlert(data.error || 'Folder picker unavailable. Type the path manually.', 'bad');
    return;
  }
  $('wowDir').value = data.path;
  validateWow();
});
$('useCandidate').addEventListener('click', () => {
  const value = $('candidateSelect').value;
  if (value) { $('wowDir').value = value; validateWow(); }
});
$('validate').addEventListener('click', validateWow);
$('wowDir').addEventListener('input', () => {
  validated = false;
  $('next1').disabled = true;
});
$('next1').addEventListener('click', () => {
  if (!validated) return;
  setAlert('');
  showStep(2);
});
$('back2').addEventListener('click', () => showStep(1));
$('next2').addEventListener('click', () => {
  const server = $('server').value.trim();
  const market = $('market').value.trim();
  const rings = parseInt($('ringSlots').value, 10);
  if (!server || !market || !(rings >= 1 && rings <= 256)) {
    setAlert('Enter a server URL, market key, and a ring slot count between 1 and 256.', 'bad');
    return;
  }
  setAlert('');
  $('sumWow').textContent = validationInfo ? validationInfo.root : $('wowDir').value;
  $('sumServer').textContent = server;
  $('sumMarket').textContent = market;
  $('sumRings').textContent = String(rings);
  $('sumInterval').textContent = $('interval').value + 's';
  showStep(3);
});
$('back3').addEventListener('click', () => showStep(2));
$('finish').addEventListener('click', async () => {
  $('finish').disabled = true;
  $('finishMsg').className = 'notice';
  $('finishMsg').textContent = 'Enrolling and installing. This can take a moment…';
  const request = {
    wow_dir: $('wowDir').value.trim(),
    server: $('server').value.trim(),
    market: $('market').value.trim(),
    ring_slots: parseInt($('ringSlots').value, 10),
    interval_seconds: parseInt($('interval').value, 10),
    addon_source: $('addonSource').value.trim()
  };
  const {status, data} = await post('/api/setup', request);
  if (status !== 200 || !data.ok) {
    $('finish').disabled = false;
    $('finishMsg').className = 'notice bad';
    $('finishMsg').textContent = data.error || 'Setup failed';
    return;
  }
  $('finishMsg').className = 'notice ok';
  $('finishMsg').textContent = 'Setup complete. Starting the sync service…';
  location.href = '/';
});
loadCandidates();
{{if .Config.WowDir}}if ($('wowDir').value) { validateWow(); }{{end}}
`

var setupTemplate = template.Must(template.New("setup").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>TradePost setup</title>
<style>` + baseCSS + `</style>
</head>
<body>
<main>
  <div class="nav"><a href="/">Dashboard</a><a href="/settings">Settings</a></div>
  <h1>TradePost setup</h1>
  <div class="sub">Step <span id="stepNum">1</span> of 3 · local only, at 127.0.0.1</div>
  {{if .Setup}}<div class="notice warn">First-run setup is required. {{.Reason}}</div>{{end}}
  <div id="alert" class="notice bad" hidden></div>
  <div class="steps">
    <div class="step active" id="chip1">1 · Game folder</div>
    <div class="step" id="chip2">2 · Server &amp; market</div>
    <div class="step" id="chip3">3 · Install</div>
  </div>

  <section id="step1" class="panel">
    <h2>Find World of Warcraft</h2>
    <p class="muted">TradePost needs the game root (the folder containing <code>_forever_</code>).</p>
    <label for="candidateSelect">Detected installs</label>
    <div class="row">
      <div><select id="candidateSelect"></select></div>
      <button type="button" class="secondary" id="useCandidate">Use selected</button>
    </div>
    <label for="wowDir">Game folder</label>
    <div class="row">
      <div><input id="wowDir" value="{{.Config.WowDir}}" placeholder="C:\Program Files (x86)\World of Warcraft or /games/World of Warcraft" autocomplete="off"></div>
      <button type="button" class="secondary" id="browse">Browse…</button>
      <button type="button" class="secondary" id="validate">Validate</button>
    </div>
    <div id="validation" class="notice" hidden></div>
    <div class="actions">
      <button type="button" id="next1" disabled>Continue</button>
    </div>
  </section>

  <section id="step2" class="panel" hidden>
    <h2>Server &amp; market</h2>
    <label for="server">Server URL</label>
    <input id="server" value="{{.Config.Server}}" placeholder="https://tradepost.b4b0.de" autocomplete="off">
    <label for="market">Market key</label>
    <input id="market" value="{{.Config.Market}}" placeholder="eu:pve:horde" autocomplete="off">
    <div class="row">
      <div>
        <label for="ringSlots">Ring slots</label>
        <input id="ringSlots" type="number" min="1" max="256" value="{{.Config.RingSlots}}">
      </div>
      <div>
        <label for="interval">Sync interval (seconds)</label>
        <input id="interval" type="number" min="5" max="3600" value="{{.Config.IntervalSeconds}}">
      </div>
    </div>
    <label for="addonSource">Addon source folder (advanced)</label>
    <input id="addonSource" value="{{.Config.AddonSource}}" placeholder="auto-detect next to tradepost" autocomplete="off">
    <div class="actions">
      <button type="button" class="secondary" id="back2">Back</button>
      <button type="button" id="next2">Continue</button>
    </div>
  </section>

  <section id="step3" class="panel" hidden>
    <h2>Install and enroll</h2>
    <ul class="summary">
      <li>Game folder: <code id="sumWow"></code></li>
      <li>Server: <code id="sumServer"></code></li>
      <li>Market: <code id="sumMarket"></code></li>
      <li>Ring slots: <code id="sumRings"></code></li>
      <li>Sync interval: <code id="sumInterval"></code></li>
    </ul>
    <p class="muted">This enrolls this machine, copies the TradePost addon and ring slots, and saves the config. Close World of Warcraft first.</p>
    <div id="finishMsg" class="notice" hidden></div>
    <div class="actions">
      <button type="button" class="secondary" id="back3">Back</button>
      <button type="button" id="finish">Finish setup</button>
    </div>
  </section>
</main>
<script>` + setupScript + `</script>
</body>
</html>
`))

var settingsTemplate = template.Must(template.New("settings").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>TradePost settings</title>
<style>` + baseCSS + `</style>
</head>
<body>
<main>
  <div class="nav"><a href="/">Dashboard</a><a href="/settings">Settings</a></div>
  <h1>Settings</h1>
  <div class="sub">Config: <code>{{.ConfigPath}}</code></div>
  {{if .Setup}}<div class="notice bad">Setup is incomplete: {{.Reason}}<br><a href="/setup" style="color:#fca5a5">Open the setup wizard</a> to enroll this machine.</div>{{end}}
  <div id="message" class="notice" hidden></div>

  <div class="panel">
    <h2>Game folder</h2>
    <label for="wowDir">WoW root containing <code>_forever_</code></label>
    <input id="wowDir" value="{{.Config.WowDir}}" autocomplete="off">
    <div class="actions">
      <button type="button" class="secondary" id="validate">Validate folder</button>
    </div>
    <div id="validation" class="notice" hidden></div>
  </div>

  <div class="panel" style="margin-top:16px">
    <h2>Sync settings</h2>
    <label for="server">Server URL</label>
    <input id="server" value="{{.Config.Server}}" autocomplete="off">
    <label for="market">Market key</label>
    <input id="market" value="{{.Config.Market}}" autocomplete="off">
    <div class="row">
      <div>
        <label for="ringSlots">Ring slots</label>
        <input id="ringSlots" type="number" min="1" max="256" value="{{.Config.RingSlots}}">
      </div>
      <div>
        <label for="interval">Sync interval (seconds)</label>
        <input id="interval" type="number" min="5" max="3600" value="{{.Config.IntervalSeconds}}">
      </div>
    </div>
    <label for="addonSource">Addon source folder (advanced)</label>
    <input id="addonSource" value="{{.Config.AddonSource}}" autocomplete="off">
    <div class="actions">
      <button type="button" id="save">Save settings</button>
      <button type="button" class="secondary" id="install">Re-run install</button>
      <a href="/setup" style="align-self:center">Run setup wizard again</a>
    </div>
  </div>

  <footer>Changes restart the sync service with the new config. Re-run the setup wizard to enroll a new machine.</footer>
</main>
<script>
const TOKEN = "{{.Token}}";
const $ = (id) => document.getElementById(id);
function setMessage(message, kind) {
  const el = $('message');
  el.hidden = !message;
  el.className = 'notice ' + (kind || '');
  el.textContent = message || '';
}
async function post(path, body) {
  const res = await fetch(path, {
    method: 'POST',
    headers: {'Content-Type': 'application/json', 'X-TradePost-Token': TOKEN},
    body: JSON.stringify(body || {})
  });
  let data = {};
  try { data = await res.json(); } catch (e) { data = {error: 'unexpected response'}; }
  return {status: res.status, data};
}
function request() {
  return {
    wow_dir: $('wowDir').value.trim(),
    server: $('server').value.trim(),
    market: $('market').value.trim(),
    ring_slots: parseInt($('ringSlots').value, 10),
    interval_seconds: parseInt($('interval').value, 10),
    addon_source: $('addonSource').value.trim()
  };
}
$('validate').addEventListener('click', async () => {
  $('validation').hidden = false;
  $('validation').className = 'notice';
  $('validation').textContent = 'Checking…';
  const {data} = await post('/api/wow/validate', {path: $('wowDir').value.trim()});
  if (!data.valid) {
    $('validation').className = 'notice bad';
    $('validation').textContent = data.error || 'Invalid game folder';
    return;
  }
  $('validation').className = 'notice ok';
  $('validation').textContent = 'Valid install: ' + data.info.root + ' · ' + data.info.accounts.length + ' account folder(s) · writable';
});
$('save').addEventListener('click', async (e) => {
  e.target.disabled = true;
  const {status, data} = await post('/api/settings', request());
  e.target.disabled = false;
  if (status !== 200) { setMessage(data.error || 'Could not save settings', 'bad'); return; }
  setMessage('Saved. The sync service was restarted with the new settings.', 'ok');
  if (data.config && data.config.wow_dir) $('wowDir').value = data.config.wow_dir;
});
$('install').addEventListener('click', async (e) => {
  e.target.disabled = true;
  setMessage('Installing…', '');
  const {status, data} = await post('/api/settings/install', {});
  e.target.disabled = false;
  if (status !== 200) { setMessage(data.error || 'Install failed', 'bad'); return; }
  setMessage('Addon, data, ring, and auth slots installed.', 'ok');
});
</script>
</body>
</html>
`))
