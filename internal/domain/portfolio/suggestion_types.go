package portfolio

import "github.com/guarzo/slabledger/internal/domain/inventory"

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
	Name                    string   `json:"name"`
	YearRange               string   `json:"yearRange,omitempty"`
	GradeRange              string   `json:"gradeRange,omitempty"`
	PriceRange              string   `json:"priceRange,omitempty"`
	BuyTermsCLPct           float64  `json:"buyTermsCLPct,omitempty"`
	BuyTermsCLPctOptimistic float64  `json:"buyTermsCLPctOptimistic,omitempty"`
	DailySpendCapCents      int      `json:"dailySpendCapCents,omitempty"`
	Subjects                []string `json:"subjects,omitempty"`
	// SubjectsAction is the polarity of Subjects on an "adjust" suggestion
	// for an *existing* campaign: SubjectsActionAdd means these subjects
	// should be added to the campaign's targeting list, SubjectsActionRemove
	// means they should be removed from it. Empty for "new"/"gap" suggestions,
	// where Subjects is the full initial targeting list for a brand-new
	// campaign and there is no existing list to add to or remove from.
	SubjectsAction string `json:"subjectsAction,omitempty"`
	PrimaryExit    string `json:"primaryExit,omitempty"`
}

// Values for CampaignSuggestionParams.SubjectsAction.
const (
	SubjectsActionAdd    = "add"
	SubjectsActionRemove = "remove"
)

// ExpectedMetrics are projected performance based on historical data.
type ExpectedMetrics struct {
	ExpectedROI       float64 `json:"expectedROI"`
	ExpectedMarginPct float64 `json:"expectedMarginPct"`
	AvgDaysToSell     float64 `json:"avgDaysToSell"`
	DataConfidence    string  `json:"dataConfidence"`
}

// SuggestionsResponse is the API response for campaign suggestions.
type SuggestionsResponse struct {
	NewCampaigns []CampaignSuggestion          `json:"newCampaigns"`
	Adjustments  []CampaignSuggestion          `json:"adjustments"`
	DataSummary  inventory.InsightsDataSummary `json:"dataSummary"`
}
