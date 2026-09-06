package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenVerifier_PEM(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(pubPEM)
	}))
	defer server.Close()

	verifier := NewTokenVerifier(server.URL + "/certs?format=pem")

	// Generate a token
	claims := CustomClaims{
		Type:  "user",
		Roles: []string{"admin"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	parsedClaims, err := verifier.VerifyToken(tokenStr)
	if err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}
	if parsedClaims.Subject != "admin" {
		t.Errorf("expected subject admin, got %s", parsedClaims.Subject)
	}
}

func TestTokenVerifier_JWKS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	nStr := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	eBytes := big.NewInt(int64(key.PublicKey.E)).Bytes()
	eStr := base64.RawURLEncoding.EncodeToString(eBytes)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]interface{}{
				{
					"kty": "RSA",
					"alg": "RS256",
					"use": "sig",
					"kid": "default",
					"n":   nStr,
					"e":   eStr,
				},
			},
		})
	}))
	defer server.Close()

	verifier := NewTokenVerifier(server.URL + "/api/v1/certs")

	claims := CustomClaims{
		Type:  "user",
		Roles: []string{"admin"},
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	parsedClaims, err := verifier.VerifyToken(tokenStr)
	if err != nil {
		t.Fatalf("failed to verify token with JWKS: %v", err)
	}
	if parsedClaims.Subject != "admin" {
		t.Errorf("expected subject admin, got %s", parsedClaims.Subject)
	}
}

func TestTokenVerifier_FallbackURL(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	pubBytes, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	mux := http.NewServeMux()
	// /certs returns 404, but /api/v1/certs returns the valid cert
	mux.HandleFunc("/api/v1/certs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(pubPEM)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Configured to /certs?format=pem (which is 404), should fallback to /api/v1/certs?format=pem
	verifier := NewTokenVerifier(server.URL + "/certs?format=pem")

	pub, err := verifier.GetPublicKey()
	if err != nil {
		t.Fatalf("expected fallback to work, got err: %v", err)
	}
	if pub.N.Cmp(key.PublicKey.N) != 0 {
		t.Errorf("public key does not match")
	}
}
