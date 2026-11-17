package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type stateFile struct {
	Server   string   `json:"server"`
	ClientID string   `json:"client_id"`
	Uploaded []string `json:"uploaded"`
}

func statePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "babo-forever", "state.json")
}

func loadUploaded(server, clientID string) map[string]bool {
	result := map[string]bool{}
	data, err := os.ReadFile(statePath())
	if err != nil {
		return result
	}
	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return result
	}
	if state.Server != server || state.ClientID != clientID {
		return result
	}
	for _, hash := range state.Uploaded {
		result[hash] = true
	}
	return result
}

func saveUploaded(server, clientID string, uploaded map[string]bool) error {
	state := stateFile{Server: server, ClientID: clientID}
	for hash := range uploaded {
		state.Uploaded = append(state.Uploaded, hash)
	}
	if len(state.Uploaded) > 2000 {
		state.Uploaded = state.Uploaded[len(state.Uploaded)-2000:]
	}
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
