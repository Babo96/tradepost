package api

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/babo96/tradepost/server/internal/store"
)

func TestMissingDetailsIncludesPlaceholder(t *testing.T) {
	m := NewMemStore()
	m.markets["eu:pve:horde"] = map[string]store.SnapshotItem{
		"i:100": {},
		"i:200": {},
	}
	m.items[100] = store.ItemMeta{
		ID: 100,
		Details: json.RawMessage(
			`{"lines":[{"left":"Retrieving item information","type":41}]}`,
		),
	}
	m.items[200] = store.ItemMeta{
		ID:      200,
		Details: json.RawMessage(`{"lines":[{"left":"Real tooltip","type":0}]}`),
	}

	ids, err := m.MissingItemDetails(context.Background(), "eu:pve:horde", false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{100}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("missing details = %v, want %v", ids, want)
	}

	all, err := m.MissingItemDetails(context.Background(), "eu:pve:horde", true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{100, 200}; !reflect.DeepEqual(all, want) {
		t.Fatalf("forced missing details = %v, want %v", all, want)
	}
}
