// Package canonjson produces a deterministic byte encoding of a JSON value so
// two independently-computed digests of the same logical payload agree.
//
// It exists because Go's encoding/json is not deterministic for the shapes this
// codebase digests: a map[string]any (which psacampaign.FieldChange.Value can
// hold) marshals with sorted keys today but that is a documented implementation
// detail rather than a guarantee, and nested values arriving from Postgres jsonb
// have already lost their original key order and whitespace anyway.
//
// The rule that makes approve-time and drain-time digests agree is that both
// sides canonicalize the *decoded* form, never the wire bytes. Postgres jsonb
// normalizes what it stores, so a digest over raw stored bytes would not survive
// the round trip. Callers should use Digest, which enforces that by routing
// every input through a marshal/unmarshal cycle before encoding.
package canonjson

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// maxExactInt is the largest integer a float64 represents exactly. JSON numbers
// decode to float64 through the any round trip, so a larger magnitude would be
// silently rounded and two different payloads could digest identically.
const maxExactInt = 1 << 53

// Digest returns the hex SHA-256 of v's canonical encoding.
//
// v is marshalled and re-decoded into any before encoding, so a value supplied
// as a typed struct and the same value read back from jsonb produce identical
// bytes by construction rather than by argument.
func Digest(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("canonjson: marshal: %w", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("canonjson: decode: %w", err)
	}
	canon, err := Encode(decoded)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

// Encode writes v in canonical form: object keys sorted bytewise, arrays in
// order, no insignificant whitespace. v must already consist of the types
// encoding/json produces when decoding into any.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := encodeValue(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeValue(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		return encodeString(buf, t)
	case float64:
		return encodeNumber(buf, t)
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeValue(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeString(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := encodeValue(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		// Reached only if a caller hands Encode a value that did not come from
		// a decode-into-any. Refusing beats digesting something whose encoding
		// this package does not define.
		return fmt.Errorf("canonjson: unsupported type %T (encode decoded values only)", v)
	}
	return nil
}

// encodeNumber writes a JSON number. Integral values are written without an
// exponent or fraction so the encoding does not depend on how the value was
// originally spelled; non-integral values use the shortest round-trippable
// form. Magnitudes beyond float64's exact-integer range are refused rather than
// silently rounded into a collision.
func encodeNumber(buf *bytes.Buffer, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("canonjson: cannot encode non-finite number %v", f)
	}
	if f == math.Trunc(f) {
		if math.Abs(f) >= maxExactInt {
			return fmt.Errorf("canonjson: integer %v exceeds exact float64 range, refusing to digest a rounded value", f)
		}
		buf.WriteString(strconv.FormatInt(int64(f), 10))
		return nil
	}
	buf.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
	return nil
}

// encodeString writes a JSON string. encoding/json's own string encoder is
// reused so escaping matches the rest of the codebase; its HTML escaping is
// disabled because it varies by encoder settings and would make the encoding
// depend on something other than the value.
func encodeString(buf *bytes.Buffer, s string) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("canonjson: encode string: %w", err)
	}
	// Encode appends a newline that is not part of the JSON value.
	buf.Write(bytes.TrimRight(b.Bytes(), "\n"))
	return nil
}
