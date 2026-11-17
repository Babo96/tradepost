package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

type stateFile struct {
	Server   string            `json:"server"`
	ClientID string            `json:"client_id"`
	Uploaded []string          `json:"uploaded,omitempty"`
	Items    map[string]string `json:"items,omitempty"`
}

func statePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "babo-forever", "state.json")
}

func readState() stateFile {
	var state stateFile
	data, err := os.ReadFile(statePath())
	if err != nil {
		return state
	}
	_ = json.Unmarshal(data, &state)
	return state
}

func writeState(state stateFile) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	path := statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// stateFor returns the state for one server/client pair. Changing server or
// re-enrolling starts a fresh upload and metadata state.
func stateFor(server, clientID string) stateFile {
	state := readState()
	if state.Server != server || state.ClientID != clientID {
		return stateFile{Server: server, ClientID: clientID}
	}
	return state
}

func loadUploaded(server, clientID string) map[string]bool {
	result := map[string]bool{}
	for _, hash := range stateFor(server, clientID).Uploaded {
		result[hash] = true
	}
	return result
}

func saveUploaded(server, clientID string, uploaded map[string]bool) error {
	state := stateFor(server, clientID)
	hashes := make([]string, 0, len(uploaded))
	for hash := range uploaded {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	if len(hashes) > 5000 {
		hashes = hashes[len(hashes)-5000:]
	}
	state.Uploaded = hashes
	return writeState(state)
}

func loadMeta(server, clientID string) map[string]string {
	result := map[string]string{}
	for id, hash := range stateFor(server, clientID).Items {
		result[id] = hash
	}
	return result
}

func saveMeta(server, clientID string, items map[string]string) error {
	state := stateFor(server, clientID)
	state.Items = items
	return writeState(state)
}
