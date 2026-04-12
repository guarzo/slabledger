package inventory_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestSentinelErrors(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrCampaignNotFound", inventory.ErrCampaignNotFound},
		{"ErrPurchaseNotFound", inventory.ErrPurchaseNotFound},
		{"ErrSaleNotFound", inventory.ErrSaleNotFound},
		{"ErrDuplicateCertNumber", inventory.ErrDuplicateCertNumber},
		{"ErrDuplicateSale", inventory.ErrDuplicateSale},
		{"ErrInvoiceNotFound", inventory.ErrInvoiceNotFound},
		{"ErrRevocationTooSoon", inventory.ErrRevocationTooSoon},
		{"ErrRevocationFlagNotFound", inventory.ErrRevocationFlagNotFound},
		{"ErrNoAISuggestion", inventory.ErrNoAISuggestion},
		{"ErrPriceFlagNotFound", inventory.ErrPriceFlagNotFound},
		{"ErrCertNotFound", inventory.ErrCertNotFound},
		{"ErrPendingItemNotFound", inventory.ErrPendingItemNotFound},
		{"ErrInvalidCashflowConfig", inventory.ErrInvalidCashflowConfig},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatalf("%s is nil", tc.name)
			}
			// Wrapping the sentinel must still be detectable via errors.Is.
			wrapped := fmt.Errorf("operation failed: %w", tc.err)
			if !errors.Is(wrapped, tc.err) {
				t.Fatalf("%s: errors.Is(wrapped, sentinel) returned false — sentinel not detectable through wrapping", tc.name)
			}
			// A different sentinel must not match through wrapping.
			if tc.name != "ErrCampaignNotFound" && errors.Is(wrapped, inventory.ErrCampaignNotFound) {
				t.Fatalf("%s: errors.Is(wrapped, ErrCampaignNotFound) unexpectedly true", tc.name)
			}
		})
	}
}
