package campaigns

import (
	"context"
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
	existingMap, err := s.repo.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", cleaned)
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
	salesMap, err := s.repo.GetSalesByPurchaseIDs(ctx, existingPurchaseIDs)
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
			if flagErr := s.repo.SetEbayExportFlag(ctx, existing.ID, now); flagErr != nil {
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
			resolved := resolvePSACategory(setName)
			if !isGenericSetName(resolved) {
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
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		if createErr := s.repo.CreatePurchase(ctx, purchase); createErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: createErr.Error()})
			continue
		}

		if s.certEnrichCh != nil {
			select {
			case s.certEnrichCh <- certNum:
			default:
				if s.logger != nil {
					s.logger.Warn(ctx, "cert enrichment channel full, skipping enrichment",
						observability.String("cert", certNum))
				}
			}
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
			ctx, cancel := context.WithTimeout(s.baseCtx, 2*time.Minute)
			defer cancel()
			s.batchResolveCardIDs(ctx, certs)
		}()
	}

	return result, nil
}
