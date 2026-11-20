package store

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// StatsItemAtBucket is the aggregated market position of one item id at a
// single hourly bucket. Equipment variants (g:<id>:<ilvl>) are folded into the
// same id: Price is the minimum variant price, Quantity is the summed stock.
type StatsItemAtBucket struct {
	Price    int64
	Quantity int64
	Offers   int
}

// StatsItemMetaInfo is the item metadata needed by the statistics builder.
type StatsItemMetaInfo struct {
	Name    string
	Quality int
	ClassID int
}

// StatsBuildInput is the data-independent input for BuildStats. Both the
// Postgres and in-memory stores prepare their buckets and hand them here so the
// aggregation semantics stay identical.
type StatsBuildInput struct {
	Market          string
	Now             time.Time
	Hours           int
	CurrentBucket   int64
	ReferenceBucket int64
	FirstBucket     int64
	Current         map[int64]StatsItemAtBucket
	Reference       map[int64]StatsItemAtBucket
	Meta            map[int64]StatsItemMetaInfo
	IndexPoints     []StatsPoint
	IndexMatched    int
	NewItems        []StatsItem
	NewItemsCount   int
	Delisted        int64
	Freshness       StatsFreshness
}

const (
	statsPriceFloor = int64(10000)
	statsMoverLimit = 6
)

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// BuildStats assembles the response from prepared per-item buckets.
func BuildStats(in StatsBuildInput) Stats {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	stats := Stats{
		Market:      in.Market,
		GeneratedAt: now.Unix(),
		Window: StatsWindow{
			TargetHours:     in.Hours,
			CurrentBucket:   in.CurrentBucket,
			ReferenceBucket: in.ReferenceBucket,
			FirstBucket:     in.FirstBucket,
		},
		Index:        StatsIndex{Points: alwaysPoints(in.IndexPoints), Matched: in.IndexMatched},
		Breadth:      StatsBreadth{},
		NewItems:     StatsNewItems{Count: in.NewItemsCount, Items: alwaysItems(in.NewItems)},
		Delisted:     in.Delisted,
		Movers:       StatsMovers{Gainers: []StatsMove{}, Losers: []StatsMove{}},
		Stock:        StatsStock{Added: []StatsStockMove{}, Removed: []StatsStockMove{}},
		Classes:      []StatsClass{},
		Quality:      []StatsQuality{},
		Bands:        []StatsBand{},
		Heavyweights: []StatsItemValue{},
		Freshness:    in.Freshness,
	}
	if in.ReferenceBucket > 0 {
		stats.Window.ReferenceHours = round1(float64(now.Unix()-in.ReferenceBucket) / 3600)
	}

	if len(stats.Index.Points) > 0 {
		stats.Index.ChangePct = round1(stats.Index.Points[len(stats.Index.Points)-1].Value - 100)
	}

	cur := in.Current
	ref := in.Reference

	// Totals.
	ct := totalsFor(cur)
	stats.Totals.Items = ct.Items
	stats.Totals.Offers = ct.Offers
	stats.Totals.Quantity = ct.Quantity
	stats.Totals.Value = ct.Value
	if in.ReferenceBucket > 0 {
		rt := totalsFor(ref)
		stats.Totals.ItemsPrev = rt.Items
		stats.Totals.OffersPrev = rt.Offers
		stats.Totals.QuantityPrev = rt.Quantity
		stats.Totals.ValuePrev = rt.Value
	}

	// Breadth and movers need a reference bucket and a comparable current one.
	if in.ReferenceBucket > 0 && in.CurrentBucket > 0 {
		moves := make([]StatsMove, 0, len(cur))
		for id, c := range cur {
			r, ok := ref[id]
			if !ok || r.Price < statsPriceFloor || c.Price <= 0 {
				continue
			}
			change := round1((float64(c.Price)/float64(r.Price) - 1) * 100)
			if c.Price > r.Price {
				stats.Breadth.Up++
			} else if c.Price < r.Price {
				stats.Breadth.Down++
			} else {
				stats.Breadth.Flat++
			}
			stats.Breadth.Matched++
			moves = append(moves, StatsMove{
				ItemID:       id,
				Name:         in.Meta[id].Name,
				Quality:      in.Meta[id].Quality,
				Price:        c.Price,
				PricePrev:    r.Price,
				ChangePct:    change,
				Quantity:     c.Quantity,
				QuantityPrev: r.Quantity,
			})
		}
		sort.Slice(moves, func(i, j int) bool {
			if moves[i].ChangePct == moves[j].ChangePct {
				return moves[i].ItemID < moves[j].ItemID
			}
			return moves[i].ChangePct > moves[j].ChangePct
		})
		gainers := make([]StatsMove, 0, statsMoverLimit)
		for _, m := range moves {
			if m.ChangePct <= 0 {
				break
			}
			if len(gainers) >= statsMoverLimit {
				break
			}
			gainers = append(gainers, m)
		}
		losers := make([]StatsMove, 0, statsMoverLimit)
		for i := len(moves) - 1; i >= 0; i-- {
			m := moves[i]
			if m.ChangePct >= 0 {
				break
			}
			if len(losers) >= statsMoverLimit {
				break
			}
			losers = append(losers, m)
		}
		stats.Movers = StatsMovers{Gainers: gainers, Losers: losers}
	}

	// Stock moves: quantity change for items present in both buckets.
	if in.ReferenceBucket > 0 && in.CurrentBucket > 0 {
		added := make([]StatsStockMove, 0)
		removed := make([]StatsStockMove, 0)
		for id, c := range cur {
			r, ok := ref[id]
			if !ok {
				continue
			}
			delta := c.Quantity - r.Quantity
			if delta == 0 {
				continue
			}
			move := StatsStockMove{
				ItemID:       id,
				Name:         in.Meta[id].Name,
				Quality:      in.Meta[id].Quality,
				Price:        c.Price,
				Quantity:     c.Quantity,
				QuantityPrev: r.Quantity,
				Delta:        delta,
			}
			if delta > 0 {
				added = append(added, move)
			} else {
				removed = append(removed, move)
			}
		}
		sort.Slice(added, func(i, j int) bool {
			if added[i].Delta == added[j].Delta {
				return added[i].ItemID < added[j].ItemID
			}
			return added[i].Delta > added[j].Delta
		})
		sort.Slice(removed, func(i, j int) bool {
			if removed[i].Delta == removed[j].Delta {
				return removed[i].ItemID < removed[j].ItemID
			}
			return removed[i].Delta < removed[j].Delta
		})
		if len(added) > statsMoverLimit {
			added = added[:statsMoverLimit]
		}
		if len(removed) > statsMoverLimit {
			removed = removed[:statsMoverLimit]
		}
		stats.Stock = StatsStock{Added: added, Removed: removed}
	}

	// Classes.
	type classAgg struct {
		items         int
		quantity      int64
		value         int64
		bothValue     int64
		bothValuePrev int64
	}
	classes := map[int]classAgg{}
	for id, c := range cur {
		classID := in.Meta[id].ClassID
		agg := classes[classID]
		agg.items++
		agg.quantity += c.Quantity
		agg.value += c.Price * c.Quantity
		if r, ok := ref[id]; ok && r.Price > 0 {
			agg.bothValue += c.Price * c.Quantity
			agg.bothValuePrev += r.Price * r.Quantity
		}
		classes[classID] = agg
	}
	classIDs := make([]int, 0, len(classes))
	for id := range classes {
		classIDs = append(classIDs, id)
	}
	sort.Slice(classIDs, func(i, j int) bool {
		a, b := classes[classIDs[i]], classes[classIDs[j]]
		if a.value == b.value {
			return classIDs[i] < classIDs[j]
		}
		return a.value > b.value
	})
	for _, id := range classIDs {
		agg := classes[id]
		change := 0.0
		if in.ReferenceBucket > 0 && agg.bothValuePrev > 0 {
			change = round1((float64(agg.bothValue)/float64(agg.bothValuePrev) - 1) * 100)
		}
		stats.Classes = append(stats.Classes, StatsClass{
			ClassID:   id,
			Items:     agg.items,
			Quantity:  agg.quantity,
			Value:     agg.value,
			ChangePct: change,
		})
		if len(stats.Classes) >= 12 {
			break
		}
	}

	// Quality buckets.
	qualities := map[int]*StatsQuality{}
	for id, c := range cur {
		q := in.Meta[id].Quality
		entry, ok := qualities[q]
		if !ok {
			entry = &StatsQuality{Quality: q}
			qualities[q] = entry
		}
		entry.Items++
		entry.Offers += c.Offers
		entry.Quantity += c.Quantity
		entry.Value += c.Price * c.Quantity
	}
	qualityKeys := make([]int, 0, len(qualities))
	for q := range qualities {
		qualityKeys = append(qualityKeys, q)
	}
	sort.Slice(qualityKeys, func(i, j int) bool {
		a, b := qualities[qualityKeys[i]], qualities[qualityKeys[j]]
		if a.Value == b.Value {
			return qualityKeys[i] < qualityKeys[j]
		}
		return a.Value > b.Value
	})
	for _, q := range qualityKeys {
		stats.Quality = append(stats.Quality, *qualities[q])
		if len(stats.Quality) >= 8 {
			break
		}
	}

	// Bands.
	bands := []StatsBand{
		{Label: "<1g"},
		{Label: "1\u201310g"},
		{Label: "10\u2013100g"},
		{Label: "100g\u20131k"},
		{Label: "1k\u201310k"},
		{Label: "10k g+"},
	}
	bandIndex := func(price int64) int {
		switch {
		case price < 10000:
			return 0
		case price < 100000:
			return 1
		case price < 1000000:
			return 2
		case price < 10000000:
			return 3
		case price < 100000000:
			return 4
		default:
			return 5
		}
	}
	for _, c := range cur {
		i := bandIndex(c.Price)
		bands[i].Items++
		bands[i].Value += c.Price * c.Quantity
	}
	stats.Bands = bands

	// Heavyweights.
	type itemValue struct {
		id    int64
		value int64
	}
	values := make([]itemValue, 0, len(cur))
	var totalValue int64
	for id, c := range cur {
		v := c.Price * c.Quantity
		totalValue += v
		values = append(values, itemValue{id: id, value: v})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].value == values[j].value {
			return values[i].id < values[j].id
		}
		return values[i].value > values[j].value
	})
	for _, v := range values {
		if len(stats.Heavyweights) >= 8 {
			break
		}
		share := 0.0
		if totalValue > 0 {
			share = round1(float64(v.value) / float64(totalValue) * 100)
		}
		stats.Heavyweights = append(stats.Heavyweights, StatsItemValue{
			ItemID:   v.id,
			Name:     in.Meta[v.id].Name,
			Value:    v.value,
			SharePct: share,
		})
	}

	return stats
}

func totalsFor(items map[int64]StatsItemAtBucket) StatsTotals {
	var t StatsTotals
	t.Items = len(items)
	for _, it := range items {
		t.Offers += it.Offers
		t.Quantity += it.Quantity
		t.Value += it.Price * it.Quantity
	}
	return t
}

func alwaysPoints(points []StatsPoint) []StatsPoint {
	if points == nil {
		return []StatsPoint{}
	}
	return points
}

func alwaysItems(items []StatsItem) []StatsItem {
	if items == nil {
		return []StatsItem{}
	}
	return items
}

// Stats aggregates the market statistics for one market and window.
func (p *Postgres) Stats(ctx context.Context, market string, hours int) (Stats, error) {
	if hours < 1 {
		hours = 1
	}
	if hours > 168 {
		hours = 168
	}
	now := time.Now()

	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Stats{}, err
	}
	defer tx.Rollback(ctx)

	var curB, refB pgtype.Timestamptz
	refCutoff := now.Add(-time.Duration(hours) * time.Hour)
	err = tx.QueryRow(ctx,
		`WITH cur AS (SELECT max(bucket) b FROM prices_hourly WHERE market = $1),
		      ref AS (SELECT max(bucket) b FROM prices_hourly WHERE market = $1 AND bucket <= $2)
		 SELECT (SELECT b FROM cur), (SELECT b FROM ref)`,
		market, refCutoff,
	).Scan(&curB, &refB)
	if err != nil {
		return Stats{}, err
	}

	var firstB pgtype.Timestamptz
	if err := tx.QueryRow(ctx,
		`SELECT min(bucket) FROM prices_hourly WHERE market = $1`, market,
	).Scan(&firstB); err != nil {
		return Stats{}, err
	}
	if !curB.Valid || !firstB.Valid {
		// No hourly history for this market: 200 with empty aggregates.
		return BuildStats(StatsBuildInput{
			Market:    market,
			Now:       now,
			Hours:     hours,
			Freshness: statsFreshness(ctx, tx, market, now),
		}), nil
	}

	curBucket := curB.Time.Unix()
	refBucket := int64(0)
	if refB.Valid {
		refBucket = refB.Time.Unix()
	} else {
		// No bucket old enough: fall back to the earliest bucket, exposing the
		// actual age via Window.ReferenceHours.
		refBucket = firstB.Time.Unix()
	}

	current, err := statsBucketItems(ctx, tx, market, curB.Time)
	if err != nil {
		return Stats{}, err
	}
	var reference map[int64]StatsItemAtBucket
	if refBucket > 0 {
		reference, err = statsBucketItems(ctx, tx, market, time.Unix(refBucket, 0))
		if err != nil {
			return Stats{}, err
		}
	}

	ids := make([]int64, 0, len(current))
	seen := make(map[int64]bool, len(current))
	for id := range current {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range reference {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	meta, err := statsMeta(ctx, tx, ids)
	if err != nil {
		return Stats{}, err
	}

	var indexPoints []StatsPoint
	indexMatched := 0
	delisted := int64(0)
	if refBucket > 0 {
		indexPoints, err = statsIndexPoints(ctx, tx, market, time.Unix(refBucket, 0))
		if err != nil {
			return Stats{}, err
		}
		indexMatched, err = statsMatched(ctx, tx, market, time.Unix(refBucket, 0), curB.Time)
		if err != nil {
			return Stats{}, err
		}
		delisted, err = statsDelisted(ctx, tx, market, time.Unix(refBucket, 0), curB.Time)
		if err != nil {
			return Stats{}, err
		}
	}

	newItems, newCount, err := statsNewItems(ctx, tx, market, now.Add(-time.Duration(hours)*time.Hour))
	if err != nil {
		return Stats{}, err
	}

	freshness := statsFreshness(ctx, tx, market, now)

	return BuildStats(StatsBuildInput{
		Market:          market,
		Now:             now,
		Hours:           hours,
		CurrentBucket:   curBucket,
		ReferenceBucket: refBucket,
		FirstBucket:     firstB.Time.Unix(),
		Current:         current,
		Reference:       reference,
		Meta:            meta,
		IndexPoints:     indexPoints,
		IndexMatched:    indexMatched,
		NewItems:        newItems,
		NewItemsCount:   newCount,
		Delisted:        delisted,
		Freshness:       freshness,
	}), nil
}

func statsBucketItems(ctx context.Context, tx pgx.Tx, market string, bucket time.Time) (map[int64]StatsItemAtBucket, error) {
	rows, err := tx.Query(ctx,
		`SELECT split_part(item_key, ':', 2)::bigint AS id,
		        min(min_price) AS price,
		        sum(quantity)::bigint AS qty,
		        count(*)::integer AS offers
		 FROM prices_hourly
		 WHERE market = $1 AND bucket = $2 AND item_key ~ '^[ig]:[0-9]+'
		 GROUP BY 1`,
		market, bucket,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[int64]StatsItemAtBucket{}
	for rows.Next() {
		var id int64
		var item StatsItemAtBucket
		if err := rows.Scan(&id, &item.Price, &item.Quantity, &item.Offers); err != nil {
			return nil, err
		}
		result[id] = item
	}
	return result, rows.Err()
}

func statsMeta(ctx context.Context, tx pgx.Tx, ids []int64) (map[int64]StatsItemMetaInfo, error) {
	result := map[int64]StatsItemMetaInfo{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT item_id, name, quality, class_id FROM items WHERE item_id = ANY($1)`, ids,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var meta StatsItemMetaInfo
		if err := rows.Scan(&id, &meta.Name, &meta.Quality, &meta.ClassID); err != nil {
			return nil, err
		}
		result[id] = meta
	}
	return result, rows.Err()
}

func statsIndexPoints(ctx context.Context, tx pgx.Tx, market string, ref time.Time) ([]StatsPoint, error) {
	rows, err := tx.Query(ctx,
		`WITH base AS (
		        SELECT split_part(item_key, ':', 2)::bigint AS id, min(min_price) AS price
		        FROM prices_hourly
		        WHERE market = $1 AND bucket = $2 AND item_key ~ '^[ig]:[0-9]+'
		        GROUP BY 1
		     ),
		     pts AS (
		        SELECT bucket, split_part(item_key, ':', 2)::bigint AS id, min(min_price) AS price
		        FROM prices_hourly
		        WHERE market = $1 AND bucket >= $2 AND item_key ~ '^[ig]:[0-9]+'
		        GROUP BY 1, 2
		     )
		 SELECT extract(epoch FROM pts.bucket)::bigint,
		        percentile_cont(0.5) WITHIN GROUP (ORDER BY pts.price::numeric / base.price) * 100
		 FROM pts JOIN base USING (id)
		 WHERE base.price >= $3
		 GROUP BY pts.bucket
		 ORDER BY pts.bucket`,
		market, ref, statsPriceFloor,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []StatsPoint{}
	for rows.Next() {
		var point StatsPoint
		if err := rows.Scan(&point.T, &point.Value); err != nil {
			return nil, err
		}
		point.Value = round1(point.Value)
		points = append(points, point)
	}
	return points, rows.Err()
}

func statsMatched(ctx context.Context, tx pgx.Tx, market string, ref, cur time.Time) (int, error) {
	var matched int
	err := tx.QueryRow(ctx,
		`WITH base AS (
		        SELECT split_part(item_key, ':', 2)::bigint AS id, min(min_price) AS price
		        FROM prices_hourly
		        WHERE market = $1 AND bucket = $2 AND item_key ~ '^[ig]:[0-9]+'
		        GROUP BY 1
		     ),
		     cur AS (
		        SELECT split_part(item_key, ':', 2)::bigint AS id, min(min_price) AS price
		        FROM prices_hourly
		        WHERE market = $1 AND bucket = $3 AND item_key ~ '^[ig]:[0-9]+'
		        GROUP BY 1
		     )
		 SELECT count(*)::integer
		 FROM cur JOIN base USING (id)
		 WHERE base.price >= $4 AND cur.price > 0`,
		market, ref, cur, statsPriceFloor,
	).Scan(&matched)
	return matched, err
}

func statsDelisted(ctx context.Context, tx pgx.Tx, market string, ref, cur time.Time) (int64, error) {
	var delisted int64
	err := tx.QueryRow(ctx,
		`SELECT count(*)
		 FROM (SELECT DISTINCT item_key FROM prices_hourly WHERE market = $1 AND bucket = $2) r
		 WHERE NOT EXISTS (
		        SELECT 1 FROM prices_hourly c
		        WHERE c.market = $1 AND c.bucket = $3 AND c.item_key = r.item_key
		 )`,
		market, ref, cur,
	).Scan(&delisted)
	return delisted, err
}

func statsNewItems(ctx context.Context, tx pgx.Tx, market string, since time.Time) ([]StatsItem, int, error) {
	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*)::integer
		 FROM (
		        SELECT item_key
		        FROM prices_hourly
		        WHERE market = $1 AND item_key ~ '^[ig]:[0-9]+'
		        GROUP BY 1
		        HAVING min(bucket) >= $2
		 ) f`,
		market, since,
	).Scan(&count); err != nil {
		return nil, 0, err
	}

	rows, err := tx.Query(ctx,
		`WITH firsts AS (
		        SELECT item_key, min(bucket) AS first_bucket
		        FROM prices_hourly
		        WHERE market = $1 AND item_key ~ '^[ig]:[0-9]+'
		        GROUP BY 1
		        HAVING min(bucket) >= $2
		 )
		 SELECT split_part(f.item_key, ':', 2)::bigint,
		        f.item_key,
		        COALESCE(i.name, ''),
		        COALESCE(i.quality, 0),
		        COALESCE(pl.price, 0),
		        COALESCE(pl.quantity, 0)
		 FROM firsts f
		 LEFT JOIN items i ON i.item_id = split_part(f.item_key, ':', 2)::bigint
		 LEFT JOIN prices_latest pl ON pl.market = $1 AND pl.item_key = f.item_key
		 ORDER BY COALESCE(pl.price, 0) * COALESCE(pl.quantity, 0) DESC, f.item_key
		 LIMIT 8`,
		market, since,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []StatsItem{}
	for rows.Next() {
		var item StatsItem
		if err := rows.Scan(&item.ItemID, &item.Key, &item.Name, &item.Quality, &item.Price, &item.Quantity); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, count, rows.Err()
}

func statsFreshness(ctx context.Context, tx pgx.Tx, market string, now time.Time) StatsFreshness {
	var fresh StatsFreshness
	var last pgtype.Timestamptz
	err := tx.QueryRow(ctx,
		`SELECT count(*)::integer,
		        count(DISTINCT client_id)::integer,
		        max(scanned_at)
		 FROM scans
		 WHERE market = $1 AND scanned_at >= $2`,
		market, now.Add(-24*time.Hour),
	).Scan(&fresh.Scans24h, &fresh.Clients24h, &last)
	if err != nil {
		return StatsFreshness{}
	}
	if last.Valid {
		fresh.LastScan = last.Time.Unix()
		fresh.LastScanAgeMin = round1(now.Sub(last.Time).Minutes())
	}
	return fresh
}
