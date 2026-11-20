package store

import (
	"strings"
	"testing"
)

func TestSearchOrderUsesRequestedDirection(t *testing.T) {
	cases := []struct {
		name string
		sort string
		dir  string
		want string
	}{
		{"price asc", "price", "asc", "p.price ASC"},
		{"price desc", "price", "desc", "p.price DESC"},
		{"name asc", "name", "asc", "i.name ASC"},
		{"name desc", "name", "desc", "i.name DESC"},
		{"ilvl asc", "ilvl", "asc", "i.item_level ASC"},
		{"ilvl desc", "ilvl", "desc", "i.item_level DESC"},
		{"quality asc", "quality", "asc", "i.quality ASC"},
		{"quality desc", "quality", "desc", "i.quality DESC"},
		{"scanned asc", "scanned", "asc", "p.scanned_at ASC"},
		{"scanned desc", "scanned", "desc", "p.scanned_at DESC"},
		{"price default", "price", "", "p.price ASC"},
		{"ilvl default", "ilvl", "", "i.item_level DESC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := searchOrder(tc.sort, tc.dir)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("searchOrder(%q, %q) = %q, want %q", tc.sort, tc.dir, got, tc.want)
			}
		})
	}
}
