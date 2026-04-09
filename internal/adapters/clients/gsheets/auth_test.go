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

func mustMarshalCreds(t *testing.T, creds ServiceAccountCredentials) string {
	t.Helper()
	b, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshal creds: %v", err)
	}
	return string(b)
}

func TestParseServiceAccountCredentials(t *testing.T) {
	key := generateTestKey(t)

	pkcs1DER := x509.MarshalPKCS1PrivateKey(key)
	pkcs1PEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: pkcs1DER}))

	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	pkcs8PEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8DER}))

	tests := []struct {
		name      string
		input     string
		wantEmail string
		wantKey   bool
		wantErr   bool
	}{
		{
			name: "valid PKCS1",
			input: mustMarshalCreds(t, ServiceAccountCredentials{
				ClientEmail: "test@project.iam.gserviceaccount.com",
				PrivateKey:  pkcs1PEM,
				TokenURI:    "https://oauth2.googleapis.com/token",
			}),
			wantEmail: "test@project.iam.gserviceaccount.com",
			wantKey:   true,
		},
		{
			name: "valid PKCS8",
			input: mustMarshalCreds(t, ServiceAccountCredentials{
				ClientEmail: "test@project.iam.gserviceaccount.com",
				PrivateKey:  pkcs8PEM,
				TokenURI:    "https://oauth2.googleapis.com/token",
			}),
			wantEmail: "test@project.iam.gserviceaccount.com",
			wantKey:   true,
		},
		{
			name: "missing client_email",
			input: mustMarshalCreds(t, ServiceAccountCredentials{
				ClientEmail: "",
				PrivateKey:  pkcs1PEM,
				TokenURI:    "https://oauth2.googleapis.com/token",
			}),
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   "not json",
			wantErr: true,
		},
		{
			name: "invalid private key",
			input: mustMarshalCreds(t, ServiceAccountCredentials{
				ClientEmail: "test@example.com",
				PrivateKey:  "not a pem key",
				TokenURI:    "https://oauth2.googleapis.com/token",
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := parseServiceAccountCredentials(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if creds.ClientEmail != tt.wantEmail {
				t.Errorf("got email %q, want %q", creds.ClientEmail, tt.wantEmail)
			}
			if tt.wantKey && creds.parsedKey == nil {
				t.Fatal("parsedKey is nil")
			}
		})
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
