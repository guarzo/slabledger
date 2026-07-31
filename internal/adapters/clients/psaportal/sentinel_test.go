package psaportal

import "testing"

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
