package auth

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// clientSecretJWT builds the ES256-signed client secret Apple's OAuth2
// endpoint expects. Claims per Apple Ads API docs: sub=clientID, iss=teamID,
// aud=appleid.apple.com, 180-day max expiry (we use 1h; it's minted per run).
func clientSecretJWT(c *Credentials, now time.Time) (string, error) {
	key, err := parseECPrivateKey(c.PrivateKey)
	if err != nil {
		return "", err
	}
	header := map[string]string{"alg": "ES256", "kid": c.KeyID}
	claims := map[string]any{
		"sub": c.ClientID,
		"iss": c.TeamID,
		"aud": "https://appleid.apple.com",
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signing := b64(hb) + "." + b64(cb)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing failed: %w", err)
	}
	// JOSE ES256 signature is raw R||S, each padded to 32 bytes.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + b64(sig), nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func parseECPrivateKey(pemStr string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("private key is not valid PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		ec, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not an EC key (Apple Ads requires EC P-256)")
		}
		return ec, nil
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	return nil, errors.New("could not parse private key (expected PKCS#8 or SEC1 EC PEM)")
}

// verifyKeyUsable checks the key parses and can sign, without any network.
func verifyKeyUsable(c *Credentials) error {
	key, err := parseECPrivateKey(c.PrivateKey)
	if err != nil {
		return err
	}
	if key.Curve.Params().Name != "P-256" {
		return fmt.Errorf("key curve is %s; Apple Ads requires P-256 (prime256v1)", key.Curve.Params().Name)
	}
	digest := sha256.Sum256([]byte("adastra-selftest"))
	if _, _, err := ecdsa.Sign(rand.Reader, key, digest[:]); err != nil {
		return err
	}
	return nil
}
