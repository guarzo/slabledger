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

	result := &CertImportResult{}
	now := time.Now()

	for _, certNum := range cleaned {
		existing, lookupErr := s.repo.GetPurchaseByCertNumber(ctx, "PSA", certNum)
		if lookupErr == nil && existing != nil {
			if flagErr := s.repo.SetEbayExportFlag(ctx, existing.ID, now); flagErr != nil && s.logger != nil {
				s.logger.Warn(ctx, "cert import: failed to set ebay export flag",
					observability.String("cert", certNum),
					observability.Err(flagErr))
			}
			result.AlreadyExisted++
			continue
		}

		if s.certLookup == nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: "cert lookup not configured"})
			continue
		}

		info, lookupErr := s.certLookup.LookupCert(ctx, certNum)
		if lookupErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{CertNumber: certNum, Error: lookupErr.Error()})
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
			PSASourcingFeeCents:  0,
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
			}
		}

		result.Imported++
	}

	return result, nil
}
