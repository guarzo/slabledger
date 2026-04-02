package scoring

// Signal is a human-readable interpretation of a factor.
type Signal struct {
	Factor    string `json:"factor"`
	Direction string `json:"direction"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Metric    string `json:"metric"`
}

// StructuredResult is the base output schema shared by all flows.
type StructuredResult struct {
	ScoreCard        ScoreCard `json:"score_card"`
	Verdict          Verdict   `json:"verdict"`
	AdjustmentReason *string   `json:"adjustment_reason"`
	KeyInsight       string    `json:"key_insight"`
	Signals          []Signal  `json:"signals"`
}

type PurchaseAssessmentResult struct {
	StructuredResult
	ExpectedROI     float64         `json:"expected_roi"`
	PortfolioImpact PortfolioImpact `json:"portfolio_impact"`
	GradeFitDetail  GradeFitDetail  `json:"grade_fit"`
}

type PortfolioImpact struct {
	CharacterConcentration string  `json:"character_concentration"`
	GradeConcentration     string  `json:"grade_concentration"`
	CampaignGradeROI       float64 `json:"campaign_grade_roi"`
}

type GradeFitDetail struct {
	Grade                       string  `json:"grade"`
	CampaignAvgROIForGrade      float64 `json:"campaign_avg_roi_for_grade"`
	CampaignSellThroughForGrade float64 `json:"campaign_sell_through_for_grade"`
}

type CampaignAnalysisResult struct {
	StructuredResult
	HealthStatus    string           `json:"health_status"`
	Recommendations []Recommendation `json:"recommendations"`
	ProblemAreas    []ProblemArea    `json:"problem_areas"`
}

type Recommendation struct {
	Type           string `json:"type"`
	Action         string `json:"action"`
	ExpectedImpact string `json:"expected_impact"`
	Priority       string `json:"priority"`
}

type ProblemArea struct {
	Area       string `json:"area"`
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion"`
}

type LiquidationResult struct {
	StructuredResult
	CapitalFreedCents  int       `json:"capital_freed_cents"`
	Urgency            string    `json:"urgency"`
	CrackOpportunity   *CrackOpp `json:"crack_opportunity,omitempty"`
	RecommendedAction  string    `json:"recommended_action"`
	RecommendedChannel string    `json:"recommended_channel"`
	RecommendedPrice   int       `json:"recommended_price_cents"`
}

type CrackOpp struct {
	IsCandidate    bool    `json:"is_candidate"`
	CrackROI       float64 `json:"crack_roi"`
	GradedROI      float64 `json:"graded_roi"`
	AdvantageCents int     `json:"advantage_cents"`
}

type CampaignSuggestionResult struct {
	StructuredResult
	SuggestionType string         `json:"suggestion_type"`
	ProjectedROI   float64        `json:"projected_roi"`
	MarginEstimate float64        `json:"margin_estimate"`
	CoverageDetail CoverageDetail `json:"coverage_impact"`
}

type CoverageDetail struct {
	FillsGap                bool     `json:"fills_gap"`
	Character               string   `json:"character"`
	SegmentsCovered         []string `json:"segments_covered"`
	ExistingCampaignOverlap int      `json:"existing_campaign_overlap"`
}

type InsufficientDataResult struct {
	Status   string    `json:"status"`
	DataGaps []DataGap `json:"data_gaps"`
	Message  string    `json:"message"`
}
