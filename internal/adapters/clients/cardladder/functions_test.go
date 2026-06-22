package cardladder

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

func TestClient_BuildCollectionCard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Fatalf("unexpected auth: %s", auth)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var req callableRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		dataMap, ok := req.Data.(map[string]any)
		if !ok {
			t.Fatalf("data is not a map: %T", req.Data)
		}
		if dataMap["cert"] != "69145695" {
			t.Errorf("cert = %v, want 69145695", dataMap["cert"])
		}
		if dataMap["grader"] != "psa" {
			t.Errorf("grader = %v, want psa", dataMap["grader"])
		}

		json.NewEncoder(w).Encode(callableResponse[BuildCardResponse]{ //nolint:errcheck
			Result: BuildCardResponse{
				Pop:              4673,
				Year:             "2019",
				Set:              "Pokemon Sm Black Star Promo",
				Category:         "Pokemon",
				Number:           "SM162",
				Player:           "Pikachu-Holo",
				Variation:        "Promo-Tm.up Sngl.pk.blst.",
				Condition:        "PSA 9",
				GemRateID:        "fa48643a1a3fa08799b6913f46d1643427b5d6e8",
				GemRateCondition: "g9",
				SlabSerial:       "69145695",
				GradingCompany:   "psa",
			},
		})
	}))
	defer server.Close()

	client := NewClient(
		WithFunctionsURL(server.URL),
		WithStaticToken("test-token"),
	)
	resp, err := client.BuildCollectionCard(context.Background(), "69145695", "psa")
	if err != nil {
		t.Fatalf("BuildCollectionCard failed: %v", err)
	}
	if resp.GemRateID != "fa48643a1a3fa08799b6913f46d1643427b5d6e8" {
		t.Errorf("gemRateID = %q, want fa48...", resp.GemRateID)
	}
	if resp.Player != "Pikachu-Holo" {
		t.Errorf("player = %q, want Pikachu-Holo", resp.Player)
	}
	if resp.Pop != 4673 {
		t.Errorf("pop = %d, want 4673", resp.Pop)
	}
}

func TestClient_CardEstimate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var req callableRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		dataMap, ok := req.Data.(map[string]any)
		if !ok {
			t.Fatalf("data is not a map: %T", req.Data)
		}
		if dataMap["profileId"] != "psa-1813135" {
			t.Errorf("profileId = %v, want psa-1813135", dataMap["profileId"])
		}

		json.NewEncoder(w).Encode(callableResponse[CardEstimateResponse]{ //nolint:errcheck
			Result: CardEstimateResponse{
				EstimatedValue: 210,
				Confidence:     5,
				Grader:         "psa",
				Grade:          "g9",
				Index:          "Pikachu",
				TwoWeekData:    VelocityData{Velocity: 37, AveragePrice: 200.54},
			},
		})
	}))
	defer server.Close()

	client := NewClient(
		WithFunctionsURL(server.URL),
		WithStaticToken("test-token"),
	)
	resp, err := client.CardEstimate(context.Background(), CardEstimateRequest{
		GemRateID:      "psa-1813135",
		GradingCompany: "psa",
		Condition:      "g8",
		Description:    "Test Card",
	})
	if err != nil {
		t.Fatalf("CardEstimate failed: %v", err)
	}
	if resp.EstimatedValue != 210 {
		t.Errorf("estimatedValue = %f, want 210", resp.EstimatedValue)
	}
	if resp.Confidence != 5 {
		t.Errorf("confidence = %d, want 5", resp.Confidence)
	}
	if resp.TwoWeekData.Velocity != 37 {
		t.Errorf("twoWeekData.velocity = %d, want 37", resp.TwoWeekData.Velocity)
	}
}

func TestClient_CreateCollectionCard(t *testing.T) {
	// defaultFirestoreBaseURL is a const and can't be overridden with httptest,
	// so we test buildCardDocument directly instead of the full HTTP flow.
	input := AddCollectionCardInput{
		Label:            "2019 Pokemon Sm Black Star Promo Pikachu-Holo SM162",
		Player:           "Pikachu-Holo",
		PlayerIndexID:    "Pikachu",
		Category:         "Pokemon",
		Year:             "2019",
		Set:              "Pokemon Sm Black Star Promo",
		Number:           "SM162",
		Variation:        "Promo-Tm.up Sngl.pk.blst.",
		Condition:        "PSA 9",
		GradingCompany:   "psa",
		GemRateID:        "abc123",
		GemRateCondition: "g9",
		SlabSerial:       "69145695",
		Pop:              4673,
		CurrentValue:     210,
		Investment:       200,
		DatePurchased:    time.Date(2025, 12, 8, 3, 24, 0, 0, time.UTC),
	}
	doc := buildCardDocument("user123", "coll456", input, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))

	if v := firestoreString(doc.Fields, "player"); v != "Pikachu-Holo" {
		t.Errorf("player = %q, want Pikachu-Holo", v)
	}
	if v := firestoreString(doc.Fields, "uid"); v != "user123" {
		t.Errorf("uid = %q, want user123", v)
	}
	if v := firestoreString(doc.Fields, "collectionId"); v != "coll456" {
		t.Errorf("collectionId = %q, want coll456", v)
	}
	if doc.Fields["investment"].IntegerValue == nil || *doc.Fields["investment"].IntegerValue != "200" {
		t.Errorf("investment = %v, want 200", doc.Fields["investment"].IntegerValue)
	}
	if doc.Fields["profit"].IntegerValue == nil || *doc.Fields["profit"].IntegerValue != "10" {
		t.Errorf("profit = %v, want 10", doc.Fields["profit"].IntegerValue)
	}
	if doc.Fields["pop"].IntegerValue == nil || *doc.Fields["pop"].IntegerValue != "4673" {
		t.Errorf("pop = %v, want 4673", doc.Fields["pop"].IntegerValue)
	}
	if doc.Fields["datePurchased"].TimestampValue == nil {
		t.Error("datePurchased should be set")
	}
}

func TestClient_BuildCollectionCard_NewContract(t *testing.T) {
	// Real httpbuildcollectioncard shape as of the 2026-06 CL migration:
	// profileId + grade present; gemRateId / gemRateCondition / condition absent.
	raw := `{"result":{"profileId":"psa-1813135","grade":"g8","player":"Articuno-Holo",` +
		`"set":"Pokemon Japanese Web","year":"1999","number":"17","category":"Pokemon","pop":42}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(raw)) //nolint:errcheck
	}))
	defer server.Close()

	client := NewClient(WithFunctionsURL(server.URL), WithStaticToken("test-token"))
	resp, err := client.BuildCollectionCard(context.Background(), "158507531", "psa")
	if err != nil {
		t.Fatalf("BuildCollectionCard failed: %v", err)
	}
	if resp.GemRateID != "psa-1813135" {
		t.Errorf("GemRateID = %q, want psa-1813135 (from profileId)", resp.GemRateID)
	}
	if resp.GemRateCondition != "g8" {
		t.Errorf("GemRateCondition = %q, want g8 (from grade)", resp.GemRateCondition)
	}
	if resp.Condition != "PSA 8" {
		t.Errorf("Condition = %q, want PSA 8 (derived display form)", resp.Condition)
	}
	if resp.Set != "Pokemon Japanese Web" {
		t.Errorf("Set = %q, want Pokemon Japanese Web", resp.Set)
	}
}

func TestDoCallable_MissingResult(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{
			name:     "no result key",
			response: `{}`,
		},
		{
			name:     "result is null",
			response: `{"result": null}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tc.response)) //nolint:errcheck
			}))
			defer server.Close()

			client := NewClient(
				WithFunctionsURL(server.URL),
				WithStaticToken("test-token"),
			)

			_, err := client.BuildCollectionCard(context.Background(), "12345", "psa")
			if err == nil {
				t.Fatal("expected error for missing/null result, got nil")
			}

			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected AppError, got %T: %v", err, err)
			}
			if appErr.Code != apperrors.ErrCodeProviderInvalidResp {
				t.Errorf("error code = %q, want %q", appErr.Code, apperrors.ErrCodeProviderInvalidResp)
			}
		})
	}
}
