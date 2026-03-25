package crypto

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestNewAESEncryptor(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "valid 32-byte key",
			key:     "12345678901234567890123456789012", // exactly 32 bytes
			wantErr: false,
		},
		{
			name:    "valid 64-byte key",
			key:     strings.Repeat("a", 64),
			wantErr: false,
		},
		{
			name:    "invalid short key",
			key:     "short",
			wantErr: true,
		},
		{
			name:    "empty key",
			key:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAESEncryptor(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAESEncryptor() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAESEncryptor_EncryptDecrypt(t *testing.T) {
	key := "12345678901234567890123456789012" // exactly 32 bytes
	encryptor, err := NewAESEncryptor(key)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "simple text",
			plaintext: "hello world",
		},
		{
			name:      "oauth token",
			plaintext: "v^1.1#i^1#r^0#p^3#f^0#I^3#t^H4sIAAAAAAAAAOVYa2wUVRS...",
		},
		{
			name:      "empty string",
			plaintext: "",
		},
		{
			name:      "unicode text",
			plaintext: "Hello 世界! 🚀",
		},
		{
			name:      "long text",
			plaintext: strings.Repeat("a", 1000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			ciphertext, err := encryptor.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			// Empty string should return empty string
			if tt.plaintext == "" {
				if ciphertext != "" {
					t.Errorf("Expected empty ciphertext for empty plaintext")
				}
				return
			}

			// Ciphertext should be different from plaintext
			if ciphertext == tt.plaintext {
				t.Errorf("Ciphertext should not match plaintext")
			}

			// Decrypt
			decrypted, err := encryptor.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			// Decrypted should match original
			if decrypted != tt.plaintext {
				t.Errorf("Decrypted text does not match original.\nGot:  %q\nWant: %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestAESEncryptor_EncryptNonDeterministic(t *testing.T) {
	key := "12345678901234567890123456789012" // exactly 32 bytes
	encryptor, err := NewAESEncryptor(key)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	plaintext := "test data"

	// Encrypt same plaintext multiple times
	ciphertext1, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	ciphertext2, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Ciphertexts should be different (due to random nonce)
	if ciphertext1 == ciphertext2 {
		t.Errorf("Expected different ciphertexts for same plaintext (non-deterministic encryption)")
	}

	// But both should decrypt to same plaintext
	decrypted1, err := encryptor.Decrypt(ciphertext1)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	decrypted2, err := encryptor.Decrypt(ciphertext2)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Errorf("Both decrypted texts should match original plaintext")
	}
}

func TestAESEncryptor_DecryptInvalidData(t *testing.T) {
	key := "12345678901234567890123456789012" // exactly 32 bytes
	encryptor, err := NewAESEncryptor(key)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	tests := []struct {
		name       string
		ciphertext string
	}{
		{
			name:       "invalid base64",
			ciphertext: "not valid base64!!!",
		},
		{
			name:       "too short",
			ciphertext: "YQ==", // base64 for "a"
		},
		{
			name:       "random data",
			ciphertext: "cmFuZG9tZGF0YXRoYXRpc25vdGVuY3J5cHRlZA==",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encryptor.Decrypt(tt.ciphertext)
			if err == nil {
				t.Errorf("Expected error decrypting invalid data, got nil")
			}
		})
	}
}

func TestAESEncryptor_DifferentKeys(t *testing.T) {
	key1 := "11111111112222222222333333333344" // exactly 32 bytes
	key2 := "44444444445555555555666666666677" // exactly 32 bytes

	encryptor1, err := NewAESEncryptor(key1)
	if err != nil {
		t.Fatalf("Failed to create encryptor1: %v", err)
	}

	encryptor2, err := NewAESEncryptor(key2)
	if err != nil {
		t.Fatalf("Failed to create encryptor2: %v", err)
	}

	plaintext := "secret message"

	// Encrypt with first key
	ciphertext, err := encryptor1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Try to decrypt with second key (should fail)
	_, err = encryptor2.Decrypt(ciphertext)
	if err == nil {
		t.Errorf("Expected error decrypting with wrong key, got nil")
	}
}

func TestAESEncryptor_HexKeyPreservesEntropy(t *testing.T) {
	// Generate a 32-byte key and encode as 64-char hex string
	rawKey := []byte("12345678901234567890123456789012") // exactly 32 bytes
	hexKey := hex.EncodeToString(rawKey)                 // 64 hex characters

	if len(hexKey) != 64 {
		t.Fatalf("Expected hex key length 64, got %d", len(hexKey))
	}

	encryptor, err := NewAESEncryptor(hexKey)
	if err != nil {
		t.Fatalf("Failed to create encryptor with hex key: %v", err)
	}

	// Verify the key was used directly (not hashed)
	// The internal key should be the decoded hex value
	if string(encryptor.key) != string(rawKey) {
		t.Errorf("Hex key should be decoded directly, not hashed")
	}

	// Test encryption/decryption still works
	plaintext := "test message"
	ciphertext, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	decrypted, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted text does not match: got %q, want %q", decrypted, plaintext)
	}
}

func TestAESEncryptor_InvalidHexFallsBackToHash(t *testing.T) {
	// 64 characters but not valid hex (contains 'g' which is invalid hex)
	invalidHexKey := "12345678901234567890123456789012345678901234567890123456789012gg"

	if len(invalidHexKey) != 64 {
		t.Fatalf("Expected key length 64, got %d", len(invalidHexKey))
	}

	encryptor, err := NewAESEncryptor(invalidHexKey)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// Test that encryption/decryption works (uses SHA-256 fallback)
	plaintext := "test message"
	ciphertext, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	decrypted, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted text does not match: got %q, want %q", decrypted, plaintext)
	}
}
