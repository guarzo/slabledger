package demand

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// Acceleration thresholds used by CampaignSignals. Tune by code edit, not
// config — these are calibration constants, not user preferences.
const (
	// AccelerationThresholdPct is the minimum velocity_change_pct a character
	// must exhibit (over DH's 7d window) to count as "accelerating".
	AccelerationThresholdPct = 5.0
	// DecelerationThresholdPct is the maximum velocity_change_pct a character
	// may exhibit before being counted as "decelerating". Negative value.
	DecelerationThresholdPct = -5.0
	// TopContributorsLimit caps the TopAccelerating / TopDecelerating arrays
	// per campaign. Keeps response size bounded.
	TopContributorsLimit = 5
)

// DataQuality values returned by CampaignSignals.
//
// Market analytics either have velocity data or they don't — there's no
// "proxy" tier for this surface (that concept is specific to demand signals).
const (
	DataQualityFull  = "full"
	DataQualityEmpty = "empty"
)

// CampaignSignalsResponse is the return of Service.CampaignSignals.
type CampaignSignalsResponse struct {
	// ComputedAt is the min analytics_computed_at across all contributing rows,
	// or nil when the response is empty.
	ComputedAt *time.Time
	// DataQuality is "full" when at least one signal has contributors, else "empty".
	DataQuality string
	// Signals contains one entry per active campaign with at least one tracked
	// character. Campaigns whose character set doesn't intersect the cache are
	// omitted entirely — not included with TrackedCharacters=0.
	Signals []CampaignSignal
}

// CampaignSignal summarises DH market acceleration for a single campaign's
// character slice.
type CampaignSignal struct {
	CampaignID              int64
	CampaignName            string
	TrackedCharacters       int // Contributing characters (those with parseable velocity_change_pct).
	AcceleratingCount       int // Subset where velocity_change_pct >= AccelerationThresholdPct.
	DeceleratingCount       int // Subset where velocity_change_pct <= DecelerationThresholdPct.
	MedianVelocityChangePct float64
	DataQuality             string // Always "full" when TrackedCharacters > 0.
	ComputedAt              *time.Time
	// TopAccelerating is sorted by velocity_change_pct desc, capped at TopContributorsLimit.
	TopAccelerating []CampaignSignalContributor
	// TopDecelerating is sorted by velocity_change_pct asc, capped at TopContributorsLimit.
	TopDecelerating []CampaignSignalContributor
}

// CampaignSignalContributor is a single character's contribution to a signal.
type CampaignSignalContributor struct {
	Character         string   // Display name (original casing from the cache row).
	VelocityChangePct float64
	MedianDaysToSell  *float64 // Nil if DH's string value failed to parse.
	SampleSize        int
}

// --- Parsed velocity entry (internal only) ---

// signalEntry is the parsed representation of a single CharacterCache row used
// during CampaignSignals computation. Only rows with a non-nil
// velocity_change_pct survive indexing.
type signalEntry struct {
	displayName  string
	vChange      float64
	medianDays   *float64
	sampleSize   int
	computedAt   *time.Time
}

// signalVelocityJSON is the domain-local mirror of the velocity_json blob
// stored in dh_character_cache. Numeric-as-string fields are typed as strings
// to match DH's wire format exactly.
type signalVelocityJSON struct {
	MedianDaysToSell  string   `json:"median_days_to_sell"`
	SampleSize        int      `json:"sample_size"`
	VelocityChangePct *float64 `json:"velocity_change_pct,omitempty"`
}

// --- CampaignSignals ---

// CampaignSignals reads all characters from the 30d character cache and, for
// each active campaign, produces a velocity-acceleration summary covering the
// campaign's character slice. Campaigns with no matching characters in the
// cache are omitted from the result.
func (s *Service) CampaignSignals(ctx context.Context) (CampaignSignalsResponse, error) {
	campaigns, err := s.campaigns.ActiveCampaigns(ctx)
	if err != nil {
		return CampaignSignalsResponse{}, fmt.Errorf("active campaigns: %w", err)
	}

	rows, err := s.repo.ListCharacterCache(ctx, "30d")
	if err != nil {
		return CampaignSignalsResponse{}, fmt.Errorf("list character cache: %w", err)
	}

	// Build an index from lowercased character name → signalEntry.
	// Only rows with a valid VelocityJSON, non-nil AnalyticsComputedAt, and a
	// non-nil velocity_change_pct are included.
	index := buildSignalIndex(rows)

	var signals []CampaignSignal
	for _, c := range campaigns {
		contributors := collectContributors(c, index)
		if len(contributors) == 0 {
			continue
		}
		sig := buildCampaignSignal(c, contributors)
		signals = append(signals, sig)
	}

	resp := CampaignSignalsResponse{
		Signals: signals,
	}
	if len(signals) > 0 {
		resp.DataQuality = DataQualityFull
		resp.ComputedAt = minComputedAt(signals)
	} else {
		resp.DataQuality = DataQualityEmpty
	}
	return resp, nil
}

// buildSignalIndex constructs a map from lowercased character name to its
// parsed signalEntry. Rows missing VelocityJSON, AnalyticsComputedAt, invalid
// JSON, or a nil velocity_change_pct are silently skipped.
func buildSignalIndex(rows []CharacterCache) map[string]signalEntry {
	idx := make(map[string]signalEntry, len(rows))
	for _, row := range rows {
		if row.VelocityJSON == nil || row.AnalyticsComputedAt == nil {
			continue
		}
		var v signalVelocityJSON
		if err := json.Unmarshal([]byte(*row.VelocityJSON), &v); err != nil {
			continue // unparseable — skip row
		}
		if v.VelocityChangePct == nil {
			continue // no change metric — exclude from contributors
		}
		var medianDays *float64
		if parsed, err := strconv.ParseFloat(v.MedianDaysToSell, 64); err == nil {
			medianDays = &parsed
		}
		idx[strings.ToLower(row.Character)] = signalEntry{
			displayName: row.Character,
			vChange:     *v.VelocityChangePct,
			medianDays:  medianDays,
			sampleSize:  v.SampleSize,
			computedAt:  row.AnalyticsComputedAt,
		}
	}
	return idx
}

// collectContributors returns the signalEntry values from idx that belong to
// the given campaign's character slice. Matching mirrors
// characterMatchesInclusion in campaign_coverage.go: case-insensitive
// substring check when an inclusion list is set; all entries when open-net.
func collectContributors(c ActiveCampaign, idx map[string]signalEntry) []signalEntry {
	var out []signalEntry
	trimmed := strings.TrimSpace(c.InclusionList)

	if trimmed == "" {
		// Open-net: every indexed character contributes.
		for _, entry := range idx {
			out = append(out, entry)
		}
		return out
	}

	entries := inventory.SplitInclusionList(trimmed)
	for key, entry := range idx {
		matched := false
		for _, e := range entries {
			if strings.Contains(key, strings.ToLower(strings.TrimSpace(e))) {
				matched = true
				break
			}
		}
		if c.ExclusionMode {
			if !matched {
				out = append(out, entry)
			}
		} else {
			if matched {
				out = append(out, entry)
			}
		}
	}
	return out
}

// buildCampaignSignal assembles a CampaignSignal from a non-empty contributor
// slice.
func buildCampaignSignal(c ActiveCampaign, contributors []signalEntry) CampaignSignal {
	var accel, decel []signalEntry
	for _, e := range contributors {
		if e.vChange >= AccelerationThresholdPct {
			accel = append(accel, e)
		}
		if e.vChange <= DecelerationThresholdPct {
			decel = append(decel, e)
		}
	}

	// Sort accelerating desc, decelerating asc.
	sort.Slice(accel, func(i, j int) bool { return accel[i].vChange > accel[j].vChange })
	sort.Slice(decel, func(i, j int) bool { return decel[i].vChange < decel[j].vChange })

	topAccel := toContributors(accel, TopContributorsLimit)
	topDecel := toContributors(decel, TopContributorsLimit)

	median := medianVelocityChange(contributors)
	computedAt := minEntryComputedAt(contributors)

	return CampaignSignal{
		CampaignID:              c.ID,
		CampaignName:            c.Name,
		TrackedCharacters:       len(contributors),
		AcceleratingCount:       len(accel),
		DeceleratingCount:       len(decel),
		MedianVelocityChangePct: median,
		DataQuality:             DataQualityFull,
		ComputedAt:              computedAt,
		TopAccelerating:         topAccel,
		TopDecelerating:         topDecel,
	}
}

// toContributors converts a sorted signalEntry slice to CampaignSignalContributor,
// capped at limit.
func toContributors(entries []signalEntry, limit int) []CampaignSignalContributor {
	if len(entries) > limit {
		entries = entries[:limit]
	}
	out := make([]CampaignSignalContributor, len(entries))
	for i, e := range entries {
		out[i] = CampaignSignalContributor{
			Character:         e.displayName,
			VelocityChangePct: e.vChange,
			MedianDaysToSell:  e.medianDays,
			SampleSize:        e.sampleSize,
		}
	}
	return out
}

// medianVelocityChange computes the median velocity_change_pct across all
// contributors. Odd count: middle element; even count: average of two middles.
func medianVelocityChange(contributors []signalEntry) float64 {
	if len(contributors) == 0 {
		return 0
	}
	vals := make([]float64, len(contributors))
	for i, e := range contributors {
		vals[i] = e.vChange
	}
	sort.Float64s(vals)
	n := len(vals)
	if n%2 == 1 {
		return vals[n/2]
	}
	return (vals[n/2-1] + vals[n/2]) / 2
}

// minEntryComputedAt returns the earliest analytics_computed_at across the
// contributors, or nil if none have a non-nil value.
func minEntryComputedAt(contributors []signalEntry) *time.Time {
	var earliest *time.Time
	for _, e := range contributors {
		if e.computedAt == nil {
			continue
		}
		if earliest == nil || e.computedAt.Before(*earliest) {
			t := *e.computedAt
			earliest = &t
		}
	}
	return earliest
}

// minComputedAt returns the minimum ComputedAt across all signals.
func minComputedAt(signals []CampaignSignal) *time.Time {
	var earliest *time.Time
	for _, sig := range signals {
		if sig.ComputedAt == nil {
			continue
		}
		if earliest == nil || sig.ComputedAt.Before(*earliest) {
			t := *sig.ComputedAt
			earliest = &t
		}
	}
	return earliest
}
