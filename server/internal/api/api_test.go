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
