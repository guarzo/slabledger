package demand

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// gradeBuckets enumerated for Phase 1 graded niche buckets.
// `raw_near_mint` is mentioned in the design doc but comes from
// price_distribution.by_grade — defer to a later pass.
var gradeBuckets = []int{7, 8, 9, 10}

// Sort modes supported by LeaderboardOpts.Sort.
const (
	SortOpportunityScore  = "opportunity_score"
	SortDemandScore       = "demand_score"
	SortVelocityChangePct = "velocity_change_pct"
	SortLowCoverage       = "low_coverage"
)

// Data-quality filter values.
const (
	QualityProxy = "proxy"
	QualityFull  = "full"
	// QualityEmpty is returned by CampaignSignals when no signal contributors
	// were found (distinct from "proxy"/"full" which apply to demand signals).
	QualityEmpty = "empty"
)

// ErrInvalidWindow is returned when LeaderboardOpts.Window is not "7d" or "30d".
var ErrInvalidWindow = errors.New("demand: window must be \"7d\" or \"30d\"")

// Service orchestrates niche-opportunity computation on top of the demand
// Repository and a CampaignCoverageLookup.
type Service struct {
	repo      Repository
	campaigns CampaignCoverageLookup
	logger    observability.Logger
}

// NewService constructs a Service. Both dependencies are required.
// Logging defaults to a no-op; use WithLogger to inject a real logger.
func NewService(repo Repository, campaigns CampaignCoverageLookup) *Service {
	return &Service{repo: repo, campaigns: campaigns, logger: observability.NewNoopLogger()}
}

// WithLogger injects a logger and returns the Service for chaining.
// A nil argument is ignored; the existing logger is kept.
func (s *Service) WithLogger(l observability.Logger) *Service {
	if l != nil {
		s.logger = l
	}
	return s
}

// LeaderboardOpts controls a Leaderboard call.
type LeaderboardOpts struct {
	Window         string // "7d" or "30d"
	Limit          int    // <=0 means no limit
	Sort           string // one of the Sort* constants; empty → SortOpportunityScore
	MinDataQuality string // "", "proxy", or "full"; "full" excludes proxy rows
	Era            string // optional filter; empty = no filter
	Grade          int    // optional filter; 0 = no filter (emits all grades)
}

// Leaderboard returns the top niche opportunities with 3-axis scoring.
// It reads character-cache rows for the given window — already decoded into
// CharacterDemand/CharacterVelocity/CharacterSaturation by the adapter —
// enumerates grade buckets, joins to campaign coverage, computes opportunity
// scores, sorts, and limits.
func (s *Service) Leaderboard(ctx context.Context, opts LeaderboardOpts) ([]NicheOpportunity, error) {
	if opts.Window != "7d" && opts.Window != "30d" {
		return nil, ErrInvalidWindow
	}
	rows, err := s.repo.ListCharacterCache(ctx, opts.Window)
	if err != nil {
		return nil, fmt.Errorf("list character cache: %w", err)
	}

	grades := gradeBuckets
	if opts.Grade != 0 {
		grades = []int{opts.Grade}
	}

	// Coverage is keyed on (character, grade) today — era is a no-op in the
	// SQLite-backed CampaignCoverageLookup because the campaign schema has no
	// era field. Memoize to cut CampaignsCovering + UnsoldCountFor from once
	// per (character, era, grade) triple down to once per (character, grade)
	// pair, which also survives intact if CampaignCoverageLookup starts
	// honoring era later: at worst we redo the lookup per era if the cache
	// key gains the era dimension.
	type coverageKey struct {
		character string
		grade     int
	}
	coverageCache := make(map[coverageKey]NicheCoverage)
	coverageFor := func(character, era string, grade int) (NicheCoverage, error) {
		key := coverageKey{character: character, grade: grade}
		if cov, ok := coverageCache[key]; ok {
			return cov, nil
		}
		campaignIDs, err := s.campaigns.CampaignsCovering(ctx, character, era, grade)
		if err != nil {
			return NicheCoverage{}, fmt.Errorf("campaigns covering (%s/%s/%d): %w", character, era, grade, err)
		}
		unsold, err := s.campaigns.UnsoldCountFor(ctx, character, era, grade)
		if err != nil {
			return NicheCoverage{}, fmt.Errorf("unsold count (%s/%s/%d): %w", character, era, grade, err)
		}
		cov := NicheCoverage{
			OurUnsoldCount:    unsold,
			ActiveCampaignIDs: campaignIDs,
			Covered:           len(campaignIDs) > 0,
		}
		coverageCache[key] = cov
		return cov, nil
	}

	out := make([]NicheOpportunity, 0, len(rows)*len(grades))
	for _, row := range rows {
		if row.Demand == nil {
			// A nil Demand is normal for a row whose payload column is
			// legitimately NULL. Only warn when a MalformedPayloads entry
			// says the nil is actually a decode failure — otherwise this
			// would fire on every ordinary row with no demand data yet.
			for _, mp := range row.MalformedPayloads {
				if mp.Column != MalformedColumnDemand {
					continue
				}
				s.logger.Warn(ctx, "demand_json unmarshal failed",
					observability.String("character", row.Character),
					observability.Err(mp.Err))
			}
			continue
		}
		demand := row.Demand

		// Build one bucket per (era, grade), plus the "all eras" bucket for
		// characters whose decoded demand payload has no ByEra breakdown.
		market := s.parseCharacterMarket(ctx, row)
		eras := erasForRow(demand, opts.Era)
		for _, era := range eras {
			eraDemand, eraOK := eraDemandFor(demand, era)
			if !eraOK {
				continue
			}
			if !qualityAllowed(opts.MinDataQuality, eraDemand.DataQuality) {
				continue
			}
			for _, grade := range grades {
				bucket, bErr := s.buildBucket(row.Character, era, grade, eraDemand, market, coverageFor)
				if bErr != nil {
					return nil, bErr
				}
				out = append(out, bucket)
			}
		}
	}

	sortOpportunities(out, opts.Sort)
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

// buildBucket joins a (character, era, grade) triple to coverage and returns
// the fully-scored NicheOpportunity. The coverage callback memoizes
// CampaignsCovering + UnsoldCountFor across buckets in a single Leaderboard
// call to avoid per-triple SQL round trips.
func (s *Service) buildBucket(
	character, era string,
	grade int,
	demand *NicheDemand,
	market *NicheMarket,
	coverageFor func(character, era string, grade int) (NicheCoverage, error),
) (NicheOpportunity, error) {
	coverage, err := coverageFor(character, era, grade)
	if err != nil {
		return NicheOpportunity{}, err
	}

	demandScore := 0.0
	if demand != nil {
		demandScore = demand.Score
	}
	var velocityChange *float64
	activeListingCount := 0
	if market != nil {
		velocityChange = market.VelocityChangePct
		activeListingCount = market.ActiveListingCount
	}

	niche := NicheOpportunity{
		Character:        character,
		Era:              era,
		Grade:            grade,
		Demand:           demand,
		Market:           market,
		Coverage:         coverage,
		OpportunityScore: OpportunityScore(demandScore, velocityChange, activeListingCount, coverage),
	}
	if market != nil && market.VelocityChangePct != nil {
		accel := &NicheAcceleration{
			MedianVelocityChangePct: *market.VelocityChangePct,
			TotalCount:              1,
			AcceleratingCount:       0,
			DataQuality:             QualityFull,
			ComputedAt:              market.ComputedAt,
		}
		if *market.VelocityChangePct >= AccelerationThresholdPct {
			accel.AcceleratingCount = 1
		}
		niche.Acceleration = accel
	}
	return niche, nil
}

// --- Sort ---

func sortOpportunities(out []NicheOpportunity, mode string) {
	switch mode {
	case SortDemandScore:
		sort.SliceStable(out, func(i, j int) bool {
			return demandOf(out[i]) > demandOf(out[j])
		})
	case SortVelocityChangePct:
		sort.SliceStable(out, func(i, j int) bool {
			return velocityOf(out[i]) > velocityOf(out[j])
		})
	case SortLowCoverage:
		sort.SliceStable(out, func(i, j int) bool {
			ci, cj := out[i].Coverage.Covered, out[j].Coverage.Covered
			if ci != cj {
				return !ci // uncovered first
			}
			return out[i].OpportunityScore > out[j].OpportunityScore
		})
	default: // SortOpportunityScore (or empty)
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].OpportunityScore > out[j].OpportunityScore
		})
	}
}

func demandOf(o NicheOpportunity) float64 {
	if o.Demand == nil {
		return 0
	}
	return o.Demand.Score
}

func velocityOf(o NicheOpportunity) float64 {
	if o.Market == nil || o.Market.VelocityChangePct == nil {
		return 0
	}
	return *o.Market.VelocityChangePct
}

// erasForRow returns the set of eras to emit buckets for. If the caller
// filtered by opts.Era we honour it; otherwise we emit every era present in
// the row's ByEra map. If there is no ByEra map, we emit a single "" bucket
// for the character overall.
func erasForRow(demand *CharacterDemand, filter string) []string {
	if filter != "" {
		return []string{filter}
	}
	if len(demand.ByEra) == 0 {
		return []string{""}
	}
	eras := make([]string, 0, len(demand.ByEra))
	for era := range demand.ByEra {
		eras = append(eras, era)
	}
	sort.Strings(eras)
	return eras
}

// eraDemandFor returns the NicheDemand for a given era within a character's
// demand payload. An empty era means "character overall".
func eraDemandFor(demand *CharacterDemand, era string) (*NicheDemand, bool) {
	if era == "" {
		return &NicheDemand{
			Score:        demand.AvgDemandScore,
			Views:        demand.TotalViews,
			WishlistAdds: demand.TotalWishlistAdds,
			DataQuality:  demand.DataQuality,
			ComputedAt:   parseTime(demand.ComputedAt),
		}, true
	}
	entry, ok := demand.ByEra[era]
	if !ok {
		return nil, false
	}
	return &NicheDemand{
		Score:        entry.AvgDemandScore,
		Views:        entry.TotalViews,
		WishlistAdds: entry.TotalWishlistAdds,
		DataQuality:  demand.DataQuality,
		ComputedAt:   parseTime(demand.ComputedAt),
	}, true
}

// parseCharacterMarket extracts the market-axis view (velocity + saturation)
// from a character cache row. Returns nil if both surfaces are absent. It also
// emits a Warn for each MalformedPayloads entry naming the velocity column, so
// a decode failure the adapter could not log is still visible.
func (s *Service) parseCharacterMarket(ctx context.Context, row CharacterCache) *NicheMarket {
	m := &NicheMarket{}
	has := false

	if row.Velocity != nil {
		v := row.Velocity
		m.MedianDaysToSell = v.MedianDaysToSell
		m.VelocityChangePct = v.VelocityChangePct
		m.SampleSize = v.SampleSize
		m.AvgDailySales = v.AvgDailySales
		m.SellThroughRate30d = v.SellThroughRate30d
		m.SalesVolume7d = v.SalesVolume7d
		m.SalesVolume30d = v.SalesVolume30d
		m.SupplyCount = v.SupplyCount
		has = true
	}
	if row.Saturation != nil {
		m.ActiveListingCount = row.Saturation.ActiveListingCount
		has = true
	}
	for _, mp := range row.MalformedPayloads {
		if mp.Column != MalformedColumnVelocity {
			continue
		}
		s.logger.Warn(ctx, "velocity_json unmarshal failed",
			observability.String("character", row.Character),
			observability.Err(mp.Err))
	}
	if !has {
		// Row has no analytics — flag for callers.
		if row.AnalyticsComputedAt == nil {
			return &NicheMarket{AnalyticsNotComputed: true}
		}
		return nil
	}
	m.ComputedAt = row.AnalyticsComputedAt
	return m
}

// qualityAllowed returns true if the row's data_quality satisfies the filter.
func qualityAllowed(minQuality, rowQuality string) bool {
	switch minQuality {
	case "", QualityProxy:
		return true
	case QualityFull:
		return rowQuality == QualityFull
	default:
		return true
	}
}

// parseTime parses a DH-style timestamp. Best-effort — returns the zero
// time.Time on any error, which is fine for downstream API shaping.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
