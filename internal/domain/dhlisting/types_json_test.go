package dhlisting

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestDHListingResultJSONShape pins the wire format of the
// POST /api/purchases/{purchaseId}/list-on-dh success body.
//
// This exists because the struct previously had no JSON tags, so it serialised
// with Go field names ("Listed") while web/src/js/api/campaignPurchases.ts
// declared the lowercase shape. Nothing failed, because every caller discards
// the value — the mismatch would only have surfaced as a silent `undefined` for
// whoever first read a field. Comparing whole documents rather than individual
// keys is deliberate: it catches a *new* field leaking onto the wire too.
func TestDHListingResultJSONShape(t *testing.T) {
	tests := []struct {
		name string
		in   DHListingResult
		want string
	}{
		{
			name: "success path carries lowercase counters only",
			in:   DHListingResult{Listed: 1, Synced: 1, Skipped: 0, Total: 1},
			want: `{"listed":1,"synced":1,"skipped":0,"total":1,"paused":false}`,
		},
		{
			// The handler converts both error fields into status codes before it
			// writes a 200, so this state never reaches the wire. Asserting it
			// anyway proves the `json:"-"` tags hold: without them a non-nil
			// error marshals as a content-free `{}`.
			name: "error fields never reach the wire",
			in: DHListingResult{
				Error:       errors.New("db lookup failed"),
				FailedCerts: map[string]error{"12345678": errors.New("unmatched")},
			},
			want: `{"listed":0,"synced":0,"skipped":0,"total":0,"paused":false}`,
		},
		{
			name: "paused batch",
			in:   DHListingResult{Skipped: 2, Total: 2, Paused: true},
			want: `{"listed":0,"synced":0,"skipped":2,"total":2,"paused":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("json.Marshal() =\n  %s\nwant\n  %s", got, tt.want)
			}
		})
	}
}
