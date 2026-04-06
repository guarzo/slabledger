package fusionprice

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// Compile-time check that DHAdapter implements SecondaryPriceSource.
var _ fusion.SecondaryPriceSource = (*DHAdapter)(nil)

// dhSourceConfidence is the confidence score assigned to DH price data.
const dhSourceConfidence = 0.90

// DHMarketDataClient is the subset of the dh.Client used by the adapter.
type DHMarketDataClient interface {
	RecentSales(ctx context.Context, cardID int) ([]dh.RecentSale, error)
}

// DHCardIDLookup resolves card names to DH card IDs.
type DHCardIDLookup interface {
	GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
}

// DHAdapter wraps a DH market data client and implements
// SecondaryPriceSource. It resolves card names to DH card IDs via a
// DHCardIDLookup, fetches recent sales from the enterprise API, and
// converts them to fusion-compatible grade data.
type DHAdapter struct {
	client     DHMarketDataClient
	idResolver DHCardIDLookup
	logger     observability.Logger
}

// NewDHAdapter creates a new adapter.
func NewDHAdapter(
	client DHMarketDataClient,
	idResolver DHCardIDLookup,
	logger observability.Logger,
) *DHAdapter {
	return &DHAdapter{
		client:     client,
		idResolver: idResolver,
		logger:     logger,
	}
}

// FetchFusionData fetches market data from DH and converts recent sales
// to fusion format. If no card ID mapping exists the card is silently skipped.
func (a *DHAdapter) FetchFusionData(ctx context.Context, card pricing.Card) (*fusion.FetchResult, *fusion.ResponseMeta, error) {
	if a.client == nil {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, fmt.Errorf("dh: client not configured")
	}
	if a.idResolver == nil {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, fmt.Errorf("dh: idResolver not configured")
	}

	// Step 1: Look up DH card ID.
	dhCardID, err := a.idResolver.GetExternalID(ctx, card.Name, card.Set, card.Number, pricing.SourceDH)
	if err != nil {
		if a.logger != nil {
			a.logger.Warn(ctx, "dh: card ID lookup failed",
				observability.String("card", card.Name),
				observability.String("set", card.Set),
				observability.Err(err))
		}
		return nil, &fusion.ResponseMeta{StatusCode: 0}, fmt.Errorf("dh: card ID lookup: %w", err)
	}
	if dhCardID == "" {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, nil
	}

	// Step 2: Fetch recent sales from enterprise API.
	cardIDInt, convErr := strconv.Atoi(dhCardID)
	if convErr != nil {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, fmt.Errorf("dh: invalid card ID %q: %w", dhCardID, convErr)
	}

	sales, err := a.client.RecentSales(ctx, cardIDInt)
	if err != nil {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, fmt.Errorf("dh: recent sales failed for card_id=%s: %w", dhCardID, err)
	}

	if len(sales) == 0 {
		return nil, &fusion.ResponseMeta{StatusCode: 200}, nil
	}

	// Step 3: Convert recent sales to grade data.
	gradeData := convertDHSalesToGradeData(sales)

	return &fusion.FetchResult{
		GradeData: gradeData,
	}, &fusion.ResponseMeta{StatusCode: 200}, nil
}

// Available returns true if the underlying client and ID resolver are set.
func (a *DHAdapter) Available() bool {
	return a.client != nil && a.idResolver != nil
}

// Name returns the source identifier.
func (a *DHAdapter) Name() string {
	return pricing.SourceDH
}

// convertDHSalesToGradeData groups recent sales by grade and returns
// fusion-compatible price data keyed by grade string.
func convertDHSalesToGradeData(sales []dh.RecentSale) map[string][]fusion.PriceData {
	result := make(map[string][]fusion.PriceData)

	for _, sale := range sales {
		fusionKey := dhGradeToFusionKey(sale.GradingCompany, sale.Grade)
		if fusionKey == "" {
			continue
		}
		if sale.Price <= 0 {
			continue
		}

		result[fusionKey] = append(result[fusionKey], fusion.PriceData{
			Value:    sale.Price,
			Currency: "USD",
			Source: fusion.DataSource{
				Name:       pricing.SourceDH,
				Confidence: dhSourceConfidence,
			},
		})
	}

	return result
}

// dhGradeToFusionKey converts a DH grading company + grade pair to a fusion
// grade key string. Returns "" for unknown or unsupported grades.
func dhGradeToFusionKey(company, grade string) string {
	key := strings.ToUpper(strings.TrimSpace(company)) + " " + strings.TrimSpace(grade)

	switch key {
	case "PSA 10":
		return pricing.GradePSA10.String()
	case "PSA 9":
		return pricing.GradePSA9.String()
	case "PSA 9.5":
		return pricing.GradePSA95.String()
	case "PSA 8":
		return pricing.GradePSA8.String()
	case "PSA 7":
		return pricing.GradePSA7.String()
	case "PSA 6":
		return pricing.GradePSA6.String()
	case "BGS 10":
		return pricing.GradeBGS10.String()
	case "BGS 9.5":
		return pricing.GradePSA95.String()
	case "CGC 9.5":
		return pricing.GradePSA95.String()
	default:
		return ""
	}
}
