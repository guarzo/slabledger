package dhlisting

import (
	"context"
	"fmt"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHInventorySnapshotFetcher returns the authoritative set of DH inventory IDs
// currently present on DoubleHolo. Implementations must paginate internally
// and return an error on any page failure — reconciliation relies on a
// complete snapshot to avoid resetting healthy items.
type DHInventorySnapshotFetcher interface {
	FetchAllInventoryIDs(ctx context.Context) (map[int]struct{}, error)
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

// ReconcileResult summarises a reconciliation run.
type ReconcileResult struct {
	Scanned     int      // DH-linked unsold purchases examined (DHInventoryID != 0)
	MissingOnDH int      // purchases whose DHInventoryID was not present on DH
	Reset       int      // purchases successfully flipped to pending
	Errors      []string // per-item reset errors (purchaseID: message)
	ResetIDs    []string // purchase IDs that were reset
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
	dhIDs, err := s.fetcher.FetchAllInventoryIDs(ctx)
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
		if _, ok := dhIDs[p.DHInventoryID]; ok {
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
		observability.Int("errors", len(result.Errors)),
		observability.Int("dhInventoryCount", len(dhIDs)))

	return result, nil
}
