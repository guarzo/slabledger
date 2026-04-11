package campaigns_test

import (
	"errors"
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
			// errors.Is must match against itself (wrapping compatibility).
			if !errors.Is(tc.err, tc.err) {
				t.Fatalf("%s: errors.Is(err, err) returned false", tc.name)
			}
			// A different sentinel must not match.
			if errors.Is(tc.err, campaigns.ErrCampaignNotFound) && tc.name != "ErrCampaignNotFound" {
				t.Fatalf("%s unexpectedly matches ErrCampaignNotFound", tc.name)
			}
		})
	}
}
