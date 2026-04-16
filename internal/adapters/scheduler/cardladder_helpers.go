package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CL failure reason tags. Short, stable strings used by the /failures admin endpoint.
const (
	CLReasonNoImageMatch     = "no_image_match"
	CLReasonNoCertMatch      = "no_cert_match"
	CLReasonNoValue          = "no_value"
	CLReasonAPIError         = "api_error"
	CLReasonCatalogFallback  = "catalog_fallback"
)

// fetchCatalogFallbackValue queries the CL cards catalog (grade-specific) for a
// non-zero market value when the user's collectioncards entry reports $0. The
// collection-side valuation can sit at zero indefinitely for cards CL's batch
// pipeline lacks sales data on (niche sets, lower grades, new releases); the
// catalog is CL's market-wide view and often has a usable value.
//
// Returns 0 if the catalog also has no usable value or if the API call fails.
// Requires gemRateID and a PSA grade on the purchase — returns 0 otherwise.
func (s *CardLadderRefreshScheduler) fetchCatalogFallbackValue(ctx context.Context, client *cardladder.Client, purchase *inventory.Purchase) int {
	if purchase.GemRateID == "" || purchase.GradeValue <= 0 {
		return 0
	}

	grader := purchase.Grader
	if grader == "" {
		grader = "PSA"
	}
	filters := map[string]string{
		"gemRateId":      purchase.GemRateID,
		"condition":      fmt.Sprintf("%s %s", grader, mathutil.FormatGrade(purchase.GradeValue)),
		"gradingCompany": strings.ToLower(grader),
	}

	resp, err := client.FetchCardCatalog(ctx, "", filters, 0, 1)
	if err != nil {
		s.logger.Warn(ctx, "CL refresh: catalog fallback fetch failed",
			observability.String("cert", purchase.CertNumber),
			observability.String("gemRateId", purchase.GemRateID),
			observability.Err(err))
		return 0
	}
	if len(resp.Hits) == 0 {
		return 0
	}

	hit := resp.Hits[0]
	// Prefer CurrentValue (what CL displays); fall back to MarketValue if CurrentValue is also zero.
	value := hit.CurrentValue
	if value <= 0 {
		value = hit.MarketValue
	}
	if value <= 0 {
		return 0
	}
	return mathutil.ToCentsInt(value)
}

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

// catalogValueKey builds the lookup key used in the catalog value map.
// Condition here is the grade-string form returned by the CL `cards` index
// (e.g. "PSA 9"), NOT the Firestore `g9` form.
func catalogValueKey(gemRateID, condition string) string {
	return gemRateID + "|" + condition
}

// collectGemRateIDs returns the union of gemRateIDs found in firestore data
// and existing mappings, with blanks removed. Order is not guaranteed.
func collectGemRateIDs(firestoreData map[string]cardladder.FirestoreCardData, mappings []sqlite.CLCardMapping) []string {
	seen := make(map[string]struct{}, len(firestoreData)+len(mappings))
	for _, fd := range firestoreData {
		if fd.GemRateID != "" {
			seen[fd.GemRateID] = struct{}{}
		}
	}
	for _, m := range mappings {
		if m.CLGemRateID != "" {
			seen[m.CLGemRateID] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

// pickCLValue returns the authoritative CL value for a matched card. It
// prefers the CL `cards` catalog value (live market value), falling back to
// the collectioncards-index value when the catalog has no entry for this
// (gemRateID, condition) pair or the catalog value is non-positive.
//
// The collection index stores a snapshot that CL does not refresh, so relying
// on it alone causes prices to freeze shortly after a card is added.
func pickCLValue(catalog map[string]float64, gemRateID, condition string, collectionValue float64) float64 {
	if gemRateID != "" && condition != "" {
		if v, ok := catalog[catalogValueKey(gemRateID, condition)]; ok && v > 0 {
			return v
		}
	}
	return collectionValue
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
