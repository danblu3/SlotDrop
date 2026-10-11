// Package settings remembers a few things between runs, in
// %APPDATA%\SlotDrop\settings.json. The RomM password or token is only
// stored encrypted (see package secret), never as plain text.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	RommURL  string `json:"romm_url"`
	Username string `json:"username"`
	UseToken bool   `json:"use_token"`
	// Login is the password or token, encrypted with Windows DPAPI.
	Login  []byte `json:"login_protected,omitempty"`
	SDCard string `json:"sd_card"`
	// DeviceID names this PC to RomM when signing in with a phone, so
	// pairing again replaces the old token instead of adding a device.
	DeviceID string `json:"device_id,omitempty"`
}

func path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "SlotDrop", "settings.json"), nil
}

// oldPath is where settings lived before the app was called SlotDrop.
func oldPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "SlotLabelMaker", "settings.json")
}

// Load returns saved settings, or empty ones.
func Load() Settings {
	var s Settings
	if p, err := path(); err == nil {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) && oldPath() != "" {
			data, err = os.ReadFile(oldPath()) // carry over the login from before the rename
		}
		if err == nil {
			_ = json.Unmarshal(data, &s)
		}
	}
	return s
}

// Save writes the settings, readable only by you where the OS supports it.
func Save(s Settings) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(s, "", "  ")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
