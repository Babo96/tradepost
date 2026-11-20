package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/babo96/tradepost/server/internal/store"
)

func intPtr(v int) *int { return &v }

func TestSortSearchResultsAllDirections(t *testing.T) {
	items := []store.SearchResult{
		{Key: "a", Name: "Beta", Price: 20, Quality: 2, ItemLevel: intPtr(10), Scanned: 2},
		{Key: "b", Name: "Alpha", Price: 10, Quality: 4, ItemLevel: intPtr(30), Scanned: 1},
		{Key: "c", Name: "Gamma", Price: 30, Quality: 1, ItemLevel: intPtr(20), Scanned: 3},
	}
	cases := []struct {
		name string
		sort string
		dir  string
		want []string
	}{
		{"price asc", "price", "asc", []string{"b", "a", "c"}},
		{"price desc", "price", "desc", []string{"c", "a", "b"}},
		{"name asc", "name", "asc", []string{"b", "a", "c"}},
		{"name desc", "name", "desc", []string{"c", "a", "b"}},
		{"ilvl asc", "ilvl", "asc", []string{"a", "c", "b"}},
		{"ilvl desc", "ilvl", "desc", []string{"b", "c", "a"}},
		{"quality asc", "quality", "asc", []string{"c", "a", "b"}},
		{"quality desc", "quality", "desc", []string{"b", "a", "c"}},
		{"scanned asc", "scanned", "asc", []string{"b", "a", "c"}},
		{"scanned desc", "scanned", "desc", []string{"c", "a", "b"}},
		{"price default", "price", "", []string{"b", "a", "c"}},
		{"ilvl default", "ilvl", "", []string{"b", "c", "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotItems := append([]store.SearchResult(nil), items...)
			sortSearchResults(gotItems, tc.sort, tc.dir)
			got := make([]string, len(gotItems))
			for i, item := range gotItems {
				got[i] = item.Key
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("keys = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInvalidSortDirection(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet,
		"/v1/items?market=eu:pve:horde&dir=sideways", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}
