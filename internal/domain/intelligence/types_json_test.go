package intelligence

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSuggestionJSONShape pins the wire format of the
// GET /api/dh/suggestions and GET /api/dh/suggestions/inventory-alerts bodies,
// which serve intelligence.Suggestion directly.
//
// This exists because the struct previously had no JSON tags, so it was the one
// endpoint in the API serving Go field names ("SuggestionDate") while every
// other endpoint used camelCase. Nothing broke, because nothing in web/ consumed
// it yet — which is exactly why the shape is worth pinning now, before a
// consumer appears and makes the keys expensive to change.
//
// The whole document is compared rather than individual keys, so a new field
// leaking onto the wire fails here too.
func TestSuggestionJSONShape(t *testing.T) {
	tests := []struct {
		name string
		in   Suggestion
		want string
	}{
		{
			name: "fully populated suggestion",
			in: Suggestion{
				SuggestionDate:      "2025-01-15",
				Type:                "cards",
				Category:            "hottest_cards",
				Rank:                1,
				IsManual:            false,
				DHCardID:            "12345",
				CardName:            "Charizard",
				SetName:             "Base Set",
				CardNumber:          "4",
				ImageURL:            "https://example.test/charizard.png",
				CurrentPriceCents:   120000,
				ConfidenceScore:     0.95,
				Reasoning:           "Strong demand",
				StructuredReasoning: `{"driver":"demand"}`,
				Metrics:             `{"views":400}`,
				SentimentScore:      0.4,
				SentimentTrend:      0.1,
				SentimentMentions:   12,
				FetchedAt:           time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
			},
			want: `{"suggestionDate":"2025-01-15","type":"cards","category":"hottest_cards",` +
				`"rank":1,"isManual":false,"dhCardId":"12345","cardName":"Charizard",` +
				`"setName":"Base Set","cardNumber":"4",` +
				`"imageUrl":"https://example.test/charizard.png",` +
				`"currentPriceCents":120000,"confidenceScore":0.95,"reasoning":"Strong demand",` +
				`"structuredReasoning":"{\"driver\":\"demand\"}","metrics":"{\"views\":400}",` +
				`"sentimentScore":0.4,"sentimentTrend":0.1,"sentimentMentions":12,` +
				`"fetchedAt":"2025-01-15T10:00:00Z"}`,
		},
		{
			// StructuredReasoning and Metrics hold pre-encoded JSON. They are
			// declared as strings, so they arrive quoted and escaped rather than
			// as nested objects — a consumer has to parse them a second time.
			// Asserting the empty case proves they are not omitempty, so that
			// second parse always has something to fail on rather than
			// intermittently seeing a missing key.
			name: "zero value carries every key",
			in:   Suggestion{},
			want: `{"suggestionDate":"","type":"","category":"","rank":0,"isManual":false,` +
				`"dhCardId":"","cardName":"","setName":"","cardNumber":"","imageUrl":"",` +
				`"currentPriceCents":0,"confidenceScore":0,"reasoning":"",` +
				`"structuredReasoning":"","metrics":"","sentimentScore":0,"sentimentTrend":0,` +
				`"sentimentMentions":0,"fetchedAt":"0001-01-01T00:00:00Z"}`,
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
