// Package dhprice implements pricing.PriceProvider backed by DH recent-sales data.
package dhprice

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
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
	CardLookup(ctx context.Context, cardID int) (*dh.CardLookupResponse, error)
}

// CardIDLookup resolves a card to its external DH card ID.
type CardIDLookup interface {
	GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
}

// gradeKey maps a combined "Company Grade" string to the canonical pricing.Grade.
// Only high-value grades (PSA 6-10, BGS 10/9.5, CGC 9.5) are tracked because
// the business focuses on PSA-graded cards in this range. Sales for unmapped
// grades (e.g. PSA 1-5, raw) are intentionally skipped.
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

	price := buildPrice(card.Name, sales)
	if price == nil {
		return nil, nil
	}

	// Enrich with listing data from CardLookup (BestAsk, ActiveAsks, LastSale).
	// Non-fatal: if the call fails we still return sales-based pricing.
	lookup, err := p.client.CardLookup(ctx, cardID)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn(ctx, "dhprice: CardLookup failed (non-fatal)",
				observability.String("card", card.Name),
				observability.Err(err))
		}
	} else if lookup != nil && hasMarketData(&lookup.MarketData) {
		applyMarketData(price, &lookup.MarketData)
	}

	return price, nil
}

// LookupCard delegates to GetPrice after constructing a pricing.Card.
func (p *Provider) LookupCard(ctx context.Context, setName string, card pricing.CardLookup) (*pricing.Price, error) {
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

// ebayConfidence returns a confidence label based on sale count.
func ebayConfidence(saleCount int) string {
	switch {
	case saleCount >= 10:
		return "high"
	case saleCount >= 3:
		return "medium"
	default:
		return "low"
	}
}

// buildPrice groups sales by grade, computes per-platform and aggregate medians, and assembles a Price.
func buildPrice(productName string, sales []dh.RecentSale) *pricing.Price {
	type gradeplatform struct {
		grade    pricing.Grade
		platform string
	}

	// lastSale tracks the most recent sale for each grade.
	type lastSale struct {
		soldAt string
		price  float64
		count  int
	}

	// Group sale prices by (grade, platform) and by grade (aggregate).
	byGradePlatform := make(map[gradeplatform][]float64)
	byGrade := make(map[pricing.Grade][]float64)
	platforms := make(map[string]bool)
	lastByGrade := make(map[pricing.Grade]*lastSale)

	for _, s := range sales {
		key := s.GradingCompany + " " + s.Grade
		g, ok := gradeKey[key]
		if !ok {
			continue
		}
		platform := strings.ToLower(s.Platform)
		byGrade[g] = append(byGrade[g], s.Price)
		if platform != "" {
			byGradePlatform[gradeplatform{g, platform}] = append(byGradePlatform[gradeplatform{g, platform}], s.Price)
			platforms[platform] = true
		}

		// Track the most recent sale per grade (lexicographic comparison works for ISO dates).
		if ls, ok := lastByGrade[g]; !ok {
			lastByGrade[g] = &lastSale{soldAt: s.SoldAt, price: s.Price, count: 1}
		} else {
			ls.count++
			if s.SoldAt > ls.soldAt {
				ls.soldAt = s.SoldAt
				ls.price = s.Price
			}
		}
	}

	if len(byGrade) == 0 {
		return nil
	}

	var grades pricing.GradedPrices
	details := make(map[string]*pricing.GradeDetail, len(byGrade))

	for g, prices := range byGrade {
		med := median(prices)
		cents := mathutil.ToCents(med)
		pricing.SetGradePrice(&grades, g, cents)

		lo, hi := priceRange(prices)
		detail := &pricing.GradeDetail{
			Estimate: &pricing.EstimateGradeDetail{
				PriceCents: cents,
				LowCents:   mathutil.ToCents(lo),
				HighCents:  mathutil.ToCents(hi),
				Confidence: dhConfidence,
			},
		}

		// Populate eBay-specific detail from eBay sales for this grade.
		if ebayPrices, ok := byGradePlatform[gradeplatform{g, "ebay"}]; ok && len(ebayPrices) > 0 {
			eMed := median(ebayPrices)
			eLo, eHi := priceRange(ebayPrices)
			detail.Ebay = &pricing.EbayGradeDetail{
				PriceCents:  mathutil.ToCents(eMed),
				MedianCents: mathutil.ToCents(eMed),
				MinCents:    mathutil.ToCents(eLo),
				MaxCents:    mathutil.ToCents(eHi),
				SalesCount:  len(ebayPrices),
				Confidence:  ebayConfidence(len(ebayPrices)),
			}
		}

		details[g.String()] = detail
	}

	// Pick the best available grade price for Amount.
	amount := grades.PSA10Cents
	if amount == 0 {
		for _, fallback := range []int64{
			grades.BGS10Cents,
			grades.Grade95Cents,
			grades.PSA9Cents,
			grades.PSA8Cents,
			grades.PSA7Cents,
			grades.PSA6Cents,
		} {
			if fallback != 0 {
				amount = fallback
				break
			}
		}
	}

	// Sources = distinct platforms seen in sales data (sorted for deterministic output).
	sources := make([]string, 0, len(platforms))
	for p := range platforms {
		sources = append(sources, p)
	}
	sort.Strings(sources)

	// Build last-sold data from the most recent sale per grade.
	var lsbg *pricing.LastSoldByGrade
	if len(lastByGrade) > 0 {
		lsbg = &pricing.LastSoldByGrade{}
		for g, ls := range lastByGrade {
			info := &pricing.GradeSaleInfo{
				LastSoldPrice: mathutil.ToCents(ls.price),
				LastSoldDate:  ls.soldAt,
				SaleCount:     ls.count,
			}
			switch g {
			case pricing.GradePSA10:
				lsbg.PSA10 = info
			case pricing.GradePSA9:
				lsbg.PSA9 = info
			case pricing.GradePSA8:
				lsbg.PSA8 = info
			case pricing.GradePSA7:
				lsbg.PSA7 = info
			case pricing.GradePSA6:
				lsbg.PSA6 = info
			case pricing.GradePSA95, pricing.GradeBGS10:
				// GradePSA95 (CGC/BGS 9.5) and BGS10 are tracked in GradedPrices
				// but LastSoldByGrade only has PSA tiers — skip.
			case pricing.GradeRaw:
				lsbg.Raw = info
			}
		}
	}

	return &pricing.Price{
		ProductName:     productName,
		Amount:          amount,
		Currency:        "USD",
		Source:          pricing.SourceDH,
		Grades:          grades,
		Confidence:      dhConfidence,
		GradeDetails:    details,
		Sources:         sources,
		LastSoldByGrade: lsbg,
	}
}

// hasMarketData reports whether md contains at least one meaningful value.
// Returns false when the API response has all zero/nil fields, preventing
// applyMarketData from setting price.Market to an empty struct.
func hasMarketData(md *dh.CardLookupMarketData) bool {
	if md == nil {
		return false
	}
	return (md.BestAsk != nil && *md.BestAsk > 0) ||
		md.ActiveAsks > 0 ||
		md.Volume24h > 0 ||
		(md.MidPrice != nil && *md.MidPrice > 0) ||
		(md.LastSale != nil && *md.LastSale > 0)
}

// applyMarketData enriches a Price with listing/market data from the DH CardLookup API.
func applyMarketData(price *pricing.Price, md *dh.CardLookupMarketData) {
	if md == nil {
		return
	}
	market := &pricing.MarketData{
		ActiveListings: md.ActiveAsks,
	}
	if md.BestAsk != nil && *md.BestAsk > 0 {
		market.LowestListing = mathutil.ToCents(*md.BestAsk)
	}
	if md.MidPrice != nil && *md.MidPrice > 0 {
		market.MidPrice = mathutil.ToCents(*md.MidPrice)
	}
	if md.LastSale != nil && *md.LastSale > 0 {
		market.LastSoldCents = mathutil.ToCents(*md.LastSale)
	}
	if md.LastSaleDate != nil {
		market.LastSoldDate = *md.LastSaleDate
	}
	if md.Volume24h > 0 {
		// Extrapolate 24h volume to 30d/90d estimates.
		market.SalesLast30d = md.Volume24h * 30
		market.SalesLast90d = md.Volume24h * 90
	}
	price.Market = market
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

// priceRange returns the min and max of a float64 slice.
func priceRange(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	mn, mx := vals[0], vals[0]
	for _, v := range vals[1:] {
		mn = min(mn, v)
		mx = max(mx, v)
	}
	return mn, mx
}
