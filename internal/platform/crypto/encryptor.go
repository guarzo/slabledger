package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

var (
	// ErrInvalidKey is returned when the encryption key is invalid
	ErrInvalidKey = errors.New("invalid encryption key: must be 32 bytes")
	// ErrInvalidCiphertext is returned when decrypting invalid data
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
)

// Encryptor handles encryption and decryption of sensitive data
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// AESEncryptor implements Encryptor using AES-256-GCM
type AESEncryptor struct {
	key []byte
}

// NewAESEncryptor creates a new AES-256-GCM encryptor
// key must be at least 32 bytes for AES-256
// If key is a valid 64-character hex string (32 bytes decoded), it is used directly
// to preserve full entropy. Otherwise, the key is hashed with SHA-256.
func NewAESEncryptor(key string) (*AESEncryptor, error) {
	if key == "" || len(key) < 32 {
		return nil, ErrInvalidKey
	}

	// If key is a valid 64-char hex string, decode it directly to preserve entropy
	if len(key) == 64 {
		decoded, err := hex.DecodeString(key)
		if err == nil && len(decoded) == 32 {
			return &AESEncryptor{key: decoded}, nil
		}
	}

	// Fallback: use SHA-256 to derive a consistent 32-byte key for non-hex keys
	hash := sha256.Sum256([]byte(key))
	return &AESEncryptor{key: hash[:]}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns base64-encoded ciphertext
func (e *AESEncryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Encrypt and append nonce to ciphertext
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Return base64-encoded result
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext using AES-256-GCM
func (e *AESEncryptor) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// Decode base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrInvalidCiphertext
	}

	// Extract nonce and ciphertext
	nonce, cipherBytes := data[:nonceSize], data[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	return string(plaintext), nil
}
