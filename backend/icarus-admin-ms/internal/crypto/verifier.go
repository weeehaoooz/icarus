package crypto

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenVerifier struct {
	mu          sync.RWMutex
	publicKey   *rsa.PublicKey
	certsURL    string
	lastFetched time.Time
}

func NewTokenVerifier(certsURL string) *TokenVerifier {
	return &TokenVerifier{
		certsURL: certsURL,
	}
}

type UserGroup struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type CustomClaims struct {
	Type        string      `json:"type"` // "user" or "client"
	TenantID    string      `json:"tenant_id,omitempty"`
	ModuleCode  string      `json:"module_code,omitempty"`
	Roles       []string    `json:"roles,omitempty"`
	Permissions []string    `json:"permissions,omitempty"`
	Groups      []UserGroup `json:"groups,omitempty"`
	jwt.RegisteredClaims
}

func (v *TokenVerifier) GetPublicKey() (*rsa.PublicKey, error) {
	v.mu.RLock()
	if v.publicKey != nil && time.Since(v.lastFetched) < 10*time.Minute {
		pub := v.publicKey
		v.mu.RUnlock()
		return pub, nil
	}
	v.mu.RUnlock()

	v.mu.Lock()
	defer v.mu.Unlock()

	// Double check
	if v.publicKey != nil && time.Since(v.lastFetched) < 10*time.Minute {
		return v.publicKey, nil
	}

	resp, err := http.Get(v.certsURL)
	if err != nil {
		if v.publicKey != nil {
			// Fallback to cached key on failure
			return v.publicKey, nil
		}
		return nil, fmt.Errorf("failed to fetch certs from %s: %w", v.certsURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if v.publicKey != nil {
			return v.publicKey, nil
		}
		return nil, fmt.Errorf("failed to read certs body: %w", err)
	}

	block, _ := pem.Decode(body)
	if block == nil {
		if v.publicKey != nil {
			return v.publicKey, nil
		}
		return nil, errors.New("failed to decode pem certs")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		if v.publicKey != nil {
			return v.publicKey, nil
		}
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		if v.publicKey != nil {
			return v.publicKey, nil
		}
		return nil, errors.New("cert is not an RSA public key")
	}

	v.publicKey = rsaPub
	v.lastFetched = time.Now()
	return rsaPub, nil
}

func (v *TokenVerifier) VerifyToken(tokenStr string) (*CustomClaims, error) {
	pubKey, err := v.GetPublicKey()
	if err != nil {
		return nil, err
	}

	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return pubKey, nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}
