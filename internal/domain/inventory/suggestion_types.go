package inventory

// CampaignSuggestion is a data-driven recommendation for a new or modified campaign.
type CampaignSuggestion struct {
	Type            string                   `json:"type"`
	Title           string                   `json:"title"`
	Rationale       string                   `json:"rationale"`
	Confidence      string                   `json:"confidence"`
	DataPoints      int                      `json:"dataPoints"`
	SuggestedParams CampaignSuggestionParams `json:"suggestedParams"`
	ExpectedMetrics ExpectedMetrics          `json:"expectedMetrics"`
}

// CampaignSuggestionParams holds the suggested campaign configuration.
type CampaignSuggestionParams struct {
	Name                    string  `json:"name"`
	YearRange               string  `json:"yearRange,omitempty"`
	GradeRange              string  `json:"gradeRange,omitempty"`
	PriceRange              string  `json:"priceRange,omitempty"`
	BuyTermsCLPct           float64 `json:"buyTermsCLPct,omitempty"`
	BuyTermsCLPctOptimistic float64 `json:"buyTermsCLPctOptimistic,omitempty"`
	DailySpendCapCents      int     `json:"dailySpendCapCents,omitempty"`
	InclusionList           string  `json:"inclusionList,omitempty"`
	PrimaryExit             string  `json:"primaryExit,omitempty"`
}

// ExpectedMetrics are projected performance based on historical data.
type ExpectedMetrics struct {
	ExpectedROI       float64 `json:"expectedROI"`
	ExpectedMarginPct float64 `json:"expectedMarginPct"`
	AvgDaysToSell     float64 `json:"avgDaysToSell"`
	DataConfidence    string  `json:"dataConfidence"`
}

// SuggestionsResponse is the API response for campaign suggestions.
type SuggestionsResponse struct {
	NewCampaigns []CampaignSuggestion `json:"newCampaigns"`
	Adjustments  []CampaignSuggestion `json:"adjustments"`
	DataSummary  InsightsDataSummary  `json:"dataSummary"`
}
