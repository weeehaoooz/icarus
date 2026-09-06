package crypto

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
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

type CustomClaims struct {
	Type         string   `json:"type"` // "user" or "client"
	TenantID     string   `json:"tenant_id,omitempty"`
	ModuleCode   string   `json:"module_code,omitempty"`
	Roles        []string `json:"roles,omitempty"`
	Permissions  []string `json:"permissions,omitempty"`
	OwnedModules []string `json:"owned_modules,omitempty"`
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

	if v.publicKey != nil && time.Since(v.lastFetched) < 10*time.Minute {
		return v.publicKey, nil
	}

	urlsToTry := []string{v.certsURL}
	if strings.Contains(v.certsURL, "/certs") && !strings.Contains(v.certsURL, "/api/v1/certs") {
		urlsToTry = append(urlsToTry, strings.Replace(v.certsURL, "/certs", "/api/v1/certs", 1))
	} else if strings.Contains(v.certsURL, "/api/v1/certs") {
		urlsToTry = append(urlsToTry, strings.Replace(v.certsURL, "/api/v1/certs", "/certs", 1))
	}

	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error

	for _, targetURL := range urlsToTry {
		resp, err := client.Get(targetURL)
		if err != nil {
			lastErr = fmt.Errorf("failed to fetch certs from %s: %w", targetURL, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read certs body from %s: %w", targetURL, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("fetch certs from %s returned HTTP status %d: %s", targetURL, resp.StatusCode, string(body))
			continue
		}

		// 1. Try parsing as standard PEM
		block, _ := pem.Decode(body)
		if block != nil {
			pub, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err == nil {
				if rsaPub, ok := pub.(*rsa.PublicKey); ok {
					v.publicKey = rsaPub
					v.lastFetched = time.Now()
					return rsaPub, nil
				}
			}
		}

		// 2. Try parsing as JWKS JSON in case raw endpoint or JWKS format was returned
		var jwks struct {
			Keys []struct {
				Kty string `json:"kty"`
				N   string `json:"n"`
				E   string `json:"e"`
			} `json:"keys"`
		}
		if err := json.Unmarshal(body, &jwks); err == nil && len(jwks.Keys) > 0 {
			key := jwks.Keys[0]
			if key.Kty == "RSA" {
				nBytes, err1 := base64.RawURLEncoding.DecodeString(key.N)
				if err1 != nil {
					nBytes, err1 = base64.URLEncoding.DecodeString(key.N)
				}
				eBytes, err2 := base64.RawURLEncoding.DecodeString(key.E)
				if err2 != nil {
					eBytes, err2 = base64.URLEncoding.DecodeString(key.E)
				}
				if err1 == nil && err2 == nil {
					var eInt int
					for _, b := range eBytes {
						eInt = (eInt << 8) | int(b)
					}
					rsaPub := &rsa.PublicKey{
						N: new(big.Int).SetBytes(nBytes),
						E: eInt,
					}
					v.publicKey = rsaPub
					v.lastFetched = time.Now()
					return rsaPub, nil
				}
			}
		}

		lastErr = fmt.Errorf("failed to parse public key from %s (invalid PEM/JWKS)", targetURL)
	}

	if v.publicKey != nil {
		return v.publicKey, nil
	}
	return nil, lastErr
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
