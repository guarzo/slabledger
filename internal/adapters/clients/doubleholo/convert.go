package doubleholo

import (
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

// ConvertToIntelligence converts a MarketDataResponse to domain intelligence.
func ConvertToIntelligence(resp *MarketDataResponse, cardName, setName, cardNumber, dhCardID string) *intelligence.MarketIntelligence {
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

	for _, roi := range resp.GradingROI {
		intel.GradingROI = append(intel.GradingROI, intelligence.GradeROI{
			Grade:        roi.Grade,
			AvgSaleCents: mathutil.ToCents(roi.AvgSalePrice),
			ROI:          roi.ROI,
		})
	}

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
