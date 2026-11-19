package wow

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const foreverDir = "_forever_"

// Candidates returns every path discovery would try, in priority order. It
// includes paths that do not exist so the setup wizard can show what was
// checked.
func Candidates(explicit string) []string {
	candidates := []string{}
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if env := os.Getenv("WOW_DIR"); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, defaultCandidates()...)

	unique := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, raw := range candidates {
		candidate := normalizeRoot(raw)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		unique = append(unique, candidate)
	}
	return unique
}

// Detect returns the candidates that contain a _forever_ install.
func Detect(explicit string) []string {
	found := []string{}
	for _, candidate := range Candidates(explicit) {
		if info, err := os.Stat(filepath.Join(candidate, foreverDir)); err == nil && info.IsDir() {
			found = append(found, candidate)
		}
	}
	return found
}

func Find(explicit string) (string, error) {
	tried := Candidates(explicit)
	for _, candidate := range tried {
		if _, err := os.Stat(filepath.Join(candidate, foreverDir)); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find a WoW install with %s (tried %s; set --wow or WOW_DIR)",
		foreverDir, strings.Join(tried, ", "))
}

// normalizeRoot accepts either the game root or its _forever_ directory and
// returns the root that contains _forever_ (and the launcher files).
func normalizeRoot(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimRight(path, `/\`)
	if path == "" {
		return ""
	}
	trimmed := strings.TrimRight(path, `/\`)
	index := strings.LastIndexAny(trimmed, `/\`)
	base := trimmed
	if index >= 0 {
		base = trimmed[index+1:]
	}
	if strings.EqualFold(base, foreverDir) {
		if index > 0 {
			return strings.TrimRight(trimmed[:index], `/\`)
		}
		return "."
	}
	return path
}

func AddonsDir(wow string) string {
	return filepath.Join(wow, foreverDir, "Interface", "AddOns")
}

// InstallInfo describes a validated WoW Forever install.
type InstallInfo struct {
	Root      string   `json:"root"`
	Forever   string   `json:"forever_dir"`
	AddonsDir string   `json:"addons_dir"`
	Accounts  []string `json:"accounts"`
	Writable  bool     `json:"writable"`
}

// Validate checks that path points at a usable WoW Forever install: the
// _forever_/Interface/AddOns layout exists, at least one account directory is
// present, and the addon folder is writable. The returned Root is normalized,
// so passing a path ending in _forever_ is accepted.
func Validate(path string) (InstallInfo, error) {
	root := normalizeRoot(path)
	if root == "" {
		return InstallInfo{}, errors.New("no game folder given")
	}
	forever := filepath.Join(root, foreverDir)
	if info, err := os.Stat(forever); err != nil || !info.IsDir() {
		return InstallInfo{Root: root}, fmt.Errorf("not a WoW Forever install: missing %s", forever)
	}
	addons := AddonsDir(root)
	if info, err := os.Stat(addons); err != nil || !info.IsDir() {
		return InstallInfo{Root: root}, fmt.Errorf("missing addon folder %s", addons)
	}
	accounts, err := AccountDirs(root)
	if err != nil {
		return InstallInfo{Root: root, Forever: forever, AddonsDir: addons}, fmt.Errorf("account folders: %w", err)
	}
	result := InstallInfo{
		Root:      root,
		Forever:   forever,
		AddonsDir: addons,
		Accounts:  accounts,
	}
	probe, err := os.CreateTemp(addons, ".tradepost-write-test-*")
	if err != nil {
		return result, fmt.Errorf("addon folder is not writable: %w", err)
	}
	probeName := probe.Name()
	probe.Close()
	if err := os.Remove(probeName); err != nil {
		return result, fmt.Errorf("addon folder is not writable: %w", err)
	}
	result.Writable = true
	return result, nil
}

func AccountDirs(wow string) ([]string, error) {
	root := filepath.Join(wow, foreverDir, "WTF", "Account")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "SavedVariables" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(path, "SavedVariables")); err == nil {
			dirs = append(dirs, path)
		}
	}
	if len(dirs) == 0 {
		return nil, errors.New("no WoW account directories found")
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i] < dirs[j]
	})
	return dirs, nil
}

func SavedVariablesFile(wow, account string) string {
	if filepath.IsAbs(account) {
		return filepath.Join(account, "SavedVariables", "TradePost.lua")
	}
	return filepath.Join(wow, foreverDir, "WTF", "Account", account, "SavedVariables", "TradePost.lua")
}

func WriteAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

const AuthSlots = 16
const SyncSlots = 8

// MetaAckSlots is the number of LoadOnDemand addons the addon rotates through
// while waiting for an acknowledgement of a metadata batch.
const MetaAckSlots = 16

func PriceSlots(ringSlots int) int {
	if ringSlots <= SyncSlots {
		return ringSlots
	}
	return ringSlots - SyncSlots
}

func SyncSlotIndex(ringSlots, index int) int {
	if ringSlots <= SyncSlots {
		return index % ringSlots
	}
	return PriceSlots(ringSlots) + (index % SyncSlots)
}

func AuthSlotName(index int) string {
	return fmt.Sprintf("TradePost_Auth_%02d", index)
}

func AuthSlotPath(wow string, index int) string {
	return filepath.Join(AddonsDir(wow), AuthSlotName(index), "Data.lua")
}

// EnsureAuthSlots creates the load-on-demand addons the companion uses to
// return a short login code to the game. Existing data is never overwritten.
func EnsureAuthSlots(wow string) error {
	addons := AddonsDir(wow)
	if _, err := os.Stat(addons); err != nil {
		return fmt.Errorf("addons directory: %w", err)
	}
	for index := 0; index < AuthSlots; index++ {
		name := AuthSlotName(index)
		dir := filepath.Join(addons, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		tocPath := filepath.Join(dir, name+".toc")
		if _, err := os.Stat(tocPath); os.IsNotExist(err) {
			toc := fmt.Sprintf(`## Interface: 16001
## Title: Sync %02d
## Group: TradePost_LiveData
## Category: TradePost
## Notes: Generated by the TradePost Companion. Do not edit.
## Author: babo
## Version: 0.1.0
## LoadOnDemand: 1
## X-Category: Auction/Economy

Data.lua
`, index)
			if err := os.WriteFile(tocPath, []byte(toc), 0o644); err != nil {
				return err
			}
		}
		dataPath := filepath.Join(dir, "Data.lua")
		if _, err := os.Stat(dataPath); os.IsNotExist(err) {
			placeholder := "-- Generated login-code slot. The companion rewrites this file.\nTRADEPOST_AUTH_REPLY = nil\n"
			if err := os.WriteFile(dataPath, []byte(placeholder), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func WriteAuthReply(wow string, seq int64, code, character string, expires int64) error {
	content := fmt.Sprintf(`TRADEPOST_AUTH_REPLY = {
  seq = %d,
  code = %q,
  character = %q,
  expires = %d,
}
`, seq, code, character, expires)
	// The addon cannot know which slot is still unloaded, so publish the same
	// reply to every slot. It consumes at most one unloaded slot per poll.
	for index := 0; index < AuthSlots; index++ {
		if err := WriteAtomic(AuthSlotPath(wow, index), []byte(content)); err != nil {
			return err
		}
	}
	return nil
}

func MetaAckSlotName(index int) string {
	return fmt.Sprintf("TradePost_Ack_%02d", index)
}

func MetaAckSlotPath(wow string, index int) string {
	return filepath.Join(AddonsDir(wow), MetaAckSlotName(index), "Data.lua")
}

// EnsureMetaAckSlots creates the load-on-demand acknowledgement addons used by
// the metadata backfill. They are grouped with the live data addons. Existing
// Data.lua files are never overwritten, so an acknowledgement pending across a
// restart survives.
func EnsureMetaAckSlots(wow string) error {
	addons := AddonsDir(wow)
	if _, err := os.Stat(addons); err != nil {
		return fmt.Errorf("addons directory: %w", err)
	}
	for index := 0; index < MetaAckSlots; index++ {
		name := MetaAckSlotName(index)
		dir := filepath.Join(addons, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		tocPath := filepath.Join(dir, name+".toc")
		if _, err := os.Stat(tocPath); os.IsNotExist(err) {
			toc := fmt.Sprintf(`## Interface: 16001
## Title: Meta Ack %02d
## Group: TradePost_LiveData
## Category: TradePost
## Notes: Generated by the TradePost Companion. Do not edit.
## Author: babo
## Version: 0.1.0
## LoadOnDemand: 1
## X-Category: Auction/Economy

Data.lua
`, index)
			if err := os.WriteFile(tocPath, []byte(toc), 0o644); err != nil {
				return err
			}
		}
		dataPath := filepath.Join(dir, "Data.lua")
		if _, err := os.Stat(dataPath); os.IsNotExist(err) {
			placeholder := "-- Generated metadata acknowledgement slot. The companion rewrites this file.\nTRADEPOST_META_ACK = nil\n"
			if err := os.WriteFile(dataPath, []byte(placeholder), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteMetaAck acknowledges a metadata batch. The addon cannot know which slot
// is still unloaded and a slot can only be loaded once per client session, so
// every slot gets the current acknowledgement. The addon consumes exactly one
// unloaded slot per batch, which keeps the 16-slot rotation race-free.
func WriteMetaAck(wow string, batch int64) error {
	content := fmt.Sprintf(`TRADEPOST_META_ACK = {
  batch = %d,
  ok = true,
}
`, batch)
	for index := 0; index < MetaAckSlots; index++ {
		if err := WriteAtomic(MetaAckSlotPath(wow, index), []byte(content)); err != nil {
			return err
		}
	}
	return nil
}

func Install(source, wow string, count int) error {
	if count < 1 || count > 256 {
		return errors.New("ring slot count out of range")
	}
	addons := AddonsDir(wow)
	if _, err := os.Stat(addons); err != nil {
		return fmt.Errorf("addons directory: %w", err)
	}

	if err := copyDir(filepath.Join(source, "addon", "TradePost"), filepath.Join(addons, "TradePost")); err != nil {
		return err
	}
	if err := copyDir(filepath.Join(source, "data-addon", "TradePost_Data"), filepath.Join(addons, "TradePost_Data")); err != nil {
		return err
	}
	if err := copyDir(filepath.Join(source, "data-addon", "TradePost_LiveData"), filepath.Join(addons, "TradePost_LiveData")); err != nil {
		return err
	}
	if err := EnsureAuthSlots(wow); err != nil {
		return err
	}
	if err := EnsureMetaAckSlots(wow); err != nil {
		return err
	}

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("TradePost_Data_%02d", i)
		path := filepath.Join(addons, name)
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
		toc := fmt.Sprintf(`## Interface: 16001
## Title: Live Data %02d
## Group: TradePost_LiveData
## Category: TradePost
## Notes: Generated by the TradePost Companion. Do not edit.
## Author: babo
## Version: 0.1.0
## LoadOnDemand: 1
## X-Category: Auction/Economy

Data.lua
`, i)
		if err := os.WriteFile(filepath.Join(path, name+".toc"), []byte(toc), 0o644); err != nil {
			return err
		}
		placeholder := `-- Generated live ring slot. The companion rewrites this file.
TRADEPOST_RING = {
  schema = 1,
  market = "",
  window = 0,
  generated = 0,
  base = 0,
  seq = 0,
  items = {},
  removed = {},
  pins = {},
}
`
		if err := os.WriteFile(filepath.Join(path, "Data.lua"), []byte(placeholder), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("addon source %s: %w", src, err)
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

func SlotName(index int) string {
	return fmt.Sprintf("TradePost_Data_%02d", index)
}

func SlotPath(wow string, index int) string {
	return filepath.Join(AddonsDir(wow), SlotName(index), "Data.lua")
}

func DataAddonPath(wow string) string {
	return filepath.Join(AddonsDir(wow), "TradePost_Data", "Data.lua")
}

func MarketSlug(market string) string {
	return strings.ReplaceAll(market, ":", "_")
}
