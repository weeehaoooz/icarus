package handler

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"icarus-auth-ms/internal/securitylog"
	"math/big"
	"net/http"
	"strings"
)

type ClientRegisterRequest struct {
	ClientID  string `json:"client_id"`
	PublicKey string `json:"public_key"` // PEM format RSA public key
}

type ClientTokenRequest struct {
	ClientAssertion string `json:"client_assertion"` // JWT signed by client private key
}

// Note: Admin client management handlers (List, Create, Get, Update, Delete) are located in admin_rbac.go

// ClientRegisterHandler registers a client's public key.
func (s *HandlerServer) ClientRegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req ClientRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ClientID == "" || req.PublicKey == "" {
		s.respondWithError(w, http.StatusBadRequest, "client_id and public_key are required")
		return
	}

	block, _ := pem.Decode([]byte(req.PublicKey))
	if block == nil || !strings.Contains(block.Type, "PUBLIC KEY") {
		s.respondWithError(w, http.StatusBadRequest, "invalid public key PEM format")
		return
	}

	if err := s.Repo.CreateClient(req.ClientID, req.PublicKey); err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to register client: "+err.Error())
		return
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainClientMgmt,
		Action:         "CLIENT_REGISTER",
		Severity:       securitylog.SeverityInfo,
		Actor:          req.ClientID,
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: fmt.Sprintf("client:%s", req.ClientID),
		Status:         securitylog.StatusSuccess,
	})

	s.respondWithJSON(w, http.StatusCreated, map[string]string{
		"message":   "client registered successfully",
		"client_id": req.ClientID,
	})
}

// ClientTokenHandler validates client assertions and issues client access tokens.
func (s *HandlerServer) ClientTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req ClientTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ClientAssertion == "" {
		s.respondWithError(w, http.StatusBadRequest, "client_assertion is required")
		return
	}

	clientID, err := s.TokenMgr.VerifyClientAssertion(req.ClientAssertion, s.Repo)
	if err != nil {
		s.SecLogger.LogEvent(r.Context(), securitylog.Event{
			EventType: securitylog.DomainAuth,
			Action:    "CLIENT_TOKEN_ISSUANCE",
			Severity:  securitylog.SeverityWarn,
			ActorIP:   securitylog.GetClientIP(r),
			UserAgent: r.UserAgent(),
			Status:    securitylog.StatusFailure,
			Details:   map[string]interface{}{"reason": "invalid client assertion: " + err.Error()},
		})
		s.respondWithError(w, http.StatusUnauthorized, "invalid client assertion: "+err.Error())
		return
	}

	roles, err := s.Repo.GetClientRoles(clientID)
	if err != nil {
		roles = []string{}
	}
	permissions, err := s.Repo.GetClientPermissions(clientID)
	if err != nil {
		permissions = []string{}
	}
	accessToken, err := s.TokenMgr.GenerateClientTokenWithRoles(clientID, roles, permissions)
	if err != nil {
		s.respondWithError(w, http.StatusInternalServerError, "failed to generate client token")
		return
	}

	s.SecLogger.LogEvent(r.Context(), securitylog.Event{
		EventType:      securitylog.DomainAuth,
		Action:         "CLIENT_TOKEN_ISSUANCE",
		Severity:       securitylog.SeverityInfo,
		Actor:          clientID,
		ActorIP:        securitylog.GetClientIP(r),
		UserAgent:      r.UserAgent(),
		TargetResource: fmt.Sprintf("client:%s", clientID),
		Status:         securitylog.StatusSuccess,
	})

	s.respondWithJSON(w, http.StatusOK, map[string]string{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   "300",
	})
}

// CertsHandler returns the Auth Server's JWKS and raw PEM keys.
func (s *HandlerServer) CertsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondWithError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	nStr := base64.RawURLEncoding.EncodeToString(s.TokenMgr.PublicKey.N.Bytes())
	eBytes := big.NewInt(int64(s.TokenMgr.PublicKey.E)).Bytes()
	eStr := base64.RawURLEncoding.EncodeToString(eBytes)

	jwk := map[string]interface{}{
		"kty": "RSA",
		"alg": "RS256",
		"use": "sig",
		"kid": "auth-ms-key-default",
		"n":   nStr,
		"e":   eStr,
	}

	jwks := map[string]interface{}{
		"keys": []interface{}{jwk},
	}

	if r.URL.Query().Get("format") == "pem" {
		pubBytes, err := x509.MarshalPKIXPublicKey(s.TokenMgr.PublicKey)
		if err != nil {
			s.respondWithError(w, http.StatusInternalServerError, "failed to export public key")
			return
		}
		pubPem := pem.EncodeToMemory(&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: pubBytes,
		})
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pubPem)
		return
	}

	s.respondWithJSON(w, http.StatusOK, jwks)
}

// VerifyTokenHandler verifies an authorization token.
func (s *HandlerServer) VerifyTokenHandler(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		s.respondWithError(w, http.StatusBadRequest, "missing authorization header")
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		s.respondWithError(w, http.StatusBadRequest, "invalid authorization format")
		return
	}

	tokenStr := parts[1]
	claims, err := s.TokenMgr.VerifyToken(tokenStr)
	if err != nil {
		s.respondWithError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
		return
	}

	s.respondWithJSON(w, http.StatusOK, claims)
}
