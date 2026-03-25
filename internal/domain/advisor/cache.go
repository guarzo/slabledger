package advisor

import (
	"context"
	"time"
)

// AnalysisType identifies the kind of cached analysis.
type AnalysisType string

const (
	AnalysisDigest      AnalysisType = "digest"
	AnalysisLiquidation AnalysisType = "liquidation"
)

// CacheStatus tracks the lifecycle of a cached analysis.
type CacheStatus string

const (
	StatusEmpty    CacheStatus = "empty"
	StatusRunning  CacheStatus = "running"
	StatusComplete CacheStatus = "complete"
	StatusError    CacheStatus = "error"
)

// CachedAnalysis represents a persisted advisor analysis result.
type CachedAnalysis struct {
	AnalysisType AnalysisType
	Status       CacheStatus
	Content      string
	ErrorMessage string
	StartedAt    time.Time
	CompletedAt  time.Time
	UpdatedAt    time.Time
}

// CacheStore persists and retrieves advisor analysis results.
//
// Methods that transition an entry to "running" return a lease string that
// uniquely identifies the run (currently the started_at timestamp).  Callers
// must pass the lease to SaveResult so that a superseded goroutine cannot
// overwrite a newer run's results.
type CacheStore interface {
	Get(ctx context.Context, analysisType AnalysisType) (*CachedAnalysis, error)
	// MarkRunning unconditionally transitions the entry to "running" and
	// returns the lease for the new run.
	MarkRunning(ctx context.Context, analysisType AnalysisType) (lease string, err error)
	// AcquireRefresh atomically transitions the entry to "running" only if it
	// is not already running. Returns the lease and true if the caller
	// acquired the lock.
	AcquireRefresh(ctx context.Context, analysisType AnalysisType) (lease string, acquired bool, err error)
	// ForceAcquireStale atomically transitions a "running" entry to "running"
	// with a fresh timestamp, but only if started_at is older than staleThreshold.
	// Returns the lease and true if the caller acquired the lock.
	ForceAcquireStale(ctx context.Context, analysisType AnalysisType, staleThreshold time.Duration) (lease string, acquired bool, err error)
	// SaveResult persists the analysis outcome.  The lease must match the
	// value returned by the method that started this run; if it doesn't
	// (because another run superseded this one) the write is silently skipped.
	SaveResult(ctx context.Context, analysisType AnalysisType, lease string, content string, errMsg string) error
}
