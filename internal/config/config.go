// Package config manages ~/.asacli/config.json: profiles, default ad
// account, output preferences, and paths for local state.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the persisted CLI configuration.
type Config struct {
	// DefaultAccount is the ad account id used when --account is not given.
	DefaultAccount string `json:"defaultAccount,omitempty"`
	// DefaultOutput is table|json|csv|markdown. Empty means auto (TTY→table, pipe→json).
	DefaultOutput string `json:"defaultOutput,omitempty"`
	// DefaultCountry for ASO commands (e.g. "us").
	DefaultCountry string `json:"defaultCountry,omitempty"`
	// APIBase overrides the Apple Ads API base URL.
	APIBase string `json:"apiBase,omitempty"`
	// BypassKeychain stores credentials on disk (0600) instead of macOS keychain.
	BypassKeychain bool `json:"bypassKeychain,omitempty"`
	// TelemetryOptOut — asacli sends no telemetry; field reserved so it never can silently.
	TelemetryOptOut bool `json:"telemetryOptOut,omitempty"`
}

// Dir returns the asacli state directory (~/.asacli), creating it if needed.
func Dir() (string, error) {
	if d := os.Getenv("ASACLI_DIR"); d != "" {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", err
		}
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, ".asacli")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

func path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load reads the config, returning defaults if none exists.
func Load() (*Config, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save writes the config atomically.
func Save(c *Config) error {
	p, err := path()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
