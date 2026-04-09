package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// pushNewCards pushes unsold purchases with cert numbers to the CL Firestore
// collection if they don't already have a mapping. Returns the number of cards pushed.
func (s *CardLadderRefreshScheduler) pushNewCards(
	ctx context.Context,
	uid, collectionID string,
	purchases []campaigns.Purchase,
	existingMappings []sqlite.CLCardMapping,
) int {
	// Build set of already-mapped certs for quick lookup
	mappedCerts := make(map[string]bool, len(existingMappings))
	for _, m := range existingMappings {
		mappedCerts[m.SlabSerial] = true
	}

	pushed := 0
	for i := range purchases {
		p := &purchases[i]
		if p.CertNumber == "" || mappedCerts[p.CertNumber] {
			continue
		}

		select {
		case <-ctx.Done():
			return pushed
		default:
		}

		grader := strings.ToLower(p.Grader)
		if grader == "" {
			grader = "psa"
		}

		if err := s.pushSingleCard(ctx, uid, collectionID, p, grader); err != nil {
			s.logger.Warn(ctx, "CL push: failed to push card",
				observability.String("cert", p.CertNumber),
				observability.Err(err))
			continue
		}
		pushed++
	}

	if pushed > 0 {
		s.logger.Info(ctx, "CL push: complete",
			observability.Int("pushed", pushed))
	}
	return pushed
}

// pushSingleCard resolves a cert, estimates value, and writes to Firestore.
func (s *CardLadderRefreshScheduler) pushSingleCard(
	ctx context.Context,
	uid, collectionID string,
	p *campaigns.Purchase,
	grader string,
) error {
	buildResp, err := s.client.BuildCollectionCard(ctx, p.CertNumber, grader)
	if err != nil {
		return fmt.Errorf("resolve cert %s: %w", p.CertNumber, err)
	}

	estimateResp, err := s.client.CardEstimate(ctx, cardladder.CardEstimateRequest{
		GemRateID:      buildResp.GemRateID,
		GradingCompany: buildResp.GradingCompany,
		Condition:      buildResp.GemRateCondition,
		Description:    buildResp.Player,
	})
	if err != nil {
		return fmt.Errorf("estimate cert %s: %w", p.CertNumber, err)
	}

	label := fmt.Sprintf("%s %s %s %s #%s %s",
		buildResp.Year, buildResp.Set, buildResp.Player,
		buildResp.Variation, buildResp.Number, buildResp.Condition)

	var datePurchased time.Time
	if p.PurchaseDate != "" {
		datePurchased, _ = time.Parse("2006-01-02", p.PurchaseDate)
	}

	input := cardladder.AddCollectionCardInput{
		Label:            label,
		Player:           buildResp.Player,
		PlayerIndexID:    estimateResp.IndexID,
		Category:         buildResp.Category,
		Year:             buildResp.Year,
		Set:              buildResp.Set,
		Number:           buildResp.Number,
		Variation:        buildResp.Variation,
		Condition:        buildResp.Condition,
		GradingCompany:   buildResp.GradingCompany,
		GemRateID:        buildResp.GemRateID,
		GemRateCondition: buildResp.GemRateCondition,
		SlabSerial:       buildResp.SlabSerial,
		Pop:              buildResp.Pop,
		ImageURL:         buildResp.ImageURL,
		ImageBackURL:     buildResp.ImageBackURL,
		CurrentValue:     estimateResp.EstimatedValue,
		Investment:       float64(p.BuyCostCents) / 100.0,
		DatePurchased:    datePurchased,
	}

	docName, err := s.client.CreateCollectionCard(ctx, uid, collectionID, input)
	if err != nil {
		return fmt.Errorf("create card in Firestore: %w", err)
	}

	// Save the local mapping
	if err := s.store.SaveMapping(ctx, p.CertNumber, docName, buildResp.GemRateID, buildResp.GemRateCondition); err != nil {
		s.logger.Warn(ctx, "CL push: failed to save mapping after Firestore write",
			observability.String("cert", p.CertNumber), observability.Err(err))
	}

	// Update cl_synced_at on the purchase
	if s.syncUpdater != nil {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := s.syncUpdater.UpdatePurchaseCLSyncedAt(ctx, p.ID, now); err != nil {
			s.logger.Warn(ctx, "CL push: failed to update cl_synced_at",
				observability.String("cert", p.CertNumber), observability.Err(err))
		}
	}

	return nil
}

// removeSoldCards detects sold cards (mapped certs no longer in the unsold set)
// and removes them from the CL Firestore collection. Returns the number removed.
func (s *CardLadderRefreshScheduler) removeSoldCards(
	ctx context.Context,
	unsoldPurchases []campaigns.Purchase,
	existingMappings []sqlite.CLCardMapping,
) int {
	// Build set of unsold cert numbers
	unsoldCerts := make(map[string]bool, len(unsoldPurchases))
	for _, p := range unsoldPurchases {
		if p.CertNumber != "" {
			unsoldCerts[p.CertNumber] = true
		}
	}

	removed := 0
	for _, m := range existingMappings {
		if unsoldCerts[m.SlabSerial] {
			continue // still unsold, keep in CL
		}

		select {
		case <-ctx.Done():
			return removed
		default:
		}

		// Card was sold — remove from Firestore
		if m.CLCollectionCardID != "" {
			if err := s.client.DeleteCollectionCard(ctx, m.CLCollectionCardID); err != nil {
				s.logger.Warn(ctx, "CL remove: failed to delete from Firestore",
					observability.String("cert", m.SlabSerial),
					observability.String("docName", m.CLCollectionCardID),
					observability.Err(err))
				continue
			}
		}

		// Remove local mapping
		if err := s.store.DeleteMapping(ctx, m.SlabSerial); err != nil {
			s.logger.Warn(ctx, "CL remove: failed to delete mapping",
				observability.String("cert", m.SlabSerial),
				observability.Err(err))
		}

		removed++
	}

	if removed > 0 {
		s.logger.Info(ctx, "CL remove: complete",
			observability.Int("removed", removed))
	}
	return removed
}
