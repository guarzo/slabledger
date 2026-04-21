package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ inventory.PricingEnqueuer = (*PricingEnrichJob)(nil)
var _ Scheduler = (*PricingEnrichJob)(nil)

// Pricing-enrich throughput tunables. The previous single-worker / depth-200
// setup dropped intake bursts because each cert blocks on 5–7 upstream HTTP
// calls (CL + MM) — IO-bound, not CPU-bound — so parallelism is cheap on a
// shared-cpu VM and deeper buffering absorbs CSV-scale imports.
const (
	pricingQueueSize   = 1000
	pricingWorkerCount = 4

	// FreshPriceWindow is how long an on-demand CL/MM price is considered
	// current. Within this window, re-enqueuing the same cert (e.g. a CSV
	// re-import, a double-scan at intake) skips the upstream call. Sized to
	// comfortably cover burst re-imports without staling out the freshly
	// scanned inventory that an operator is watching live.
	FreshPriceWindow = 15 * time.Minute
)

// isPriceFresh reports whether an RFC3339 timestamp is within FreshPriceWindow
// of now. An empty/unparseable timestamp is treated as stale so first-time
// pricing always runs.
func isPriceFresh(updatedAtRFC3339 string) bool {
	if updatedAtRFC3339 == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, updatedAtRFC3339)
	if err != nil {
		return false
	}
	return time.Since(t) < FreshPriceWindow
}

// SinglePurchasePricer drives on-demand pricing for one purchase against a
// specific provider. Both CardLadderRefreshScheduler and
// MarketMoversRefreshScheduler expose a method with this shape so the enrich
// job can fan out without coupling to either package beyond the interface.
//
// Concurrency invariant: PricingEnrichJob.priceOne calls multiple pricers
// concurrently, each on its own shallow copy of *Purchase. Implementations
// MUST NOT mutate through Purchase pointer fields (ReceivedAt,
// EbayExportFlaggedAt, DHUnlistedDetectedAt, etc.) — those pointers are
// shared across the concurrent copies and writing through them would race.
// Mutating value fields on the local copy is always safe.
type SinglePurchasePricer interface {
	PriceSinglePurchase(ctx context.Context, p *inventory.Purchase) error
}

// PricingEnrichJob bridges the intake-time pricing enqueuer to the daily
// refresh schedulers. Given a cert number, a pool of workers loads the
// purchase and fans it out to every configured pricer (CL, MM) so freshly
// scanned inventory gets priced immediately instead of waiting for the next
// daily cycle.
//
// Within one cert, pricers run concurrently — each on its own shallow copy of
// the Purchase so concurrent writes to value fields (CLValueCents,
// DHPushStatus) can't race. Pointer fields on Purchase remain shared across
// copies; see the SinglePurchasePricer interface doc for the invariant
// pricers must uphold. MM never reads fields CL mutates, so there is no
// logical cross-pricer coupling today.
//
// Pricers are optional — if CL/MM isn't configured, the scheduler constructor
// is passed nil and this job drops the enqueue with a warning.
type PricingEnrichJob struct {
	StopHandle
	ch       chan string
	repo     inventory.PurchaseRepository
	pricerMu sync.RWMutex
	pricers  []SinglePurchasePricer
	logger   observability.Logger
}

// NewPricingEnrichJob creates a pricing enrichment worker pool. Pricers can be
// injected here or later via SetPricers — the latter is needed when the
// provider schedulers are constructed after the inventory service that owns
// the enqueuer reference.
func NewPricingEnrichJob(repo inventory.PurchaseRepository, logger observability.Logger, pricers ...SinglePurchasePricer) *PricingEnrichJob {
	j := &PricingEnrichJob{
		StopHandle: NewStopHandle(),
		ch:         make(chan string, pricingQueueSize),
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
				observability.String("cert", certNumber),
				observability.Int("pricers", len(pricers)),
				observability.Int("queueDepth", len(j.ch)))
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
				observability.String("cert", certNumber),
				observability.Int("pricers", len(pricers)),
				observability.Int("queueDepth", len(j.ch)))
		}
	}
}

// Start launches the pool of background pricing workers.
func (j *PricingEnrichJob) Start(ctx context.Context) {
	wg := j.WG()
	j.logger.Info(ctx, "pricing-enrich scheduler started",
		observability.Int("workers", pricingWorkerCount),
		observability.Int("queueSize", pricingQueueSize))
	for i := range pricingWorkerCount {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			j.worker(ctx, id)
		}(i)
	}
}

func (j *PricingEnrichJob) worker(ctx context.Context, id int) {
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
			j.priceOne(ctx, certNum, id)
		}
	}
}

// priceOne loads the purchase for a cert and runs every configured pricer
// concurrently. Each pricer gets its own shallow copy of the purchase so
// concurrent field mutations (CLValueCents, DHPushStatus) can't race.
// Per-pricer errors don't short-circuit the others; each pricer records its
// own per-purchase failure reason for the admin UI.
func (j *PricingEnrichJob) priceOne(ctx context.Context, certNum string, workerID int) {
	j.logger.Info(ctx, "pricing-enrich: processing cert",
		observability.String("cert", certNum),
		observability.Int("worker", workerID))
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

	var wg sync.WaitGroup
	for _, p := range pricers {
		wg.Add(1)
		go func(pricer SinglePurchasePricer) {
			defer wg.Done()
			// Shallow copy: value fields are isolated per goroutine, but pointer
			// fields (ReceivedAt, EbayExportFlaggedAt, DHUnlistedDetectedAt) are
			// still shared. See the SinglePurchasePricer interface doc — pricers
			// must not mutate through those pointers. Neither CL nor MM does
			// today; enforce it in review for any new pricer.
			pCopy := *purchase
			if err := pricer.PriceSinglePurchase(ctx, &pCopy); err != nil {
				j.logger.Warn(ctx, "pricing-enrich: pricer failed",
					observability.String("cert", certNum),
					observability.Err(err))
			}
		}(p)
	}
	wg.Wait()
}
