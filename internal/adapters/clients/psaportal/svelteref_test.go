package psaportal

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Missing metadata or logging a redirect's credentials would hide the failure
// cause or leak session state through the returned error.
func TestMissingEnvelopeDataDiagnostics(t *testing.T) {
	cases := []struct {
		name, raw string
		want      []string
	}{
		{"skip nodes", `{"type":"data","nodes":[null,{"type":"skip"},{"type":"skip"}]}`,
			[]string{`type="data"`, `nodes=["" "skip" "skip"]`}},
		{"unknown node", `{"type":"data","nodes":[{"type":"future","body":"PRIVATE"}]}`,
			[]string{`nodes=["future"]`}},
		{"empty envelope", `{}`, []string{`type=""`, `nodes=[]`}},
		{"relative redirect", `{"type":"redirect","location":"/signin?token=PRIVATE#PRIVATE"}`,
			[]string{`type="redirect"`, `redirect="/signin"`}},
		{"absolute redirect", `{"type":"redirect","location":"https://PRIVATE:PRIVATE@app.collectors.com/signin?token=PRIVATE#PRIVATE"}`,
			[]string{`redirect="https://app.collectors.com/signin"`}},
		{"invalid redirect", `{"type":"redirect","location":"https://PRIVATE%zz"}`,
			[]string{`redirect="[redacted]"`}},
		{"opaque redirect", `{"type":"redirect","location":"javascript:PRIVATE"}`,
			[]string{`redirect="[redacted]"`}},
		{"path token", `{"type":"redirect","location":"https://app.collectors.com/auth/callback/PRIVATE"}`,
			[]string{`redirect="https://app.collectors.com/REDACTED"`}},
		{"path parameter", `{"type":"redirect","location":"/signin;jsessionid=PRIVATE"}`,
			[]string{`redirect="/REDACTED"`}},
		{"query only", `{"type":"redirect","location":"?code=PRIVATE"}`,
			[]string{`redirect="[redacted]"`}},
		{"bounded node metadata", `{"type":"data","nodes":[` + strings.Repeat(`{"type":"skip"},`, 8) + `{"type":"PRIVATE"}]}`,
			[]string{`nodes=["skip" "skip" "skip" "skip" "skip" "skip" "skip" "skip" "…"]`}},
	}
	decoders := []struct {
		name string
		run  func([]byte) error
	}{
		{"reference", func(raw []byte) error { _, err := packedFromEnvelope(raw); return err }},
		{"JSON", func(raw []byte) error { _, err := DecodeSvelteKitValue(raw, "formData"); return err }},
	}
	for _, decoder := range decoders {
		for _, tc := range cases {
			t.Run(decoder.name+"/"+tc.name, func(t *testing.T) {
				err := decoder.run([]byte(tc.raw))
				if err == nil || !strings.Contains(err.Error(), "no data node") {
					t.Fatalf("expected missing-data error, got %v", err)
				}
				for _, want := range tc.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing %q", err, want)
					}
				}
				if strings.Contains(err.Error(), "PRIVATE") {
					t.Errorf("error exposed private response fields: %s", err)
				}
			})
		}
	}
}

func TestEnvelopeLocationDoesNotAffectData(t *testing.T) {
	for _, location := range []string{`{}`, `42`, `false`} {
		t.Run(location, func(t *testing.T) {
			raw := []byte(`{"location":` + location + `,"nodes":[{"type":"data","data":[{"formData":1},"ok"]}]}`)
			if _, err := packedFromEnvelope(raw); err != nil {
				t.Errorf("reference decode rejected valid data: %v", err)
			}
			if got, err := DecodeSvelteKitValue(raw, "formData"); err != nil || string(got) != `"ok"` {
				t.Errorf("JSON decode = %s, %v; want ok", got, err)
			}
		})
	}
}

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
	raw, err := os.ReadFile("testdata/psa-campaign-edit-raw.json")
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
