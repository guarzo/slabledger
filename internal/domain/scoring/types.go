package scoring

import "time"

// Verdict represents the scoring engine's classification of an entity.
type Verdict string

const (
	VerdictStrongBuy  Verdict = "strong_buy"
	VerdictBuy        Verdict = "buy"
	VerdictLeanBuy    Verdict = "lean_buy"
	VerdictHold       Verdict = "hold"
	VerdictLeanSell   Verdict = "lean_sell"
	VerdictSell       Verdict = "sell"
	VerdictStrongSell Verdict = "strong_sell"
)

// VerdictFromComposite derives a verdict from a composite score.
func VerdictFromComposite(composite float64) Verdict {
	switch {
	case composite >= 0.6:
		return VerdictStrongBuy
	case composite >= 0.3:
		return VerdictBuy
	case composite >= 0.1:
		return VerdictLeanBuy
	case composite <= -0.6:
		return VerdictStrongSell
	case composite <= -0.3:
		return VerdictSell
	case composite <= -0.1:
		return VerdictLeanSell
	default:
		return VerdictHold
	}
}

// verdictOrder maps verdicts to ordinal positions for step distance calculation.
var verdictOrder = map[Verdict]int{
	VerdictStrongSell: 0,
	VerdictSell:       1,
	VerdictLeanSell:   2,
	VerdictHold:       3,
	VerdictLeanBuy:    4,
	VerdictBuy:        5,
	VerdictStrongBuy:  6,
}

// VerdictDistance returns the absolute step distance between two verdicts.
func VerdictDistance(a, b Verdict) int {
	d := verdictOrder[a] - verdictOrder[b]
	if d < 0 {
		return -d
	}
	return d
}

// Factor represents a single normalized scoring dimension.
type Factor struct {
	Name       string  `json:"name"`
	Value      float64 `json:"value"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

// DataGap tracks a factor that could not be computed.
type DataGap struct {
	FactorName string `json:"factor"`
	Reason     string `json:"reason"`
}

// ScoreCard is the output of scoring a single entity.
type ScoreCard struct {
	EntityID     string    `json:"entity_id"`
	EntityType   string    `json:"entity_type"`
	Factors      []Factor  `json:"factors"`
	Composite    float64   `json:"composite"`
	Confidence   float64   `json:"confidence"`
	Verdict      Verdict   `json:"engine_verdict"`
	DataGaps     []DataGap `json:"data_gaps"`
	MixedSignals bool      `json:"mixed_signals,omitempty"`
	ScoredAt     time.Time `json:"scored_at"`
}

// ScoreRequest bundles the entity identity with computed factors.
type ScoreRequest struct {
	EntityID   string
	EntityType string
	Factors    []Factor
	DataGaps   []DataGap
}

// FactorWeight defines a factor's contribution to the composite.
type FactorWeight struct {
	Name   string
	Weight float64
}

// WeightProfile is a named set of weights for a specific flow.
type WeightProfile struct {
	Name    string
	Weights []FactorWeight
}

// ErrInsufficientData is returned when fewer than MinFactors non-gap factors are available.
type ErrInsufficientData struct {
	Available int
	Required  int
	Gaps      []DataGap
}

func (e *ErrInsufficientData) Error() string {
	return "insufficient data for scoring"
}

// MinFactors is the minimum number of non-gap factors required to produce a score.
const MinFactors = 2
