package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// transientImportErrorCodes are provider failure modes that are expected to
// resolve on their own: the PSA API was unreachable, the daily call quota was
// exhausted, the request timed out, or the circuit breaker is open. A cert that
// fails for one of these reasons should be retried, not discarded — unlike a
// genuine "cert not found" or a malformed response, which will fail identically
// on every retry.
var transientImportErrorCodes = []apperrors.ErrorCode{
	apperrors.ErrCodeProviderUnavailable,
	apperrors.ErrCodeProviderRateLimit,
	apperrors.ErrCodeProviderTimeout,
	apperrors.ErrCodeProviderCircuitOpen,
	// An exhausted PSA token pool reports ERR_PROV_AUTH. It belongs here for
	// the same reason ERR_PROV_RATE_LIMIT does: the row is intact and a later
	// attempt can succeed — quota returns at UTC rollover, and a key retired
	// for repeated 401s is restored on restart. Its neighbours in the same
	// batch are staged retryable regardless (the batch latch queues them), so
	// leaving this one out only made the first cert of a failed batch look
	// permanently broken when it is not (SLA-108).
	apperrors.ErrCodeProviderAuth,
	apperrors.ErrCodeNetworkUnavailable,
	apperrors.ErrCodeNetworkTimeout,
}

// isRetryableImportError reports whether a per-cert import failure was caused by
// a transient provider/network condition (see transientImportErrorCodes). It
// walks the wrapped error chain, so it works on errors the PSA adapter has
// wrapped with fmt.Errorf("...%w", err).
//
// A bare context.Canceled/DeadlineExceeded (e.g. a request deadline firing mid
// import) is also treated as transient: it carries no AppError code but is, by
// definition, a "try again" condition. Defense-in-depth — adapters should wrap
// context errors in ProviderTimeout, but we don't rely on every one doing so.
func isRetryableImportError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	for _, code := range transientImportErrorCodes {
		if apperrors.HasErrorCode(err, code) {
			return true
		}
	}
	return false
}

// IsPSAUnavailableError reports whether an error means PSA is unusable for
// every cert right now — the circuit breaker is open, the token pool is rate
// limited, or every token has been rejected — as opposed to a failure specific
// to one cert (not-found, a single timeout).
//
// Two callers depend on the distinction. The import loop stops issuing lookups
// for the remaining certs and queues them for retry, because hammering an open
// breaker only produces instant ERR_PROV_CIRCUIT_OPEN noise and delays
// recovery. The cert-enrichment job uses it to decide whether a failed image
// lookup says anything about that cert at all; a pool-wide failure says
// nothing, so the cert must not be added to the skip set (SLA-108).
//
// Auth is included because an exhausted PSA token pool surfaces as
// ERR_PROV_AUTH once every token is dead, which is a property of the pool and
// not of the cert being looked up.
func IsPSAUnavailableError(err error) bool {
	return apperrors.HasErrorCode(err, apperrors.ErrCodeProviderCircuitOpen) ||
		apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit) ||
		apperrors.HasErrorCode(err, apperrors.ErrCodeProviderAuth)
}

func (s *service) ImportCerts(ctx context.Context, certNumbers []string) (*CertImportResult, error) {
	// Deduplicate and clean input
	seen := make(map[string]bool, len(certNumbers))
	cleaned := make([]string, 0, len(certNumbers))
	for _, cn := range certNumbers {
		cn = strings.TrimSpace(cn)
		if cn == "" || seen[cn] {
			continue
		}
		seen[cn] = true
		cleaned = append(cleaned, cn)
	}

	// Ensure external campaign exists
	_, err := EnsureExternalCampaign(ctx, s.campaigns)
	if err != nil {
		return nil, fmt.Errorf("ensure external campaign: %w", err)
	}

	// Batch lookup: find all certs that already exist in one query
	existingMap, err := s.purchases.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", cleaned)
	if err != nil {
		return nil, fmt.Errorf("batch cert lookup: %w", err)
	}

	result := &CertImportResult{Errors: []CertImportError{}, SoldItems: []CertImportSoldItem{}}
	now := time.Now()
	var importedCerts []string

	// Batch lookup: find which existing purchases have sales
	var existingPurchaseIDs []string
	for _, p := range existingMap {
		existingPurchaseIDs = append(existingPurchaseIDs, p.ID)
	}
	salesMap, err := s.sales.GetSalesByPurchaseIDs(ctx, existingPurchaseIDs)
	if err != nil {
		return nil, fmt.Errorf("batch sale lookup: %w", err)
	}

	// Once PSA signals it is unavailable batch-wide (breaker open or rate
	// limited), stop issuing lookups for the remaining certs — they would only
	// produce instant circuit-open failures. Queue them for retry instead.
	psaUnavailable := false

	for _, certNum := range cleaned {
		if existing, ok := existingMap[certNum]; ok {
			_, hasSale := salesMap[existing.ID]
			s.importExistingCert(ctx, existing, certNum, now, hasSale, result)
			continue
		}

		if s.certLookup == nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: "cert lookup not configured"})
			continue
		}

		// PSA already signalled batch-wide unavailability earlier in this
		// import — do not issue another doomed lookup; queue this cert.
		if psaUnavailable {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{
				CertNumber: certNum,
				Error:      "PSA temporarily unavailable — queued for retry",
				Retryable:  true,
			})
			continue
		}

		imported, psaDown := s.importNewCert(ctx, certNum, now, result)
		if psaDown {
			psaUnavailable = true
		}
		if imported {
			importedCerts = append(importedCerts, certNum)
		}
	}

	// Kick off background cert→card_id resolution for imported certs.
	if s.cardIDResolver != nil && len(importedCerts) > 0 {
		certs := append([]string(nil), importedCerts...)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			s.batchResolveCardIDs(ctx, certs)
		}()
	}

	return result, nil
}

// GetPurchasesByCertNumbers delegates to the repository to look up purchases by cert number.
func (s *service) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*Purchase, error) {
	return s.purchases.GetPurchasesByCertNumbers(ctx, certNumbers)
}

// ScanCert checks a single cert against the database and returns its status.
// For existing (unsold) certs, it also sets the eBay export flag.
func (s *service) ScanCert(ctx context.Context, certNumber string) (*ScanCertResult, error) {
	certNumber = strings.TrimSpace(certNumber)
	if certNumber == "" {
		return nil, fmt.Errorf("cert number is required")
	}

	existing, err := s.purchases.GetPurchaseByCertNumber(ctx, "PSA", certNumber)
	if err != nil {
		if errors.Is(err, ErrPurchaseNotFound) {
			return &ScanCertResult{Status: "new"}, nil
		}
		return nil, fmt.Errorf("scan cert lookup: %w", err)
	}

	// Check if sold
	salesMap, err := s.sales.GetSalesByPurchaseIDs(ctx, []string{existing.ID})
	if err != nil {
		return nil, fmt.Errorf("scan cert sale check: %w", err)
	}

	if _, hasSale := salesMap[existing.ID]; hasSale {
		return newScanCertResult("sold", existing, s.buildEnrichedSnapshot(existing)), nil
	}

	// Existing and not sold — flag for eBay export and mark received
	now := time.Now()
	if flagErr := s.purchases.SetEbayExportFlag(ctx, existing.ID, now); flagErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "scan cert: failed to set ebay export flag",
				observability.String("cert", certNumber),
				observability.Err(flagErr))
		}
	}
	if recvErr := s.purchases.SetReceivedAt(ctx, existing.ID, now); recvErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "scan cert: failed to set received_at",
				observability.String("cert", certNumber),
				observability.Err(recvErr))
		}
	} else {
		// Reflect the DB write on the in-memory struct so the subsequent
		// NeedsDHPush() guard sees the receipt.
		receivedStr := now.Format(time.RFC3339)
		existing.ReceivedAt = &receivedStr
	}
	s.enrollExistingInDHPushPipeline(ctx, existing, certNumber, "scan cert")
	if s.pricingQueue != nil {
		s.pricingQueue.Enqueue(certNumber)
	}

	return newScanCertResult("existing", existing, s.buildEnrichedSnapshot(existing)), nil
}

// pointerToString dereferences a string pointer, returning "" for nil.
func pointerToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// newScanCertResult builds a ScanCertResult for an existing-or-sold cert,
// copying the identity + search-helper metadata fields off the Purchase, plus
// the DH pipeline state the intake screen uses to gate listability.
func newScanCertResult(status string, p *Purchase, market *MarketSnapshot) *ScanCertResult {
	return &ScanCertResult{
		Status:              status,
		CardName:            p.CardName,
		PurchaseID:          p.ID,
		CampaignID:          p.CampaignID,
		BuyCostCents:        p.BuyCostCents,
		Market:              market,
		FrontImageURL:       p.FrontImageURL,
		SetName:             p.SetName,
		CardNumber:          p.CardNumber,
		CardYear:            p.CardYear,
		GradeValue:          p.GradeValue,
		Population:          p.Population,
		DHSearchQuery:       cardutil.BuildCardMatchQuery(p.SetName, p.CardName, p.CardNumber),
		DHCardID:            p.DHCardID,
		DHInventoryID:       p.DHInventoryID,
		DHPushStatus:        p.DHPushStatus,
		DHStatus:            p.DHStatus,
		DHListingPriceCents: p.DHListingPriceCents,
		ReceivedAt:          pointerToString(p.ReceivedAt),
	}
}

// ScanCerts runs ScanCert for each supplied cert number, partitioning results:
// successes land in Results (keyed by cert number), per-cert failures land in
// Errors. Duplicate and empty inputs are coalesced. The cert-intake polling
// loop uses this endpoint to avoid the per-row fan-out that was tripping the
// server's rate limiter.
func (s *service) ScanCerts(ctx context.Context, certNumbers []string) (*ScanCertsResult, error) {
	seen := make(map[string]struct{}, len(certNumbers))
	out := &ScanCertsResult{Results: make(map[string]*ScanCertResult, len(certNumbers))}
	for _, raw := range certNumbers {
		cert := strings.TrimSpace(raw)
		if cert == "" {
			continue
		}
		if _, dup := seen[cert]; dup {
			continue
		}
		seen[cert] = struct{}{}

		result, err := s.ScanCert(ctx, cert)
		if err != nil {
			out.Errors = append(out.Errors, CertImportError{CertNumber: cert, Error: err.Error()})
			continue
		}
		out.Results[cert] = result
	}
	return out, nil
}

// enrollExistingInDHPushPipeline flips dh_push_status to 'pending' for a
// received, unsold purchase that hasn't already been matched/held/dismissed,
// and resets an exhausted snapshot so the pricing scheduler will retry. This
// is what transitions a PSA-sheet-synced row into the DH sync pipeline at the
// point of physical receipt. The guard relies on Purchase.NeedsDHPush() to
// leave terminal states alone.
func (s *service) enrollExistingInDHPushPipeline(ctx context.Context, p *Purchase, cert, source string) {
	if p == nil || !p.NeedsDHPush() {
		return
	}
	if err := s.purchases.UpdatePurchaseDHPushStatus(ctx, p.ID, DHPushStatusPending); err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, source+": failed to enroll in DH push pipeline",
				observability.String("cert", cert),
				observability.String("purchaseID", p.ID),
				observability.Err(err))
		}
		return
	}
	if s.logger != nil {
		s.logger.Info(ctx, source+": enrolled in DH push pipeline",
			observability.String("cert", cert),
			observability.String("purchaseID", p.ID))
	}
	s.recordEvent(ctx, dhevents.Event{
		PurchaseID:    p.ID,
		CertNumber:    p.CertNumber,
		Type:          dhevents.TypeEnrolled,
		NewPushStatus: DHPushStatusPending,
		Source:        dhevents.SourceCertIntake,
	})
	if p.SnapshotStatus == SnapshotStatusExhausted {
		if err := s.purchases.UpdatePurchaseSnapshotStatus(ctx, p.ID, SnapshotStatusPending, 0); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, source+": failed to reset exhausted snapshot status",
					observability.String("cert", cert),
					observability.String("purchaseID", p.ID),
					observability.Err(err))
			}
			return
		}
		if s.logger != nil {
			s.logger.Info(ctx, source+": reset exhausted snapshot status to pending",
				observability.String("cert", cert),
				observability.String("purchaseID", p.ID))
		}
	}
}

// ResolveCert looks up a PSA cert number via the external PSA API.
// Returns card info for preview; does NOT create a purchase.
func (s *service) ResolveCert(ctx context.Context, certNumber string) (*CertInfo, error) {
	certNumber = strings.TrimSpace(certNumber)
	if certNumber == "" {
		return nil, fmt.Errorf("cert number is required")
	}

	if s.certLookup == nil {
		return nil, ErrCertLookupNotConfigured
	}

	info, err := s.certLookup.LookupCert(ctx, certNumber)
	if err != nil {
		return nil, fmt.Errorf("resolve cert %s: %w", certNumber, err)
	}
	if info == nil {
		return nil, ErrCertNotFound
	}

	return info, nil
}
