package inventory

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
		{"legacy tcgplayer maps to ebay", SaleChannelTCGPlayer, 10000, 1235},
		{"website 3pct", SaleChannelWebsite, 10000, 300},
		{"inperson no fee", SaleChannelInPerson, 10000, 0},
		{"legacy local maps to inperson", SaleChannelLocal, 10000, 0},
		{"legacy gamestop maps to inperson", SaleChannelGameStop, 10000, 0},
		{"legacy other maps to inperson", SaleChannelOther, 10000, 0},
		{"legacy cardshow maps to inperson", SaleChannelCardShow, 10000, 0},
		{"legacy doubleholo maps to inperson", SaleChannelDoubleHolo, 10000, 0},
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
	campaign := &Campaign{EbayFeePct: 0}
	got := CalculateSaleFee(SaleChannelEbay, 10000, campaign)
	if got != 1235 {
		t.Errorf("CalculateSaleFee with default fee = %d, want 1235", got)
	}
}

func TestCalculateNetProfit(t *testing.T) {
	net := CalculateNetProfit(75000, 50000, 300, 9263)
	want := 75000 - 50000 - 300 - 9263
	if net != want {
		t.Errorf("CalculateNetProfit = %d, want %d", net, want)
	}
}

func TestNormalizeChannel(t *testing.T) {
	tests := []struct {
		input SaleChannel
		want  SaleChannel
	}{
		{SaleChannelEbay, SaleChannelEbay},
		{SaleChannelTCGPlayer, SaleChannelEbay},
		{SaleChannelWebsite, SaleChannelWebsite},
		{SaleChannelInPerson, SaleChannelInPerson},
		{SaleChannelLocal, SaleChannelInPerson},
		{SaleChannelGameStop, SaleChannelInPerson},
		{SaleChannelCardShow, SaleChannelInPerson},
		{SaleChannelOther, SaleChannelInPerson},
		{SaleChannelDoubleHolo, SaleChannelInPerson},
		{SaleChannel("unknown"), SaleChannelInPerson},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := NormalizeChannel(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeChannel(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}
