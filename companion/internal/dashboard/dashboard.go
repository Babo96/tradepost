package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/babo96/tradepost/companion/internal/config"
	"github.com/babo96/tradepost/companion/internal/picker"
	syncsvc "github.com/babo96/tradepost/companion/internal/sync"
	"github.com/babo96/tradepost/companion/internal/update"
	"github.com/babo96/tradepost/companion/internal/wow"
)

// Backend is the runtime the dashboard drives. It is implemented by internal/app.
type Backend interface {
	Config() config.Config
	ConfigPath() string
	SetupRequired() bool
	SetupReason() string
	Status() syncsvc.Status
	Detect() []string
	Candidates() []string
	ValidateWow(dir string) (wow.InstallInfo, error)
	PickFolder(title string) (string, error)
	CompleteSetup(context.Context, config.SetupRequest) (config.Config, error)
	UpdateSettings(context.Context, config.SetupRequest) (config.Config, error)
	Reinstall(context.Context) error
	Sync(context.Context) error
	UpdateStatus() update.Info
	CheckUpdate(context.Context) (update.Info, error)
	ApplyUpdate(context.Context) (update.Info, error)
}

type Server struct {
	backend Backend
	token   string
}

func New(backend Backend) *Server {
	return &Server{backend: backend, token: newToken()}
}

func newToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "tradepost-local-dashboard"
	}
	return hex.EncodeToString(buf)
}

// ListenAddr forces the dashboard onto loopback regardless of the configured
// host, so setup and settings are never exposed on the network.
func ListenAddr(addr string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil || port == "" {
		return config.DefaultAddr
	}
	return net.JoinHostPort("127.0.0.1", port)
}

// SetupURL returns the loopback URL the browser should open.
func SetupURL(addr string) string {
	_, port, err := net.SplitHostPort(ListenAddr(addr))
	if err != nil || port == "" {
		port = "8787"
	}
	return "http://127.0.0.1:" + port + "/setup"
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handlePage)
	mux.HandleFunc("GET /setup", s.handleSetup)
	mux.HandleFunc("GET /settings", s.handleSettings)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/detect", s.handleDetect)
	mux.HandleFunc("POST /api/wow/validate", s.handleValidateWow)
	mux.HandleFunc("POST /api/wow/pick", s.handlePickFolder)
	mux.HandleFunc("POST /api/setup", s.handleSetupSubmit)
	mux.HandleFunc("POST /api/settings", s.handleSettingsSubmit)
	mux.HandleFunc("POST /api/settings/install", s.handleReinstall)
	mux.HandleFunc("POST /api/sync", s.handleSync)
	mux.HandleFunc("POST /api/update/check", s.handleUpdateCheck)
	mux.HandleFunc("POST /api/update/apply", s.handleUpdateApply)
	return s.secure(mux)
}

func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodPost {
			// Require the per-process token embedded in the served page. A
			// cross-origin form cannot read it, which prevents CSRF.
			if r.Header.Get("X-TradePost-Token") != s.token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || parsed.Host != r.Host {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Start(addr string) *http.Server {
	server := &http.Server{
		Addr:              ListenAddr(addr),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      10 * time.Minute,
	}
	go func() {
		log.Printf("dashboard: http://%s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("dashboard: %v", err)
		}
	}()
	return server
}

type pageData struct {
	Token      string
	Config     config.Config
	ConfigPath string
	Setup      bool
	Reason     string
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if s.backend.SetupRequired() {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTemplate.Execute(w, pageData{Token: s.token, Config: s.backend.Config(), ConfigPath: s.backend.ConfigPath()})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = setupTemplate.Execute(w, pageData{
		Token:      s.token,
		Config:     s.backend.Config(),
		ConfigPath: s.backend.ConfigPath(),
		Setup:      s.backend.SetupRequired(),
		Reason:     s.backend.SetupReason(),
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = settingsTemplate.Execute(w, pageData{
		Token:      s.token,
		Config:     s.backend.Config(),
		ConfigPath: s.backend.ConfigPath(),
		Setup:      s.backend.SetupRequired(),
		Reason:     s.backend.SetupReason(),
	})
}

type statusResponse struct {
	Config map[string]any `json:"config"`
	Status syncsvc.Status `json:"status"`
	Setup  bool           `json:"setup_required"`
	Reason string         `json:"setup_reason,omitempty"`
	Update update.Info    `json:"update"`
}

func (s *Server) statusPayload() statusResponse {
	cfg := s.backend.Config()
	return statusResponse{
		Config: map[string]any{
			"server":           cfg.Server,
			"wow_dir":          cfg.WowDir,
			"market":           cfg.Market,
			"ring_slots":       cfg.RingSlots,
			"interval_seconds": cfg.IntervalSeconds,
			"dashboard":        cfg.DashboardAddr,
			"client_id":        cfg.ClientID,
			"has_token":        cfg.Token != "",
			"config_path":      s.backend.ConfigPath(),
			"missing":          cfg.Missing(),
		},
		Status: s.backend.Status(),
		Setup:  s.backend.SetupRequired(),
		Reason: s.backend.SetupReason(),
		Update: s.backend.UpdateStatus(),
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.statusPayload())
}

func (s *Server) handleDetect(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"candidates": s.backend.Candidates(),
		"detected":   s.backend.Detect(),
		"current":    s.backend.Config().WowDir,
	})
}

func (s *Server) handleValidateWow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	info, err := s.backend.ValidateWow(body.Path)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "info": info})
}

func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	_ = decodeJSON(r, &body)
	if body.Title == "" {
		body.Title = "Select the World of Warcraft folder"
	}
	path, err := s.backend.PickFolder(body.Title)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          false,
			"error":       err.Error(),
			"unavailable": errors.Is(err, picker.ErrUnavailable),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path})
}

func (s *Server) handleSetupSubmit(w http.ResponseWriter, r *http.Request) {
	var req config.SetupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	cfg, err := s.backend.CompleteSetup(ctx, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"wow_dir":   cfg.WowDir,
		"client_id": cfg.ClientID,
		"server":    cfg.Server,
	})
}

func (s *Server) handleSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	var req config.SetupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cfg, err := s.backend.UpdateSettings(ctx, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": cfg})
}

func (s *Server) handleReinstall(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := s.backend.Reinstall(ctx); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	err := s.backend.Sync(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":  err.Error(),
			"status": s.backend.Status(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": s.backend.Status()})
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	info, err := s.backend.CheckUpdate(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "update": info})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	info, err := s.backend.ApplyUpdate(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "update": info})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(r *http.Request, value any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}
