package insights

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Service produces a composed Overview for the Insights page.
type Service interface {
	GetOverview(ctx context.Context) (*Overview, error)
}

// Deps holds the collaborators composed into an Overview.
// All fields except Logger are optional; the service degrades gracefully.
type Deps struct {
	Campaigns inventory.CampaignRepository
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
	return &Overview{
		Actions:     []Action{},
		Signals:     Signals{},
		Campaigns:   []TuningRow{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
