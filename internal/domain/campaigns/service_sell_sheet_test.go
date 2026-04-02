package campaigns

import (
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertIntel_Nil(t *testing.T) {
	assert.Nil(t, convertIntel(nil))
}

func TestConvertIntel_Sparse(t *testing.T) {
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	mi := &intelligence.MarketIntelligence{
		CardName:  "Pikachu",
		SetName:   "Jungle",
		FetchedAt: now,
	}

	out := convertIntel(mi)
	require.NotNil(t, out)

	assert.Zero(t, out.SentimentScore)
	assert.Empty(t, out.SentimentTrend)
	assert.Zero(t, out.ForecastCents)
	assert.Empty(t, out.ForecastDate)
	assert.Empty(t, out.InsightHeadline)
	assert.Equal(t, 0, out.RecentSalesCount)
	assert.Nil(t, out.RecentSales)
	assert.Nil(t, out.Population)
	assert.Nil(t, out.GradingROI)
	assert.Contains(t, out.FetchedAt, "2026-04-02")
}

func TestConvertIntel_Full(t *testing.T) {
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	mi := &intelligence.MarketIntelligence{
		CardName: "Charizard", SetName: "Base Set", CardNumber: "4",
		Sentiment: &intelligence.Sentiment{Score: 0.85, MentionCount: 42, Trend: "rising"},
		Forecast:  &intelligence.Forecast{PredictedPriceCents: 50000, Confidence: 0.9, ForecastDate: now},
		Insights:  &intelligence.Insights{Headline: "Hot card", Detail: "Prices surging"},
		RecentSales: []intelligence.Sale{
			{SoldAt: now, GradingCompany: "PSA", Grade: "10", PriceCents: 45000, Platform: "eBay"},
			{SoldAt: now, GradingCompany: "PSA", Grade: "9", PriceCents: 20000, Platform: "TCGPlayer"},
		},
		Population: []intelligence.PopulationEntry{
			{GradingCompany: "PSA", Grade: "10", Count: 1234},
			{GradingCompany: "BGS", Grade: "9.5", Count: 56},
			{GradingCompany: "PSA", Grade: "9", Count: 5678},
		},
		GradingROI: []intelligence.GradeROI{
			{Grade: "10", AvgSaleCents: 45000, ROI: 0.42},
		},
		FetchedAt: now,
	}

	out := convertIntel(mi)
	require.NotNil(t, out)

	// Sentiment
	assert.Equal(t, 0.85, out.SentimentScore)
	assert.Equal(t, "rising", out.SentimentTrend)
	assert.Equal(t, 42, out.SentimentMentions)

	// Forecast
	assert.Equal(t, int64(50000), out.ForecastCents)
	assert.Equal(t, 0.9, out.ForecastConfidence)
	assert.NotEmpty(t, out.ForecastDate)

	// Insights
	assert.Equal(t, "Hot card", out.InsightHeadline)
	assert.Equal(t, "Prices surging", out.InsightDetail)

	// Recent sales (all 2)
	assert.Equal(t, 2, out.RecentSalesCount)
	assert.Len(t, out.RecentSales, 2)
	assert.Equal(t, int64(45000), out.RecentSales[0].PriceCents)

	// Population — only PSA entries (BGS filtered out)
	assert.Len(t, out.Population, 2)
	assert.Equal(t, "10", out.Population[0].Grade)
	assert.Equal(t, 1234, out.Population[0].Count)

	// Grading ROI
	assert.Len(t, out.GradingROI, 1)
	assert.Equal(t, 0.42, out.GradingROI[0].ROI)

	// FetchedAt
	assert.Contains(t, out.FetchedAt, "2026-04-02")
}
