package campaigns_test

import (
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// recentDate returns a YYYY-MM-DD string for N days ago, keeping tests stable.
func recentDate(daysAgo int) string {
	return time.Now().AddDate(0, 0, -daysAgo).Format("2006-01-02")
}

func TestComputeInventorySignals(t *testing.T) {
	tests := []struct {
		name     string
		item     campaigns.AgingItem
		isCrack  bool
		expected campaigns.InventorySignals
	}{
		{
			name: "profit capture declining — recent sold, profitable, trend down",
			item: campaigns.AgingItem{
				DaysHeld: 10,
				Purchase: campaigns.Purchase{
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					LastSoldCents: 8000,
					LastSoldDate:  recentDate(5),
					SalesLast30d:  3,
					Trend30d:      -0.08,
					MedianCents:   7500,
				},
			},
			expected: campaigns.InventorySignals{ProfitCaptureDeclining: true},
		},
		{
			name: "profit capture spike — price up >10%, recent sales, profitable",
			item: campaigns.AgingItem{
				DaysHeld: 10,
				Purchase: campaigns.Purchase{
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					LastSoldCents: 9000,
					SalesLast30d:  4,
					Trend30d:      0.15,
					MedianCents:   8500,
				},
			},
			expected: campaigns.InventorySignals{ProfitCaptureSpike: true},
		},
		{
			name: "crack candidate from lookup",
			item: campaigns.AgingItem{
				DaysHeld: 5,
				Purchase: campaigns.Purchase{BuyCostCents: 5000},
			},
			isCrack:  true,
			expected: campaigns.InventorySignals{CrackCandidate: true},
		},
		{
			name: "stale listing — held >14 days",
			item: campaigns.AgingItem{
				DaysHeld: 20,
				Purchase: campaigns.Purchase{BuyCostCents: 5000},
			},
			expected: campaigns.InventorySignals{StaleListing: true},
		},
		{
			name: "deep stale — held >30 days",
			item: campaigns.AgingItem{
				DaysHeld: 35,
				Purchase: campaigns.Purchase{BuyCostCents: 5000},
			},
			expected: campaigns.InventorySignals{StaleListing: true, DeepStale: true},
		},
		{
			name: "cut loss — deep stale + declining trend",
			item: campaigns.AgingItem{
				DaysHeld: 40,
				Purchase: campaigns.Purchase{
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					Trend30d:    -0.05,
					MedianCents: 4000,
				},
			},
			expected: campaigns.InventorySignals{StaleListing: true, DeepStale: true, CutLoss: true},
		},
		{
			name: "cut loss — deep stale + negative unrealized PL",
			item: campaigns.AgingItem{
				DaysHeld: 40,
				Purchase: campaigns.Purchase{
					BuyCostCents:        8000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					MedianCents: 5000,
				},
			},
			expected: campaigns.InventorySignals{StaleListing: true, DeepStale: true, CutLoss: true},
		},
		{
			name: "no signals — fresh card, no market data",
			item: campaigns.AgingItem{
				DaysHeld: 3,
				Purchase: campaigns.Purchase{BuyCostCents: 5000},
			},
			expected: campaigns.InventorySignals{},
		},
		{
			name: "no signals — healthy card, good market, recent",
			item: campaigns.AgingItem{
				DaysHeld: 10,
				Purchase: campaigns.Purchase{
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					LastSoldCents: 7000,
					SalesLast30d:  2,
					Trend30d:      0.02,
					MedianCents:   6800,
				},
			},
			expected: campaigns.InventorySignals{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := campaigns.ComputeInventorySignals(&tt.item, tt.isCrack)
			if got != tt.expected {
				t.Errorf("ComputeInventorySignals() = %+v, want %+v", got, tt.expected)
			}
		})
	}
}
