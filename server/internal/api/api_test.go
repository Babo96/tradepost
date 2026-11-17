package api

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	srv, _ := newTestServer(t)
	client := enroll(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(ingestBody()))
	req.Header.Set("Authorization", "Bearer "+client.Token)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest status %d: %s", rec.Code, rec.Body.String())
	}
	var result ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 1 || result.Items != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}

	metaReq := httptest.NewRequest(http.MethodGet, "/v1/meta", nil)
	metaRec := httptest.NewRecorder()
	srv.ServeHTTP(metaRec, metaReq)
	if metaRec.Code != http.StatusOK {
		t.Fatalf("meta status %d", metaRec.Code)
	}

	snapReq := httptest.NewRequest(http.MethodGet, "/v1/snapshot/eu:pve:horde", nil)
	snapRec := httptest.NewRecorder()
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
	srv, _ := newTestServer(t)
	client := enroll(t, srv)

	body := ingestBody()
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+client.Token)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("ingest status %d", rec.Code)
		}
		var result ingestResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		if i == 0 && result.Accepted != 1 {
			t.Fatalf("first ingest: %+v", result)
		}
		if i == 1 && result.Rejected != 1 {
			t.Fatalf("second ingest: %+v", result)
		}
	}
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
	srv, _ := newTestServer(t)
	client := enroll(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", bytes.NewReader(ingestBody()))
	req.Header.Set("Authorization", "Bearer "+client.Token)
	srv.ServeHTTP(httptest.NewRecorder(), req)

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
		if file.Name != "BaboForever_Data/Data.lua" {
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
	srv, _ := newTestServer(t)
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
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest status %d: %s", rec.Code, rec.Body.String())
	}

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
	if rec.Code != http.StatusOK {
		t.Fatalf("metadata ingest status %d: %s", rec.Code, rec.Body.String())
	}
	var result ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Meta != 1 || result.Accepted != 0 {
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
	if !strings.Contains(rec.Body.String(), "Babo Forever") {
		t.Fatal("index does not contain the app title")
	}
}
