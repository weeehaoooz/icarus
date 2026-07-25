package crypto

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type CustomClaims struct {
	Type         string   `json:"type"` // "user" or "client"
	TenantID     string   `json:"tenant_id,omitempty"`
	ModuleCode   string   `json:"module_code,omitempty"`
	Roles        []string `json:"roles,omitempty"`
	Permissions  []string `json:"permissions,omitempty"`
	OwnedModules []string `json:"owned_modules,omitempty"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
	Issuer     string
}

type ClientPublicKeyProvider interface {
	GetClientPublicKey(clientID string) (string, error)
}

func NewTokenManager(privKey *rsa.PrivateKey, pubKey *rsa.PublicKey, issuer string) *TokenManager {
	return &TokenManager{
		PrivateKey: privKey,
		PublicKey:  pubKey,
		Issuer:     issuer,
	}
}

// GenerateUserToken generates a 5-minute access token for a user.
func (m *TokenManager) GenerateUserToken(username string) (string, error) {
	claims := CustomClaims{
		Type: "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			Issuer:    m.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.PrivateKey)
}

// GenerateUserTokenWithRoles generates a 5-minute access token for a user with specific roles/permissions.
func (m *TokenManager) GenerateUserTokenWithRoles(username string, roles []string, permissions []string, ownedModules []string) (string, error) {
	claims := CustomClaims{
		Type:         "user",
		Roles:        roles,
		Permissions:  permissions,
		OwnedModules: ownedModules,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			Issuer:    m.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.PrivateKey)
}

// GenerateScopedUserToken generates a 5-minute access token for a user scoped to a tenant and module.
func (m *TokenManager) GenerateScopedUserToken(username, tenantID, moduleCode string, roles []string, permissions []string, ownedModules []string) (string, error) {
	claims := CustomClaims{
		Type:         "user",
		TenantID:     tenantID,
		ModuleCode:   moduleCode,
		Roles:        roles,
		Permissions:  permissions,
		OwnedModules: ownedModules,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			Issuer:    m.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.PrivateKey)
}

// GenerateScopedClientToken generates a 5-minute access token for a client scoped to a tenant and module.
func (m *TokenManager) GenerateScopedClientToken(clientID, tenantID, moduleCode string, roles []string, permissions []string) (string, error) {
	claims := CustomClaims{
		Type:        "client",
		TenantID:    tenantID,
		ModuleCode:  moduleCode,
		Roles:       roles,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   clientID,
			Issuer:    m.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.PrivateKey)
}


// GenerateClientToken generates a 5-minute access token for a client.
func (m *TokenManager) GenerateClientToken(clientID string) (string, error) {
	claims := CustomClaims{
		Type: "client",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   clientID,
			Issuer:    m.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.PrivateKey)
}

// GenerateClientTokenWithRoles generates a 5-minute access token for a client with specific roles/permissions.
func (m *TokenManager) GenerateClientTokenWithRoles(clientID string, roles []string, permissions []string) (string, error) {
	claims := CustomClaims{
		Type:        "client",
		Roles:       roles,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   clientID,
			Issuer:    m.Issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.PrivateKey)
}

// VerifyToken verifies a JWT token.
func (m *TokenManager) VerifyToken(tokenStr string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.PublicKey, nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token claims")
}

// VerifyClientAssertion parses and verifies a client assertion JWT.
func (m *TokenManager) VerifyClientAssertion(assertion string, provider ClientPublicKeyProvider) (string, error) {
	var claims jwt.RegisteredClaims
	_, _, err := jwt.NewParser().ParseUnverified(assertion, &claims)
	if err != nil {
		return "", fmt.Errorf("failed to parse unverified assertion: %w", err)
	}

	clientID := claims.Issuer
	if clientID == "" {
		clientID = claims.Subject
	}
	if clientID == "" {
		return "", errors.New("assertion must contain issuer or subject identifying the client")
	}

	// Fetch client public key PEM
	pubKeyPEM, err := provider.GetClientPublicKey(clientID)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve client public key: %w", err)
	}

	// Decode PEM
	block, _ := pem.Decode([]byte(pubKeyPEM))
	if block == nil {
		return "", errors.New("failed to decode client public key PEM")
	}

	// Parse RSA public key
	clientPubKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		pub, errPKIX := x509.ParsePKIXPublicKey(block.Bytes)
		if errPKIX != nil {
			return "", fmt.Errorf("failed to parse client public key: %w", errPKIX)
		}
		var ok bool
		clientPubKey, ok = pub.(*rsa.PublicKey)
		if !ok {
			return "", errors.New("client public key is not an RSA public key")
		}
	}

	// Verify JWT assertion signature and claims
	var verifiedClaims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(assertion, &verifiedClaims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected client signing method: %v", token.Header["alg"])
		}
		return clientPubKey, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to verify client assertion signature: %w", err)
	}

	if !token.Valid {
		return "", errors.New("client assertion token is invalid")
	}

	now := time.Now()
	if verifiedClaims.ExpiresAt == nil || verifiedClaims.ExpiresAt.Before(now) {
		return "", errors.New("client assertion token has expired")
	}

	return clientID, nil
}
