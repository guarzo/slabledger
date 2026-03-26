package campaigns

import (
	"context"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) batchResolveCardIDs(ctx context.Context, certs []string) {
	if s.cardIDResolver == nil {
		return
	}
	resolved, err := s.cardIDResolver.ResolveCardIDsByCerts(ctx, certs, "PSA")
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "batch cert→card_id resolution failed",
				observability.Int("certs", len(certs)),
				observability.Err(err))
		}
		return
	}
	if s.logger != nil && len(resolved) > 0 {
		s.logger.Info(ctx, "batch cert→card_id resolution complete",
			observability.Int("requested", len(certs)),
			observability.Int("resolved", len(resolved)))
	}
}

func (s *service) certEnrichWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case certNum, ok := <-s.certEnrichCh:
			if !ok {
				return
			}
			s.enrichSingleCert(ctx, certNum)
		}
	}
}

func (s *service) enrichSingleCert(ctx context.Context, certNum string) {
	if s.certLookup == nil {
		return
	}

	info, err := s.certLookup.LookupCert(ctx, certNum)
	if err != nil {
		if s.logger != nil {
			s.logger.Debug(ctx, "cert enrichment failed",
				observability.String("cert", certNum),
				observability.Err(err))
		}
		return
	}
	if info == nil {
		return
	}

	purchase, lookupErr := s.repo.GetPurchaseByCertNumber(ctx, "PSA", certNum)
	if lookupErr != nil || purchase == nil {
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
		resolved := resolvePSACategory(info.Category)
		if !isGenericSetName(resolved) {
			setName = resolved
		}
	}

	if info.Variety != "" && !strings.Contains(strings.ToUpper(cardName), strings.ToUpper(info.Variety)) {
		cardName = cardName + " " + info.Variety
	}

	if err := s.repo.UpdatePurchaseCardMetadata(ctx, purchase.ID, cardName, cardNumber, setName); err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "cert enrichment: failed to update purchase",
				observability.String("cert", certNum),
				observability.Err(err))
		}
		return
	}

	if info.Year != "" && purchase.CardYear == "" {
		if err := s.repo.UpdatePurchaseCardYear(ctx, purchase.ID, info.Year); err != nil && s.logger != nil {
			s.logger.Warn(ctx, "cert enrichment: failed to update card year",
				observability.String("cert", certNum),
				observability.Err(err))
		}
	}

	// Persist grade from cert if it differs from the purchase
	grade := info.Grade
	if grade == 0 {
		grade = purchase.GradeValue
	}
	if info.Grade != 0 && info.Grade != purchase.GradeValue {
		if err := s.repo.UpdatePurchaseGrade(ctx, purchase.ID, info.Grade); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "cert enrichment: failed to update grade",
					observability.String("cert", certNum),
					observability.Err(err))
			}
			// Fallback to the persisted grade so the snapshot matches the DB record.
			grade = purchase.GradeValue
		}
	}

	card := CardIdentity{CardName: cardName, CardNumber: cardNumber, SetName: setName, PSAListingTitle: purchase.PSAListingTitle}
	if s.recaptureMarketSnapshot(ctx, purchase.ID, card, grade, purchase.CLValueCents) {
		// Clear pending status so ProcessPendingSnapshots doesn't re-process this purchase
		if purchase.SnapshotStatus != SnapshotStatusNone {
			if err := s.repo.UpdatePurchaseSnapshotStatus(ctx, purchase.ID, SnapshotStatusNone, 0); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "cert enrichment: failed to clear snapshot status",
					observability.String("cert", certNum),
					observability.Err(err))
			}
		}
	}
}
