// Card Ladder Cloud Function + Firestore integration tests.
// Run with: go test ./internal/integration/ -tags integration -v -run TestCardLadder -timeout 2m
//
//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	_ "github.com/joho/godotenv/autoload"
)

// newCLClient logs in with CL_EMAIL / CL_PASSWORD / CL_FIREBASE_API_KEY from .env
// and returns a ready client. Skips the test if credentials are not set.
func newCLClient(t *testing.T) (*cardladder.Client, string) {
	t.Helper()
	email := os.Getenv("CL_EMAIL")
	password := os.Getenv("CL_PASSWORD")
	apiKey := os.Getenv("CL_FIREBASE_API_KEY")
	if email == "" || password == "" || apiKey == "" {
		t.Skip("CL_EMAIL, CL_PASSWORD, and CL_FIREBASE_API_KEY required — add to .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	auth := cardladder.NewFirebaseAuth(apiKey)
	resp, err := auth.Login(ctx, email, password)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	t.Logf("Logged in as %q (uid=%s)", email, resp.LocalID)

	c := cardladder.NewClient(
		cardladder.WithStaticToken(resp.IDToken),
	)
	return c, resp.LocalID
}

// TestCardLadder_BuildCollectionCard tests resolving a cert number to card metadata.
func TestCardLadder_BuildCollectionCard(t *testing.T) {
	client, _ := newCLClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use a known cert number from the Fiddler captures
	cert := os.Getenv("CL_TEST_CERT")
	if cert == "" {
		cert = "69145695" // Pikachu-Holo SM162 PSA 9
	}

	resp, err := client.BuildCollectionCard(ctx, cert, "psa")
	if err != nil {
		t.Fatalf("BuildCollectionCard: %v", err)
	}

	t.Logf("BuildCollectionCard result:")
	t.Logf("  Player:       %s", resp.Player)
	t.Logf("  Set:          %s", resp.Set)
	t.Logf("  Number:       %s", resp.Number)
	t.Logf("  Condition:    %s", resp.Condition)
	t.Logf("  GemRateID:    %s", resp.GemRateID)
	t.Logf("  GemRateCond:  %s", resp.GemRateCondition)
	t.Logf("  Pop:          %d", resp.Pop)
	t.Logf("  Year:         %s", resp.Year)
	t.Logf("  Category:     %s", resp.Category)
	t.Logf("  Variation:    %s", resp.Variation)
	t.Logf("  GradingCo:    %s", resp.GradingCompany)

	if resp.GemRateID == "" {
		t.Error("expected non-empty gemRateID")
	}
	if resp.Player == "" {
		t.Error("expected non-empty player")
	}
}

// TestCardLadder_CardEstimate tests getting a market value estimate.
func TestCardLadder_CardEstimate(t *testing.T) {
	client, _ := newCLClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First build the card to get the gemRateID
	cert := os.Getenv("CL_TEST_CERT")
	if cert == "" {
		cert = "69145695"
	}

	buildResp, err := client.BuildCollectionCard(ctx, cert, "psa")
	if err != nil {
		t.Fatalf("BuildCollectionCard: %v", err)
	}

	// Now estimate its value
	estResp, err := client.CardEstimate(ctx, cardladder.CardEstimateRequest{
		GemRateID:      buildResp.GemRateID,
		GradingCompany: buildResp.GradingCompany,
		Condition:      buildResp.GemRateCondition,
		Description:    buildResp.Player,
	})
	if err != nil {
		t.Fatalf("CardEstimate: %v", err)
	}

	t.Logf("CardEstimate result:")
	t.Logf("  EstimatedValue: $%.2f", estResp.EstimatedValue)
	t.Logf("  Confidence:     %d/5", estResp.Confidence)
	t.Logf("  LastSalePrice:  $%.2f", estResp.LastSalePrice)
	t.Logf("  LastSaleDate:   %s", estResp.LastSaleDate)
	t.Logf("  Index:          %s", estResp.Index)
	t.Logf("  2wk velocity:   %d sales, avg $%.2f", estResp.TwoWeekData.Velocity, estResp.TwoWeekData.AveragePrice)
	t.Logf("  1mo velocity:   %d sales, avg $%.2f", estResp.OneMonthData.Velocity, estResp.OneMonthData.AveragePrice)
	t.Logf("  1qtr velocity:  %d sales, avg $%.2f", estResp.OneQuarterData.Velocity, estResp.OneQuarterData.AveragePrice)
	t.Logf("  1yr velocity:   %d sales, avg $%.2f", estResp.OneYearData.Velocity, estResp.OneYearData.AveragePrice)

	if estResp.EstimatedValue <= 0 {
		t.Error("expected positive estimated value")
	}
}

// TestCardLadder_CreateCollectionCard tests writing a card to Firestore.
// This actually creates a card in the collection — use with care.
// Set CL_TEST_WRITE=true to enable.
func TestCardLadder_CreateCollectionCard(t *testing.T) {
	if os.Getenv("CL_TEST_WRITE") != "true" {
		t.Skip("CL_TEST_WRITE=true required to run Firestore write test")
	}

	client, uid := newCLClient(t)
	collectionID := os.Getenv("CL_COLLECTION_ID")
	if collectionID == "" {
		t.Skip("CL_COLLECTION_ID required for write test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cert := os.Getenv("CL_TEST_CERT")
	if cert == "" {
		cert = "69145695"
	}

	// Build + Estimate
	buildResp, err := client.BuildCollectionCard(ctx, cert, "psa")
	if err != nil {
		t.Fatalf("BuildCollectionCard: %v", err)
	}

	estResp, err := client.CardEstimate(ctx, cardladder.CardEstimateRequest{
		GemRateID:      buildResp.GemRateID,
		GradingCompany: buildResp.GradingCompany,
		Condition:      buildResp.GemRateCondition,
		Description:    buildResp.Player,
	})
	if err != nil {
		t.Fatalf("CardEstimate: %v", err)
	}

	// Write to Firestore
	input := cardladder.AddCollectionCardInput{
		Label:            buildResp.Year + " " + buildResp.Set + " " + buildResp.Player + " #" + buildResp.Number + " " + buildResp.Condition,
		Player:           buildResp.Player,
		PlayerIndexID:    estResp.IndexID,
		Category:         buildResp.Category,
		Year:             buildResp.Year,
		Set:              buildResp.Set,
		Number:           buildResp.Number,
		Variation:        buildResp.Variation,
		Condition:        buildResp.Condition,
		GradingCompany:   buildResp.GradingCompany,
		GemRateID:        buildResp.GemRateID,
		GemRateCondition: buildResp.GemRateCondition,
		SlabSerial:       buildResp.SlabSerial,
		Pop:              buildResp.Pop,
		ImageURL:         buildResp.ImageURL,
		ImageBackURL:     buildResp.ImageBackURL,
		CurrentValue:     estResp.EstimatedValue,
		Investment:       200, // test value
		DatePurchased:    time.Date(2025, 12, 8, 0, 0, 0, 0, time.UTC),
	}

	docName, err := client.CreateCollectionCard(ctx, uid, collectionID, input)
	if err != nil {
		t.Fatalf("CreateCollectionCard: %v", err)
	}

	t.Logf("Created Firestore document: %s", docName)
	t.Logf("Card: %s — $%.2f", input.Label, input.CurrentValue)
}
