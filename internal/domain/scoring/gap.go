package scoring

import (
	"context"
	"time"
)

type GapRecord struct {
	FactorName string
	Reason     string
	EntityType string
	EntityID   string
	CardName   string
	SetName    string
	RecordedAt time.Time
}

type GapFactorSummary struct {
	Factor    string  `json:"factor"`
	Count     int     `json:"count"`
	Pct       float64 `json:"pct"`
	TopReason string  `json:"top_reason"`
}

type GapSetSummary struct {
	SetName        string   `json:"set"`
	GapCount       int      `json:"gap_count"`
	MissingFactors []string `json:"missing_factors"`
}

type GapReport struct {
	Period        string             `json:"period"`
	TotalScorings int                `json:"total_scorings"`
	TotalGaps     int                `json:"total_gaps"`
	GapRate       float64            `json:"gap_rate"`
	ByFactor      []GapFactorSummary `json:"by_factor"`
	MostAffected  []GapSetSummary    `json:"most_affected_sets"`
	Suggestions   []string           `json:"suggestions"`
}

type GapStore interface {
	RecordGaps(ctx context.Context, gaps []GapRecord) error
	GetGapReport(ctx context.Context, since time.Time) (*GapReport, error)
	PruneOldGaps(ctx context.Context, olderThan time.Time) (int64, error)
}
