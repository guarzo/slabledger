package psaportal

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestDecodeSvelteKitValue_Analytics(t *testing.T) {
	raw, err := os.ReadFile("testdata/analytics_data.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeSvelteKitValue(raw, "embedUrl")
	if err != nil {
		t.Fatalf("DecodeSvelteKitValue: %v", err)
	}
	want := `"https://collectors.lightdash.cloud/embed/e4995db3-cb94-4a66-9b19-7bb36f156e33#TOKEN_REDACTED"`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDecodeSvelteKitValue_Nested(t *testing.T) {
	raw, _ := os.ReadFile("testdata/campaigns_data.json")
	got, err := DecodeSvelteKitValue(raw, "campaignsResponse")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"items":[],"pageNumber":0,"pageSize":50,"totalCount":0}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDecodeSvelteKitValue_MissingKey(t *testing.T) {
	raw, _ := os.ReadFile("testdata/analytics_data.json")
	if _, err := DecodeSvelteKitValue(raw, "nope"); err == nil {
		t.Fatal("expected error for missing key")
	}
}

// unflatten serves DecodeSvelteKitValue, which returns raw JSON bytes and never
// re-encodes, so sentinels are intentionally flattened to null there. What must
// NOT happen is an error that aborts the whole decode.
func TestUnflatten_SentinelsBecomeNull(t *testing.T) {
	tests := []struct {
		name string
		ptr  int
	}{
		{name: "undefined", ptr: -1},
		{name: "hole", ptr: -2},
		{name: "nan", ptr: -3},
		{name: "posinf", ptr: -4},
		{name: "neginf", ptr: -5},
		{name: "negzero", ptr: -6},
		{name: "sparse", ptr: -7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []json.RawMessage{json.RawMessage(fmt.Sprintf(`{"k":%d}`, tt.ptr))}
			got, err := unflatten(data, 0, map[int]json.RawMessage{})
			if err != nil {
				t.Fatalf("unflatten: %v", err)
			}
			if string(got) != `{"k":null}` {
				t.Errorf("got %s, want {\"k\":null}", got)
			}
		})
	}
}

func TestUnflatten_MalformedPointerStillErrors(t *testing.T) {
	data := []json.RawMessage{json.RawMessage(`{"k":-8}`)}
	if _, err := unflatten(data, 0, map[int]json.RawMessage{}); err == nil {
		t.Fatal("expected error for pointer -8")
	}
}
