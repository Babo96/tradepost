package api

import (
	"bytes"
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
	"strconv"
	"strings"
	"time"

	"github.com/babo96/tradepost/server/internal/store"
)

var (
	marketRE = regexp.MustCompile(`^[a-z]{2}:[a-z0-9-]+:(alliance|horde|neutral)$`)
	itemRE   = regexp.MustCompile(`^(i|g|p):[0-9]+(:[0-9]+)*$`)
	statRE   = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
)

const (
	maxBodyBytes       = 16 << 20
	maxDecompressed    = 64 << 20
	maxScansPerRequest = 10
	maxItemsPerScan    = 100000
	maxItemsTotal      = 300000
	maxMetaPerRequest  = 20000
	maxQueryLength     = 100
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
	s.mux.HandleFunc("GET /v1/live", s.handleLive)
	s.mux.HandleFunc("GET /v1/snapshot/{market}", s.handleSnapshot)
	s.mux.HandleFunc("GET /v1/snapshot/{market}/data-addon.zip", s.handleDataAddon)
	s.mux.HandleFunc("GET /v1/items", s.handleItems)
	s.mux.HandleFunc("GET /v1/items/missing-details", s.handleMissingDetails)
	s.mux.HandleFunc("GET /v1/item/detail", s.handleItemDetail)
	s.mux.HandleFunc("GET /v1/item/history", s.handleItemHistory)
	s.mux.HandleFunc("GET /v1/item/markets", s.handleItemMarkets)
	s.mux.HandleFunc("POST /v1/auth/code", s.handleAuthCode)
	s.mux.HandleFunc("POST /v1/auth/redeem", s.handleAuthRedeem)
	s.mux.HandleFunc("GET /v1/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("POST /v1/auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("GET /v1/watch", s.handleWatchGet)
	s.mux.HandleFunc("PUT /v1/watch", s.handleWatchPut)
	s.mux.Handle("GET /", s.webHandler())
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
	Items []ingestMeta `json:"items"`
}

type ingestMeta struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	Quality       int             `json:"quality"`
	ClassID       int             `json:"class_id"`
	SubclassID    int             `json:"subclass_id"`
	Icon          int64           `json:"icon"`
	ItemLevel     *int            `json:"item_level"`
	RequiredLevel *int            `json:"required_level"`
	BindType      *int            `json:"bind_type"`
	InventoryType *int            `json:"inventory_type"`
	EquipLoc      *string         `json:"equip_loc"`
	Armor         *int            `json:"armor"`
	DamageMin     *int            `json:"damage_min"`
	DamageMax     *int            `json:"damage_max"`
	Delay         *int            `json:"delay"`
	VendorPrice   *int64          `json:"vendor_price"`
	MaxStack      *int            `json:"max_stack"`
	UniqueItem    *bool           `json:"unique_item"`
	SetID         *int            `json:"set_id"`
	Stats         json.RawMessage `json:"stats"`
	Details       json.RawMessage `json:"details"`
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
	Queued   int `json:"queued"`
	Rejected int `json:"rejected"`
	Meta     int `json:"meta"`
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
	if len(req.Scans) > maxScansPerRequest {
		writeError(w, http.StatusBadRequest, "invalid scan count")
		return
	}
	if len(req.Scans) == 0 && len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "empty payload")
		return
	}
	if len(req.Items) > maxMetaPerRequest {
		writeError(w, http.StatusRequestEntityTooLarge, "too many item metadata entries")
		return
	}

	meta := make([]store.ItemMeta, 0, len(req.Items))
	for _, item := range req.Items {
		if item.ID <= 0 || item.Name == "" || len(item.Name) > 200 {
			writeError(w, http.StatusBadRequest, "invalid item metadata")
			return
		}
		if item.Quality < 0 || item.Quality > 10 || item.ClassID < 0 || item.SubclassID < 0 || item.Icon < 0 {
			writeError(w, http.StatusBadRequest, "invalid item metadata values")
			return
		}
		if !validOptionalInts(item.ItemLevel, item.RequiredLevel, item.BindType, item.InventoryType,
			item.Armor, item.DamageMin, item.DamageMax, item.Delay, item.MaxStack, item.SetID) ||
			(item.VendorPrice != nil && *item.VendorPrice < 0) {
			writeError(w, http.StatusBadRequest, "invalid item metadata values")
			return
		}
		if !validJSON(item.Stats) || !validJSON(item.Details) {
			writeError(w, http.StatusBadRequest, "invalid item metadata json")
			return
		}
		meta = append(meta, store.ItemMeta{
			ID:            item.ID,
			Name:          item.Name,
			Quality:       item.Quality,
			ClassID:       item.ClassID,
			SubclassID:    item.SubclassID,
			Icon:          item.Icon,
			ItemLevel:     item.ItemLevel,
			RequiredLevel: item.RequiredLevel,
			BindType:      item.BindType,
			InventoryType: item.InventoryType,
			EquipLoc:      item.EquipLoc,
			Armor:         item.Armor,
			DamageMin:     item.DamageMin,
			DamageMax:     item.DamageMax,
			Delay:         item.Delay,
			VendorPrice:   item.VendorPrice,
			MaxStack:      item.MaxStack,
			UniqueItem:    item.UniqueItem,
			SetID:         item.SetID,
			Stats:         normalizeJSON(item.Stats),
			Details:       normalizeJSON(item.Details),
		})
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

	if err := s.store.UpsertItems(r.Context(), meta); err != nil {
		writeError(w, http.StatusInternalServerError, "metadata ingest failed")
		return
	}

	result := ingestResponse{Meta: len(meta)}
	for _, scan := range parsed {
		payload, err := encodePayload(scan)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ingest failed")
			return
		}
		queued, err := s.store.EnqueueIngest(r.Context(), client.ID, store.IngestJob{
			Market:    scan.Market,
			ScannedAt: scan.ScannedAt,
			Checksum:  scan.Checksum,
			Payload:   payload,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ingest failed")
			return
		}
		if queued {
			result.Queued++
		} else {
			result.Rejected++
		}
	}
	writeJSON(w, http.StatusAccepted, result)
}

func encodePayload(scan store.Scan) ([]byte, error) {
	body, err := json.Marshal(store.ScanPayload{
		Market:        scan.Market,
		ScannedAt:     scan.ScannedAt.Unix(),
		ClientVersion: scan.ClientVersion,
		Items:         scan.Items,
	})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(body); err != nil {
		gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func validOptionalInts(values ...*int) bool {
	for _, value := range values {
		if value != nil && *value < 0 {
			return false
		}
	}
	return true
}

func validJSON(raw json.RawMessage) bool {
	return len(raw) == 0 || json.Valid(raw)
}

func normalizeJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

const liveWindow = 10 * time.Minute

// handleLive reports how many companions requested a login code recently.
// The companion refreshes a code every 8 minutes, so this is a cheap heartbeat.
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.CountLiveClients(r.Context(), time.Now().Add(-liveWindow))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "live count failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"live_clients":   count,
		"window_seconds": int(liveWindow.Seconds()),
	})
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

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	market := query.Get("market")
	if !marketRE.MatchString(market) {
		writeError(w, http.StatusBadRequest, "invalid market")
		return
	}
	search := strings.TrimSpace(query.Get("q"))
	if len(search) > maxQueryLength {
		writeError(w, http.StatusBadRequest, "query too long")
		return
	}

	stats, ok := parseStatFilters(query.Get("stats"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid stat filter")
		return
	}
	statKey := strings.TrimSpace(query.Get("stat"))
	if statKey != "" && !statRE.MatchString(statKey) {
		writeError(w, http.StatusBadRequest, "invalid stat")
		return
	}
	minStat, ok := parseFloatPtr(query.Get("min_stat"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid stat range")
		return
	}
	maxStat, ok := parseFloatPtr(query.Get("max_stat"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid stat range")
		return
	}
	if statKey != "" {
		stats = append(stats, store.StatFilter{Key: statKey, Min: minStat, Max: maxStat})
	}

	filters := store.SearchQuery{
		Query:         search,
		Qualities:     parseCSVInts(query.Get("quality")),
		ClassID:       parseIntPtr(query.Get("class_id")),
		SubclassID:    parseIntPtr(query.Get("subclass_id")),
		InventoryType: parseIntPtr(query.Get("inventory_type")),
		MinIlvl:       parseIntPtr(query.Get("min_ilvl")),
		MaxIlvl:       parseIntPtr(query.Get("max_ilvl")),
		MinReqLevel:   parseIntPtr(query.Get("min_req_level")),
		MaxReqLevel:   parseIntPtr(query.Get("max_req_level")),
		MinPrice:      parseInt64Ptr(query.Get("min_price")),
		MaxPrice:      parseInt64Ptr(query.Get("max_price")),
		InStock:       query.Get("in_stock") == "1",
		Stats:         stats,
		HasStats:      query.Get("has_stats") == "1",
		Sort:          query.Get("sort"),
		Page:          parsePositive(query.Get("page"), 1),
		Limit:         parseLimit(query.Get("limit"), 50, 200),
	}

	page, err := s.store.SearchItems(r.Context(), market, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	items := page.Items
	if items == nil {
		items = []store.SearchResult{}
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, map[string]any{
		"market": market,
		"query":  search,
		"items":  items,
		"total":  page.Total,
		"page":   filters.Page,
		"limit":  filters.Limit,
	})
}

func (s *Server) handleMissingDetails(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	market := query.Get("market")
	if !marketRE.MatchString(market) {
		writeError(w, http.StatusBadRequest, "invalid market")
		return
	}
	limit := parseLimit(query.Get("limit"), 500, 2000)
	force := query.Get("force") == "1"
	ids, err := s.store.MissingItemDetails(r.Context(), market, force, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "missing details failed")
		return
	}
	if ids == nil {
		ids = []int64{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": ids,
		"count": len(ids),
	})
}

func (s *Server) handleItemDetail(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	market := query.Get("market")
	key := query.Get("key")
	if !marketRE.MatchString(market) {
		writeError(w, http.StatusBadRequest, "invalid market")
		return
	}
	if !itemRE.MatchString(key) {
		writeError(w, http.StatusBadRequest, "invalid item key")
		return
	}
	detail, err := s.store.ItemDetail(r.Context(), market, key)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "item detail failed")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleItemHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	market := query.Get("market")
	key := query.Get("key")
	if !marketRE.MatchString(market) {
		writeError(w, http.StatusBadRequest, "invalid market")
		return
	}
	if !itemRE.MatchString(key) {
		writeError(w, http.StatusBadRequest, "invalid item key")
		return
	}
	rangeKey := query.Get("range")
	if rangeKey == "" {
		rangeKey = "all"
	}
	var resolution string
	var since time.Time
	var limit int
	switch rangeKey {
	case "12h":
		resolution = "1m"
		since = time.Now().Add(-12 * time.Hour)
		limit = parseLimit(query.Get("limit"), 1000, 5000)
	case "7d":
		resolution = "15m"
		since = time.Now().Add(-7 * 24 * time.Hour)
		limit = parseLimit(query.Get("limit"), 2000, 10000)
	case "all":
		resolution = "hour"
		limit = parseLimit(query.Get("limit"), 5000, 20000)
	default:
		writeError(w, http.StatusBadRequest, "invalid range")
		return
	}
	history, err := s.store.HistoryPoints(r.Context(), market, key, resolution, since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "history failed")
		return
	}
	history.Range = rangeKey
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, history)
}

func (s *Server) handleItemMarkets(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if !itemRE.MatchString(key) {
		writeError(w, http.StatusBadRequest, "invalid item key")
		return
	}
	prices, err := s.store.ItemMarkets(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "markets failed")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, map[string]any{
		"key":     key,
		"markets": prices,
	})
}

func parseLimit(raw string, fallback, max int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func parsePositive(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func parseIntPtr(raw string) *int {
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return nil
	}
	return &value
}

func parseInt64Ptr(raw string) *int64 {
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return nil
	}
	return &value
}

func parseFloatPtr(raw string) (*float64, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, true
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, false
	}
	return &value, true
}

// parseStatFilters parses a comma-separated list of stat filter segments of
// the form KEY, KEY:MIN, KEY::MAX, or KEY:MIN:MAX.
func parseStatFilters(raw string) ([]store.StatFilter, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	segments := strings.Split(raw, ",")
	filters := make([]store.StatFilter, 0, len(segments))
	for _, segment := range segments {
		parts := strings.Split(strings.TrimSpace(segment), ":")
		if len(parts) == 0 || len(parts) > 3 {
			return nil, false
		}
		key := strings.TrimSpace(parts[0])
		if !statRE.MatchString(key) {
			return nil, false
		}
		filter := store.StatFilter{Key: key}
		if len(parts) >= 2 {
			min, ok := parseFloatPtr(parts[1])
			if !ok {
				return nil, false
			}
			filter.Min = min
		}
		if len(parts) == 3 {
			max, ok := parseFloatPtr(parts[2])
			if !ok {
				return nil, false
			}
			filter.Max = max
		}
		filters = append(filters, filter)
	}
	return filters, true
}

func parseCSVInts(raw string) []int {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var result []int
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && value >= 0 {
			result = append(result, value)
		}
	}
	return result
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
