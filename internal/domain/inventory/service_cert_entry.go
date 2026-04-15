package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

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
	_, err := s.EnsureExternalCampaign(ctx)
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

	for _, certNum := range cleaned {
		if existing, ok := existingMap[certNum]; ok {
			// Check if this purchase has been sold (using batch result)
			if _, hasSale := salesMap[existing.ID]; hasSale {
				result.SoldExisting++
				result.SoldItems = append(result.SoldItems, CertImportSoldItem{
					CertNumber: certNum,
					PurchaseID: existing.ID,
					CardName:   existing.CardName,
					CampaignID: existing.CampaignID,
				})
				continue
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
				})
				continue
			}
			if recvErr := s.purchases.SetReceivedAt(ctx, existing.ID, now); recvErr != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "cert import: failed to set received_at",
						observability.String("cert", certNum),
						observability.Err(recvErr))
				}
			}
			s.enrollExistingInDHPushPipeline(ctx, existing, certNum, "cert import")
			result.AlreadyExisted++
			continue
		}

		if s.certLookup == nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: "cert lookup not configured"})
			continue
		}

		info, certErr := s.certLookup.LookupCert(ctx, certNum)
		if certErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: certErr.Error()})
			continue
		}
		if info == nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: "cert not found"})
			continue
		}

		setName := info.Category
		if setName != "" {
			resolved := ResolvePSACategory(setName)
			if !IsGenericSetName(resolved) {
				setName = resolved
			}
		}

		cardName := info.CardName
		if info.Variety != "" && !strings.Contains(strings.ToUpper(cardName), strings.ToUpper(info.Variety)) {
			cardName = cardName + " " + info.Variety
		}

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
			DHPushStatus:        DHPushStatusPending,
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		if createErr := s.purchases.CreatePurchase(ctx, purchase); createErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: createErr.Error()})
			continue
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

		importedCerts = append(importedCerts, certNum)
		result.Imported++
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
		return &ScanCertResult{
			Status:     "sold",
			CardName:   existing.CardName,
			PurchaseID: existing.ID,
			CampaignID: existing.CampaignID,
		}, nil
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
	}
	s.enrollExistingInDHPushPipeline(ctx, existing, certNumber, "scan cert")

	return &ScanCertResult{
		Status:     "existing",
		CardName:   existing.CardName,
		PurchaseID: existing.ID,
		CampaignID: existing.CampaignID,
	}, nil
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
