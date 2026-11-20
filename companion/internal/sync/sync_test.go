package sync

import (
	"testing"

	"github.com/babo96/tradepost/companion/internal/api"
	"github.com/babo96/tradepost/companion/internal/config"
	"github.com/babo96/tradepost/companion/internal/export"
)

func TestBuildRingPayloadFullThenDelta(t *testing.T) {
	s := &Service{cfg: config.Config{RingSlots: 64}}
	first := api.Snapshot{
		Market: "eu:pve:horde", Generated: 1, Version: 3,
		Items: []api.SnapshotItem{
			{Key: "i:1", Price: 10, Quantity: 1, Listings: 1, Scanned: 1},
			{Key: "i:2", Price: 20, Quantity: 2, Listings: 2, Scanned: 1},
			{Key: "i:3", Price: 30, Quantity: 3, Listings: 3, Scanned: 1},
		},
	}
	full := s.buildRingPayload(&first, 10)
	if !full.Full || full.Base != full.Seq || len(full.Items) != 3 {
		t.Fatalf("expected full payload, got %+v", full)
	}

	second := api.Snapshot{
		Market: "eu:pve:horde", Generated: 2, Version: 4,
		Items: []api.SnapshotItem{
			{Key: "i:1", Price: 10, Quantity: 1, Listings: 1, Scanned: 2},
			{Key: "i:2", Price: 25, Quantity: 2, Listings: 2, Scanned: 2},
			{Key: "i:4", Price: 40, Quantity: 4, Listings: 4, Scanned: 2},
		},
	}
	delta := s.buildRingPayload(&second, 11)
	if delta.Full || delta.Base != full.Seq || delta.Seq <= full.Seq {
		t.Fatalf("expected delta payload, got %+v", delta)
	}
	if len(delta.Items) != 2 || len(delta.Removed) != 1 || delta.Removed[0] != "i:3" {
		t.Fatalf("unexpected delta contents: %+v", delta)
	}

	periodic := s.buildRingPayload(&second, 16)
	if !periodic.Full || periodic.Base != periodic.Seq {
		t.Fatalf("expected periodic full after six windows, got %+v", periodic)
	}
}

func TestApplyRingRequestForcesFull(t *testing.T) {
	s := &Service{cfg: config.Config{Market: "eu:pve:horde"}, ringBase: map[string]api.SnapshotItem{"i:1": {Key: "i:1"}}}
	s.applyRingRequest(&export.RingRequest{Market: "eu:pve:horde", At: 10})
	if s.ringBase != nil {
		t.Fatal("expected full resync request to clear the base")
	}
	s.ringBase = map[string]api.SnapshotItem{"i:1": {Key: "i:1"}}
	s.applyRingRequest(&export.RingRequest{Market: "eu:pve:horde", At: 10})
	if s.ringBase == nil {
		t.Fatal("duplicate request should not force another full")
	}
	s.applyRingRequest(&export.RingRequest{Market: "us:pve:alliance", At: 11})
	if s.ringBase == nil {
		t.Fatal("request for another market should not force a full")
	}
}

func TestPickSnapshotUsesActiveMarket(t *testing.T) {
	snapshots := []api.Snapshot{
		{Market: "eu:pve:horde", Items: []api.SnapshotItem{{Key: "i:1"}}},
		{Market: "eu:pvp:alliance", Items: []api.SnapshotItem{{Key: "i:2"}}},
	}
	if got := pickSnapshot(snapshots, "eu:pvp:alliance"); got == nil || got.Market != "eu:pvp:alliance" {
		t.Fatalf("active market not selected: %+v", got)
	}
	if got := pickSnapshot(snapshots, "missing"); got != nil {
		t.Fatalf("missing market should return nil: %+v", got)
	}
	if got := pickSnapshot(snapshots, ""); got != nil {
		t.Fatalf("empty market should return nil: %+v", got)
	}
}
