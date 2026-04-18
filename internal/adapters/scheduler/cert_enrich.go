package scheduler

import (
	"context"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ inventory.CertEnrichEnqueuer = (*CertEnrichJob)(nil)
var _ Scheduler = (*CertEnrichJob)(nil)

// CertEnrichJob handles background PSA certificate enrichment.
// It processes cert numbers sequentially, respecting PSA API rate limits (100/day).
type CertEnrichJob struct {
	StopHandle
	ch         chan string
	certLookup inventory.CertLookup
	repo       inventory.PurchaseRepository
	logger     observability.Logger
}

// NewCertEnrichJob creates a new cert enrichment job.
func NewCertEnrichJob(
	certLookup inventory.CertLookup,
	repo inventory.PurchaseRepository,
	logger observability.Logger,
) *CertEnrichJob {
	return &CertEnrichJob{
		StopHandle: NewStopHandle(),
		ch:         make(chan string, 200), // bounded channel matching previous implementation
		certLookup: certLookup,
		repo:       repo,
		logger:     logger.With(context.Background(), observability.String("component", "cert-enrich")),
	}
}

// Enqueue submits a cert number for background enrichment (non-blocking).
// If the queue is full, the cert is dropped silently.
func (j *CertEnrichJob) Enqueue(certNumber string) {
	select {
	case j.ch <- certNumber:
	default:
		if j.logger != nil {
			j.logger.Warn(context.Background(), "cert enrichment queue full, dropping cert",
				observability.String("cert", certNumber))
		}
	}
}

// Start begins the background cert enrichment worker.
func (j *CertEnrichJob) Start(ctx context.Context) {
	wg := j.WG()
	wg.Add(1)
	go func() {
		defer wg.Done()
		j.logger.Info(ctx, "cert-enrich scheduler started")
		j.worker(ctx)
		j.logger.Info(ctx, "cert-enrich scheduler stopped")
	}()
}

// worker reads from the cert channel and enriches each cert.
func (j *CertEnrichJob) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case certNum, ok := <-j.ch:
			if !ok {
				return
			}
			// Stop processing buffered certs after shutdown
			if ctx.Err() != nil {
				return
			}
			j.enrichSingleCert(ctx, certNum)
		}
	}
}

// enrichSingleCert enriches a single PSA cert by looking up its metadata and updating the purchase.
func (j *CertEnrichJob) enrichSingleCert(ctx context.Context, certNum string) {
	if j.certLookup == nil {
		return
	}

	info, err := j.certLookup.LookupCert(ctx, certNum)
	if err != nil {
		if j.logger != nil {
			j.logger.Warn(ctx, "cert enrichment: PSA lookup failed",
				observability.String("cert", certNum),
				observability.Err(err))
		}
		return
	}
	if info == nil {
		return
	}

	purchase, lookupErr := j.repo.GetPurchaseByCertNumber(ctx, "PSA", certNum)
	if lookupErr != nil {
		if j.logger != nil {
			j.logger.Warn(ctx, "cert enrichment: failed to lookup purchase",
				observability.String("cert", certNum),
				observability.Err(lookupErr))
		}
		return
	}
	if purchase == nil {
		return
	}

	cardName := info.CardName
	if cardName == "" {
		cardName = purchase.CardName
	}
	cardNumber := info.CardNumber
	if cardNumber == "" {
		cardNumber = purchase.CardNumber
	}

	setName := purchase.SetName
	if info.Category != "" {
		resolved := inventory.ResolvePSACategory(info.Category)
		if !inventory.IsGenericSetName(resolved) {
			setName = resolved
		}
	}

	if info.Variety != "" && !strings.Contains(strings.ToUpper(cardName), strings.ToUpper(info.Variety)) {
		cardName = cardName + " " + info.Variety
	}

	if err := j.repo.UpdatePurchaseCardMetadata(ctx, purchase.ID, cardName, cardNumber, setName); err != nil {
		if j.logger != nil {
			j.logger.Warn(ctx, "cert enrichment: failed to update purchase",
				observability.String("cert", certNum),
				observability.Err(err))
		}
		return
	}

	if info.Year != "" && purchase.CardYear == "" {
		if err := j.repo.UpdatePurchaseCardYear(ctx, purchase.ID, info.Year); err != nil && j.logger != nil {
			j.logger.Warn(ctx, "cert enrichment: failed to update card year",
				observability.String("cert", certNum),
				observability.Err(err))
		}
	}

	// Persist grade from cert if it differs from the purchase.
	// Fallback chain: cert info → existing purchase → parsed from PSA listing title.
	if info.Grade == 0 && purchase.GradeValue != 0 {
		// Use existing purchase grade
		return
	}
	if info.Grade == 0 {
		extractedGrade := inventory.ExtractGrade(purchase.PSAListingTitle)
		if extractedGrade != 0 {
			if err := j.repo.UpdatePurchaseGrade(ctx, purchase.ID, extractedGrade); err != nil {
				if j.logger != nil {
					j.logger.Warn(ctx, "cert enrichment: failed to persist title-extracted grade",
						observability.String("cert", certNum),
						observability.Err(err))
				}
			}
		}
		return
	}
	if info.Grade != 0 && info.Grade != purchase.GradeValue {
		if err := j.repo.UpdatePurchaseGrade(ctx, purchase.ID, info.Grade); err != nil {
			if j.logger != nil {
				j.logger.Warn(ctx, "cert enrichment: failed to update grade",
					observability.String("cert", certNum),
					observability.Err(err))
			}
		}
	}

	j.enrichImages(ctx, purchase, certNum)

	// Card metadata is now enriched. Snapshots will be captured separately via ProcessPendingSnapshots
	// if needed, or by other background jobs.
}

// enrichImages fetches slab images from PSA and updates the purchase when both
// image URLs are currently empty. Skipping when either field is already set
// preserves sheet-supplied URLs and avoids spending PSA budget on rows that
// have already been populated.
func (j *CertEnrichJob) enrichImages(ctx context.Context, purchase *inventory.Purchase, certNum string) {
	if purchase.FrontImageURL != "" || purchase.BackImageURL != "" {
		return
	}

	front, back, err := j.certLookup.LookupImages(ctx, certNum)
	if err != nil {
		if j.logger != nil {
			j.logger.Warn(ctx, "cert enrichment: PSA image lookup failed",
				observability.String("cert", certNum),
				observability.Err(err))
		}
		return
	}
	if front == "" && back == "" {
		return
	}

	if err := j.repo.UpdatePurchaseImages(ctx, purchase.ID, front, back); err != nil {
		if j.logger != nil {
			j.logger.Warn(ctx, "cert enrichment: failed to update images",
				observability.String("cert", certNum),
				observability.Err(err))
		}
	}
}
