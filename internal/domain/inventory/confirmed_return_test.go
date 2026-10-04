package inventory_test

import (
	"encoding/json"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnCanonicalOrder(t *testing.T) {
	for _, tt := range []struct {
		order string
		valid bool
	}{{"ext-443", true}, {"ext-848", true}, {"ext-0", false}, {"ext-01", false}, {"ext--1", false}, {"ext-+1", false}, {"ext-9223372036854775808", false}, {"848", false}} {
		t.Run(tt.order, func(t *testing.T) { require.Equal(t, tt.valid, inventory.IsCanonicalExternalOrder(tt.order)) })
	}
}
func TestConfirmedReturnDTOExcludesKeys(t *testing.T) {
	state := inventory.ConfirmedReturnState{Operation: &inventory.ConfirmedReturnEpisode{ID: "op", Key: "private-return-key", ObservedReceipt: &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848, Restored: false}}, Sale: &inventory.Sale{ID: "sale", DHIdempotencyKey: "private-sale-key"}, PrecedingAttempt: &inventory.DHMutationAttempt{ID: "attempt", Key: "private-attempt-key", PayloadIdentity: "private-request"}}
	b, err := json.Marshal(state)
	require.NoError(t, err)
	text := string(b)
	require.NotContains(t, text, "private-")
	require.Contains(t, text, `"observedReceipt":{"dhInventoryId":364577,"itemStatus":"in_stock","externalSaleId":848,"restored":false}`)
	require.Contains(t, text, `"expectedSaleId":null`)
}
