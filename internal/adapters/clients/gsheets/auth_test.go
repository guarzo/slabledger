package gsheets

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"testing"
	"time"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func generateTestCredentials(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	creds := ServiceAccountCredentials{
		ClientEmail: "test@project.iam.gserviceaccount.com",
		PrivateKey:  string(pemBlock),
		TokenURI:    "https://oauth2.googleapis.com/token",
	}
	b, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshal creds: %v", err)
	}
	return string(b)
}

func TestParseServiceAccountCredentials(t *testing.T) {
	key := generateTestKey(t)
	credsJSON := generateTestCredentials(t, key)

	creds, err := parseServiceAccountCredentials(credsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.ClientEmail != "test@project.iam.gserviceaccount.com" {
		t.Errorf("got email %q, want test@project.iam.gserviceaccount.com", creds.ClientEmail)
	}
	if creds.parsedKey == nil {
		t.Fatal("parsedKey is nil")
	}
}

func TestParseServiceAccountCredentials_PKCS8(t *testing.T) {
	key := generateTestKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	creds := ServiceAccountCredentials{
		ClientEmail: "test@project.iam.gserviceaccount.com",
		PrivateKey:  string(pemBlock),
		TokenURI:    "https://oauth2.googleapis.com/token",
	}
	b, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshal creds: %v", err)
	}

	parsed, err := parseServiceAccountCredentials(string(b))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.parsedKey == nil {
		t.Fatal("parsedKey is nil")
	}
	if parsed.ClientEmail != "test@project.iam.gserviceaccount.com" {
		t.Errorf("got email %q, want test@project.iam.gserviceaccount.com", parsed.ClientEmail)
	}
}

func TestParseServiceAccountCredentials_MissingClientEmail(t *testing.T) {
	key := generateTestKey(t)
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	creds := ServiceAccountCredentials{
		ClientEmail: "",
		PrivateKey:  string(pemBlock),
		TokenURI:    "https://oauth2.googleapis.com/token",
	}
	b, _ := json.Marshal(creds)
	_, err := parseServiceAccountCredentials(string(b))
	if err == nil {
		t.Fatal("expected error for missing client_email")
	}
}

func TestParseServiceAccountCredentials_InvalidJSON(t *testing.T) {
	_, err := parseServiceAccountCredentials("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseServiceAccountCredentials_InvalidKey(t *testing.T) {
	creds := ServiceAccountCredentials{
		ClientEmail: "test@example.com",
		PrivateKey:  "not a pem key",
		TokenURI:    "https://oauth2.googleapis.com/token",
	}
	b, _ := json.Marshal(creds)
	_, err := parseServiceAccountCredentials(string(b))
	if err == nil {
		t.Fatal("expected error for invalid private key")
	}
}

func TestBuildJWT(t *testing.T) {
	key := generateTestKey(t)
	credsJSON := generateTestCredentials(t, key)
	creds, err := parseServiceAccountCredentials(credsJSON)
	if err != nil {
		t.Fatalf("parse creds: %v", err)
	}

	jwt, err := buildJWT(creds)
	if err != nil {
		t.Fatalf("buildJWT: %v", err)
	}
	if jwt == "" {
		t.Fatal("JWT is empty")
	}
	// JWT should have 3 dot-separated parts
	parts := 0
	for _, b := range jwt {
		if b == '.' {
			parts++
		}
	}
	if parts != 2 {
		t.Errorf("JWT has %d dots, want 2", parts)
	}
}

func TestCachedToken_IsExpired(t *testing.T) {
	tests := []struct {
		name   string
		expiry time.Time
		want   bool
	}{
		{"zero value is expired", time.Time{}, true},
		{"past expiry is expired", time.Now().Add(-1 * time.Minute), true},
		{"within margin is expired", time.Now().Add(4 * time.Minute), true},
		{"future is valid", time.Now().Add(10 * time.Minute), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := &cachedToken{}
			if !tt.expiry.IsZero() {
				tok.set("test", tt.expiry)
			}
			if got := tok.isExpired(); got != tt.want {
				t.Errorf("isExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}
