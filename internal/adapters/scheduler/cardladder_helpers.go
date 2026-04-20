package scheduler

import (
	"context"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CL failure reason tags. Short, stable strings used by the /failures admin endpoint.
const (
	CLReasonNoCert            = "no_cert"             // Purchase has no cert number to look up.
	CLReasonCertResolveFailed = "cert_resolve_failed" // BuildCollectionCard returned no gemRateID/condition.
	CLReasonNoValue           = "no_value"            // Resolved to gemRateID but catalog had no value.
	CLReasonAPIError          = "api_error"           // Transient CL API error during cert resolution.
)

// recordCLError persists a failure reason (or clears it when reason=="") on a
// purchase. Never fails the refresh loop — diagnostics are best-effort — but
// the admin UI depends on these rows, so a persistence failure is logged at
// Warn level so operators see it.
func (s *CardLadderRefreshScheduler) recordCLError(ctx context.Context, purchaseID, reason string) {
	var reasonAt string
	if reason != "" {
		reasonAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := s.valueUpdater.UpdatePurchaseCLError(ctx, purchaseID, reason, reasonAt); err != nil {
		s.logger.Warn(ctx, "CL refresh: failed to persist error reason",
			observability.String("purchaseId", purchaseID),
			observability.String("reason", reason),
			observability.Err(err))
	}
}

// filterUnmappedCerts returns the subset of purchases that have a cert number
// and are not already in the provided set of mapped certs. This is the pure
// filter logic used by pushNewCards before it makes any API calls.
func filterUnmappedCerts(purchases []inventory.Purchase, existingMappings []postgres.CLCardMapping) []*inventory.Purchase {
	mappedCerts := make(map[string]bool, len(existingMappings))
	for _, m := range existingMappings {
		// A mapping counts as "pushed to CL remote" only when it has a
		// Firestore document ID — cert-first pricing mappings have an empty
		// CLCollectionCardID and still need to be pushed for UI hygiene.
		if m.CLCollectionCardID != "" {
			mappedCerts[m.SlabSerial] = true
		}
	}
	var result []*inventory.Purchase
	for i := range purchases {
		p := &purchases[i]
		if p.CertNumber == "" || mappedCerts[p.CertNumber] {
			continue
		}
		result = append(result, p)
	}
	return result
}

// identifySoldMappings returns the subset of existingMappings whose cert
// number is no longer present in unsoldPurchases. This is the pure filter
// logic used by removeSoldCards before it makes any API calls.
func identifySoldMappings(unsoldPurchases []inventory.Purchase, existingMappings []postgres.CLCardMapping) []postgres.CLCardMapping {
	unsoldCerts := make(map[string]bool, len(unsoldPurchases))
	for _, p := range unsoldPurchases {
		if p.CertNumber != "" {
			unsoldCerts[p.CertNumber] = true
		}
	}
	var result []postgres.CLCardMapping
	for _, m := range existingMappings {
		if !unsoldCerts[m.SlabSerial] {
			result = append(result, m)
		}
	}
	return result
}

// firestoreConditionFor converts the display-form condition stored in
// cl_card_mappings.cl_condition (e.g. "PSA 10", "PSA 8.5") into the Firestore
// form ("g10", "g8_5") expected by CardEstimate and other Firebase callables.
// Unrecognised inputs fall through unchanged so upstream code can still
// surface them.
func firestoreConditionFor(displayCondition string) string {
	d := strings.TrimSpace(displayCondition)
	upper := strings.ToUpper(d)
	if strings.HasPrefix(upper, "PSA ") {
		grade := strings.TrimSpace(d[len("PSA "):])
		return "g" + strings.ReplaceAll(grade, ".", "_")
	}
	return displayCondition
}

// compKey is a unique (gemRateID, condition) pair used for sales-comp dedup.
type compKey struct {
	gemRateID string
	condition string
}

// dedupGemRateConditionPairs returns the unique (gemRateID, condition) pairs
// from mappings that have both fields populated. This is the pure dedup logic
// used by refreshSalesComps to avoid redundant API calls.
func dedupGemRateConditionPairs(mappings []postgres.CLCardMapping) []compKey {
	seen := make(map[compKey]bool, len(mappings))
	for _, m := range mappings {
		if m.CLGemRateID == "" || m.CLCondition == "" {
			continue
		}
		seen[compKey{m.CLGemRateID, m.CLCondition}] = true
	}
	result := make([]compKey, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}
	return result
}
