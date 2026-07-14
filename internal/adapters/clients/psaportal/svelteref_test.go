package psaportal

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestEncodeRefPacked_RoundTrip(t *testing.T) {
	// A packed array equivalent to {"id":"x","formData":{"bidPercentage":72}}
	src := map[string]any{
		"id": "x",
		"formData": map[string]any{
			"bidPercentage": float64(72),
			"names":         []any{"a", "a"}, // dup to exercise scalar reuse safety
		},
	}
	packed, err := EncodeRefPacked(src)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	back, err := DecodeRefPacked(packed)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reflect.DeepEqual(back, src) {
		b1, _ := json.Marshal(src)
		b2, _ := json.Marshal(back)
		t.Errorf("round-trip mismatch:\n want %s\n got  %s", b1, b2)
	}
}

func TestDecodeRefPacked_RealEditPayload(t *testing.T) {
	raw, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	data, err := packedFromEnvelope(raw)
	if err != nil {
		t.Fatalf("envelope: %v", err)
	}
	v, err := DecodeRefPacked(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	root, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("root not object: %T", v)
	}
	if _, ok := root["formData"]; !ok {
		t.Error("expected formData key in decoded edit payload")
	}
}
