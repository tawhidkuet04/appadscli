// Package auth implements Apple Ads API authentication: ES256 client-secret
// JWTs, the OAuth2 client-credentials exchange, token caching, and credential
// storage (macOS keychain by default, 0600 file with --bypass-keychain).
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/appadscli/appadscli/internal/config"
)

const keychainService = "appadscli-apple-ads"

// Credentials are the per-user Apple Ads API credentials.
type Credentials struct {
	ClientID   string `json:"clientId"`
	TeamID     string `json:"teamId"` // a.k.a. key issuer org id
	KeyID      string `json:"keyId"`
	PrivateKey string `json:"privateKey"` // PEM, EC P-256
}

// ErrNotLoggedIn is returned when no credentials are stored.
var ErrNotLoggedIn = errors.New("not logged in — run `appadscli auth login` first")

func credsFile() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "credentials.json"), nil
}

// SaveCredentials stores credentials in the keychain (macOS) or a 0600 file.
func SaveCredentials(c *Credentials, bypassKeychain bool) (where string, err error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" && !bypassKeychain {
		// -U updates an existing item in place.
		cmd := exec.Command("security", "add-generic-password", "-U",
			"-s", keychainService, "-a", c.ClientID, "-w", string(b))
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("keychain write failed: %s (use --bypass-keychain to store on disk)", strings.TrimSpace(string(out)))
		}
		// Remember which account name to read back.
		f, err := credsFile()
		if err != nil {
			return "", err
		}
		ref := map[string]string{"keychain": keychainService, "account": c.ClientID}
		rb, _ := json.Marshal(ref)
		if err := os.WriteFile(f, rb, 0o600); err != nil {
			return "", err
		}
		return "keychain", nil
	}
	f, err := credsFile()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(f, b, 0o600); err != nil {
		return "", err
	}
	return f, nil
}

// LoadCredentials retrieves stored credentials from keychain or file.
func LoadCredentials() (*Credentials, error) {
	f, err := credsFile()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(f)
	if os.IsNotExist(err) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, err
	}
	var probe map[string]string
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("corrupt credentials file: %w", err)
	}
	if probe["keychain"] != "" {
		out, err := exec.Command("security", "find-generic-password",
			"-s", probe["keychain"], "-a", probe["account"], "-w").Output()
		if err != nil {
			return nil, fmt.Errorf("keychain read failed (%w) — re-run `appadscli auth login`", err)
		}
		b = []byte(strings.TrimSpace(string(out)))
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("corrupt credentials: %w", err)
	}
	if c.ClientID == "" || c.PrivateKey == "" {
		return nil, ErrNotLoggedIn
	}
	return &c, nil
}

// DeleteCredentials removes stored credentials and cached tokens.
func DeleteCredentials() error {
	f, err := credsFile()
	if err != nil {
		return err
	}
	if b, err := os.ReadFile(f); err == nil {
		var probe map[string]string
		if json.Unmarshal(b, &probe) == nil && probe["keychain"] != "" {
			_ = exec.Command("security", "delete-generic-password",
				"-s", probe["keychain"], "-a", probe["account"]).Run()
		}
	}
	_ = os.Remove(f)
	d, err := config.Dir()
	if err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(d, "token.json"))
	return nil
}
