package intelligence

import (
	"slices"
	"time"
)

// WeeklyBucket is a per-week price aggregate for a single card. WeekStart is
// always Monday 00:00 UTC so buckets from different sources / timezones align.
type WeeklyBucket struct {
	WeekStart        time.Time
	SaleCount        int
	AvgPriceCents    int64
	MedianPriceCents int64
}

// TrajectoryScore captures whether a card's market price is drifting up,
// down, or flat over the recent past, and how confident we are. LowConfidence
// is true when the underlying sale count is below the noise floor — callers
// should downweight the score in operator-facing lenses.
type TrajectoryScore struct {
	SlopeCentsPerWeek   float64
	NormalizedByCLValue float64
	WindowWeeks         int
	TotalSales          int
	LowConfidence       bool
}

// minSalesForConfidence is the sale-count threshold below which trajectory
// scores are flagged low-confidence. Picked from the Story 3 spec.
const minSalesForConfidence = 5

// defaultTrajectoryWindow is the lookback window used by ComputeTrajectoryScore.
const defaultTrajectoryWindow = 8

// BucketSalesByWeek groups sales into Monday-00:00-UTC weekly buckets and
// returns them in chronological order. Empty input produces an empty slice.
// Sales with zero timestamps are skipped (they would bucket to the zero week
// and poison the score).
func BucketSalesByWeek(sales []Sale) []WeeklyBucket {
	if len(sales) == 0 {
		return nil
	}
	grouped := make(map[time.Time][]int64)
	for _, s := range sales {
		if s.SoldAt.IsZero() {
			continue
		}
		week := weekStartUTC(s.SoldAt)
		grouped[week] = append(grouped[week], s.PriceCents)
	}
	out := make([]WeeklyBucket, 0, len(grouped))
	for week, prices := range grouped {
		out = append(out, WeeklyBucket{
			WeekStart:        week,
			SaleCount:        len(prices),
			AvgPriceCents:    avgInt64(prices),
			MedianPriceCents: medianInt64(prices),
		})
	}
	slices.SortFunc(out, func(a, b WeeklyBucket) int { return a.WeekStart.Compare(b.WeekStart) })
	return out
}

// ComputeTrajectoryScore fits a least-squares slope to avg price over the
// trailing `defaultTrajectoryWindow` buckets, normalizes by CL value at the
// window start, and returns a score. Buckets are expected in chronological
// order (as BucketSalesByWeek returns them). LowConfidence is set when the
// total sale count in the window is below minSalesForConfidence.
func ComputeTrajectoryScore(buckets []WeeklyBucket, clValueCents int64) TrajectoryScore {
	score := TrajectoryScore{WindowWeeks: defaultTrajectoryWindow}
	if len(buckets) == 0 {
		score.LowConfidence = true
		return score
	}
	start := 0
	if len(buckets) > defaultTrajectoryWindow {
		start = len(buckets) - defaultTrajectoryWindow
	}
	window := buckets[start:]

	var total int
	for _, b := range window {
		total += b.SaleCount
	}
	score.TotalSales = total
	score.LowConfidence = total < minSalesForConfidence

	// Least-squares slope: x = week index from window start, y = avg price.
	n := float64(len(window))
	if n < 2 {
		return score
	}
	var sumX, sumY, sumXY, sumXX float64
	for i, b := range window {
		x := float64(i)
		y := float64(b.AvgPriceCents)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return score
	}
	slope := (n*sumXY - sumX*sumY) / denom
	score.SlopeCentsPerWeek = slope
	if clValueCents > 0 {
		score.NormalizedByCLValue = slope / float64(clValueCents)
	}
	return score
}

// weekStartUTC truncates to Monday 00:00 UTC so weekly buckets are stable
// across sources regardless of the original sale's local time.
func weekStartUTC(t time.Time) time.Time {
	u := t.UTC()
	// Go's Weekday is Sunday=0..Saturday=6; we want Monday=0 for the offset.
	offset := (int(u.Weekday()) + 6) % 7
	day := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -offset)
}

func avgInt64(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	var sum int64
	for _, x := range xs {
		sum += x
	}
	return sum / int64(len(xs))
}

func medianInt64(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := make([]int64, len(xs))
	copy(sorted, xs)
	slices.Sort(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
