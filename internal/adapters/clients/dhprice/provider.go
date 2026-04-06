// Package dhprice implements pricing.PriceProvider backed by DH recent-sales data.
package dhprice

import (
	"context"
	"math"
	"sort"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

const (
	dhConfidence = 0.90
	providerKey  = "doubleholo"
)

// MarketDataClient is the subset of the DH client needed for price lookups.
type MarketDataClient interface {
	RecentSales(ctx context.Context, cardID int) ([]dh.RecentSale, error)
}

// CardIDLookup resolves a card to its external DH card ID.
type CardIDLookup interface {
	GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
}

// gradeKey maps a combined "Company Grade" string to the canonical pricing.Grade.
var gradeKey = map[string]pricing.Grade{
	"PSA 10":  pricing.GradePSA10,
	"PSA 9":   pricing.GradePSA9,
	"PSA 9.5": pricing.GradePSA95,
	"PSA 8":   pricing.GradePSA8,
	"PSA 7":   pricing.GradePSA7,
	"PSA 6":   pricing.GradePSA6,
	"BGS 10":  pricing.GradeBGS10,
	"BGS 9.5": pricing.GradePSA95,
	"CGC 9.5": pricing.GradePSA95,
}

// Provider implements pricing.PriceProvider using DH recent sales.
type Provider struct {
	client     MarketDataClient
	idResolver CardIDLookup
	logger     observability.Logger
}

// Option configures a Provider.
type Option func(*Provider)

// WithLogger sets the structured logger.
func WithLogger(l observability.Logger) Option {
	if l == nil {
		return func(*Provider) {}
	}
	return func(p *Provider) { p.logger = l }
}

// New creates a DHPriceProvider. Both client and idResolver must be non-nil for
// the provider to be available.
func New(client MarketDataClient, idResolver CardIDLookup, opts ...Option) *Provider {
	p := &Provider{client: client, idResolver: idResolver}
	for _, o := range opts {
		if o != nil {
			o(p)
		}
	}
	return p
}

// GetPrice fetches recent sales for the card and computes per-grade median prices.
// Returns (nil, nil) when the card has no DH mapping or no sales data.
func (p *Provider) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
	if p.client == nil || p.idResolver == nil {
		return nil, nil
	}

	// Resolve DH card ID.
	extID, err := p.idResolver.GetExternalID(ctx, card.Name, card.Set, card.Number, providerKey)
	if err != nil {
		return nil, err
	}
	if extID == "" {
		return nil, nil // card not mapped
	}

	cardID, err := strconv.Atoi(extID)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn(ctx, "dhprice: invalid card ID",
				observability.String("external_id", extID),
				observability.Err(err))
		}
		return nil, nil
	}

	sales, err := p.client.RecentSales(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if len(sales) == 0 {
		return nil, nil
	}

	return buildPrice(card.Name, sales), nil
}

// LookupCard delegates to GetPrice after constructing a pricing.Card.
func (p *Provider) LookupCard(ctx context.Context, setName string, card domainCards.Card) (*pricing.Price, error) {
	pc := pricing.Card{
		Name:            card.Name,
		Number:          card.Number,
		Set:             setName,
		PSAListingTitle: card.PSAListingTitle,
	}
	return p.GetPrice(ctx, pc)
}

// Available returns true when both dependencies are present.
func (p *Provider) Available() bool {
	return p.client != nil && p.idResolver != nil
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return pricing.SourceDH }

// Close is a no-op; the underlying DH client is managed externally.
func (p *Provider) Close() error { return nil }

// GetStats returns nil (no stats tracking in this provider).
func (p *Provider) GetStats(_ context.Context) *pricing.ProviderStats { return nil }

// buildPrice groups sales by grade, computes medians, and assembles a Price.
func buildPrice(productName string, sales []dh.RecentSale) *pricing.Price {
	// Group sale prices by canonical grade.
	byGrade := make(map[pricing.Grade][]float64)
	for _, s := range sales {
		key := s.GradingCompany + " " + s.Grade
		g, ok := gradeKey[key]
		if !ok {
			continue
		}
		byGrade[g] = append(byGrade[g], s.Price)
	}

	if len(byGrade) == 0 {
		return nil
	}

	var grades pricing.GradedPrices
	details := make(map[string]*pricing.GradeDetail, len(byGrade))

	for g, prices := range byGrade {
		med := median(prices)
		cents := dollarsToCents(med)
		pricing.SetGradePrice(&grades, g, cents)

		lo, hi := priceRange(prices)
		details[g.String()] = &pricing.GradeDetail{
			Estimate: &pricing.EstimateGradeDetail{
				PriceCents: cents,
				LowCents:   dollarsToCents(lo),
				HighCents:  dollarsToCents(hi),
				Confidence: dhConfidence,
			},
		}
	}

	return &pricing.Price{
		ProductName:  productName,
		Amount:       grades.PSA10Cents,
		Currency:     "USD",
		Source:       pricing.Source(pricing.SourceDH),
		Grades:       grades,
		Confidence:   dhConfidence,
		GradeDetails: details,
		Sources:      []string{pricing.SourceDH},
	}
}

// median returns the median of a float64 slice. The slice is sorted in place.
func median(vals []float64) float64 {
	sort.Float64s(vals)
	n := len(vals)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return vals[n/2]
	}
	return (vals[n/2-1] + vals[n/2]) / 2
}

// priceRange returns the min and max of a sorted slice.
func priceRange(sorted []float64) (float64, float64) {
	if len(sorted) == 0 {
		return 0, 0
	}
	return sorted[0], sorted[len(sorted)-1]
}

// dollarsToCents converts a USD dollar amount to cents, rounding to nearest.
func dollarsToCents(d float64) int64 {
	return int64(math.Round(d * 100))
}
