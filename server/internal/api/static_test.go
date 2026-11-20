package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebDownloadPage(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/download/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download page status %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("download page content type %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Download TradePost",
		"TradePost-addon.zip",
		"tradepost-linux-amd64.tar.gz",
		"tradepost-companion-windows-amd64.zip",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("download page missing %q", want)
		}
	}
}

func TestWebDownloadRedirect(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/download", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("download redirect status %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/download/" && loc != "download/" {
		t.Fatalf("download redirect location %q", loc)
	}
}

func TestWebDownloadCSS(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/download/download.css", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download css status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Fatalf("download css content type %q", ct)
	}
}
