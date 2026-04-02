package fusionprice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/doubleholo"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// Compile-time check that DoubleHoloAdapter implements SecondaryPriceSource.
var _ fusion.SecondaryPriceSource = (*DoubleHoloAdapter)(nil)

// dhSourceConfidence is the confidence score assigned to DoubleHolo price data.
const dhSourceConfidence = 0.90

// DHMarketDataClient is the subset of the doubleholo.Client used by the adapter.
type DHMarketDataClient interface {
	MarketData(ctx context.Context, cardID string) (*doubleholo.MarketDataResponse, error)
}

// DHCardIDLookup resolves card names to DoubleHolo card IDs.
type DHCardIDLookup interface {
	GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
}

// DHIntelligenceStore persists market intelligence data from DoubleHolo.
type DHIntelligenceStore interface {
	Store(ctx context.Context, intel *intelligence.MarketIntelligence) error
}

// DoubleHoloAdapter wraps a DoubleHolo market data client and implements
// SecondaryPriceSource. It resolves card names to DH card IDs via a
// DHCardIDLookup, fetches market data, and converts recent sales to
// fusion-compatible grade data.
type DoubleHoloAdapter struct {
	client     DHMarketDataClient
	idResolver DHCardIDLookup
	intelStore DHIntelligenceStore
	logger     observability.Logger
}

// DoubleHoloAdapterOption is a functional option for DoubleHoloAdapter.
type DoubleHoloAdapterOption func(*DoubleHoloAdapter)

// WithDHIntelligenceStore sets the intelligence store for persisting DH market data.
func WithDHIntelligenceStore(s DHIntelligenceStore) DoubleHoloAdapterOption {
	return func(a *DoubleHoloAdapter) { a.intelStore = s }
}

// NewDoubleHoloAdapter creates a new adapter.
func NewDoubleHoloAdapter(
	client DHMarketDataClient,
	idResolver DHCardIDLookup,
	logger observability.Logger,
	opts ...DoubleHoloAdapterOption,
) *DoubleHoloAdapter {
	a := &DoubleHoloAdapter{
		client:     client,
		idResolver: idResolver,
		logger:     logger,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// FetchFusionData fetches market data from DoubleHolo and converts recent sales
// to fusion format. If no card ID mapping exists the card is silently skipped.
func (a *DoubleHoloAdapter) FetchFusionData(ctx context.Context, card pricing.Card) (*fusion.FetchResult, *fusion.ResponseMeta, error) {
	if a.client == nil {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, fmt.Errorf("doubleholo: client not configured")
	}

	// Step 1: Look up DH card ID.
	dhCardID, err := a.idResolver.GetExternalID(ctx, card.Name, card.Set, card.Number, pricing.SourceDoubleHolo)
	if err != nil {
		if a.logger != nil {
			a.logger.Debug(ctx, "doubleholo: card ID lookup failed",
				observability.String("card", card.Name),
				observability.String("set", card.Set),
				observability.Err(err))
		}
		return nil, &fusion.ResponseMeta{StatusCode: 0}, nil
	}
	if dhCardID == "" {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, nil
	}

	// Step 2: Fetch market data.
	resp, err := a.client.MarketData(ctx, dhCardID)
	if err != nil {
		return nil, &fusion.ResponseMeta{StatusCode: 0}, fmt.Errorf("doubleholo: market data failed for card_id=%s: %w", dhCardID, err)
	}

	// Step 3: Check for data.
	if !resp.HasData {
		return nil, &fusion.ResponseMeta{StatusCode: 200}, nil
	}

	// Step 4: Convert recent sales to grade data.
	gradeData := convertDHSalesToGradeData(resp.RecentSales)

	// Step 5: Optionally store intelligence data.
	if a.intelStore != nil {
		intel := ConvertDHToIntelligence(resp, card, dhCardID)
		if storeErr := a.intelStore.Store(ctx, intel); storeErr != nil {
			if a.logger != nil {
				a.logger.Warn(ctx, "doubleholo: failed to store intelligence",
					observability.String("card", card.Name),
					observability.String("dh_card_id", dhCardID),
					observability.Err(storeErr))
			}
		}
	}

	return &fusion.FetchResult{
		GradeData: gradeData,
	}, &fusion.ResponseMeta{StatusCode: 200}, nil
}

// Available returns true if the underlying client and ID resolver are set.
func (a *DoubleHoloAdapter) Available() bool {
	return a.client != nil && a.idResolver != nil
}

// Name returns the source identifier.
func (a *DoubleHoloAdapter) Name() string {
	return pricing.SourceDoubleHolo
}

// convertDHSalesToGradeData groups recent sales by grade and returns
// fusion-compatible price data keyed by grade string.
func convertDHSalesToGradeData(sales []doubleholo.RecentSale) map[string][]fusion.PriceData {
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
				Name:       pricing.SourceDoubleHolo,
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

// ConvertDHToIntelligence converts a MarketDataResponse to domain intelligence.
func ConvertDHToIntelligence(resp *doubleholo.MarketDataResponse, card pricing.Card, dhCardID string) *intelligence.MarketIntelligence {
	intel := &intelligence.MarketIntelligence{
		CardName:   card.Name,
		SetName:    card.Set,
		CardNumber: card.Number,
		DHCardID:   dhCardID,
		FetchedAt:  time.Now(),
	}

	// Sentiment
	if resp.Sentiment != nil {
		intel.Sentiment = &intelligence.Sentiment{
			Score:        resp.Sentiment.Score,
			MentionCount: resp.Sentiment.MentionCount,
			Trend:        resp.Sentiment.Trend,
		}
	}

	// Forecast
	if resp.PriceForecast != nil {
		fc := &intelligence.Forecast{
			PredictedPriceCents: mathutil.ToCents(resp.PriceForecast.PredictedPrice),
			Confidence:          resp.PriceForecast.Confidence,
		}
		if t, err := time.Parse("2006-01-02", resp.PriceForecast.ForecastDate); err == nil {
			fc.ForecastDate = t
		}
		intel.Forecast = fc
	}

	// Grading ROI
	for _, roi := range resp.GradingROI {
		intel.GradingROI = append(intel.GradingROI, intelligence.GradeROI{
			Grade:        roi.Grade,
			AvgSaleCents: mathutil.ToCents(roi.AvgSalePrice),
			ROI:          roi.ROI,
		})
	}

	// Recent sales
	for _, sale := range resp.RecentSales {
		s := intelligence.Sale{
			GradingCompany: sale.GradingCompany,
			Grade:          sale.Grade,
			PriceCents:     mathutil.ToCents(sale.Price),
			Platform:       sale.Platform,
		}
		if t, err := time.Parse(time.RFC3339, sale.SoldAt); err == nil {
			s.SoldAt = t
		}
		intel.RecentSales = append(intel.RecentSales, s)
	}

	// Population
	for _, pop := range resp.Population {
		intel.Population = append(intel.Population, intelligence.PopulationEntry{
			GradingCompany: pop.GradingCompany,
			Grade:          pop.Grade,
			Count:          pop.Count,
		})
	}

	// Insights
	if resp.Insights != nil {
		intel.Insights = &intelligence.Insights{
			Headline: resp.Insights.Headline,
			Detail:   resp.Insights.Detail,
		}
	}

	return intel
}
