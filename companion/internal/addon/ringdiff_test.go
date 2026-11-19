package addon

import (
	"testing"

	"github.com/babo96/tradepost/companion/internal/api"
)

func TestRingDiffIgnoresTimestampOnly(t *testing.T) {
	base := map[string]api.SnapshotItem{
		"i:1": {Key: "i:1", Price: 100, Quantity: 5, Listings: 1, Scanned: 1},
		"i:2": {Key: "i:2", Price: 200, Quantity: 6, Listings: 2, Scanned: 1},
		"i:3": {Key: "i:3", Price: 300, Quantity: 7, Listings: 3, Scanned: 1},
	}
	items := []api.SnapshotItem{
		{Key: "i:1", Price: 100, Quantity: 5, Listings: 1, Scanned: 999},
		{Key: "i:2", Price: 250, Quantity: 6, Listings: 2, Scanned: 999},
		{Key: "i:4", Price: 400, Quantity: 4, Listings: 1, Scanned: 999},
	}
	changed, removed := RingDiff(base, items)
	if len(changed) != 2 || changed[0].Key != "i:2" || changed[1].Key != "i:4" {
		t.Fatalf("unexpected changed items: %+v", changed)
	}
	if len(removed) != 1 || removed[0] != "i:3" {
		t.Fatalf("unexpected removed keys: %+v", removed)
	}
}
