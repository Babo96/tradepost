package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) UpsertItems(ctx context.Context, items []ItemMeta) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for _, item := range items {
		batch.Queue(
			`INSERT INTO items (item_id, name, quality, class_id, subclass_id, icon,
			                    item_level, required_level, bind_type, inventory_type, equip_loc,
			                    armor, damage_min, damage_max, delay, vendor_price, max_stack,
			                    unique_item, set_id, stats, details, details_version, details_updated, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6,
			         $7, $8, $9, $10, $11,
			         $12, $13, $14, $15, $16, $17,
			         $18, $19, $20::jsonb, $21::jsonb,
			         CASE WHEN $21::jsonb IS NOT NULL THEN 1 ELSE 0 END,
			         CASE WHEN $21::jsonb IS NOT NULL THEN now() ELSE NULL END,
			         now())
			 ON CONFLICT (item_id) DO UPDATE SET
			   name = EXCLUDED.name,
			   quality = EXCLUDED.quality,
			   class_id = EXCLUDED.class_id,
			   subclass_id = EXCLUDED.subclass_id,
			   icon = EXCLUDED.icon,
			   item_level = COALESCE(EXCLUDED.item_level, items.item_level),
			   required_level = COALESCE(EXCLUDED.required_level, items.required_level),
			   bind_type = COALESCE(EXCLUDED.bind_type, items.bind_type),
			   inventory_type = COALESCE(EXCLUDED.inventory_type, items.inventory_type),
			   equip_loc = COALESCE(EXCLUDED.equip_loc, items.equip_loc),
			   armor = COALESCE(EXCLUDED.armor, items.armor),
			   damage_min = COALESCE(EXCLUDED.damage_min, items.damage_min),
			   damage_max = COALESCE(EXCLUDED.damage_max, items.damage_max),
			   delay = COALESCE(EXCLUDED.delay, items.delay),
			   vendor_price = COALESCE(EXCLUDED.vendor_price, items.vendor_price),
			   max_stack = COALESCE(EXCLUDED.max_stack, items.max_stack),
			   unique_item = COALESCE(EXCLUDED.unique_item, items.unique_item),
			   set_id = COALESCE(EXCLUDED.set_id, items.set_id),
			   stats = COALESCE(EXCLUDED.stats, items.stats),
			   details = COALESCE(EXCLUDED.details, items.details),
			   details_version = items.details_version
			     + CASE WHEN EXCLUDED.details IS NOT NULL THEN 1 ELSE 0 END,
			   details_updated = CASE WHEN EXCLUDED.details IS NOT NULL THEN now() ELSE items.details_updated END,
			   updated_at = now()`,
			item.ID, item.Name, item.Quality, item.ClassID, item.SubclassID, item.Icon,
			item.ItemLevel, item.RequiredLevel, item.BindType, item.InventoryType, item.EquipLoc,
			item.Armor, item.DamageMin, item.DamageMax, item.Delay, item.VendorPrice, item.MaxStack,
			item.UniqueItem, item.SetID, jsonParam(item.Stats), jsonParam(item.Details),
		)
	}
	results := tx.SendBatch(ctx, batch)
	for range items {
		if _, err := results.Exec(); err != nil {
			results.Close()
			return err
		}
	}
	if err := results.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func jsonParam(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}

func (p *Postgres) SearchItems(ctx context.Context, market string, q SearchQuery) (SearchPage, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 50
	}
	if q.Limit > 200 {
		q.Limit = 200
	}

	where, args := searchWhere(market, q)
	from := `FROM prices_latest p
	         LEFT JOIN items i
	           ON i.item_id = NULLIF(split_part(p.item_key, ':', 2), '')::bigint`

	var total int64
	if err := p.pool.QueryRow(ctx,
		`SELECT count(*) `+from+` WHERE `+where, args...,
	).Scan(&total); err != nil {
		return SearchPage{}, err
	}

	order := searchOrder(q.Sort)
	offset := (q.Page - 1) * q.Limit
	args = append(args, q.Limit, offset)
	limitIdx := len(args) - 1
	offsetIdx := len(args)

	rows, err := p.pool.Query(ctx,
		fmt.Sprintf(
			`SELECT p.item_key,
			        COALESCE(NULLIF(split_part(p.item_key, ':', 2), '')::bigint, 0),
			        COALESCE(i.name, ''),
			        COALESCE(i.quality, 1),
			        p.price, p.quantity, p.listings,
			        extract(epoch FROM p.scanned_at)::bigint,
			        i.item_level, i.required_level, i.bind_type, i.inventory_type,
			        i.equip_loc, i.armor, i.damage_min, i.damage_max, i.delay,
			        i.vendor_price, i.max_stack, i.unique_item, i.set_id,
			        (i.details IS NOT NULL)
			 %s WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`,
			from, where, order, limitIdx, offsetIdx,
		),
		args...,
	)
	if err != nil {
		return SearchPage{}, err
	}
	defer rows.Close()

	items := make([]SearchResult, 0, q.Limit)
	for rows.Next() {
		item, err := scanSearchResult(rows)
		if err != nil {
			return SearchPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return SearchPage{}, err
	}
	return SearchPage{Items: items, Total: total}, nil
}

func searchWhere(market string, q SearchQuery) (string, []any) {
	args := []any{market}
	clauses := []string{"p.market = $1"}
	next := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if query := strings.TrimSpace(q.Query); query != "" {
		placeholder := next(query)
		clauses = append(clauses, fmt.Sprintf(
			`(p.item_key = %s OR i.name ILIKE '%%' || %s || '%%' OR split_part(p.item_key, ':', 2) = %s)`,
			placeholder, placeholder, placeholder,
		))
	}
	if len(q.Qualities) > 0 {
		ors := make([]string, 0, len(q.Qualities))
		for _, quality := range q.Qualities {
			ors = append(ors, "i.quality = "+next(quality))
		}
		clauses = append(clauses, "("+strings.Join(ors, " OR ")+")")
	}
	if q.ClassID != nil {
		clauses = append(clauses, "i.class_id = "+next(*q.ClassID))
	}
	if q.SubclassID != nil {
		clauses = append(clauses, "i.subclass_id = "+next(*q.SubclassID))
	}
	if q.InventoryType != nil {
		clauses = append(clauses, "i.inventory_type = "+next(*q.InventoryType))
	}
	if q.MinIlvl != nil {
		clauses = append(clauses, "i.item_level >= "+next(*q.MinIlvl))
	}
	if q.MaxIlvl != nil {
		clauses = append(clauses, "i.item_level <= "+next(*q.MaxIlvl))
	}
	if q.MinReqLevel != nil {
		clauses = append(clauses, "i.required_level >= "+next(*q.MinReqLevel))
	}
	if q.MaxReqLevel != nil {
		clauses = append(clauses, "i.required_level <= "+next(*q.MaxReqLevel))
	}
	if q.MinPrice != nil {
		clauses = append(clauses, "p.price >= "+next(*q.MinPrice))
	}
	if q.MaxPrice != nil {
		clauses = append(clauses, "p.price <= "+next(*q.MaxPrice))
	}
	if q.InStock {
		clauses = append(clauses, "p.quantity > 0")
	}
	return strings.Join(clauses, " AND "), args
}

func searchOrder(sortKey string) string {
	switch sortKey {
	case "name":
		return "i.name ASC NULLS LAST, p.item_key ASC"
	case "ilvl":
		return "i.item_level DESC NULLS LAST, p.item_key ASC"
	case "quality":
		return "i.quality DESC NULLS LAST, p.item_key ASC"
	case "scanned":
		return "p.scanned_at DESC, p.item_key ASC"
	default:
		return "p.price ASC, i.name ASC, p.item_key ASC"
	}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSearchResult(row rowScanner) (SearchResult, error) {
	var item SearchResult
	var itemLevel, requiredLevel, bindType, inventoryType sql.NullInt64
	var equipLoc sql.NullString
	var armor, damageMin, damageMax, delay, maxStack, setID sql.NullInt64
	var vendorPrice sql.NullInt64
	var uniqueItem sql.NullBool
	if err := row.Scan(
		&item.Key, &item.ItemID, &item.Name, &item.Quality,
		&item.Price, &item.Quantity, &item.Listings, &item.Scanned,
		&itemLevel, &requiredLevel, &bindType, &inventoryType,
		&equipLoc, &armor, &damageMin, &damageMax, &delay,
		&vendorPrice, &maxStack, &uniqueItem, &setID,
		&item.HasDetails,
	); err != nil {
		return SearchResult{}, err
	}
	item.ItemLevel = nullInt(itemLevel)
	item.RequiredLevel = nullInt(requiredLevel)
	item.BindType = nullInt(bindType)
	item.InventoryType = nullInt(inventoryType)
	if equipLoc.Valid {
		item.EquipLoc = equipLoc.String
	}
	item.Armor = nullInt(armor)
	item.DamageMin = nullInt(damageMin)
	item.DamageMax = nullInt(damageMax)
	item.Delay = nullInt(delay)
	if vendorPrice.Valid {
		value := vendorPrice.Int64
		item.VendorPrice = &value
	}
	item.MaxStack = nullInt(maxStack)
	if uniqueItem.Valid {
		value := uniqueItem.Bool
		item.UniqueItem = &value
	}
	item.SetID = nullInt(setID)
	return item, nil
}

func nullInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}

func (p *Postgres) MissingItemDetails(ctx context.Context, market string, force bool, limit int) ([]int64, error) {
	if limit < 1 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	query := `SELECT DISTINCT split_part(p.item_key, ':', 2)::bigint AS item_id
	          FROM prices_latest p
	          LEFT JOIN items i
	            ON i.item_id = NULLIF(split_part(p.item_key, ':', 2), '')::bigint
	          WHERE p.market = $1 AND split_part(p.item_key, ':', 2) ~ '^[0-9]+$'`
	if !force {
		query += ` AND i.details IS NULL`
	}
	query += ` ORDER BY item_id LIMIT $2`

	rows, err := p.pool.Query(ctx, query, market, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (p *Postgres) ItemDetail(ctx context.Context, market, key string) (ItemDetail, error) {
	var detail ItemDetail
	var itemLevel, requiredLevel, bindType, inventoryType sql.NullInt64
	var equipLoc sql.NullString
	var armor, damageMin, damageMax, delay, maxStack, setID sql.NullInt64
	var vendorPrice sql.NullInt64
	var uniqueItem sql.NullBool
	var stats, details []byte
	var detailsUpdated sql.NullTime

	err := p.pool.QueryRow(ctx,
		`SELECT item_id, name, quality, class_id, subclass_id, icon,
		        item_level, required_level, bind_type, inventory_type, equip_loc,
		        armor, damage_min, damage_max, delay, vendor_price, max_stack,
		        unique_item, set_id, stats, details, details_version, details_updated
		 FROM items WHERE item_id = $1`,
		ItemID(key),
	).Scan(
		&detail.ID, &detail.Name, &detail.Quality, &detail.ClassID, &detail.SubclassID, &detail.Icon,
		&itemLevel, &requiredLevel, &bindType, &inventoryType, &equipLoc,
		&armor, &damageMin, &damageMax, &delay, &vendorPrice, &maxStack,
		&uniqueItem, &setID, &stats, &details, &detail.DetailsVersion, &detailsUpdated,
	)
	if err == pgx.ErrNoRows {
		return ItemDetail{}, ErrNotFound
	}
	if err != nil {
		return ItemDetail{}, err
	}
	detail.ItemLevel = nullInt(itemLevel)
	detail.RequiredLevel = nullInt(requiredLevel)
	detail.BindType = nullInt(bindType)
	detail.InventoryType = nullInt(inventoryType)
	if equipLoc.Valid {
		detail.EquipLoc = &equipLoc.String
	}
	detail.Armor = nullInt(armor)
	detail.DamageMin = nullInt(damageMin)
	detail.DamageMax = nullInt(damageMax)
	detail.Delay = nullInt(delay)
	if vendorPrice.Valid {
		value := vendorPrice.Int64
		detail.VendorPrice = &value
	}
	detail.MaxStack = nullInt(maxStack)
	if uniqueItem.Valid {
		value := uniqueItem.Bool
		detail.UniqueItem = &value
	}
	detail.SetID = nullInt(setID)
	detail.Stats = json.RawMessage(stats)
	detail.Details = json.RawMessage(details)
	if detailsUpdated.Valid {
		detail.DetailsUpdated = detailsUpdated.Time.Unix()
	}

	err = p.pool.QueryRow(ctx,
		`SELECT price, quantity, listings, extract(epoch FROM scanned_at)::bigint
		 FROM prices_latest WHERE market = $1 AND item_key = $2`,
		market, key,
	).Scan(&detail.Price, &detail.Quantity, &detail.Listings, &detail.Scanned)
	if err != nil && err != pgx.ErrNoRows {
		return ItemDetail{}, err
	}
	return detail, nil
}

func (p *Postgres) ItemHistory(ctx context.Context, market, key string, limit int) (History, error) {
	return p.HistoryPoints(ctx, market, key, "hour", time.Unix(0, 0), limit)
}

func (p *Postgres) HistoryPoints(ctx context.Context, market, key, resolution string, since time.Time, limit int) (History, error) {
	history := History{Market: market, Key: key, Resolution: resolution, Points: []HistoryPoint{}}
	if since.IsZero() {
		since = time.Unix(0, 0)
	}
	if limit < 1 {
		limit = 1000
	}

	itemID := ItemID(key)
	if itemID > 0 {
		_ = p.pool.QueryRow(ctx,
			`SELECT name FROM items WHERE item_id = $1`, itemID,
		).Scan(&history.Name)
	}

	var query string
	switch resolution {
	case "1m":
		query = `SELECT extract(epoch FROM bucket)::bigint, min_price, max_price, quantity, 0, samples
		         FROM prices_1m
		         WHERE market = $1 AND item_key = $2 AND bucket >= $3
		         ORDER BY bucket DESC
		         LIMIT $4`
	case "15m":
		query = `SELECT extract(epoch FROM bucket)::bigint, min_price, max_price, quantity, 0, samples
		         FROM prices_15m
		         WHERE market = $1 AND item_key = $2 AND bucket >= $3
		         ORDER BY bucket DESC
		         LIMIT $4`
	default:
		query = `SELECT extract(epoch FROM bucket)::bigint, min_price, max_price, quantity, 0, samples
		         FROM prices_hourly
		         WHERE market = $1 AND item_key = $2 AND bucket >= $3
		         ORDER BY bucket DESC
		         LIMIT $4`
	}

	rows, err := p.pool.Query(ctx, query, market, key, since, limit)
	if err != nil {
		return History{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var point HistoryPoint
		if err := rows.Scan(&point.Bucket, &point.Min, &point.Max, &point.Quantity, &point.Listings, &point.Samples); err != nil {
			return History{}, err
		}
		history.Points = append(history.Points, point)
	}
	if err := rows.Err(); err != nil {
		return History{}, err
	}
	for i, j := 0, len(history.Points)-1; i < j; i, j = i+1, j-1 {
		history.Points[i], history.Points[j] = history.Points[j], history.Points[i]
	}
	return history, nil
}

func (p *Postgres) CleanupHistory(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx,
		`DELETE FROM prices_1m WHERE bucket < now() - interval '12 hours'`,
	); err != nil {
		return err
	}
	_, err := p.pool.Exec(ctx,
		`DELETE FROM prices_15m WHERE bucket < now() - interval '7 days'`,
	)
	return err
}

func (p *Postgres) ItemMarkets(ctx context.Context, key string) ([]MarketPrice, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT market, price, quantity, listings, extract(epoch FROM scanned_at)::bigint
		 FROM prices_latest
		 WHERE item_key = $1
		 ORDER BY price ASC`,
		key,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []MarketPrice{}
	for rows.Next() {
		var price MarketPrice
		if err := rows.Scan(&price.Market, &price.Price, &price.Quantity, &price.Listings, &price.Scanned); err != nil {
			return nil, err
		}
		result = append(result, price)
	}
	return result, rows.Err()
}
