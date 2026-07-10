package psaportal

import (
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
