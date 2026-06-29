package dhlisting

import (
	"context"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHInventorySnapshotFetcher returns the authoritative DH inventory currently
// present on DoubleHolo, keyed by inventory ID with the DH-side status as the
// value (in_stock/listed/sold). Implementations must paginate internally and
// return an error on any page failure — reconciliation relies on a complete
// snapshot to avoid resetting healthy items.
type DHInventorySnapshotFetcher interface {
	FetchAllInventory(ctx context.Context) (map[int]string, error)
}

// DHReconcilePurchaseLister lists unsold purchases for reconciliation.
type DHReconcilePurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error)
}

// DHReconcileResetter atomically clears DH inventory linkage on a purchase.
//
// ResetDHFieldsForRepush is used by the inline listing service when a stale
// inventory ID is detected mid-push. ResetDHFieldsForRepushDueToDelete is used
// by the reconciler when a purchase has been unlisted on DH (inventory ID
// missing from the authoritative snapshot); it additionally stamps
// dh_unlisted_detected_at so the UI can surface the state.
type DHReconcileResetter interface {
	ResetDHFieldsForRepush(ctx context.Context, purchaseID string) error
	ResetDHFieldsForRepushDueToDelete(ctx context.Context, purchaseID string) error
}

// DHStatusRepairer corrects a local dh_status that disagrees with DH's
// authoritative snapshot — notably the "" we persist for an undocumented
// psa_import status like "skipped", which the checkpoint-gated inventory poll
// would otherwise never backfill for an unchanged item. Kept separate from
// DHReconcileResetter so the inline listing service (which only needs reset)
// isn't forced to implement it.
type DHStatusRepairer interface {
	UpdatePurchaseDHStatus(ctx context.Context, purchaseID, status string) error
}

// ReconcileResult summarises a reconciliation run.
type ReconcileResult struct {
	Scanned        int      `json:"scanned"`        // DH-linked unsold purchases examined (DHInventoryID != 0)
	MissingOnDH    int      `json:"missingOnDH"`    // purchases whose DHInventoryID was not present on DH
	Reset          int      `json:"reset"`          // purchases successfully flipped to pending
	StatusRepaired int      `json:"statusRepaired"` // purchases whose local dh_status was corrected from DH's snapshot
	Errors         []string `json:"errors"`         // per-item reset errors (purchaseID: message)
	ResetIDs       []string `json:"resetIds"`       // purchase IDs that were reset
}

// Reconciler detects drift between local DH linkage and the authoritative DH
// inventory, resetting local purchases so the push scheduler re-enrolls them.
type Reconciler interface {
	Reconcile(ctx context.Context) (ReconcileResult, error)
}

// reconcileService implements Reconciler.
type reconcileService struct {
	fetcher   DHInventorySnapshotFetcher
	purchases DHReconcilePurchaseLister
	resetter  DHReconcileResetter
	repairer  DHStatusRepairer  // optional: when set, repairs local dh_status drift from DH's snapshot
	eventRec  dhevents.Recorder // optional: when set, every reset emits a TypeUnlisted event
	logger    observability.Logger
}

// ReconcilerOption configures optional dependencies on a Reconciler.
type ReconcilerOption func(*reconcileService)

// WithReconcileEventRecorder injects an event recorder so each reset emits a
// dh_state_events row with source='dh_reconcile'. Without it, runs are
// invisible outside the logs.
func WithReconcileEventRecorder(r dhevents.Recorder) ReconcilerOption {
	return func(s *reconcileService) { s.eventRec = r }
}

// WithReconcileStatusRepairer enables repair of local dh_status drift against
// DH's authoritative snapshot. Without it, the reconciler only detects
// unlisted-on-DH items and never corrects a stale/empty local status.
func WithReconcileStatusRepairer(r DHStatusRepairer) ReconcilerOption {
	return func(s *reconcileService) { s.repairer = r }
}

// NewReconciler constructs a Reconciler. All positional dependencies are required.
func NewReconciler(
	fetcher DHInventorySnapshotFetcher,
	purchases DHReconcilePurchaseLister,
	resetter DHReconcileResetter,
	logger observability.Logger,
	opts ...ReconcilerOption,
) (Reconciler, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("fetcher is required")
	}
	if purchases == nil {
		return nil, fmt.Errorf("purchases lister is required")
	}
	if resetter == nil {
		return nil, fmt.Errorf("resetter is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	s := &reconcileService{
		fetcher:   fetcher,
		purchases: purchases,
		resetter:  resetter,
		logger:    logger,
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// Reconcile fetches the DH inventory snapshot, compares it against local
// unsold purchases, and resets any local entries whose DHInventoryID is no
// longer present on DH. Fails closed: any error while fetching the snapshot
// aborts the run with zero resets, so a partial snapshot never flips healthy
// items back to pending.
func (s *reconcileService) Reconcile(ctx context.Context) (ReconcileResult, error) {
	dhInv, err := s.fetcher.FetchAllInventory(ctx)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("fetch DH snapshot: %w", err)
	}

	purchases, err := s.purchases.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("list unsold purchases: %w", err)
	}

	result := ReconcileResult{}
	for _, p := range purchases {
		if p.DHInventoryID == 0 {
			continue
		}
		result.Scanned++
		dhStatus, onDH := dhInv[p.DHInventoryID]
		if onDH {
			// Healthy linkage. Repair the local dh_status if it disagrees with
			// DH's authoritative value. This is the durable backfill for rows
			// we deliberately persisted as "" (an undocumented psa_import status
			// like "skipped"): the checkpoint-gated inventory poll never
			// re-fetches an unchanged item, but this full sweep always sees it.
			// Only adopt real inventory statuses, and never overwrite a local
			// "sold" (owned by the orders poll, which also creates the sale row).
			if s.repairer != nil {
				normalized := inventory.NormalizeDHStatus(dhStatus)
				if normalized != "" && normalized != string(p.DHStatus) && string(p.DHStatus) != inventory.DHStatusSold {
					if err := s.repairer.UpdatePurchaseDHStatus(ctx, p.ID, normalized); err != nil {
						s.logger.Warn(ctx, "dh reconcile: status repair failed",
							observability.String("purchaseID", p.ID),
							observability.Int("dhInventoryID", p.DHInventoryID),
							observability.String("from", string(p.DHStatus)),
							observability.String("to", normalized),
							observability.Err(err))
					} else {
						result.StatusRepaired++
					}
				}
			}
			continue
		}
		result.MissingOnDH++

		if err := s.resetter.ResetDHFieldsForRepushDueToDelete(ctx, p.ID); err != nil {
			s.logger.Warn(ctx, "dh reconcile: reset failed",
				observability.String("purchaseID", p.ID),
				observability.Int("dhInventoryID", p.DHInventoryID),
				observability.Err(err))
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", p.ID, err))
			continue
		}
		result.Reset++
		result.ResetIDs = append(result.ResetIDs, p.ID)

		if s.eventRec != nil {
			if recErr := s.eventRec.Record(ctx, dhevents.Event{
				PurchaseID:    p.ID,
				CertNumber:    p.CertNumber,
				Type:          dhevents.TypeUnlisted,
				PrevDHStatus:  string(p.DHStatus),
				DHInventoryID: p.DHInventoryID,
				DHCardID:      p.DHCardID,
				Source:        dhevents.SourceDHReconcile,
				Notes:         "inventory ID missing from DH snapshot",
			}); recErr != nil {
				s.logger.Warn(ctx, "dh reconcile: record event failed",
					observability.String("purchaseID", p.ID),
					observability.Err(recErr))
			}
		}
	}

	sort.Strings(result.ResetIDs)

	s.logger.Info(ctx, "dh reconcile completed",
		observability.Int("scanned", result.Scanned),
		observability.Int("missingOnDH", result.MissingOnDH),
		observability.Int("reset", result.Reset),
		observability.Int("statusRepaired", result.StatusRepaired),
		observability.Int("errors", len(result.Errors)),
		observability.Int("dhInventoryCount", len(dhInv)))

	return result, nil
}
