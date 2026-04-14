package portfolio

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestComputeInHandStatsByCampaign(t *testing.T) {
	received := "2026-04-01"

	tests := []struct {
		name string
		data []inventory.PurchaseWithSale
		want map[string][4]int
	}{
		{
			name: "empty input returns empty map",
			data: nil,
			want: map[string][4]int{},
		},
		{
			name: "sold purchases are skipped",
			data: []inventory.PurchaseWithSale{
				{
					Purchase: inventory.Purchase{
						CampaignID:          "c1",
						BuyCostCents:        10000,
						PSASourcingFeeCents: 300,
						ReceivedAt:          &received,
					},
					Sale: &inventory.Sale{SalePriceCents: 15000},
				},
			},
			want: map[string][4]int{},
		},
		{
			name: "received unsold counts as in-hand",
			data: []inventory.PurchaseWithSale{
				{
					Purchase: inventory.Purchase{
						CampaignID:          "c1",
						BuyCostCents:        10000,
						PSASourcingFeeCents: 300,
						ReceivedAt:          &received,
					},
				},
				{
					Purchase: inventory.Purchase{
						CampaignID:          "c1",
						BuyCostCents:        5000,
						PSASourcingFeeCents: 300,
						ReceivedAt:          &received,
					},
				},
			},
			want: map[string][4]int{
				"c1": {2, 15600, 0, 0},
			},
		},
		{
			name: "nil ReceivedAt counts as in-transit",
			data: []inventory.PurchaseWithSale{
				{
					Purchase: inventory.Purchase{
						CampaignID:          "c1",
						BuyCostCents:        8000,
						PSASourcingFeeCents: 300,
						ReceivedAt:          nil,
					},
				},
			},
			want: map[string][4]int{
				"c1": {0, 0, 1, 8300},
			},
		},
		{
			name: "mixed buckets across multiple campaigns",
			data: []inventory.PurchaseWithSale{
				{
					Purchase: inventory.Purchase{
						CampaignID:          "c1",
						BuyCostCents:        10000,
						PSASourcingFeeCents: 300,
						ReceivedAt:          &received,
					},
				},
				{
					Purchase: inventory.Purchase{
						CampaignID:          "c1",
						BuyCostCents:        7000,
						PSASourcingFeeCents: 300,
						ReceivedAt:          nil,
					},
				},
				{
					Purchase: inventory.Purchase{
						CampaignID:          "c2",
						BuyCostCents:        20000,
						PSASourcingFeeCents: 300,
						ReceivedAt:          &received,
					},
					Sale: &inventory.Sale{SalePriceCents: 25000}, // sold, skipped
				},
				{
					Purchase: inventory.Purchase{
						CampaignID:          "c2",
						BuyCostCents:        3000,
						PSASourcingFeeCents: 300,
						ReceivedAt:          nil,
					},
				},
			},
			want: map[string][4]int{
				"c1": {1, 10300, 1, 7300},
				"c2": {0, 0, 1, 3300},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeInHandStatsByCampaign(tc.data)
			if len(got) != len(tc.want) {
				t.Fatalf("result len = %d, want %d (got=%v)", len(got), len(tc.want), got)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("bucket[%q] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}
