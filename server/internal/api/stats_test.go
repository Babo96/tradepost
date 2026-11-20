package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/babo96/tradepost/server/internal/store"
)

func getStats(t *testing.T, srv *Server, query string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/stats?"+query, nil))
	return rec
}

func TestStatsInvalidMarket(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, market := range []string{"", "bad", "eu:pve:horde:x", "EU:pve:horde"} {
		rec := getStats(t, srv, "market="+market)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("market %q: expected 400, got %d: %s", market, rec.Code, rec.Body.String())
		}
	}
}

func TestStatsEmptyMarket(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := getStats(t, srv, "market=eu:pve:horde&hours=24")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"market":"eu:pve:horde"`, `"classes":[]`, `"quality":[]`, `"heavyweights":[]`, `"points":[]`, `"items":[]`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
	var stats store.Stats
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Window.CurrentBucket != 0 || stats.Window.ReferenceBucket != 0 {
		t.Fatalf("unexpected window: %+v", stats.Window)
	}
	if len(stats.Bands) != 6 {
		t.Fatalf("bands should always be present: %+v", stats.Bands)
	}
}

func ingestStatsHistory(t *testing.T, st *MemStore, now time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := st.UpsertItems(ctx, []store.ItemMeta{
		{ID: 100, Name: "Alpha", Quality: 4, ClassID: 0},
		{ID: 200, Name: "Beta", Quality: 3, ClassID: 1},
		{ID: 300, Name: "Gamma", Quality: 2, ClassID: 2},
		{ID: 400, Name: "Delta", Quality: 1, ClassID: 3},
		{ID: 500, Name: "Variant", Quality: 4, ClassID: 0},
	}); err != nil {
		t.Fatal(err)
	}

	// Reference bucket, ~26h before the current one.
	st.applyScanLocked("eu:pve:horde", now.Add(-26*time.Hour), []store.Item{
		{Key: "i:100", Price: 20000, Quantity: 1, Listings: 1},
		{Key: "i:200", Price: 30000, Quantity: 10, Listings: 3},
		{Key: "i:300", Price: 15000, Quantity: 3, Listings: 1},
	})
	// Current bucket.
	st.applyScanLocked("eu:pve:horde", now, []store.Item{
		{Key: "i:100", Price: 25000, Quantity: 5, Listings: 2},
		{Key: "i:200", Price: 24000, Quantity: 4, Listings: 2},
		{Key: "i:400", Price: 50000, Quantity: 2, Listings: 1},
		{Key: "g:500:70", Price: 60000, Quantity: 1, Listings: 1},
		{Key: "g:500:80", Price: 80000, Quantity: 3, Listings: 1},
	})
}

func TestStatsHistory(t *testing.T) {
	srv, st := newTestServer(t)
	ingestStatsHistory(t, st, time.Now())

	rec := getStats(t, srv, "market=eu:pve:horde&hours=24")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var stats store.Stats
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}

	if stats.Window.TargetHours != 24 {
		t.Fatalf("target hours: %+v", stats.Window)
	}
	if stats.Window.CurrentBucket == 0 || stats.Window.ReferenceBucket == 0 {
		t.Fatalf("missing buckets: %+v", stats.Window)
	}
	if stats.Window.ReferenceBucket >= stats.Window.CurrentBucket {
		t.Fatalf("reference not before current: %+v", stats.Window)
	}
	if stats.Window.ReferenceHours < 25 || stats.Window.ReferenceHours > 27 {
		t.Fatalf("unexpected reference age: %+v", stats.Window)
	}

	if stats.Totals.Items != 4 {
		t.Fatalf("current items: %+v", stats.Totals)
	}
	if stats.Totals.ItemsPrev != 3 {
		t.Fatalf("previous items: %+v", stats.Totals)
	}
	if stats.Totals.Quantity != 15 || stats.Totals.QuantityPrev != 14 {
		t.Fatalf("quantity totals: %+v", stats.Totals)
	}
	if stats.Totals.Offers != 5 || stats.Totals.OffersPrev != 3 {
		t.Fatalf("offers totals: %+v", stats.Totals)
	}

	if stats.Breadth.Up != 1 || stats.Breadth.Down != 1 || stats.Breadth.Flat != 0 || stats.Breadth.Matched != 2 {
		t.Fatalf("breadth: %+v", stats.Breadth)
	}
	if stats.Index.Matched != 2 {
		t.Fatalf("index matched: %+v", stats.Index)
	}
	if stats.Index.ChangePct != 2.5 {
		t.Fatalf("index change: %+v", stats.Index)
	}
	if len(stats.Index.Points) != 2 {
		t.Fatalf("index points: %+v", stats.Index.Points)
	}

	if stats.NewItems.Count != 3 || len(stats.NewItems.Items) != 3 {
		t.Fatalf("new items: %+v", stats.NewItems)
	}
	if stats.Delisted != 1 {
		t.Fatalf("delisted: %d", stats.Delisted)
	}

	if len(stats.Movers.Gainers) != 1 || stats.Movers.Gainers[0].ItemID != 100 || stats.Movers.Gainers[0].ChangePct != 25 {
		t.Fatalf("gainers: %+v", stats.Movers.Gainers)
	}
	if len(stats.Movers.Losers) != 1 || stats.Movers.Losers[0].ItemID != 200 || stats.Movers.Losers[0].ChangePct != -20 {
		t.Fatalf("losers: %+v", stats.Movers.Losers)
	}

	if len(stats.Stock.Added) != 1 || stats.Stock.Added[0].ItemID != 100 || stats.Stock.Added[0].Delta != 4 {
		t.Fatalf("stock added: %+v", stats.Stock.Added)
	}
	if len(stats.Stock.Removed) != 1 || stats.Stock.Removed[0].ItemID != 200 || stats.Stock.Removed[0].Delta != -6 {
		t.Fatalf("stock removed: %+v", stats.Stock.Removed)
	}

	// g:500:70 and g:500:80 are separate new keys, but fold into one item in
	// the per-item totals above (current Items=4, Offers=5).
	keys := map[string]bool{}
	for _, item := range stats.NewItems.Items {
		keys[item.Key] = true
	}
	if !keys["g:500:70"] || !keys["g:500:80"] || !keys["i:400"] {
		t.Fatalf("unexpected new item keys: %+v", stats.NewItems.Items)
	}
}

func TestStatsHoursClamp(t *testing.T) {
	srv, st := newTestServer(t)
	ingestStatsHistory(t, st, time.Now())

	for _, tc := range []struct {
		query string
		want  int
	}{
		{"market=eu:pve:horde&hours=0", 1},
		{"market=eu:pve:horde&hours=-5", 1},
		{"market=eu:pve:horde&hours=999", 168},
	} {
		rec := getStats(t, srv, tc.query)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", tc.query, rec.Code, rec.Body.String())
		}
		var stats store.Stats
		if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
			t.Fatal(err)
		}
		if stats.Window.TargetHours != tc.want {
			t.Fatalf("%s: target hours %d, want %d", tc.query, stats.Window.TargetHours, tc.want)
		}
	}
}
