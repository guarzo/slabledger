package insights

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/tuning"
)

// Service produces a composed Overview for the Insights page.
type Service interface {
	GetOverview(ctx context.Context) (*Overview, error)
}

// PricingService is the subset of inventory.Service that produces
// price-override stats. Defined here so the insights package depends on
// a narrow interface instead of the full inventory service.
type PricingService interface {
	GetPriceOverrideStats(ctx context.Context) (*inventory.PriceOverrideStats, error)
}

// Deps holds the collaborators composed into an Overview.
// All fields except Logger are optional; the service degrades gracefully.
type Deps struct {
	Campaigns inventory.CampaignRepository
	Tuning    tuning.Service
	Pricing   PricingService
	Logger    observability.Logger
}

// NewService constructs a Service from its dependencies.
func NewService(deps Deps) Service {
	return &service{deps: deps}
}

type service struct {
	deps Deps
}

func (s *service) GetOverview(ctx context.Context) (*Overview, error) {
	rows, err := s.campaignRows(ctx)
	if err != nil {
		return nil, fmt.Errorf("campaign rows: %w", err)
	}
	signals, err := s.signals(ctx)
	if err != nil {
		return nil, fmt.Errorf("signals: %w", err)
	}
	return &Overview{
		Actions:     []Action{},
		Signals:     signals,
		Campaigns:   rows,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *service) signals(ctx context.Context) (Signals, error) {
	var out Signals
	if s.deps.Pricing != nil {
		stats, err := s.deps.Pricing.GetPriceOverrideStats(ctx)
		if err != nil {
			if s.deps.Logger != nil {
				s.deps.Logger.Warn(ctx, "price override stats fetch failed",
					observability.String("err", err.Error()))
			}
		} else if stats != nil {
			// Resolved = accepted + dismissed. PriceOverrideStats does not currently
			// expose dismissed; in v1 we treat resolved == accepted, which makes Pct
			// render as 100% whenever any have been accepted and 0% when none have.
			// When PriceOverrideStats is extended with a Dismissed field, include it
			// in the resolved denominator here.
			accepted := stats.AIAcceptedCount
			resolved := accepted
			pct := 0.0
			if resolved > 0 {
				pct = (float64(accepted) / float64(resolved)) * 100.0
			}
			out.AIAcceptRate = AIAcceptRate{Pct: pct, Accepted: accepted, Resolved: resolved}
		}
	}
	return out, nil
}

func (s *service) campaignRows(ctx context.Context) ([]TuningRow, error) {
	if s.deps.Campaigns == nil || s.deps.Tuning == nil {
		return []TuningRow{}, nil
	}
	campaigns, err := s.deps.Campaigns.ListCampaigns(ctx, true) // activeOnly
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	rows := make([]TuningRow, 0, len(campaigns))
	for _, c := range campaigns {
		resp, err := s.deps.Tuning.GetCampaignTuning(ctx, c.ID)
		if err != nil {
			if s.deps.Logger != nil {
				s.deps.Logger.Warn(ctx, "tuning fetch failed for campaign",
					observability.String("campaignId", c.ID),
					observability.String("err", err.Error()))
			}
			continue
		}
		rows = append(rows, buildTuningRow(c, resp))
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CampaignName < rows[j].CampaignName })
	return rows, nil
}

func buildTuningRow(c inventory.Campaign, resp *inventory.TuningResponse) TuningRow {
	cells := make(map[string]TuningCell)
	for _, rec := range resp.Recommendations {
		col := MapParameterToColumn(rec.Parameter)
		if col == "" {
			continue
		}
		sev := DeriveCellSeverity(rec.Confidence)
		// Keep the highest-severity recommendation per column.
		if existing, ok := cells[col]; ok && severityRank(existing.Severity) >= severityRank(sev) {
			continue
		}
		cells[col] = TuningCell{
			Recommendation: formatRecommendation(rec),
			Severity:       sev,
		}
	}
	return TuningRow{
		CampaignID:   c.ID,
		CampaignName: c.Name,
		Cells:        cells,
		Status:       DeriveRowStatus(cells),
	}
}

func severityRank(s Severity) int {
	switch s {
	case SeverityAct:
		return 3
	case SeverityTune:
		return 2
	case SeverityOK:
		return 1
	default:
		return 0
	}
}

func formatRecommendation(r inventory.TuningRecommendation) string {
	if r.Impact != "" {
		return r.Impact
	}
	if r.SuggestedVal != "" && r.CurrentVal != "" {
		return fmt.Sprintf("%s → %s", r.CurrentVal, r.SuggestedVal)
	}
	return r.Reasoning
}
