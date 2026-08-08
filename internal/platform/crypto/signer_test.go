package crypto

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// A 64-char hex key is decoded rather than hashed, so these two constants are
// the same 32 bytes reached by two different paths.
const (
	hexKey        = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	passphraseKey = "a-passphrase-long-enough-to-be-accepted"
)

func TestNewHMACSigner(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		keyID     string
		wantErr   error
		wantKeyID string
	}{
		{name: "hex key accepted", key: hexKey, keyID: "k1", wantKeyID: "k1"},
		{name: "passphrase accepted", key: passphraseKey, keyID: "k1", wantKeyID: "k1"},
		{name: "empty keyID defaults", key: hexKey, keyID: "", wantKeyID: "default"},
		// A 64-char string that is not valid hex must fall through to the hash
		// path rather than being rejected.
		{name: "non-hex 64-char key hashes", key: strings.Repeat("z", 64), keyID: "k1", wantKeyID: "k1"},
		{name: "short key rejected", key: strings.Repeat("a", 31), keyID: "k1", wantErr: ErrInvalidSigningKey},
		{name: "empty key rejected", key: "", keyID: "k1", wantErr: ErrInvalidSigningKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewHMACSigner(tt.key, tt.keyID)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewHMACSigner error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewHMACSigner: %v", err)
			}
			if s.KeyID() != tt.wantKeyID {
				t.Errorf("KeyID() = %q, want %q", s.KeyID(), tt.wantKeyID)
			}
			if len(s.key) != 32 {
				t.Errorf("derived key is %d bytes, want 32", len(s.key))
			}
		})
	}
}

// The hex path must preserve the caller's entropy rather than hashing it, or a
// deliberately-provisioned 256-bit key would be silently replaced by the hash of
// its ASCII spelling.
func TestNewHMACSignerDecodesHexKeyRatherThanHashingIt(t *testing.T) {
	s, err := NewHMACSigner(hexKey, "k1")
	if err != nil {
		t.Fatalf("NewHMACSigner: %v", err)
	}
	want, err := hex.DecodeString(hexKey)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(s.key) != string(want) {
		t.Errorf("hex key was not decoded directly: got %x, want %x", s.key, want)
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	s, err := NewHMACSigner(hexKey, "k1")
	if err != nil {
		t.Fatalf("NewHMACSigner: %v", err)
	}
	const input = `{"pushId":"p1","payloadDigest":"abc"}`

	sig, err := s.Sign(input)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !s.Verify(input, sig) {
		t.Fatal("Verify rejected a signature this signer produced")
	}
}

func TestSignIsDeterministic(t *testing.T) {
	s, err := NewHMACSigner(hexKey, "k1")
	if err != nil {
		t.Fatalf("NewHMACSigner: %v", err)
	}
	first, _ := s.Sign("payload")
	second, _ := s.Sign("payload")
	if first != second {
		t.Errorf("Sign is not deterministic: %q vs %q", first, second)
	}
}

// Every one of these must fail closed. This is the whole security property: an
// adversary who can write the queue row controls the input and the stored
// signature, but not the key.
func TestVerifyRejects(t *testing.T) {
	s, err := NewHMACSigner(hexKey, "k1")
	if err != nil {
		t.Fatalf("NewHMACSigner: %v", err)
	}
	const input = "approved-payload"
	sig, err := s.Sign(input)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	other, err := NewHMACSigner(strings.Repeat("b", 64), "k2")
	if err != nil {
		t.Fatalf("NewHMACSigner: %v", err)
	}
	otherSig, err := other.Sign(input)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	tests := []struct {
		name  string
		input string
		sig   string
	}{
		{name: "tampered input", input: "tampered-payload", sig: sig},
		{name: "signature from another key", input: input, sig: otherSig},
		{name: "empty signature", input: input, sig: ""},
		{name: "malformed hex", input: input, sig: "not-hex!!"},
		{name: "odd-length hex", input: input, sig: "abc"},
		// Truncation must not authenticate: hmac.Equal is length-sensitive.
		{name: "truncated signature", input: input, sig: sig[:len(sig)-2]},
		{name: "extended signature", input: input, sig: sig + "00"},
		{name: "flipped nibble", input: input, sig: flipLastNibble(sig)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if s.Verify(tt.input, tt.sig) {
				t.Errorf("Verify accepted %s", tt.name)
			}
		})
	}
}

// Stored hex casing and stray whitespace are transport artifacts, not evidence
// of tampering — a round-trip through a column that upper-cased the value must
// still verify.
func TestVerifyIsCaseAndWhitespaceInsensitive(t *testing.T) {
	s, err := NewHMACSigner(hexKey, "k1")
	if err != nil {
		t.Fatalf("NewHMACSigner: %v", err)
	}
	const input = "approved-payload"
	sig, err := s.Sign(input)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	for _, variant := range []string{strings.ToUpper(sig), "  " + sig + "\n"} {
		if !s.Verify(input, variant) {
			t.Errorf("Verify rejected an equivalent encoding: %q", variant)
		}
	}
}

// Two signers built from different keys must not authenticate each other's
// output even for identical input — otherwise key rotation would be cosmetic.
func TestDifferentKeysProduceDifferentSignatures(t *testing.T) {
	a, err := NewHMACSigner(hexKey, "k1")
	if err != nil {
		t.Fatalf("NewHMACSigner: %v", err)
	}
	b, err := NewHMACSigner(passphraseKey, "k2")
	if err != nil {
		t.Fatalf("NewHMACSigner: %v", err)
	}
	sigA, _ := a.Sign("payload")
	sigB, _ := b.Sign("payload")
	if sigA == sigB {
		t.Fatal("distinct keys produced the same signature")
	}
}

func flipLastNibble(sig string) string {
	last := sig[len(sig)-1]
	replacement := byte('0')
	if last == '0' {
		replacement = '1'
	}
	return sig[:len(sig)-1] + string(replacement)
}
