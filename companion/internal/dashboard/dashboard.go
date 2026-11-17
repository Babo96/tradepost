package dashboard

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"

	"babo-forever/companion/internal/config"
	syncsvc "babo-forever/companion/internal/sync"
)

type Server struct {
	service *syncsvc.Service
	cfg     config.Config
}

func New(service *syncsvc.Service, cfg config.Config) *Server {
	return &Server{service: service, cfg: cfg}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handlePage)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/sync", s.handleSync)
	return mux
}

func (s *Server) Start(addr string) *http.Server {
	server := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		log.Printf("dashboard: http://%s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("dashboard: %v", err)
		}
	}()
	return server
}

type statusResponse struct {
	Config map[string]any `json:"config"`
	Status any            `json:"status"`
}

func (s *Server) statusPayload() statusResponse {
	cfg := map[string]any{
		"server":     s.cfg.Server,
		"wow_dir":    s.cfg.WowDir,
		"market":     s.cfg.Market,
		"ring_slots": s.cfg.RingSlots,
		"dashboard":  s.cfg.DashboardAddr,
		"client_id":  s.cfg.ClientID,
		"has_token":  s.cfg.Token != "",
	}
	return statusResponse{Config: cfg, Status: s.service.Status()}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.statusPayload())
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	err := s.service.Sync(ctx)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":  err.Error(),
			"status": s.service.Status(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": s.service.Status()})
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTemplate.Execute(w, nil)
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Babo Forever Companion</title>
<style>
  :root { color-scheme: dark; }
  body { margin: 0; font: 14px/1.5 system-ui, sans-serif; background: #111827; color: #e5e7eb; }
  main { max-width: 860px; margin: 40px auto; padding: 0 20px; }
  h1 { font-size: 22px; margin: 0 0 4px; color: #86efac; }
  .sub { color: #9ca3af; margin-bottom: 24px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; }
  .card { background: #1f2937; border: 1px solid #374151; border-radius: 10px; padding: 14px 16px; }
  .label { color: #9ca3af; font-size: 12px; text-transform: uppercase; letter-spacing: .05em; }
  .value { font-size: 18px; margin-top: 4px; word-break: break-word; }
  .ok { color: #86efac; } .bad { color: #fca5a5; } .warn { color: #fcd34d; }
  button { background: #16a34a; color: white; border: 0; border-radius: 8px; padding: 10px 18px; font-weight: 600; cursor: pointer; }
  button:disabled { opacity: .5; cursor: default; }
  pre { background: #0b1220; border: 1px solid #374151; border-radius: 8px; padding: 12px; overflow: auto; }
  footer { color: #6b7280; margin-top: 28px; font-size: 12px; }
</style>
</head>
<body>
<main>
  <h1>Babo Forever Companion</h1>
  <div class="sub">Crowd-sourced auction data for WoW Forever</div>

  <div class="grid">
    <div class="card"><div class="label">Server</div><div class="value" id="server">–</div></div>
    <div class="card"><div class="label">Market</div><div class="value" id="market">–</div></div>
    <div class="card"><div class="label">Last sync</div><div class="value" id="lastSync">–</div></div>
    <div class="card"><div class="label">Snapshot</div><div class="value" id="snapshot">–</div></div>
    <div class="card"><div class="label">Last upload</div><div class="value" id="lastUpload">–</div></div>
    <div class="card"><div class="label">Last download</div><div class="value" id="lastDownload">–</div></div>
  </div>

  <p>
    <button id="sync">Sync now</button>
    <span id="message" class="sub"></span>
  </p>

  <pre id="json">loading…</pre>
  <footer>Local dashboard. No data leaves this machine except scans sent to your server.</footer>
</main>
<script>
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
  const msg = document.getElementById('message');
  msg.textContent = s.error ? s.error : (s.running ? 'syncing…' : '');
  msg.className = s.error ? 'bad' : 'sub';
  document.getElementById('json').textContent = JSON.stringify(data, null, 2);
}
document.getElementById('sync').addEventListener('click', async (e) => {
  e.target.disabled = true;
  await fetch('/api/sync', {method: 'POST'});
  e.target.disabled = false;
  refresh();
});
refresh();
setInterval(refresh, 5000);
</script>
</body>
</html>
`))
