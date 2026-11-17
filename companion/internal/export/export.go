package export

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

type Item struct {
	Key      string
	Price    int64
	Quantity int64
	Listings int32
}

type Scan struct {
	Market    string
	ScannedAt int64
	Items     []Item
	Hash      string
}

// Meta is the client-side item cache uploaded separately from price scans.
type Meta struct {
	ID         int64
	Name       string
	Quality    int
	ClassID    int
	SubclassID int
	Icon       int64
}

func (m Meta) Hash() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d|%d|%d|%d",
		m.ID, m.Name, m.Quality, m.ClassID, m.SubclassID, m.Icon)))
	return hex.EncodeToString(sum[:8])
}

type Data struct {
	Scans []Scan
	Items []Meta
}

func Load(path string) ([]Scan, error) {
	data, err := LoadData(path)
	if err != nil {
		return nil, err
	}
	return data.Scans, nil
}

func LoadData(path string) (Data, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return Data{}, nil
		}
		return Data{}, err
	}

	L := lua.NewState()
	defer L.Close()
	if err := L.DoFile(path); err != nil {
		return Data{}, fmt.Errorf("parse SavedVariables: %w", err)
	}

	data := Data{Scans: loadScans(L), Items: loadItems(L)}
	sort.Slice(data.Items, func(i, j int) bool { return data.Items[i].ID < data.Items[j].ID })
	return data, nil
}

func loadScans(L *lua.LState) []Scan {
	value := L.GetGlobal("BABOFOREVER_EXPORT")
	table, ok := value.(*lua.LTable)
	if !ok {
		return nil
	}

	scansValue := table.RawGetString("scans")
	scansTable, ok := scansValue.(*lua.LTable)
	if !ok {
		return nil
	}

	var result []Scan
	scansTable.ForEach(func(_, value lua.LValue) {
		record, ok := value.(lua.LString)
		if !ok {
			return
		}
		scan, err := parseRecord(string(record))
		if err == nil && len(scan.Items) > 0 {
			result = append(result, scan)
		}
	})
	return result
}

func loadItems(L *lua.LState) []Meta {
	value := L.GetGlobal("BABOFOREVER_ITEMS")
	table, ok := value.(*lua.LTable)
	if !ok {
		return nil
	}
	var result []Meta
	table.ForEach(func(key, value lua.LValue) {
		id := int64(lua.LVAsNumber(key))
		fields, ok := value.(*lua.LTable)
		if id <= 0 || !ok {
			return
		}
		name := lua.LVAsString(fields.RawGetString("n"))
		if name == "" {
			return
		}
		result = append(result, Meta{
			ID:         id,
			Name:       name,
			Quality:    int(lua.LVAsNumber(fields.RawGetString("q"))),
			ClassID:    int(lua.LVAsNumber(fields.RawGetString("c"))),
			SubclassID: int(lua.LVAsNumber(fields.RawGetString("s"))),
			Icon:       int64(lua.LVAsNumber(fields.RawGetString("i"))),
		})
	})
	return result
}

func parseRecord(record string) (Scan, error) {
	parts := strings.SplitN(record, "|", 3)
	if len(parts) != 3 {
		return Scan{}, fmt.Errorf("invalid scan header")
	}
	scannedAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Scan{}, fmt.Errorf("invalid scan time: %w", err)
	}
	sum := sha256.Sum256([]byte(record))
	scan := Scan{Market: parts[0], ScannedAt: scannedAt, Hash: hex.EncodeToString(sum[:])}

	for _, chunk := range strings.Split(parts[2], ";") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		fields := strings.Split(chunk, ",")
		if len(fields) != 4 {
			return Scan{}, fmt.Errorf("invalid item record: %q", chunk)
		}
		price, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return Scan{}, err
		}
		quantity, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return Scan{}, err
		}
		listings, err := strconv.ParseInt(fields[3], 10, 32)
		if err != nil {
			return Scan{}, err
		}
		scan.Items = append(scan.Items, Item{
			Key:      fields[0],
			Price:    price,
			Quantity: quantity,
			Listings: int32(listings),
		})
	}
	return scan, nil
}
