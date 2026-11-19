package api

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/babo96/tradepost/server/internal/store"
)

type testClient struct {
	ID    string `json:"client_id"`
	Token string `json:"token"`
}

func newTestServer(t *testing.T) (*Server, *MemStore) {
	t.Helper()
	st := NewMemStore()
	return New(st), st
}

// processQueued drains the async ingest queue, mirroring the production worker.
func processQueued(t *testing.T, st *MemStore) {
	t.Helper()
	ctx := context.Background()
	for {
		jobs, err := st.ClaimIngestJobs(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) == 0 {
			return
		}
		for _, job := range jobs {
			if err := st.ProcessIngestJob(ctx, job); err != nil {
				t.Fatalf("process job %d: %v", job.ID, err)
			}
			if err := st.CompleteIngestJob(ctx, job.ID); err != nil {
				t.Fatalf("complete job %d: %v", job.ID, err)
			}
		}
	}
}

func enroll(t *testing.T, srv *Server) testClient {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/enroll", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll status %d: %s", rec.Code, rec.Body.String())
	}
	var client testClient
	if err := json.Unmarshal(rec.Body.Bytes(), &client); err != nil {
		t.Fatal(err)
	}
	if client.ID == "" || client.Token == "" {
		t.Fatal("empty client")
	}
	return client
}

func ingestBody() []byte {
	return []byte(fmt.Sprintf(`{
	  "v": 1,
	  "scans": [{
	    "market": "eu:pve:horde",
	    "scanned_at": %d,
	    "client_version": "0.1.0",
	    "items": [
	      {"k": "i:2589", "p": 1234, "q": 50, "n": 2},
	      {"k": "g:12345:70", "p": 4200, "q": 1, "n": 1}
	    ]
	  }]
	}`, time.Now().Unix()))
}

func TestEnrollIngestSnapshot(t *testing.T) {
	srv, st := newTestServer(t)
	client := enroll(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(ingestBody()))
	req.Header.Set("Authorization", "Bearer "+client.Token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest status %d: %s", rec.Code, rec.Body.String())
	}
	var result ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Queued != 1 || result.Rejected != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	// Ingest is async: no price rows exist until the worker drains the queue.
	snapReq := httptest.NewRequest(http.MethodGet, "/v1/snapshot/eu:pve:horde", nil)
	snapRec := httptest.NewRecorder()
	srv.ServeHTTP(snapRec, snapReq)
	if snapRec.Code != http.StatusNotFound {
		t.Fatalf("expected snapshot 404 before worker, got %d", snapRec.Code)
	}

	processQueued(t, st)

	metaReq := httptest.NewRequest(http.MethodGet, "/v1/meta", nil)
	metaRec := httptest.NewRecorder()
	srv.ServeHTTP(metaRec, metaReq)
	if metaRec.Code != http.StatusOK {
		t.Fatalf("meta status %d", metaRec.Code)
	}

	snapReq = httptest.NewRequest(http.MethodGet, "/v1/snapshot/eu:pve:horde", nil)
	snapRec = httptest.NewRecorder()
	srv.ServeHTTP(snapRec, snapReq)
	if snapRec.Code != http.StatusOK {
		t.Fatalf("snapshot status %d: %s", snapRec.Code, snapRec.Body.String())
	}
	gz, err := gzip.NewReader(snapRec.Body)
	if err != nil {
		t.Fatal(err)
	}
	var snap struct {
		Market string `json:"market"`
		Items  []struct {
			K string `json:"k"`
			P int64  `json:"p"`
		} `json:"items"`
	}
	if err := json.NewDecoder(gz).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if snap.Market != "eu:pve:horde" || len(snap.Items) != 2 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	prices := map[string]int64{}
	for _, item := range snap.Items {
		prices[item.K] = item.P
	}
	if prices["i:2589"] != 1234 {
		t.Fatalf("unexpected prices: %+v", prices)
	}
}

func TestDuplicateIngest(t *testing.T) {
	srv, st := newTestServer(t)
	client := enroll(t, srv)

	body := ingestBody()
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+client.Token)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("ingest status %d", rec.Code)
		}
		var result ingestResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		if i == 0 && result.Queued != 1 {
			t.Fatalf("first ingest: %+v", result)
		}
		if i == 1 && result.Rejected != 1 {
			t.Fatalf("second ingest: %+v", result)
		}
	}
	processQueued(t, st)
}

func TestIngestAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(ingestBody()))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestIngestValidation(t *testing.T) {
	srv, _ := newTestServer(t)
	client := enroll(t, srv)

	cases := []string{
		`{"v":1,"scans":[{"market":"bad","scanned_at":1794947776,"items":[{"k":"i:1","p":1,"q":1,"n":1}]}]}`,
		`{"v":1,"scans":[{"market":"eu:pve:horde","scanned_at":1794947776,"items":[{"k":"bad","p":1,"q":1,"n":1}]}]}`,
		`{"v":1,"scans":[{"market":"eu:pve:horde","scanned_at":1794947776,"items":[{"k":"i:1","p":0,"q":1,"n":1}]}]}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/v1/ingest", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+client.Token)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", body, rec.Code)
		}
	}
}

func TestDataAddon(t *testing.T) {
	srv, st := newTestServer(t)
	client := enroll(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(ingestBody()))
	req.Header.Set("Authorization", "Bearer "+client.Token)
	srv.ServeHTTP(httptest.NewRecorder(), req)
	processQueued(t, st)

	zipReq := httptest.NewRequest(http.MethodGet, "/v1/snapshot/eu:pve:horde/data-addon.zip", nil)
	zipRec := httptest.NewRecorder()
	srv.ServeHTTP(zipRec, zipReq)
	if zipRec.Code != http.StatusOK {
		t.Fatalf("zip status %d: %s", zipRec.Code, zipRec.Body.String())
	}

	reader, err := zip.NewReader(bytes.NewReader(zipRec.Body.Bytes()), int64(zipRec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range reader.File {
		if file.Name != "TradePost_Data/Data.lua" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `k = "i:2589"`) {
			t.Fatalf("missing item in data addon: %s", data)
		}
		found = true
	}
	if !found {
		t.Fatal("Data.lua not found in zip")
	}
}

func TestItemMetadataSearchHistory(t *testing.T) {
	srv, st := newTestServer(t)
	client := enroll(t, srv)

	body := []byte(fmt.Sprintf(`{
	  "v": 1,
	  "scans": [{
	    "market": "eu:pve:horde",
	    "scanned_at": %d,
	    "client_version": "0.1.0",
	    "items": [
	      {"k": "i:2589", "p": 1234, "q": 50, "n": 2},
	      {"k": "g:12345:70", "p": 4200, "q": 1, "n": 1}
	    ]
	  }],
	  "items": [
	    {"id": 2589, "name": "Linen Cloth", "quality": 1, "class_id": 7, "subclass_id": 5, "icon": 132889},
	    {"id": 12345, "name": "Test Blade", "quality": 3, "class_id": 2, "subclass_id": 7, "icon": 135274}
	  ]
	}`, time.Now().Unix()))
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+client.Token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest status %d: %s", rec.Code, rec.Body.String())
	}
	processQueued(t, st)

	searchReq := httptest.NewRequest(http.MethodGet, "/v1/items?market=eu:pve:horde&q=linen", nil)
	searchRec := httptest.NewRecorder()
	srv.ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("search status %d: %s", searchRec.Code, searchRec.Body.String())
	}
	var search struct {
		Items []struct {
			Key   string `json:"key"`
			Name  string `json:"name"`
			Price int64  `json:"price"`
		} `json:"items"`
	}
	if err := json.Unmarshal(searchRec.Body.Bytes(), &search); err != nil {
		t.Fatal(err)
	}
	if len(search.Items) != 1 || search.Items[0].Key != "i:2589" || search.Items[0].Name != "Linen Cloth" {
		t.Fatalf("unexpected search result: %+v", search.Items)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/v1/item/history?market=eu:pve:horde&key=i:2589", nil)
	historyRec := httptest.NewRecorder()
	srv.ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("history status %d: %s", historyRec.Code, historyRec.Body.String())
	}
	var history struct {
		Name   string `json:"name"`
		Points []struct {
			Min int64 `json:"min"`
			Max int64 `json:"max"`
		} `json:"points"`
	}
	if err := json.Unmarshal(historyRec.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Name != "Linen Cloth" || len(history.Points) != 1 || history.Points[0].Min != 1234 {
		t.Fatalf("unexpected history: %+v", history)
	}

	marketsReq := httptest.NewRequest(http.MethodGet, "/v1/item/markets?key=i:2589", nil)
	marketsRec := httptest.NewRecorder()
	srv.ServeHTTP(marketsRec, marketsReq)
	if marketsRec.Code != http.StatusOK {
		t.Fatalf("markets status %d: %s", marketsRec.Code, marketsRec.Body.String())
	}
	var markets struct {
		Markets []struct {
			Market string `json:"market"`
			Price  int64  `json:"price"`
		} `json:"markets"`
	}
	if err := json.Unmarshal(marketsRec.Body.Bytes(), &markets); err != nil {
		t.Fatal(err)
	}
	if len(markets.Markets) != 1 || markets.Markets[0].Market != "eu:pve:horde" || markets.Markets[0].Price != 1234 {
		t.Fatalf("unexpected markets: %+v", markets.Markets)
	}
}

func TestMetadataOnlyIngest(t *testing.T) {
	srv, _ := newTestServer(t)
	client := enroll(t, srv)

	body := []byte(`{"v":1,"scans":[],"items":[{"id":2589,"name":"Linen Cloth","quality":1,"class_id":7,"subclass_id":5,"icon":1}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+client.Token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("metadata ingest status %d: %s", rec.Code, rec.Body.String())
	}
	var result ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Meta != 1 || result.Queued != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestWebIndex(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("index status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "TradePost") {
		t.Fatal("index does not contain the app title")
	}
}

func TestLoginCodeAndWatchSync(t *testing.T) {
	srv, _ := newTestServer(t)
	client := enroll(t, srv)

	codeBody := []byte(`{"character":"Schmoo-Boo"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/code", bytes.NewReader(codeBody))
	req.Header.Set("Authorization", "Bearer "+client.Token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code status %d: %s", rec.Code, rec.Body.String())
	}
	var codeResponse struct {
		Code      string `json:"code"`
		Character string `json:"character"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &codeResponse); err != nil {
		t.Fatal(err)
	}
	if len(codeResponse.Code) != 8 || codeResponse.Character != "Schmoo-Boo" {
		t.Fatalf("unexpected code response: %+v", codeResponse)
	}

	redeemBody := []byte(fmt.Sprintf(`{"code":%q}`, codeResponse.Code))
	redeemReq := httptest.NewRequest(http.MethodPost, "/v1/auth/redeem", bytes.NewReader(redeemBody))
	redeemRec := httptest.NewRecorder()
	srv.ServeHTTP(redeemRec, redeemReq)
	if redeemRec.Code != http.StatusOK {
		t.Fatalf("redeem status %d: %s", redeemRec.Code, redeemRec.Body.String())
	}
	cookies := redeemRec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookie || cookies[0].Value == "" {
		t.Fatalf("missing session cookie: %+v", cookies)
	}
	sessionCookieValue := cookies[0].Value

	// A code is single-use.
	reuseReq := httptest.NewRequest(http.MethodPost, "/v1/auth/redeem", bytes.NewReader(redeemBody))
	reuseRec := httptest.NewRecorder()
	srv.ServeHTTP(reuseRec, reuseReq)
	if reuseRec.Code != http.StatusUnauthorized {
		t.Fatalf("reused code status %d", reuseRec.Code)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	meReq.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionCookieValue})
	meRec := httptest.NewRecorder()
	srv.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("me status %d: %s", meRec.Code, meRec.Body.String())
	}

	putBody := []byte(`{"items":["i:1251","i:2589"]}`)
	putReq := httptest.NewRequest(http.MethodPut, "/v1/watch", bytes.NewReader(putBody))
	putReq.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionCookieValue})
	putRec := httptest.NewRecorder()
	srv.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("watch put status %d: %s", putRec.Code, putRec.Body.String())
	}
	var list store.WatchList
	if err := json.Unmarshal(putRec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || list.Character != "Schmoo-Boo" {
		t.Fatalf("unexpected watch list: %+v", list)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/watch", nil)
	getReq.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionCookieValue})
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("watch get status %d: %s", getRec.Code, getRec.Body.String())
	}

	// Companion-style access uses the client token and character query.
	bearerReq := httptest.NewRequest(http.MethodGet, "/v1/watch?character=Schmoo-Boo", nil)
	bearerReq.Header.Set("Authorization", "Bearer "+client.Token)
	bearerRec := httptest.NewRecorder()
	srv.ServeHTTP(bearerRec, bearerReq)
	if bearerRec.Code != http.StatusOK {
		t.Fatalf("bearer watch status %d: %s", bearerRec.Code, bearerRec.Body.String())
	}
	var bearerList store.WatchList
	if err := json.Unmarshal(bearerRec.Body.Bytes(), &bearerList); err != nil {
		t.Fatal(err)
	}
	if len(bearerList.Items) != 2 {
		t.Fatalf("unexpected bearer watch list: %+v", bearerList)
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionCookieValue})
	logoutRec := httptest.NewRecorder()
	srv.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status %d", logoutRec.Code)
	}
	meReq2 := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	meReq2.AddCookie(&http.Cookie{Name: sessionCookie, Value: sessionCookieValue})
	meRec2 := httptest.NewRecorder()
	srv.ServeHTTP(meRec2, meReq2)
	if meRec2.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status %d", meRec2.Code)
	}
}

func TestMetadataDetailsAdminAndFilters(t *testing.T) {
	srv, st := newTestServer(t)
	client := enroll(t, srv)

	body := []byte(fmt.Sprintf(`{
	  "v": 1,
	  "scans": [{
	    "market": "eu:pve:horde",
	    "scanned_at": %d,
	    "items": [
	      {"k": "i:100", "p": 500, "q": 3, "n": 1},
	      {"k": "i:200", "p": 1500, "q": 1, "n": 4}
	    ]
	  }],
	  "items": [
	    {"id": 100, "name": "Plain Cloth", "quality": 1, "class_id": 7, "subclass_id": 5, "icon": 1},
	    {"id": 200, "name": "Fancy Blade", "quality": 4, "class_id": 2, "subclass_id": 7, "icon": 2,
	     "item_level": 80, "required_level": 70, "bind_type": 1, "inventory_type": 13,
	     "equip_loc": "INVTYPE_WEAPON", "armor": 0, "damage_min": 10, "damage_max": 20, "delay": 2600,
	     "vendor_price": 999, "max_stack": 1, "unique_item": true, "set_id": 42,
	     "stats": {"stamina": 10, "agility": 5},
	     "details": {"description": "A test blade"}}
	  ]
	}`, time.Now().Unix()))
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+client.Token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest status %d: %s", rec.Code, rec.Body.String())
	}
	processQueued(t, st)

	// Missing details returns only items without details by default.
	missingRec := httptest.NewRecorder()
	srv.ServeHTTP(missingRec, httptest.NewRequest(http.MethodGet,
		"/v1/items/missing-details?market=eu:pve:horde", nil))
	if missingRec.Code != http.StatusOK {
		t.Fatalf("missing details status %d: %s", missingRec.Code, missingRec.Body.String())
	}
	var missing struct {
		Items []int64 `json:"items"`
		Count int     `json:"count"`
	}
	if err := json.Unmarshal(missingRec.Body.Bytes(), &missing); err != nil {
		t.Fatal(err)
	}
	if missing.Count != 1 || len(missing.Items) != 1 || missing.Items[0] != 100 {
		t.Fatalf("unexpected missing details: %+v", missing)
	}

	// force=1 returns every known item.
	forceRec := httptest.NewRecorder()
	srv.ServeHTTP(forceRec, httptest.NewRequest(http.MethodGet,
		"/v1/items/missing-details?market=eu:pve:horde&force=1", nil))
	var forced struct {
		Items []int64 `json:"items"`
		Count int     `json:"count"`
	}
	if err := json.Unmarshal(forceRec.Body.Bytes(), &forced); err != nil {
		t.Fatal(err)
	}
	if forced.Count != 2 {
		t.Fatalf("unexpected forced details: %+v", forced)
	}

	// Item detail carries metadata plus its latest price.
	detailRec := httptest.NewRecorder()
	srv.ServeHTTP(detailRec, httptest.NewRequest(http.MethodGet,
		"/v1/item/detail?market=eu:pve:horde&key=i:200", nil))
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status %d: %s", detailRec.Code, detailRec.Body.String())
	}
	var detail struct {
		ID        int64          `json:"id"`
		Name      string         `json:"name"`
		ItemLevel int            `json:"item_level"`
		EquipLoc  string         `json:"equip_loc"`
		Stats     map[string]any `json:"stats"`
		Details   map[string]any `json:"details"`
		Price     int64          `json:"price"`
	}
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID != 200 || detail.Name != "Fancy Blade" || detail.ItemLevel != 80 ||
		detail.EquipLoc != "INVTYPE_WEAPON" || detail.Price != 1500 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Stats["stamina"] != float64(10) || detail.Details["description"] != "A test blade" {
		t.Fatalf("unexpected detail json: %+v", detail)
	}

	// Filters, paging and totals.
	filterRec := httptest.NewRecorder()
	srv.ServeHTTP(filterRec, httptest.NewRequest(http.MethodGet,
		"/v1/items?market=eu:pve:horde&quality=4&class_id=2&subclass_id=7&inventory_type=13"+
			"&min_ilvl=50&max_ilvl=100&min_req_level=60&max_req_level=80"+
			"&min_price=1000&max_price=2000&in_stock=1&sort=ilvl&page=1&limit=10", nil))
	if filterRec.Code != http.StatusOK {
		t.Fatalf("filter status %d: %s", filterRec.Code, filterRec.Body.String())
	}
	var page struct {
		Items []struct {
			Key        string `json:"key"`
			HasDetails bool   `json:"has_details"`
			ItemLevel  int    `json:"item_level"`
		} `json:"items"`
		Total int `json:"total"`
		Page  int `json:"page"`
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(filterRec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Page != 1 || page.Limit != 10 || len(page.Items) != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}
	if page.Items[0].Key != "i:200" || !page.Items[0].HasDetails || page.Items[0].ItemLevel != 80 {
		t.Fatalf("unexpected filtered item: %+v", page.Items[0])
	}

	// Default listing remains backwards compatible and reports the total.
	allRec := httptest.NewRecorder()
	srv.ServeHTTP(allRec, httptest.NewRequest(http.MethodGet, "/v1/items?market=eu:pve:horde", nil))
	var all struct {
		Items []json.RawMessage `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(allRec.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	if all.Total != 2 || len(all.Items) != 2 {
		t.Fatalf("unexpected default listing: %+v", all)
	}
}

func TestHistoryRanges(t *testing.T) {
	srv, st := newTestServer(t)
	client := enroll(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(ingestBody()))
	req.Header.Set("Authorization", "Bearer "+client.Token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest status %d: %s", rec.Code, rec.Body.String())
	}
	processQueued(t, st)

	cases := []struct {
		query      string
		resolution string
	}{
		{"12h", "1m"},
		{"7d", "15m"},
		{"all", "hour"},
	}
	for _, tc := range cases {
		request := httptest.NewRequest(http.MethodGet,
			"/v1/item/history?market=eu:pve:horde&key=i:2589&range="+tc.query, nil)
		response := httptest.NewRecorder()
		srv.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status %d: %s", tc.query, response.Code, response.Body.String())
		}
		var history struct {
			Resolution string `json:"resolution"`
			Range      string `json:"range"`
			Points     []struct {
				Min int64 `json:"min"`
			} `json:"points"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
			t.Fatal(err)
		}
		if history.Resolution != tc.resolution || history.Range != tc.query {
			t.Fatalf("%s: unexpected history header: %+v", tc.query, history)
		}
		if len(history.Points) != 1 || history.Points[0].Min != 1234 {
			t.Fatalf("%s: unexpected points: %+v", tc.query, history.Points)
		}
	}
}
