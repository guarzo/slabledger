package scheduler

import (
	"context"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CL failure reason tags. Short, stable strings used by the /failures admin endpoint.
const (
	CLReasonNoImageMatch = "no_image_match"
	CLReasonNoCertMatch  = "no_cert_match"
	CLReasonNoValue      = "no_value"
	CLReasonAPIError     = "api_error"
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

// extractGradeValue parses "PSA 9", "PSA 9.5", or "g9" → numeric grade value.
func extractGradeValue(condition string) float64 {
	if m := gradeDigitsRe.FindString(condition); m != "" {
		v, _ := strconv.ParseFloat(m, 64)
		return v
	}
	return 0
}

// filterUnmappedCerts returns the subset of purchases that have a cert number
// and are not already in the provided set of mapped certs. This is the pure
// filter logic used by pushNewCards before it makes any API calls.
func filterUnmappedCerts(purchases []inventory.Purchase, existingMappings []sqlite.CLCardMapping) []*inventory.Purchase {
	mappedCerts := make(map[string]bool, len(existingMappings))
	for _, m := range existingMappings {
		mappedCerts[m.SlabSerial] = true
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
func identifySoldMappings(unsoldPurchases []inventory.Purchase, existingMappings []sqlite.CLCardMapping) []sqlite.CLCardMapping {
	unsoldCerts := make(map[string]bool, len(unsoldPurchases))
	for _, p := range unsoldPurchases {
		if p.CertNumber != "" {
			unsoldCerts[p.CertNumber] = true
		}
	}
	var result []sqlite.CLCardMapping
	for _, m := range existingMappings {
		if !unsoldCerts[m.SlabSerial] {
			result = append(result, m)
		}
	}
	return result
}

// compKey is a unique (gemRateID, condition) pair used for sales-comp dedup.
type compKey struct {
	gemRateID string
	condition string
}

// dedupGemRateConditionPairs returns the unique (gemRateID, condition) pairs
// from mappings that have both fields populated. This is the pure dedup logic
// used by refreshSalesComps to avoid redundant API calls.
func dedupGemRateConditionPairs(mappings []sqlite.CLCardMapping) []compKey {
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
