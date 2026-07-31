package psaportal

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSentinelFor(t *testing.T) {
	tests := []struct {
		name    string
		ptr     int
		want    Sentinel
		wantOK  bool
		wantStr string
	}{
		{name: "undefined", ptr: -1, want: SentinelUndefined, wantOK: true, wantStr: "undefined"},
		{name: "hole", ptr: -2, want: SentinelHole, wantOK: true, wantStr: "hole"},
		{name: "nan", ptr: -3, want: SentinelNaN, wantOK: true, wantStr: "NaN"},
		{name: "posinf", ptr: -4, want: SentinelPosInf, wantOK: true, wantStr: "Infinity"},
		{name: "neginf", ptr: -5, want: SentinelNegInf, wantOK: true, wantStr: "-Infinity"},
		{name: "negzero", ptr: -6, want: SentinelNegZero, wantOK: true, wantStr: "-0"},
		{name: "sparse", ptr: -7, want: SentinelSparse, wantOK: true, wantStr: "sparse"},
		{name: "below range", ptr: -8, wantOK: false},
		{name: "zero is a real slot", ptr: 0, wantOK: false},
		{name: "positive is a real slot", ptr: 5, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := sentinelFor(tt.ptr)
			if ok != tt.wantOK {
				t.Fatalf("sentinelFor(%d) ok = %v, want %v", tt.ptr, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got != tt.want {
				t.Errorf("sentinelFor(%d) = %d, want %d", tt.ptr, got, tt.want)
			}
			if got.String() != tt.wantStr {
				t.Errorf("Sentinel(%d).String() = %q, want %q", got, got.String(), tt.wantStr)
			}
		})
	}
}

func TestDecodeRefPacked_Sentinels(t *testing.T) {
	tests := []struct {
		name   string
		packed []json.RawMessage
		want   any
	}{
		{
			name:   "undefined in object",
			packed: []json.RawMessage{json.RawMessage(`{"k":-1}`)},
			want:   map[string]any{"k": SentinelUndefined},
		},
		{
			name:   "hole in object",
			packed: []json.RawMessage{json.RawMessage(`{"k":-2}`)},
			want:   map[string]any{"k": SentinelHole},
		},
		{
			name:   "nan in object",
			packed: []json.RawMessage{json.RawMessage(`{"k":-3}`)},
			want:   map[string]any{"k": SentinelNaN},
		},
		{
			name:   "positive infinity in object",
			packed: []json.RawMessage{json.RawMessage(`{"k":-4}`)},
			want:   map[string]any{"k": SentinelPosInf},
		},
		{
			name:   "negative infinity in object",
			packed: []json.RawMessage{json.RawMessage(`{"k":-5}`)},
			want:   map[string]any{"k": SentinelNegInf},
		},
		{
			name:   "negative zero in object",
			packed: []json.RawMessage{json.RawMessage(`{"k":-6}`)},
			want:   map[string]any{"k": SentinelNegZero},
		},
		{
			name:   "sparse in object",
			packed: []json.RawMessage{json.RawMessage(`{"k":-7}`)},
			want:   map[string]any{"k": SentinelSparse},
		},
		{
			name:   "sentinel in array",
			packed: []json.RawMessage{json.RawMessage(`[1,-1]`), json.RawMessage(`"a"`)},
			want:   []any{"a", SentinelUndefined},
		},
		{
			name:   "sentinel alongside real values",
			packed: []json.RawMessage{json.RawMessage(`{"a":1,"b":-1,"c":2}`), json.RawMessage(`"x"`), json.RawMessage(`3`)},
			want:   map[string]any{"a": "x", "b": SentinelUndefined, "c": float64(3)},
		},
		{
			name:   "null stays null and is distinct from undefined",
			packed: []json.RawMessage{json.RawMessage(`{"n":1,"u":-1}`), json.RawMessage(`null`)},
			want:   map[string]any{"n": nil, "u": SentinelUndefined},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeRefPacked(tt.packed)
			if err != nil {
				t.Fatalf("DecodeRefPacked: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDecodeRefPacked_MalformedPointer(t *testing.T) {
	// -8 is below the lowest devalue sentinel: still an error, not a Sentinel.
	if _, err := DecodeRefPacked([]json.RawMessage{json.RawMessage(`{"k":-8}`)}); err == nil {
		t.Fatal("expected error for pointer -8")
	}
	if _, err := DecodeRefPacked([]json.RawMessage{json.RawMessage(`{"k":99}`)}); err == nil {
		t.Fatal("expected error for out-of-range pointer 99")
	}
}
