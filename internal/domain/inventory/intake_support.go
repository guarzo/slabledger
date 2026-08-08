package inventory

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
)

// IntakeSupportService is the slice of inventory that file-driven intake needs
// in order to land a parsed row as a purchase: create it, snapshot it, record
// the DH enrollment event, and backfill card IDs.
//
// These are thin exported wrappers over internals rather than new behavior.
// They exist because internal/domain/csvimport owns the CSV half of intake
// (SLA-35) and reaches back through this interface, which keeps the dependency
// pointing one way — csvimport imports inventory, never the reverse.
type IntakeSupportService interface {
	// CreatePurchase is the same method Service exposes; intake needs it
	// for the validation, event recording, and pricing enqueue it wraps around
	// the bare repository write.
	CreatePurchase(ctx context.Context, p *Purchase) error

	// CapturePurchaseSnapshot is captureMarketSnapshot narrowed to a purchase
	// receiver, so the unexported snapshotReceiver interface stays unexported.
	// Best-effort: a failed lookup is logged, not returned.
	CapturePurchaseSnapshot(ctx context.Context, p *Purchase, card CardIdentity, grade float64, clValueCents int)

	// RecordDHEvent records a DH state-transition event. No-op when no recorder
	// is configured.
	RecordDHEvent(ctx context.Context, e dhevents.Event)

	// BatchResolveCardIDs resolves cert numbers to external card IDs. No-op when
	// no resolver is configured.
	BatchResolveCardIDs(ctx context.Context, certs []string)
}

func (s *service) CapturePurchaseSnapshot(ctx context.Context, p *Purchase, card CardIdentity, grade float64, clValueCents int) {
	_, _ = s.captureMarketSnapshot(ctx, p, card, grade, clValueCents)
}

func (s *service) RecordDHEvent(ctx context.Context, e dhevents.Event) {
	s.recordEvent(ctx, e)
}

func (s *service) BatchResolveCardIDs(ctx context.Context, certs []string) {
	s.batchResolveCardIDs(ctx, certs)
}
