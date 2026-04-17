package dh

import (
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

// ConvertToIntelligence converts a MarketDataResponse to domain intelligence.
func ConvertToIntelligence(resp *MarketDataResponse, cardName, setName, cardNumber, dhCardID string) *intelligence.MarketIntelligence {
	if resp == nil {
		return &intelligence.MarketIntelligence{
			CardName:   cardName,
			SetName:    setName,
			CardNumber: cardNumber,
			DHCardID:   dhCardID,
			FetchedAt:  time.Now(),
		}
	}
	intel := &intelligence.MarketIntelligence{
		CardName:   cardName,
		SetName:    setName,
		CardNumber: cardNumber,
		DHCardID:   dhCardID,
		FetchedAt:  time.Now(),
	}

	if resp.Sentiment != nil {
		intel.Sentiment = &intelligence.Sentiment{
			Score:        resp.Sentiment.Score,
			MentionCount: resp.Sentiment.MentionCount,
			Trend:        resp.Sentiment.Trend,
		}
	}

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

	if resp.GradingROI != nil {
		for _, roi := range resp.GradingROI.ROIData {
			intel.GradingROI = append(intel.GradingROI, intelligence.GradeROI{
				Grade:        roi.Grade,
				AvgSaleCents: mathutil.ToCents(roi.AvgSalePrice),
				ROI:          roi.ROI,
			})
		}
	}

	for _, sale := range resp.RecentSales {
		t, err := time.Parse(time.RFC3339, sale.SoldAt)
		if err != nil {
			continue // skip sales with unparseable timestamps
		}
		intel.RecentSales = append(intel.RecentSales, intelligence.Sale{
			SoldAt:         t,
			GradingCompany: sale.GradingCompany,
			Grade:          sale.Grade,
			PriceCents:     mathutil.ToCents(sale.Price),
			Platform:       sale.Platform,
		})
	}

	for _, pop := range resp.Population {
		intel.Population = append(intel.Population, intelligence.PopulationEntry{
			GradingCompany: pop.GradingCompany,
			Grade:          pop.Grade,
			Count:          pop.Count,
		})
	}

	if resp.Insights != nil {
		intel.Insights = &intelligence.Insights{
			Headline: resp.Insights.Headline,
			Detail:   resp.Insights.Detail,
		}
	}

	return intel
}

// MergeAnalyticsIntoIntelligence copies velocity + trend fields from DH's
// batch_analytics response onto the intelligence record in place. Fields
// return as nil if DH hasn't computed that surface yet, which is normal —
// callers should not treat a partial fill as an error.
func MergeAnalyticsIntoIntelligence(intel *intelligence.MarketIntelligence, ca *CardAnalytics) {
	if intel == nil || ca == nil {
		return
	}
	if ca.Trend != nil {
		intel.Trend = &intelligence.Trend{
			Volume7d:  ca.Trend.Volume7d,
			Volume30d: ca.Trend.Volume30d,
			Volume90d: ca.Trend.Volume90d,
		}
	}
	if ca.Velocity != nil {
		v := &intelligence.Velocity{
			SampleSize: ca.Velocity.SampleSize,
			LastFetch:  time.Now(),
		}
		v.SellThrough30dPct = parseSellThroughPct(ca.Velocity.SellThrough, "30d")
		v.SellThrough60dPct = parseSellThroughPct(ca.Velocity.SellThrough, "60d")
		v.SellThrough90dPct = parseSellThroughPct(ca.Velocity.SellThrough, "90d")
		intel.Velocity = v
	}
}

// parseSellThroughPct reads a window value from DH's string-keyed sell_through
// map. DH returns percentages as stringified floats ("45.2"); the zero-value
// return on missing/malformed keys is safe because a 0.0 sell-through is
// indistinguishable from "no data" for operator-facing lenses (and SampleSize
// is the authoritative confidence gate).
func parseSellThroughPct(m map[string]string, window string) float64 {
	if m == nil {
		return 0
	}
	s, ok := m[window]
	if !ok || s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
