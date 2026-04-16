package inventory

import (
	"context"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// batchResolveCardIDs resolves a batch of cert numbers to DH card IDs via
// the injected cardIDResolver and persists each result on the matching
// purchase row via UpdatePurchaseDHCardID (targeted update — does not
// touch other DH fields).
//
// Called asynchronously from PSA/CL import paths, so the function has no
// error return: all failures are logged at Warn and the caller continues.
func (s *service) batchResolveCardIDs(ctx context.Context, certs []string) {
	if s.cardIDResolver == nil || len(certs) == 0 {
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
	if len(resolved) == 0 {
		if s.logger != nil {
			s.logger.Debug(ctx, "batch cert→card_id resolution: no matches",
				observability.Int("certs", len(certs)))
		}
		return
	}

	// Look up purchase rows for just the successfully-resolved certs.
	resolvedCerts := make([]string, 0, len(resolved))
	for c := range resolved {
		resolvedCerts = append(resolvedCerts, c)
	}
	purchaseMap, err := s.purchases.GetPurchasesByCertNumbers(ctx, resolvedCerts)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "batch cert→card_id resolution: purchase lookup failed",
				observability.Int("certs", len(resolvedCerts)),
				observability.Err(err))
		}
		return
	}

	persisted := 0
	for cert, cardIDStr := range resolved {
		purchase, ok := purchaseMap[cert]
		if !ok || purchase == nil {
			if s.logger != nil {
				s.logger.Debug(ctx, "batch cert→card_id resolution: no purchase for cert",
					observability.String("cert", cert))
			}
			continue
		}
		cardID, convErr := strconv.Atoi(cardIDStr)
		if convErr != nil || cardID <= 0 {
			if s.logger != nil {
				s.logger.Warn(ctx, "batch cert→card_id resolution: non-numeric card id",
					observability.String("cert", cert),
					observability.String("cardID", cardIDStr))
			}
			continue
		}
		if err := s.purchases.UpdatePurchaseDHCardID(ctx, purchase.ID, cardID); err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "batch cert→card_id resolution: update failed",
					observability.String("purchaseID", purchase.ID),
					observability.Err(err))
			}
			continue
		}
		s.recordEvent(ctx, dhevents.Event{
			PurchaseID: purchase.ID,
			CertNumber: cert,
			Type:       dhevents.TypeCardIDResolved,
			DHCardID:   cardID,
			Source:     dhevents.SourcePSAImport,
		})
		persisted++
	}

	if s.logger != nil {
		s.logger.Info(ctx, "batch cert→card_id resolution complete",
			observability.Int("requested", len(certs)),
			observability.Int("resolved", len(resolved)),
			observability.Int("persisted", persisted))
	}
}
