package inventory_test

import (
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// recentDate returns a YYYY-MM-DD string for N days ago, keeping tests stable.
func recentDate(daysAgo int) string {
	return time.Now().AddDate(0, 0, -daysAgo).Format("2006-01-02")
}

func TestComputeInventorySignals(t *testing.T) {
	tests := []struct {
		name     string
		item     inventory.AgingItem
		isCrack  bool
		expected inventory.InventorySignals
	}{
		{
			name: "profit capture declining — recent sold, profitable, trend down",
			item: inventory.AgingItem{
				DaysHeld: 10,
				Purchase: inventory.Purchase{
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &inventory.MarketSnapshot{
					LastSoldCents: 8000,
					LastSoldDate:  recentDate(5),
					SalesLast30d:  3,
					Trend30d:      -0.08,
					MedianCents:   7500,
				},
			},
			expected: inventory.InventorySignals{ProfitCaptureDeclining: true},
		},
		{
			name: "profit capture spike — price up >10%, recent sales, profitable",
			item: inventory.AgingItem{
				DaysHeld: 10,
				Purchase: inventory.Purchase{
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &inventory.MarketSnapshot{
					LastSoldCents: 9000,
					SalesLast30d:  4,
					Trend30d:      0.15,
					MedianCents:   8500,
				},
			},
			expected: inventory.InventorySignals{ProfitCaptureSpike: true},
		},
		{
			name: "crack candidate from lookup",
			item: inventory.AgingItem{
				DaysHeld: 5,
				Purchase: inventory.Purchase{BuyCostCents: 5000},
			},
			isCrack:  true,
			expected: inventory.InventorySignals{CrackCandidate: true},
		},
		{
			name: "stale listing — held >14 days",
			item: inventory.AgingItem{
				DaysHeld: 20,
				Purchase: inventory.Purchase{BuyCostCents: 5000},
			},
			expected: inventory.InventorySignals{StaleListing: true},
		},
		{
			name: "deep stale — held >30 days",
			item: inventory.AgingItem{
				DaysHeld: 35,
				Purchase: inventory.Purchase{BuyCostCents: 5000},
			},
			expected: inventory.InventorySignals{StaleListing: true, DeepStale: true},
		},
		{
			name: "cut loss — deep stale + declining trend",
			item: inventory.AgingItem{
				DaysHeld: 40,
				Purchase: inventory.Purchase{
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &inventory.MarketSnapshot{
					Trend30d:    -0.05,
					MedianCents: 4000,
				},
			},
			expected: inventory.InventorySignals{StaleListing: true, DeepStale: true, CutLoss: true},
		},
		{
			name: "cut loss — deep stale + negative unrealized PL",
			item: inventory.AgingItem{
				DaysHeld: 40,
				Purchase: inventory.Purchase{
					BuyCostCents:        8000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &inventory.MarketSnapshot{
					MedianCents: 5000,
				},
			},
			expected: inventory.InventorySignals{StaleListing: true, DeepStale: true, CutLoss: true},
		},
		{
			name: "no signals — fresh card, no market data",
			item: inventory.AgingItem{
				DaysHeld: 3,
				Purchase: inventory.Purchase{BuyCostCents: 5000},
			},
			expected: inventory.InventorySignals{},
		},
		{
			name: "no signals — healthy card, good market, recent",
			item: inventory.AgingItem{
				DaysHeld: 10,
				Purchase: inventory.Purchase{
					BuyCostCents:        5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &inventory.MarketSnapshot{
					LastSoldCents: 7000,
					SalesLast30d:  2,
					Trend30d:      0.02,
					MedianCents:   6800,
				},
			},
			expected: inventory.InventorySignals{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inventory.ComputeInventorySignals(&tt.item, tt.isCrack)
			if got != tt.expected {
				t.Errorf("ComputeInventorySignals() = %+v, want %+v", got, tt.expected)
			}
		})
	}
}
