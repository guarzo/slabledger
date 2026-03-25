package campaigns

import "testing"

func TestCalculateSaleFee(t *testing.T) {
	campaign := &Campaign{EbayFeePct: 0.1235}

	tests := []struct {
		name           string
		channel        SaleChannel
		salePriceCents int
		wantFee        int
	}{
		{"ebay 100 dollars", SaleChannelEbay, 10000, 1235},
		{"tcgplayer 100 dollars", SaleChannelTCGPlayer, 10000, 1235},
		{"local no fee", SaleChannelLocal, 10000, 0},
		{"other no fee", SaleChannelOther, 10000, 0},
		{"ebay 500 dollars", SaleChannelEbay, 50000, 6175},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateSaleFee(tt.channel, tt.salePriceCents, campaign)
			if got != tt.wantFee {
				t.Errorf("CalculateSaleFee(%s, %d) = %d, want %d", tt.channel, tt.salePriceCents, got, tt.wantFee)
			}
		})
	}
}

func TestCalculateSaleFee_DefaultEbayPct(t *testing.T) {
	// When EbayFeePct is 0, should use default 12.35%
	campaign := &Campaign{EbayFeePct: 0}
	got := CalculateSaleFee(SaleChannelEbay, 10000, campaign)
	if got != 1235 {
		t.Errorf("CalculateSaleFee with default fee = %d, want 1235", got)
	}
}

func TestCalculateNetProfit(t *testing.T) {
	// Sale at $750, bought at $500, $3 sourcing fee, $92.63 eBay fee
	net := CalculateNetProfit(75000, 50000, 300, 9263)
	want := 75000 - 50000 - 300 - 9263 // = 15437 ($154.37)
	if net != want {
		t.Errorf("CalculateNetProfit = %d, want %d", net, want)
	}
}

func TestCalculateNetProfit_LocalSale(t *testing.T) {
	// GameStop: sell at 90% CL ($900 on $1000 CL), bought at $800, $3 sourcing, no fees
	net := CalculateNetProfit(90000, 80000, 300, 0)
	want := 90000 - 80000 - 300 // = 9700 ($97.00)
	if net != want {
		t.Errorf("CalculateNetProfit local = %d, want %d", net, want)
	}
}
