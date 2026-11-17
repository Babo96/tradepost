package export

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
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

func Load(path string) ([]Scan, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	L := lua.NewState()
	defer L.Close()
	if err := L.DoFile(path); err != nil {
		return nil, fmt.Errorf("parse SavedVariables: %w", err)
	}

	value := L.GetGlobal("BABOFOREVER_EXPORT")
	table, ok := value.(*lua.LTable)
	if !ok {
		return nil, nil
	}

	scansValue := table.RawGetString("scans")
	scansTable, ok := scansValue.(*lua.LTable)
	if !ok {
		return nil, nil
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
	return result, nil
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
