package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func testCreds(t *testing.T) *Credentials {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	return &Credentials{
		ClientID: "SEARCHADS.client", TeamID: "SEARCHADS.team", KeyID: "kid-1", PrivateKey: pemStr,
	}
}

func TestClientSecretJWT(t *testing.T) {
	creds := testCreds(t)
	tok, err := clientSecretJWT(creds, time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts", len(parts))
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var header map[string]string
	if err := json.Unmarshal(hb, &header); err != nil {
		t.Fatal(err)
	}
	if header["alg"] != "ES256" || header["kid"] != "kid-1" {
		t.Errorf("header = %v", header)
	}
	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(cb, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "SEARCHADS.client" || claims["iss"] != "SEARCHADS.team" {
		t.Errorf("claims = %v", claims)
	}
	if claims["aud"] != "https://appleid.apple.com" {
		t.Errorf("aud = %v", claims["aud"])
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Errorf("signature must be raw 64-byte R||S, got %d (%v)", len(sig), err)
	}
}

func TestVerifyKeyUsable(t *testing.T) {
	if err := verifyKeyUsable(testCreds(t)); err != nil {
		t.Errorf("valid key rejected: %v", err)
	}
	bad := &Credentials{PrivateKey: "not a pem"}
	if err := verifyKeyUsable(bad); err == nil {
		t.Error("garbage key accepted")
	}
}
