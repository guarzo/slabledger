package handlers

import (
	"net/http"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CardPricingResponse contains per-card pricing data from the fusion price provider.
// All prices are in USD (dollars), not cents.
type CardPricingResponse struct {
	Card   string `json:"card"`
	Set    string `json:"set"`
	Number string `json:"number"`

	// Grade prices (USD)
	RawUSD float64 `json:"rawUSD"`
	PSA8   float64 `json:"psa8"`
	PSA9   float64 `json:"psa9"`
	PSA10  float64 `json:"psa10"`

	// Confidence
	Confidence float64 `json:"confidence"`

	// MatchQuality: "good" (conf>=0.8), "partial" (conf>0 and <0.8). "none" is not returned — lookup failures result in a 404.
	MatchQuality string `json:"matchQuality"`

	// Conservative exit prices (USD)
	ConservativePSA10 float64 `json:"conservativePsa10,omitempty"`
	ConservativePSA9  float64 `json:"conservativePsa9,omitempty"`
	ConservativeRaw   float64 `json:"conservativeRaw,omitempty"`

	// Last sold data
	LastSold *LastSoldResponse `json:"lastSold,omitempty"`

	// Per-grade detail data (eBay sold + estimates)
	GradeData map[string]*GradeDetailResponse `json:"gradeData,omitempty"`

	// Aggregated market overview
	Market *MarketResponse `json:"market,omitempty"`

	// Sales velocity
	Velocity *VelocityResponse `json:"velocity,omitempty"`

	// Contributing sources
	Sources []string `json:"sources,omitempty"`
}

// EbayGradeResponse contains eBay sold data for a single grade.
type EbayGradeResponse struct {
	Price      float64 `json:"price"`
	Confidence string  `json:"confidence"` // "high", "medium", "low"
	SalesCount int     `json:"salesCount"`
	Trend      string  `json:"trend,omitempty"` // "up", "down", "stable"
	Median     float64 `json:"median,omitempty"`
	Min        float64 `json:"min,omitempty"`
	Max        float64 `json:"max,omitempty"`
	Avg7Day    float64 `json:"avg7day,omitempty"`
	Volume7Day float64 `json:"volume7day,omitempty"`
}

// EstimateGradeResponse contains estimate data for a single grade.
type EstimateGradeResponse struct {
	Price      float64 `json:"price"`
	Low        float64 `json:"low,omitempty"`
	High       float64 `json:"high,omitempty"`
	Confidence float64 `json:"confidence"` // 0-1
}

// GradeDetailResponse contains combined eBay + estimate data for a grade.
type GradeDetailResponse struct {
	Ebay     *EbayGradeResponse     `json:"ebay"`
	Estimate *EstimateGradeResponse `json:"estimate"`
}

// VelocityResponse contains sales velocity data.
type VelocityResponse struct {
	DailyAverage  float64 `json:"dailyAverage"`
	WeeklyAverage float64 `json:"weeklyAverage"`
	MonthlyTotal  int     `json:"monthlyTotal"`
}

// MarketResponse contains marketplace overview data.
type MarketResponse struct {
	ActiveListings int     `json:"activeListings"`
	LowestListing  float64 `json:"lowestListing"`
	Sales30d       int     `json:"sales30d"`
	Sales90d       int     `json:"sales90d"`
}

// LastSoldResponse contains last sold info by grade.
type LastSoldResponse struct {
	PSA10 *GradeSaleResponse `json:"psa10,omitempty"`
	PSA9  *GradeSaleResponse `json:"psa9,omitempty"`
	PSA8  *GradeSaleResponse `json:"psa8,omitempty"`
	Raw   *GradeSaleResponse `json:"raw,omitempty"`
}

// GradeSaleResponse contains last sold info for a single grade.
type GradeSaleResponse struct {
	LastSoldPrice float64 `json:"lastSoldPrice"`
	LastSoldDate  string  `json:"lastSoldDate"`
	SaleCount     int     `json:"saleCount"`
}

// HandleCardPricing serves GET /api/cards/pricing?name=X&set=Y&number=Z
func (h *Handler) HandleCardPricing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := r.Context()

	if h.priceProv == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pricing not available"})
		return
	}

	name := r.URL.Query().Get("name")
	setName := r.URL.Query().Get("set")
	number := r.URL.Query().Get("number")

	if len(name) > 200 || len(setName) > 200 || len(number) > 50 {
		writeError(w, http.StatusBadRequest, "query parameter too long")
		return
	}

	if name == "" {
		writeError(w, http.StatusBadRequest, "name parameter required")
		return
	}

	card := domainCards.Card{
		Name:    name,
		Number:  number,
		Set:     setName,
		SetName: setName,
	}

	price, err := h.priceProv.LookupCard(ctx, setName, card)
	if err != nil {
		h.logger.Error(ctx, "card pricing lookup failed",
			observability.Err(err),
			observability.String("name", name),
			observability.String("set", setName))
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no pricing data found"})
		return
	}
	if price == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no pricing data found"})
		return
	}

	rawUSD := mathutil.ToDollars(price.Grades.RawCents)
	psa10USD := mathutil.ToDollars(price.Grades.PSA10Cents)

	// Compute match quality from confidence
	matchQuality := "partial"
	if price.Confidence >= 0.8 {
		matchQuality = "good"
	}

	resp := CardPricingResponse{
		Card:         name,
		Set:          setName,
		Number:       number,
		RawUSD:       rawUSD,
		PSA8:         mathutil.ToDollars(price.Grades.PSA8Cents),
		PSA9:         mathutil.ToDollars(price.Grades.PSA9Cents),
		PSA10:        psa10USD,
		Confidence:   price.Confidence,
		MatchQuality: matchQuality,
	}

	if price.Market != nil {
		resp.Market = &MarketResponse{
			ActiveListings: price.Market.ActiveListings,
			LowestListing:  mathutil.ToDollars(price.Market.LowestListing),
			Sales30d:       price.Market.SalesLast30d,
			Sales90d:       price.Market.SalesLast90d,
		}
	}

	if price.Conservative != nil {
		resp.ConservativePSA10 = price.Conservative.PSA10USD
		resp.ConservativePSA9 = price.Conservative.PSA9USD
		resp.ConservativeRaw = price.Conservative.RawUSD
	}

	if price.LastSoldByGrade != nil {
		ls := &LastSoldResponse{}
		if g := price.LastSoldByGrade.PSA10; g != nil {
			ls.PSA10 = &GradeSaleResponse{LastSoldPrice: g.LastSoldPrice, LastSoldDate: g.LastSoldDate, SaleCount: g.SaleCount}
		}
		if g := price.LastSoldByGrade.PSA9; g != nil {
			ls.PSA9 = &GradeSaleResponse{LastSoldPrice: g.LastSoldPrice, LastSoldDate: g.LastSoldDate, SaleCount: g.SaleCount}
		}
		if g := price.LastSoldByGrade.PSA8; g != nil {
			ls.PSA8 = &GradeSaleResponse{LastSoldPrice: g.LastSoldPrice, LastSoldDate: g.LastSoldDate, SaleCount: g.SaleCount}
		}
		if g := price.LastSoldByGrade.Raw; g != nil {
			ls.Raw = &GradeSaleResponse{LastSoldPrice: g.LastSoldPrice, LastSoldDate: g.LastSoldDate, SaleCount: g.SaleCount}
		}
		resp.LastSold = ls
	}

	// Populate per-grade detail data
	if price.GradeDetails != nil {
		resp.GradeData = make(map[string]*GradeDetailResponse)
		for grade, detail := range price.GradeDetails {
			gdr := &GradeDetailResponse{}
			if detail.Ebay != nil {
				gdr.Ebay = &EbayGradeResponse{
					Price:      mathutil.ToDollars(detail.Ebay.PriceCents),
					Confidence: detail.Ebay.Confidence,
					SalesCount: detail.Ebay.SalesCount,
					Trend:      detail.Ebay.Trend,
					Median:     mathutil.ToDollars(detail.Ebay.MedianCents),
					Min:        mathutil.ToDollars(detail.Ebay.MinCents),
					Max:        mathutil.ToDollars(detail.Ebay.MaxCents),
					Avg7Day:    mathutil.ToDollars(detail.Ebay.Avg7DayCents),
					Volume7Day: detail.Ebay.Volume7Day,
				}
			}
			if detail.Estimate != nil {
				gdr.Estimate = &EstimateGradeResponse{
					Price:      mathutil.ToDollars(detail.Estimate.PriceCents),
					Low:        mathutil.ToDollars(detail.Estimate.LowCents),
					High:       mathutil.ToDollars(detail.Estimate.HighCents),
					Confidence: detail.Estimate.Confidence,
				}
			}
			resp.GradeData[grade] = gdr
		}
	}

	// Populate velocity
	if price.Velocity != nil {
		resp.Velocity = &VelocityResponse{
			DailyAverage:  price.Velocity.DailyAverage,
			WeeklyAverage: price.Velocity.WeeklyAverage,
			MonthlyTotal:  price.Velocity.MonthlyTotal,
		}
	}

	// Populate contributing sources
	resp.Sources = price.Sources

	writeJSON(w, http.StatusOK, resp)
}
