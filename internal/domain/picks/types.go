package picks

import "time"

type Direction string

const (
	DirectionBuy   Direction = "buy"
	DirectionWatch Direction = "watch"
	DirectionAvoid Direction = "avoid"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type SignalDirection string

const (
	SignalBullish SignalDirection = "bullish"
	SignalBearish SignalDirection = "bearish"
	SignalNeutral SignalDirection = "neutral"
)

type PickSource string

const (
	SourceAI                    PickSource = "ai"
	SourceWatchlistReassessment PickSource = "watchlist_reassessment"
)

type WatchlistSource string

const (
	WatchlistManual       WatchlistSource = "manual"
	WatchlistAutoFromPick WatchlistSource = "auto_from_pick"
)

type Pick struct {
	ID                int        `json:"id"`
	Date              time.Time  `json:"date"`
	CardName          string     `json:"card_name"`
	SetName           string     `json:"set_name"`
	Grade             string     `json:"grade"`
	Direction         Direction  `json:"direction"`
	Confidence        Confidence `json:"confidence"`
	BuyThesis         string     `json:"buy_thesis"`
	TargetBuyPrice    int        `json:"target_buy_price"`
	ExpectedSellPrice int        `json:"expected_sell_price"`
	Signals           []Signal   `json:"signals"`
	Rank              int        `json:"rank"`
	Source            PickSource `json:"source"`
	CreatedAt         time.Time  `json:"created_at"`
}

type Signal struct {
	Factor    string          `json:"factor"`
	Direction SignalDirection `json:"direction"`
	Title     string          `json:"title"`
	Detail    string          `json:"detail"`
}

type WatchlistItem struct {
	ID               int             `json:"id"`
	CardName         string          `json:"card_name"`
	SetName          string          `json:"set_name"`
	Grade            string          `json:"grade"`
	Source           WatchlistSource `json:"source"`
	Active           bool            `json:"active"`
	LatestAssessment *Pick           `json:"latest_assessment,omitempty"`
	AddedAt          time.Time       `json:"added_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type GradeProfile struct {
	Grade     string  `json:"grade"`
	AvgROI    float64 `json:"avg_roi"`
	AvgMargin int     `json:"avg_margin"`
	Count     int     `json:"count"`
}

type TierProfile struct {
	MinPrice int     `json:"min_price"`
	MaxPrice int     `json:"max_price"`
	AvgROI   float64 `json:"avg_roi"`
	Count    int     `json:"count"`
}

type ProfitabilityProfile struct {
	TopEras              []string       `json:"top_eras"`
	ProfitableGrades     []GradeProfile `json:"profitable_grades"`
	ProfitablePriceTiers []TierProfile  `json:"profitable_price_tiers"`
	AvgDaysToSell        int            `json:"avg_days_to_sell"`
	TopChannels          []string       `json:"top_channels"`
}
