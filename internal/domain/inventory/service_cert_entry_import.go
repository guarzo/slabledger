package inventory

// Extracted from ImportCerts (SLA-97) to keep that function under the funlen
// budget; the decomposition is behavior-preserving. The three `continue`
// statements in the original per-cert loop became `return` since each case
// now runs in its own call frame.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// importExistingCert handles a cert number that already has a purchase row:
// it records a sold-existing hit, or flags the cert for eBay export and
// enrolls it in the DH push pipeline when there is no sale.
func (s *service) importExistingCert(ctx context.Context, existing *Purchase, certNum string, now time.Time, hasSale bool, result *CertImportResult) {
	if hasSale {
		result.SoldExisting++
		result.SoldItems = append(result.SoldItems, CertImportSoldItem{
			CertNumber: certNum,
			PurchaseID: existing.ID,
			CardName:   existing.CardName,
			CampaignID: existing.CampaignID,
		})
		return
	}
	// Confirmed no sale — treat as normal existing cert
	if flagErr := s.purchases.SetEbayExportFlag(ctx, existing.ID, now); flagErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "cert import: failed to set ebay export flag",
				observability.String("cert", certNum),
				observability.Err(flagErr))
		}
		result.Failed++
		result.Errors = append(result.Errors, CertImportError{
			CertNumber: certNum,
			Error:      fmt.Sprintf("exists but failed to flag for export: %v", flagErr),
			Retryable:  true, // DB write failure — transient, safe to retry
		})
		return
	}
	if recvErr := s.purchases.SetReceivedAt(ctx, existing.ID, now); recvErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "cert import: failed to set received_at",
				observability.String("cert", certNum),
				observability.Err(recvErr))
		}
	} else {
		// Reflect the DB write on the in-memory struct so the
		// subsequent NeedsDHPush() guard sees the receipt.
		receivedStr := now.Format(time.RFC3339)
		existing.ReceivedAt = &receivedStr
	}
	s.enrollExistingInDHPushPipeline(ctx, existing, certNum, "cert import")
	// An existing cert re-scanned at intake is likely a receipt event for a
	// row that was created earlier (PSA sheet sync, CSV import). Price it
	// now so the operator sees CL values on the scan screen instead of
	// waiting for the daily refresh.
	if s.pricingQueue != nil {
		s.pricingQueue.Enqueue(certNum)
	}
	result.AlreadyExisted++
}

// importNewCert handles a cert number with no existing purchase row: it
// performs the PSA lookup, builds and persists the new Purchase, and enqueues
// downstream enrichment. Returns imported=true iff a new row was created;
// psaDown=true iff the lookup failure indicates PSA is unavailable
// batch-wide, so the caller can latch psaUnavailable for the remaining certs
// in the loop.
func (s *service) importNewCert(ctx context.Context, certNum string, now time.Time, result *CertImportResult) (imported bool, psaDown bool) {
	info, certErr := s.certLookup.LookupCert(ctx, certNum)
	if certErr != nil {
		result.Failed++
		result.Errors = append(result.Errors, CertImportError{
			CertNumber: certNum,
			Error:      certErr.Error(),
			Retryable:  isRetryableImportError(certErr),
		})
		// A breaker-open / rate-limit error means PSA is down for the whole
		// batch, not just this cert — latch it so remaining certs are
		// queued instead of hammering the open breaker.
		if IsPSAUnavailableError(certErr) {
			return false, true
		}
		return false, false
	}
	if info == nil {
		result.Failed++
		result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: "cert not found"})
		return false, false
	}

	// Only adopt PSA's Category when it's a real set. PSA returns generic
	// values like "TCG Cards" for many older certs; persisting those as the
	// set name pollutes inventory and breaks downstream price lookups. Leave
	// SetName empty and let CL enrichment (resolveGemRate) fill it in.
	setName := ""
	if info.Category != "" {
		resolved := ResolvePSACategory(info.Category)
		if !IsGenericSetName(resolved) {
			setName = resolved
		}
	}

	cardName := info.CardName
	if info.Variety != "" && !strings.Contains(strings.ToUpper(cardName), strings.ToUpper(info.Variety)) {
		cardName = cardName + " " + info.Variety
	}

	receivedStr := now.Format(time.RFC3339)
	// AttributionSource is deliberately left unset (the store defaults it to
	// 'inferred'): ExternalCampaignID is a fixed sentinel bucket for cards that
	// belong to no campaign, not a campaign the operator chose. Recording
	// 'manual' here would assert an attribution decision nobody made.
	purchase := &Purchase{
		ID:                  s.idGen(),
		CampaignID:          ExternalCampaignID,
		CardName:            cardName,
		CertNumber:          certNum,
		CardNumber:          info.CardNumber,
		SetName:             setName,
		Grader:              "PSA",
		GradeValue:          info.Grade,
		Population:          info.Population,
		CardYear:            info.Year,
		BuyCostCents:        0,
		CLValueCents:        0,
		PSASourcingFeeCents: 0,
		PurchaseDate:        now.Format("2006-01-02"),
		PSAListingTitle:     info.Subject,
		EbayExportFlaggedAt: &now,
		ReceivedAt:          &receivedStr,
		DHPushStatus:        DHPushStatusPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if createErr := s.purchases.CreatePurchase(ctx, purchase); createErr != nil {
		// A duplicate-cert error here means a previous (likely timed-out and
		// client-retried) call to ImportCerts already inserted this row. The
		// batch lookup at the top of this function missed it because that
		// previous call hadn't committed yet when this request started. Treat
		// the row as already-existing so the retry reports success instead of
		// confusing the operator with a phantom failure.
		if IsDuplicateCertNumber(createErr) {
			result.AlreadyExisted++
			return false, false
		}
		result.Failed++
		result.Errors = append(result.Errors, CertImportError{
			CertNumber: certNum,
			Error:      createErr.Error(),
			Retryable:  true, // DB write failure — transient, safe to retry
		})
		return false, false
	}

	if recvErr := s.purchases.SetReceivedAt(ctx, purchase.ID, now); recvErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "cert import: failed to set received_at",
				observability.String("cert", certNum),
				observability.Err(recvErr))
		}
	}

	if s.certEnrichQueue != nil {
		s.certEnrichQueue.Enqueue(certNum)
	}
	if s.pricingQueue != nil {
		s.pricingQueue.Enqueue(certNum)
	}

	result.Imported++
	return true, false
}
