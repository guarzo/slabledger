// Package gsheets provides a Google Sheets API v4 client using service account
// authentication. It fetches sheet data as [][]string, compatible with the
// existing CSV-based PSA import pipeline.
package gsheets

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	sheetsScope   = "https://www.googleapis.com/auth/spreadsheets.readonly"
	tokenMargin   = 5 * time.Minute // refresh tokens 5 min before expiry
	tokenLifetime = 3600            // 1 hour in seconds
)

// ServiceAccountCredentials represents the relevant fields from a Google
// service account JSON key file.
type ServiceAccountCredentials struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
	parsedKey   *rsa.PrivateKey
}

// cachedToken holds an OAuth2 access token and its expiry.
type cachedToken struct {
	accessToken string
	expiry      time.Time
	mu          sync.RWMutex
}

func (t *cachedToken) isExpired() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.expiry.IsZero() || time.Now().Add(tokenMargin).After(t.expiry)
}

func (t *cachedToken) set(token string, expiry time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.accessToken = token
	t.expiry = expiry
}

// parseServiceAccountCredentials parses the JSON key and validates the RSA
// private key.
func parseServiceAccountCredentials(jsonKey string) (*ServiceAccountCredentials, error) {
	var creds ServiceAccountCredentials
	if err := json.Unmarshal([]byte(jsonKey), &creds); err != nil {
		return nil, fmt.Errorf("parse service account JSON: %w", err)
	}
	if creds.ClientEmail == "" {
		return nil, fmt.Errorf("client_email is required in service account credentials")
	}
	if creds.TokenURI == "" {
		creds.TokenURI = "https://oauth2.googleapis.com/token"
	}

	block, _ := pem.Decode([]byte(creds.PrivateKey))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 as fallback
		key2, err2 := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse private key: %w (pkcs1: %v)", err, err2)
		}
		creds.parsedKey = key2
	} else {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		creds.parsedKey = rsaKey
	}
	return &creds, nil
}

// buildJWT creates a signed JWT assertion for the Google OAuth2 token endpoint.
func buildJWT(creds *ServiceAccountCredentials) (string, error) {
	now := time.Now().UTC()
	header := base64Encode([]byte(`{"alg":"RS256","typ":"JWT"}`))

	claimSet := map[string]interface{}{
		"iss":   creds.ClientEmail,
		"scope": sheetsScope,
		"aud":   creds.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Duration(tokenLifetime) * time.Second).Unix(),
	}
	claimBytes, err := json.Marshal(claimSet)
	if err != nil {
		return "", fmt.Errorf("marshal JWT claims: %w", err)
	}
	payload := base64Encode(claimBytes)

	signingInput := header + "." + payload
	hashed := sha256.Sum256([]byte(signingInput))

	sig, err := rsa.SignPKCS1v15(nil, creds.parsedKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}

	return signingInput + "." + base64Encode(sig), nil
}

func base64Encode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}
