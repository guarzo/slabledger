package inventory

import "context"

// SnapshotHistoryEntry represents a daily market snapshot archive entry.
type SnapshotHistoryEntry struct {
	CardName            string
	SetName             string
	CardNumber          string
	GradeValue          float64
	MedianCents         int
	ConservativeCents   int
	OptimisticCents     int
	LastSoldCents       int
	LowestListCents     int
	EstimatedValueCents int
	ActiveListings      int
	SalesLast30d        int
	SalesLast90d        int
	DailyVelocity       float64
	WeeklyVelocity      float64
	Trend30d            float64
	Trend90d            float64
	Volatility          float64
	SourceCount         int
	Confidence          float64
	SnapshotJSON        string
	SnapshotDate        string
}

// PopulationEntry records a point-in-time population observation.
type PopulationEntry struct {
	CardName        string
	SetName         string
	CardNumber      string
	GradeValue      float64
	Grader          string
	Population      int
	PopHigher       int
	ObservationDate string
	Source          string
}

// CLValueEntry records a point-in-time Card Ladder value observation.
type CLValueEntry struct {
	CertNumber      string
	CardName        string
	SetName         string
	CardNumber      string
	GradeValue      float64
	CLValueCents    int
	ObservationDate string
	Source          string
}

// SnapshotHistoryRecorder archives daily market snapshots.
type SnapshotHistoryRecorder interface {
	RecordSnapshot(ctx context.Context, entry SnapshotHistoryEntry) error
}

// PopulationHistoryRecorder tracks population changes over time.
type PopulationHistoryRecorder interface {
	RecordPopulation(ctx context.Context, entry PopulationEntry) error
}

// CLValueHistoryRecorder tracks Card Ladder value changes over time.
type CLValueHistoryRecorder interface {
	RecordCLValue(ctx context.Context, entry CLValueEntry) error
}
