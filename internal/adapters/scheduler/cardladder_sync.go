package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// pushNewCards pushes unsold purchases with cert numbers to the CL Firestore
// collection if they don't already have a mapping. Returns the number of cards pushed.
func (s *CardLadderRefreshScheduler) pushNewCards(
	ctx context.Context,
	client *cardladder.Client,
	uid, collectionID string,
	purchases []inventory.Purchase,
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

		if err := s.pushSingleCard(ctx, client, uid, collectionID, p, grader); err != nil {
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
	client *cardladder.Client,
	uid, collectionID string,
	p *inventory.Purchase,
	grader string,
) error {
	result, err := client.ResolveAndCreateCard(ctx, uid, collectionID, cardladder.CardPushParams{
		CertNumber:    p.CertNumber,
		Grader:        grader,
		InvestmentUSD: float64(p.BuyCostCents) / 100.0,
		DatePurchased: p.PurchaseDate,
	})
	if err != nil {
		return err
	}

	// Save the local mapping
	if err := s.store.SaveMapping(ctx, p.CertNumber, result.DocumentName, result.GemRateID, result.GemRateCondition); err != nil {
		s.logger.Error(ctx, "CL push: failed to save mapping after Firestore write — orphaned remote card",
			observability.String("cert", p.CertNumber),
			observability.String("docName", result.DocumentName),
			observability.Err(err))
		return fmt.Errorf("save mapping for cert %s: %w", p.CertNumber, err)
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
	client *cardladder.Client,
	unsoldPurchases []inventory.Purchase,
	existingMappings []sqlite.CLCardMapping,
) int {
	// Build set of unsold cert numbers
	unsoldCerts := make(map[string]bool, len(unsoldPurchases))
	for _, p := range unsoldPurchases {
		if p.CertNumber != "" {
			unsoldCerts[p.CertNumber] = true
		}
	}

	// Diagnostic: how many mappings does this phase consider "sold"?
	candidates := 0
	for _, m := range existingMappings {
		if !unsoldCerts[m.SlabSerial] {
			candidates++
		}
	}

	removed, remoteDeleteFailed, localDeleteFailed := 0, 0, 0

	// Defer the phase summary so cancelled runs still emit it. The closure
	// reads the enclosing counter variables, so whatever state we reach —
	// even an early return on ctx.Done — is captured correctly. The follow-up
	// PR needs this shape to diagnose orphan mapping sources.
	defer func() {
		s.logger.Info(ctx, "CL remove: phase summary",
			observability.Int("totalMappings", len(existingMappings)),
			observability.Int("unsoldCerts", len(unsoldCerts)),
			observability.Int("removalCandidates", candidates),
			observability.Int("removed", removed),
			observability.Int("remoteDeleteFailed", remoteDeleteFailed),
			observability.Int("localDeleteFailed", localDeleteFailed))
	}()

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
			if err := client.DeleteCollectionCard(ctx, m.CLCollectionCardID); err != nil {
				s.logger.Warn(ctx, "CL remove: failed to delete from Firestore",
					observability.String("cert", m.SlabSerial),
					observability.String("docName", m.CLCollectionCardID),
					observability.Err(err))
				remoteDeleteFailed++
				continue
			}
		}

		// Remove local mapping
		if err := s.store.DeleteMapping(ctx, m.SlabSerial); err != nil {
			s.logger.Warn(ctx, "CL remove: failed to delete mapping",
				observability.String("cert", m.SlabSerial),
				observability.Err(err))
			localDeleteFailed++
			continue
		}

		removed++
	}

	return removed
}
