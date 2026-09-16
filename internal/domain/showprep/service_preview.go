package showprep

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

const MaxPreviewPriceCents = 9007199254740991

// PricePreview is a hypothetical cached assessment, never a list-mutation token.
type PricePreview struct {
	PurchaseID          string              `json:"purchaseId"`
	CurrentPriceCents   int                 `json:"currentPriceCents"`
	TrialPriceCents     int                 `json:"trialPriceCents"`
	Status              Status              `json:"status"`
	Reason              string              `json:"reason"`
	EvidenceNeedsReview bool                `json:"evidenceNeedsReview"`
	EvidenceReason      string              `json:"evidenceReason"`
	EvidenceVersion     string              `json:"evidenceVersion"`
	PolicyVersion       string              `json:"policyVersion"`
	Recent              RecentPriceEvidence `json:"recent"`
}

func (s *Service) Preview(ctx context.Context, purchaseID string, trialPriceCents int) (PricePreview, error) {
	parsed, err := uuid.Parse(purchaseID)
	if err != nil || parsed.String() != purchaseID || trialPriceCents < 1 || trialPriceCents > MaxPreviewPriceCents {
		return PricePreview{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return PricePreview{}, err
	}
	purchases, err := s.store.ReadPurchases(ctx, []string{purchaseID})
	if canceled := ctx.Err(); canceled != nil {
		return PricePreview{}, canceled
	}
	if err != nil {
		return PricePreview{}, fmt.Errorf("preview purchase %s: %w", purchaseID, err)
	}
	purchase, ok := purchases[purchaseID]
	if !ok {
		return PricePreview{}, ErrNotFound
	}
	identity := purchase.Identity()
	snapshots, err := s.store.ReadSnapshots(ctx, []Identity{identity})
	if canceled := ctx.Err(); canceled != nil {
		return PricePreview{}, canceled
	}
	if err != nil {
		return PricePreview{}, fmt.Errorf("preview snapshot for purchase %s: %w", purchaseID, err)
	}
	// Do not use Evidence/evaluateBatch: their hold observation can write state.
	currentPrice := purchase.LocalPriceCents
	trial := purchase
	trial.LocalPriceCents = trialPriceCents
	e := Evaluate(trial, snapshots[identity], s.now())
	return PricePreview{PurchaseID: purchase.ID, CurrentPriceCents: currentPrice,
		TrialPriceCents: trialPriceCents, Status: e.Status, Reason: e.Reason,
		EvidenceNeedsReview: e.EvidenceNeedsReview, EvidenceReason: e.EvidenceReason,
		EvidenceVersion: e.EvidenceVersion, PolicyVersion: e.PolicyVersion, Recent: e.Recent}, nil
}
