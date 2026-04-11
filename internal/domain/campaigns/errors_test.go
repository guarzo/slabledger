package campaigns_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

func TestSentinelErrors(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrCampaignNotFound", campaigns.ErrCampaignNotFound},
		{"ErrPurchaseNotFound", campaigns.ErrPurchaseNotFound},
		{"ErrSaleNotFound", campaigns.ErrSaleNotFound},
		{"ErrDuplicateCertNumber", campaigns.ErrDuplicateCertNumber},
		{"ErrDuplicateSale", campaigns.ErrDuplicateSale},
		{"ErrInvoiceNotFound", campaigns.ErrInvoiceNotFound},
		{"ErrRevocationTooSoon", campaigns.ErrRevocationTooSoon},
		{"ErrRevocationFlagNotFound", campaigns.ErrRevocationFlagNotFound},
		{"ErrNoAISuggestion", campaigns.ErrNoAISuggestion},
		{"ErrPriceFlagNotFound", campaigns.ErrPriceFlagNotFound},
		{"ErrCertNotFound", campaigns.ErrCertNotFound},
		{"ErrPendingItemNotFound", campaigns.ErrPendingItemNotFound},
		{"ErrInvalidCashflowConfig", campaigns.ErrInvalidCashflowConfig},
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
			if tc.name != "ErrCampaignNotFound" && errors.Is(wrapped, campaigns.ErrCampaignNotFound) {
				t.Fatalf("%s: errors.Is(wrapped, ErrCampaignNotFound) unexpectedly true", tc.name)
			}
		})
	}
}
