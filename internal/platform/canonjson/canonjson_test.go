package canonjson

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncode(t *testing.T) {
	tests := []struct {
		name    string
		input   string // JSON, decoded into any before encoding
		want    string
		wantErr bool
	}{
		{name: "object keys sorted", input: `{"b":1,"a":2,"c":3}`, want: `{"a":2,"b":1,"c":3}`},
		{name: "nested objects sorted at every depth", input: `{"z":{"y":1,"x":2}}`, want: `{"z":{"x":2,"y":1}}`},
		{name: "array order preserved", input: `[3,1,2]`, want: `[3,1,2]`},
		{name: "whitespace dropped", input: "{\n  \"a\" : 1\n}", want: `{"a":1}`},
		{name: "integral floats lose the fraction", input: `{"a":1.0}`, want: `{"a":1}`},
		{name: "fractional preserved", input: `{"a":1.5}`, want: `{"a":1.5}`},
		{name: "negative integer", input: `-7`, want: `-7`},
		{name: "null and bools", input: `{"a":null,"b":true,"c":false}`, want: `{"a":null,"b":true,"c":false}`},
		{name: "empty containers", input: `{"a":{},"b":[]}`, want: `{"a":{},"b":[]}`},
		{name: "unicode string", input: `{"a":"café ☕"}`, want: `{"a":"café ☕"}`},
		{name: "html not escaped", input: `{"a":"x<y&z"}`, want: `{"a":"x<y&z"}`},
		{name: "quotes escaped", input: `{"a":"say \"hi\""}`, want: `{"a":"say \"hi\""}`},
		{name: "beyond exact int range refused", input: `9007199254740993`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var decoded any
			if err := json.Unmarshal([]byte(tt.input), &decoded); err != nil {
				t.Fatalf("test input is not valid JSON: %v", err)
			}
			got, err := Encode(decoded)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Encode(%s) = %s, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Encode(%s) error: %v", tt.input, err)
			}
			if string(got) != tt.want {
				t.Errorf("Encode(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

// TestEncode_KeyOrderIndependence is the property the whole digest scheme rests
// on: the same logical value spelled in a different key order must encode
// identically, because Postgres jsonb does not preserve the order it was given.
func TestEncode_KeyOrderIndependence(t *testing.T) {
	spellings := []string{
		`{"changes":[{"field":"a","new":"2","old":"1"}],"create":null}`,
		`{"create":null,"changes":[{"old":"1","new":"2","field":"a"}]}`,
		`{"changes":[{"new":"2","field":"a","old":"1"}],"create":null}`,
	}
	var want string
	for i, s := range spellings {
		var decoded any
		if err := json.Unmarshal([]byte(s), &decoded); err != nil {
			t.Fatalf("spelling %d invalid: %v", i, err)
		}
		got, err := Encode(decoded)
		if err != nil {
			t.Fatalf("spelling %d: %v", i, err)
		}
		if i == 0 {
			want = string(got)
			continue
		}
		if string(got) != want {
			t.Errorf("spelling %d encoded to %s, want %s", i, got, want)
		}
	}
}

// TestDigest_SurvivesJSONBRoundTrip models the actual approve->store->drain
// path: the approve side digests a typed Go value, the drain side digests what
// came back out of jsonb as map[string]any with keys in another order. If these
// two ever disagree, every signed row fails verification at push time.
func TestDigest_SurvivesJSONBRoundTrip(t *testing.T) {
	type fieldChange struct {
		Field string `json:"field"`
		Old   string `json:"old"`
		New   string `json:"new"`
		Value any    `json:"value,omitempty"`
	}
	type proposedDiff struct {
		Changes []fieldChange `json:"changes,omitempty"`
	}

	typed := proposedDiff{Changes: []fieldChange{
		{Field: "selectedSubjects", Old: "1:A", New: "2:B", Value: []map[string]any{{"id": 2, "name": "B"}}},
		{Field: "dailyBudget", Old: "10", New: "20"},
	}}

	fromTyped, err := Digest(typed)
	if err != nil {
		t.Fatalf("digest typed: %v", err)
	}

	// What a jsonb read produces: generic maps, arbitrary key order, numbers
	// as float64.
	raw, err := json.Marshal(typed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fromGeneric, err := Digest(generic)
	if err != nil {
		t.Fatalf("digest generic: %v", err)
	}

	if fromTyped != fromGeneric {
		t.Errorf("digest differs across the jsonb round trip:\n typed   = %s\n generic = %s", fromTyped, fromGeneric)
	}
}

func TestDigest_DetectsMutation(t *testing.T) {
	base := map[string]any{"field": "dailyBudget", "old": "10", "new": "20"}
	baseDigest, err := Digest(base)
	if err != nil {
		t.Fatalf("digest base: %v", err)
	}

	tests := []struct {
		name    string
		mutated map[string]any
	}{
		{name: "value changed", mutated: map[string]any{"field": "dailyBudget", "old": "10", "new": "9999"}},
		{name: "field renamed", mutated: map[string]any{"field": "flatFee", "old": "10", "new": "20"}},
		{name: "key added", mutated: map[string]any{"field": "dailyBudget", "old": "10", "new": "20", "extra": "x"}},
		{name: "key removed", mutated: map[string]any{"field": "dailyBudget", "new": "20"}},
		{name: "string vs number", mutated: map[string]any{"field": "dailyBudget", "old": "10", "new": 20}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Digest(tt.mutated)
			if err != nil {
				t.Fatalf("digest: %v", err)
			}
			if got == baseDigest {
				t.Errorf("mutation %q produced the same digest %s", tt.name, got)
			}
		})
	}
}

func TestEncode_RejectsUndecodedTypes(t *testing.T) {
	// A typed struct never comes out of a decode-into-any; Encode must refuse
	// rather than invent an encoding for it. Digest is the supported entry
	// point for typed values.
	_, err := Encode(struct{ A int }{A: 1})
	if err == nil {
		t.Fatal("Encode(struct) = nil error, want refusal")
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("error = %v, want an unsupported-type refusal", err)
	}
}
