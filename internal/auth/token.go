package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tawhidkuet04/adastra/internal/config"
)

const tokenURL = "https://appleid.apple.com/auth/oauth2/token"

// Token is a cached OAuth2 access token.
type Token struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// Valid reports whether the token is usable with a 60s safety margin.
func (t *Token) Valid() bool {
	return t != nil && t.AccessToken != "" && time.Now().Add(60*time.Second).Before(t.ExpiresAt)
}

func tokenFile() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "token.json"), nil
}

// AccessToken returns a valid access token, refreshing via the OAuth2
// client-credentials flow when the cache is empty or expired.
func AccessToken(ctx context.Context) (*Token, error) {
	if f, err := tokenFile(); err == nil {
		if b, err := os.ReadFile(f); err == nil {
			var t Token
			if json.Unmarshal(b, &t) == nil && t.Valid() {
				return &t, nil
			}
		}
	}
	return Refresh(ctx)
}

// Refresh forces a new token exchange and caches the result.
func Refresh(ctx context.Context) (*Token, error) {
	creds, err := LoadCredentials()
	if err != nil {
		return nil, err
	}
	secret, err := clientSecretJWT(creds, time.Now())
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"scope":         {"searchadsorg"},
		"client_id":     {creds.ClientID},
		"client_secret": {secret},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("unexpected token response: %w", err)
	}
	t := &Token{
		AccessToken: tr.AccessToken,
		TokenType:   tr.TokenType,
		ExpiresAt:   time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}
	if f, err := tokenFile(); err == nil {
		if b, err := json.Marshal(t); err == nil {
			_ = os.WriteFile(f, b, 0o600)
		}
	}
	return t, nil
}

// VerifyKey exposes the offline key self-check for `auth doctor`.
func VerifyKey(c *Credentials) error { return verifyKeyUsable(c) }
