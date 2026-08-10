package scheduler

import (
	"context"
	"strings"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ inventory.CertEnrichEnqueuer = (*CertEnrichJob)(nil)
var _ Scheduler = (*CertEnrichJob)(nil)

// CertEnrichRepository is the narrow slice of purchase persistence this job
// needs: find the purchase behind a cert, then write back the metadata PSA
// returned. Declared here rather than taking inventory.PurchaseRepository so
// the job's actual surface is visible at the seam.
type CertEnrichRepository interface {
	GetPurchaseByCertNumber(ctx context.Context, grader string, certNumber string) (*inventory.Purchase, error)
	UpdatePurchaseCardMetadata(ctx context.Context, id string, cardName, cardNumber, setName string) error
	UpdatePurchaseCardYear(ctx context.Context, id string, year string) error
	UpdatePurchaseGrade(ctx context.Context, id string, gradeValue float64) error
	UpdatePurchaseImages(ctx context.Context, id string, frontURL, backURL string) error
}

// CertEnrichJob handles background PSA certificate enrichment.
// It processes cert numbers sequentially, respecting PSA API rate limits.
type CertEnrichJob struct {
	StopHandle
	ch         chan enrichRequest
	certLookup inventory.CertLookup
	repo       CertEnrichRepository
	logger     observability.Logger

	// imagesUnavailable records certs whose image lookup already failed or came
	// back empty in this process. PSA returns a per-cert 500 from
	// GetImagesByCertNumber for slabs whose images it cannot serve, and that
	// does not resolve on its own. Without this, the BACKFILL_IMAGES sweep
	// re-requests the same doomed certs on every restart and burns the daily
	// quota that intake needs (SLA-108).
	//
	// In-process only: it is deliberately not persisted, so a genuine PSA
	// outage during one boot cannot permanently blacklist good certs.
	imagesMu          sync.Mutex
	imagesUnavailable map[string]bool
}

// enrichRequest is one unit of work for the enrichment worker.
type enrichRequest struct {
	certNumber string
	// imagesOnly skips the PSA cert lookup and fetches images alone. The
	// backfill sweep uses it: those rows already have their metadata, so the
	// cert lookup is a second PSA call bought for nothing.
	imagesOnly bool
}

// NewCertEnrichJob creates a new cert enrichment job.
func NewCertEnrichJob(
	certLookup inventory.CertLookup,
	repo CertEnrichRepository,
	logger observability.Logger,
) *CertEnrichJob {
	return &CertEnrichJob{
		StopHandle:        NewStopHandle(),
		ch:                make(chan enrichRequest, 200), // bounded channel matching previous implementation
		certLookup:        certLookup,
		repo:              repo,
		logger:            logger.With(context.Background(), observability.String("component", "cert-enrich")),
		imagesUnavailable: make(map[string]bool),
	}
}

// Enqueue submits a cert number for background enrichment (non-blocking).
// If the queue is full, the cert is dropped silently.
func (j *CertEnrichJob) Enqueue(certNumber string) {
	j.submit(enrichRequest{certNumber: certNumber})
}

// EnqueueImagesOnly submits a cert for image backfill alone, skipping the PSA
// cert lookup. Used by the startup backfill sweep, where card metadata is
// already persisted and only the image URLs are missing.
func (j *CertEnrichJob) EnqueueImagesOnly(certNumber string) {
	j.submit(enrichRequest{certNumber: certNumber, imagesOnly: true})
}

func (j *CertEnrichJob) submit(req enrichRequest) {
	select {
	case j.ch <- req:
	default:
		if j.logger != nil {
			j.logger.Warn(context.Background(), "cert enrichment queue full, dropping cert",
				observability.String("cert", req.certNumber))
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
		case req, ok := <-j.ch:
			if !ok {
				return
			}
			// Stop processing buffered certs after shutdown
			if ctx.Err() != nil {
				return
			}
			if req.imagesOnly {
				j.enrichImagesOnly(ctx, req.certNumber)
				continue
			}
			j.enrichSingleCert(ctx, req.certNumber)
		}
	}
}

// enrichImagesOnly handles a backfill request: load the purchase and fetch
// images, with no PSA cert lookup.
func (j *CertEnrichJob) enrichImagesOnly(ctx context.Context, certNum string) {
	if j.certLookup == nil {
		return
	}
	purchase, err := j.repo.GetPurchaseByCertNumber(ctx, "PSA", certNum)
	if err != nil {
		if j.logger != nil {
			j.logger.Warn(ctx, "image backfill: failed to lookup purchase",
				observability.String("cert", certNum),
				observability.Err(err))
		}
		return
	}
	if purchase == nil {
		return
	}
	j.enrichImages(ctx, purchase, certNum)
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

	// Run image backfill before the grade branches below, which have early
	// returns that would otherwise skip it for zero-grade certs.
	j.enrichImages(ctx, purchase, certNum)

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
	if j.imagesKnownUnavailable(certNum) {
		return
	}

	front, back, err := j.certLookup.LookupImages(ctx, certNum)
	if err != nil {
		// Don't ask PSA for this cert's images again in this process. Whether
		// the cause is a per-cert data fault or a wider outage, re-requesting
		// it on the next sweep spends quota we cannot spare; a restart clears
		// the mark, so a real outage self-heals.
		j.markImagesUnavailable(certNum)
		if j.logger != nil {
			j.logger.Warn(ctx, "cert enrichment: PSA image lookup failed, skipping cert until restart",
				observability.String("cert", certNum),
				observability.Err(err))
		}
		return
	}
	if front == "" && back == "" {
		// PSA answered and has no images for this slab. Re-asking cannot
		// change that, so retire the cert from the backfill sweep too.
		j.markImagesUnavailable(certNum)
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

func (j *CertEnrichJob) imagesKnownUnavailable(certNum string) bool {
	j.imagesMu.Lock()
	defer j.imagesMu.Unlock()
	return j.imagesUnavailable[certNum]
}

func (j *CertEnrichJob) markImagesUnavailable(certNum string) {
	j.imagesMu.Lock()
	defer j.imagesMu.Unlock()
	j.imagesUnavailable[certNum] = true
}
