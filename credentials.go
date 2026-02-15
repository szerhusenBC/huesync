package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// BridgeCredentials holds the API credentials for a paired Hue bridge.
type BridgeCredentials struct {
	Username  string `json:"username"`
	Clientkey string `json:"clientkey"`
}

// credentialsDir overrides the default credentials directory for testing.
// When empty, the user's home directory is used.
var credentialsDir string

func credentialsPath() (string, error) {
	if credentialsDir != "" {
		return filepath.Join(credentialsDir, "credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".huesync", "credentials.json"), nil
}

func readAllCredentials(path string) (map[string]BridgeCredentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var creds map[string]BridgeCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return creds, nil
}

// LoadCredentials loads the stored credentials for the given bridge ID.
// Returns false with no error if no credentials are found.
func LoadCredentials(bridgeID string) (BridgeCredentials, bool, error) {
	path, err := credentialsPath()
	if err != nil {
		return BridgeCredentials{}, false, err
	}

	creds, err := readAllCredentials(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BridgeCredentials{}, false, nil
		}
		return BridgeCredentials{}, false, nil
	}

	bc, ok := creds[bridgeID]
	if !ok {
		return BridgeCredentials{}, false, nil
	}
	return bc, true, nil
}

// SaveCredentials persists the credentials for the given bridge ID.
// Creates the credentials directory with 0700 if needed.
func SaveCredentials(bridgeID string, creds BridgeCredentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	all, err := readAllCredentials(path)
	if err != nil || all == nil {
		all = make(map[string]BridgeCredentials)
	}

	all[bridgeID] = creds

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// DeleteCredentials removes the stored credentials for the given bridge ID.
func DeleteCredentials(bridgeID string) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}

	all, err := readAllCredentials(path)
	if err != nil {
		return err
	}

	delete(all, bridgeID)

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
