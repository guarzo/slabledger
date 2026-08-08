package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// ErrInvalidSigningKey is returned when the approval signing key is missing or
// too short to carry meaningful entropy.
var ErrInvalidSigningKey = errors.New("invalid signing key: must be at least 32 characters")

// HMACSigner authenticates approval envelopes with HMAC-SHA256.
//
// The key is supplied by the application environment and is never written to
// the database. That is the whole point: the threat model for signed push
// approvals is an adversary who can write rows, so the authenticator must be
// rooted somewhere they cannot reach.
type HMACSigner struct {
	key   []byte
	keyID string
}

// NewHMACSigner derives a signer from key, following the same rule as
// NewAESEncryptor: a 64-character hex string is decoded directly to preserve
// its full entropy, anything else is hashed to 32 bytes.
//
// keyID labels the key in the rows it signs so a rotation does not strand
// approvals already in flight; it is an opaque identifier, not a secret. An
// empty keyID defaults to "default".
func NewHMACSigner(key, keyID string) (*HMACSigner, error) {
	if len(key) < 32 {
		return nil, ErrInvalidSigningKey
	}
	if keyID == "" {
		keyID = "default"
	}

	if len(key) == 64 {
		if decoded, err := hex.DecodeString(key); err == nil && len(decoded) == 32 {
			return &HMACSigner{key: decoded, keyID: keyID}, nil
		}
	}

	hash := sha256.Sum256([]byte(key))
	return &HMACSigner{key: hash[:], keyID: keyID}, nil
}

// KeyID returns the identifier of the key this signer uses.
func (s *HMACSigner) KeyID() string { return s.keyID }

// Sign returns the lowercase hex HMAC-SHA256 of input.
func (s *HMACSigner) Sign(input string) (string, error) {
	mac := hmac.New(sha256.New, s.key)
	// hash.Hash.Write never returns an error; the signature keeps the error for
	// the interface's benefit.
	mac.Write([]byte(input))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Verify reports whether signature authenticates input. The comparison is
// constant-time, and the decode failure path returns the same false as a
// mismatch so a caller cannot distinguish malformed from wrong.
func (s *HMACSigner) Verify(input, signature string) bool {
	want, err := s.Sign(input)
	if err != nil {
		return false
	}
	// Compare the decoded bytes so casing in the stored hex does not decide
	// authenticity.
	wantRaw, err := hex.DecodeString(want)
	if err != nil {
		return false
	}
	gotRaw, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	return hmac.Equal(wantRaw, gotRaw)
}
