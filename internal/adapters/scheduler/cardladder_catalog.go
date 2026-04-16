package scheduler

import (
	"context"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Tunables for catalog batching. Chosen so the encoded URL stays under ~2.5KB
// (each gemRateID is 40 hex chars + comma) and a single batch rarely needs
// more than a handful of pages.
const (
	catalogBatchSize     = 25
	catalogPageSize      = 200
	catalogMaxPagesBatch = 20
)

// fetchCatalogValues looks up live market values from the CL `cards` index
// for the given gemRateIDs and returns a map keyed by (gemRateID|condition).
//
// The `cards` index is the same data source CL's website renders as "CL Value",
// so its `currentValue` reflects the freshly computed market value. The
// `collectioncards` index that runOnce iterates stores a snapshot that CL
// does not refresh — relying on it causes prices to freeze after ingest. We
// use the catalog value here and fall back to the stale collection value only
// when the catalog has no entry for the (gemRateID, condition) pair.
//
// Failures of individual batches are logged and skipped; the loop does not
// abort. The returned map may therefore be partial.
func (s *CardLadderRefreshScheduler) fetchCatalogValues(ctx context.Context, client *cardladder.Client, gemRateIDs []string) map[string]float64 {
	result := make(map[string]float64, len(gemRateIDs)*4)
	if client == nil || len(gemRateIDs) == 0 {
		return result
	}

	for i := 0; i < len(gemRateIDs); i += catalogBatchSize {
		if ctx.Err() != nil {
			break
		}
		end := min(i+catalogBatchSize, len(gemRateIDs))
		chunk := gemRateIDs[i:end]
		filters := map[string]string{
			"gemRateId": strings.Join(chunk, ","),
		}

		for page := range catalogMaxPagesBatch {
			if ctx.Err() != nil {
				break
			}
			resp, err := client.FetchCardCatalog(ctx, "", filters, page, catalogPageSize)
			if err != nil {
				s.logger.Warn(ctx, "CL refresh: catalog batch fetch failed",
					observability.Int("batchStart", i),
					observability.Int("batchSize", len(chunk)),
					observability.Int("page", page),
					observability.Err(err))
				break
			}
			for _, h := range resp.Hits {
				if h.CurrentValue <= 0 || h.GemRateID == "" || h.Condition == "" {
					continue
				}
				result[catalogValueKey(h.GemRateID, h.Condition)] = h.CurrentValue
			}
			if len(resp.Hits) < catalogPageSize {
				break
			}
		}
	}
	return result
}
