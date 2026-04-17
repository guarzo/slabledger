package scheduler

import (
	"context"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ inventory.PricingEnqueuer = (*PricingEnrichJob)(nil)
var _ Scheduler = (*PricingEnrichJob)(nil)

// SinglePurchasePricer drives on-demand pricing for one purchase against a
// specific provider. Both CardLadderRefreshScheduler and
// MarketMoversRefreshScheduler expose a method with this shape so the enrich
// job can fan out without coupling to either package beyond the interface.
type SinglePurchasePricer interface {
	PriceSinglePurchase(ctx context.Context, p *inventory.Purchase) error
}

// PricingEnrichJob bridges the intake-time pricing enqueuer to the daily
// refresh schedulers. Given a cert number, the worker loads the purchase and
// fans it out to every configured pricer (CL, MM) so freshly scanned inventory
// gets priced immediately instead of waiting for the next daily cycle.
//
// Pricers are optional — if CL/MM isn't configured, the scheduler constructor
// is passed nil and this job just logs a debug line and moves on. The worker
// is sequential; bulk imports that enqueue many certs will price them one at
// a time, respecting the provider clients' own rate limiters.
type PricingEnrichJob struct {
	StopHandle
	ch       chan string
	repo     inventory.PurchaseRepository
	pricerMu sync.RWMutex
	pricers  []SinglePurchasePricer
	logger   observability.Logger
}

// NewPricingEnrichJob creates a pricing enrichment worker. Pricers can be
// injected here or later via SetPricers — the latter is needed when the
// provider schedulers are constructed after the inventory service that owns
// the enqueuer reference.
func NewPricingEnrichJob(repo inventory.PurchaseRepository, logger observability.Logger, pricers ...SinglePurchasePricer) *PricingEnrichJob {
	j := &PricingEnrichJob{
		StopHandle: NewStopHandle(),
		ch:         make(chan string, 200),
		repo:       repo,
		logger:     logger.With(context.Background(), observability.String("component", "pricing-enrich")),
	}
	j.SetPricers(pricers...)
	return j
}

// SetPricers replaces the active pricer set. Nil entries are filtered so
// callers can pass a disabled provider without guarding.
func (j *PricingEnrichJob) SetPricers(pricers ...SinglePurchasePricer) {
	active := make([]SinglePurchasePricer, 0, len(pricers))
	for _, p := range pricers {
		if p != nil {
			active = append(active, p)
		}
	}
	j.pricerMu.Lock()
	j.pricers = active
	j.pricerMu.Unlock()
}

func (j *PricingEnrichJob) activePricers() []SinglePurchasePricer {
	j.pricerMu.RLock()
	defer j.pricerMu.RUnlock()
	return j.pricers
}

// Enqueue submits a cert number for background pricing (non-blocking).
// Full queue drops silently — the daily refresh will catch up.
func (j *PricingEnrichJob) Enqueue(certNumber string) {
	if certNumber == "" {
		return
	}
	pricers := j.activePricers()
	if len(pricers) == 0 {
		if j.logger != nil {
			j.logger.Warn(context.Background(), "pricing enqueue dropped: no pricers configured",
				observability.String("cert", certNumber))
		}
		return
	}
	select {
	case j.ch <- certNumber:
		if j.logger != nil {
			j.logger.Info(context.Background(), "pricing enqueue accepted",
				observability.String("cert", certNumber),
				observability.Int("pricers", len(pricers)),
				observability.Int("queueDepth", len(j.ch)))
		}
	default:
		if j.logger != nil {
			j.logger.Warn(context.Background(), "pricing enqueue queue full, dropping cert",
				observability.String("cert", certNumber))
		}
	}
}

// Start begins the background pricing worker.
func (j *PricingEnrichJob) Start(ctx context.Context) {
	wg := j.WG()
	wg.Add(1)
	go func() {
		defer wg.Done()
		j.logger.Info(ctx, "pricing-enrich scheduler started")
		j.worker(ctx)
		j.logger.Info(ctx, "pricing-enrich scheduler stopped")
	}()
}

func (j *PricingEnrichJob) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case certNum, ok := <-j.ch:
			if !ok {
				return
			}
			if ctx.Err() != nil {
				return
			}
			j.priceOne(ctx, certNum)
		}
	}
}

// priceOne loads the purchase for a cert and runs every configured pricer.
// Errors from one provider don't short-circuit the others — each pricer already
// records its own per-purchase failure reason for the admin UI.
func (j *PricingEnrichJob) priceOne(ctx context.Context, certNum string) {
	j.logger.Info(ctx, "pricing-enrich: processing cert", observability.String("cert", certNum))
	purchase, err := j.repo.GetPurchaseByCertNumber(ctx, "PSA", certNum)
	if err != nil {
		j.logger.Warn(ctx, "pricing-enrich: failed to load purchase",
			observability.String("cert", certNum),
			observability.Err(err))
		return
	}
	if purchase == nil {
		j.logger.Warn(ctx, "pricing-enrich: purchase not found for enqueued cert",
			observability.String("cert", certNum))
		return
	}
	pricers := j.activePricers()
	j.logger.Info(ctx, "pricing-enrich: fanning out to pricers",
		observability.String("cert", certNum),
		observability.Int("pricers", len(pricers)))
	for _, p := range pricers {
		if err := p.PriceSinglePurchase(ctx, purchase); err != nil {
			j.logger.Warn(ctx, "pricing-enrich: pricer failed",
				observability.String("cert", certNum),
				observability.Err(err))
		}
	}
}
