package api

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"babo-forever/server/internal/store"
)

var (
	marketRE = regexp.MustCompile(`^[a-z]{2}:[a-z0-9-]+:(alliance|horde|neutral)$`)
	itemRE   = regexp.MustCompile(`^(i|g|p):[0-9]+(:[0-9]+)*$`)
)

const (
	maxBodyBytes       = 16 << 20
	maxDecompressed    = 64 << 20
	maxScansPerRequest = 10
	maxItemsPerScan    = 100000
	maxItemsTotal      = 300000
)

type Server struct {
	store store.Store
	mux   *http.ServeMux
}

func New(st store.Store) *Server {
	s := &Server{
		store: st,
		mux:   http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /v1/enroll", s.handleEnroll)
	s.mux.HandleFunc("POST /v1/ingest", s.handleIngest)
	s.mux.HandleFunc("GET /v1/meta", s.handleMeta)
	s.mux.HandleFunc("GET /v1/snapshot/{market}", s.handleSnapshot)
	s.mux.HandleFunc("GET /v1/snapshot/{market}/data-addon.zip", s.handleDataAddon)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	id, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "id generation failed")
		return
	}
	token, err := randomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	sum := sha256.Sum256([]byte(token))
	if err := s.store.CreateClient(r.Context(), id, sum[:]); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create client")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"client_id": id,
		"token":     token,
	})
}

type ingestRequest struct {
	V     int          `json:"v"`
	Scans []ingestScan `json:"scans"`
}

type ingestScan struct {
	Market        string       `json:"market"`
	ScannedAt     int64        `json:"scanned_at"`
	ClientVersion string       `json:"client_version"`
	Items         []ingestItem `json:"items"`
}

type ingestItem struct {
	K string `json:"k"`
	P int64  `json:"p"`
	Q int64  `json:"q"`
	N int32  `json:"n"`
}

type ingestResponse struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
	Items    int `json:"items"`
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	client, err := s.authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req ingestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.V != 1 {
		writeError(w, http.StatusBadRequest, "unsupported payload version")
		return
	}
	if len(req.Scans) == 0 || len(req.Scans) > maxScansPerRequest {
		writeError(w, http.StatusBadRequest, "invalid scan count")
		return
	}

	now := time.Now()
	totalItems := 0
	parsed := make([]store.Scan, 0, len(req.Scans))
	for _, scan := range req.Scans {
		if !marketRE.MatchString(scan.Market) {
			writeError(w, http.StatusBadRequest, "invalid market")
			return
		}
		scannedAt := time.Unix(scan.ScannedAt, 0)
		if scannedAt.Before(now.Add(-7*24*time.Hour)) || scannedAt.After(now.Add(5*time.Minute)) {
			writeError(w, http.StatusBadRequest, "scanned_at out of range")
			return
		}
		if len(scan.Items) == 0 || len(scan.Items) > maxItemsPerScan {
			writeError(w, http.StatusBadRequest, "invalid item count")
			return
		}
		totalItems += len(scan.Items)
		if totalItems > maxItemsTotal {
			writeError(w, http.StatusRequestEntityTooLarge, "too many items")
			return
		}

		items := make([]store.Item, 0, len(scan.Items))
		for _, item := range scan.Items {
			if !itemRE.MatchString(item.K) {
				writeError(w, http.StatusBadRequest, "invalid item key")
				return
			}
			if item.P <= 0 || item.Q < 0 || item.N < 0 {
				writeError(w, http.StatusBadRequest, "invalid item values")
				return
			}
			items = append(items, store.Item{
				Key:      item.K,
				Price:    item.P,
				Quantity: item.Q,
				Listings: item.N,
			})
		}

		sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
		for i := 1; i < len(items); i++ {
			if items[i].Key == items[i-1].Key {
				writeError(w, http.StatusBadRequest, "duplicate item key")
				return
			}
		}

		parsed = append(parsed, store.Scan{
			Market:        scan.Market,
			ScannedAt:     scannedAt,
			ClientVersion: scan.ClientVersion,
			Checksum:      checksum(scan.Market, scan.ScannedAt, items),
			Items:         items,
		})
	}

	result := ingestResponse{}
	for _, scan := range parsed {
		ingested, err := s.store.Ingest(r.Context(), client.ID, scan)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ingest failed")
			return
		}
		result.Accepted += ingested.Accepted
		result.Rejected += ingested.Rejected
		result.Items += ingested.Items
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	meta, err := s.store.Meta(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "meta failed")
		return
	}
	if meta == nil {
		meta = []store.MarketMeta{}
	}
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	market := r.PathValue("market")
	if !marketRE.MatchString(market) {
		writeError(w, http.StatusBadRequest, "invalid market")
		return
	}

	snapshot, err := s.store.Snapshot(r.Context(), market)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "market not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "snapshot failed")
		return
	}

	etag := fmt.Sprintf(`"m%s-v%d"`, market, snapshot.Version)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Header().Set("ETag", etag)

	gz := gzip.NewWriter(w)
	defer gz.Close()
	_ = json.NewEncoder(gz).Encode(snapshot)
}

func (s *Server) authenticate(r *http.Request) (store.Client, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return store.Client{}, errors.New("missing bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		return store.Client{}, errors.New("empty token")
	}
	sum := sha256.Sum256([]byte(token))
	client, err := s.store.ClientByTokenHash(r.Context(), sum[:])
	if err != nil {
		return store.Client{}, err
	}
	if client.Status != "active" {
		return store.Client{}, errors.New("client disabled")
	}
	return client, nil
}

func readBody(r *http.Request) ([]byte, error) {
	body := io.LimitReader(r.Body, maxBodyBytes)
	var reader io.Reader = body
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(body)
		if err != nil {
			return nil, errors.New("invalid gzip body")
		}
		defer gz.Close()
		reader = io.LimitReader(gz, maxDecompressed)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.New("could not read body")
	}
	return data, nil
}

func checksum(market string, scannedAt int64, items []store.Item) []byte {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%d|", market, scannedAt)
	for _, item := range items {
		fmt.Fprintf(h, "%s,%d,%d,%d;", item.Key, item.Price, item.Quantity, item.Listings)
	}
	return h.Sum(nil)
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
