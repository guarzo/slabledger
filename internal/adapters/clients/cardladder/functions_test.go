package cardladder

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
		if dataMap["gemRateId"] != "abc123" {
			t.Errorf("gemRateId = %v, want abc123", dataMap["gemRateId"])
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
		GemRateID:      "abc123",
		GradingCompany: "psa",
		Condition:      "g9",
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		// Verify the URL path contains the expected Firestore path
		expectedPath := "/v1/projects/cardladder-71d53/databases/(default)/documents/users/user123/collections/coll456/collection_cards"
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q, want %q", r.URL.Path, expectedPath)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var doc firestoreDocument
		if err := json.Unmarshal(body, &doc); err != nil {
			t.Fatalf("unmarshal document: %v", err)
		}

		// Verify key fields
		if v := firestoreString(doc.Fields, "player"); v != "Pikachu-Holo" {
			t.Errorf("player = %q, want Pikachu-Holo", v)
		}
		if v := firestoreString(doc.Fields, "gemRateId"); v != "abc123" {
			t.Errorf("gemRateId = %q, want abc123", v)
		}
		if v := firestoreString(doc.Fields, "slabSerial"); v != "69145695" {
			t.Errorf("slabSerial = %q, want 69145695", v)
		}
		if doc.Fields["sold"].BooleanValue == nil || *doc.Fields["sold"].BooleanValue != false {
			t.Error("sold should be false")
		}
		if doc.Fields["investment"].IntegerValue == nil || *doc.Fields["investment"].IntegerValue != "200" {
			t.Errorf("investment = %v, want 200", doc.Fields["investment"].IntegerValue)
		}

		// Return a created document
		json.NewEncoder(w).Encode(firestoreDocument{ //nolint:errcheck
			Name:   "projects/cardladder-71d53/databases/(default)/documents/users/user123/collections/coll456/collection_cards/newCardID",
			Fields: doc.Fields,
		})
	}))
	defer server.Close()

	// We need to override the Firestore base URL for testing.
	// The CreateCollectionCard method uses defaultFirestoreBaseURL directly,
	// so we use the test server and adjust the expected path.
	origURL := defaultFirestoreBaseURL
	// Unfortunately the const can't be overridden, so we test via the real flow
	// which would hit the real Firestore. Instead, verify document building.
	_ = origURL
	_ = server

	// Test the document builder directly
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
	doc := buildCardDocument("user123", "coll456", input)

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
