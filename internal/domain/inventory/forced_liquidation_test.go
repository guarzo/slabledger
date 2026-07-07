package inventory

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

func TestIsForcedLiquidation(t *testing.T) {
	invoices := []Invoice{
		{ID: "inv1", DueDate: "2026-06-20"},
		{ID: "inv2", DueDate: ""}, // malformed/empty due date ignored
	}
	tests := []struct {
		name     string
		channel  SaleChannel
		saleDate string
		want     bool
	}{
		{"inperson 6 days before due", constants.SaleChannelInPerson, "2026-06-14", true},
		{"inperson same day as due", constants.SaleChannelInPerson, "2026-06-20", true},
		{"inperson 7 days before due", constants.SaleChannelInPerson, "2026-06-13", false},
		{"inperson after due date", constants.SaleChannelInPerson, "2026-06-21", false},
		{"cardshow inside window", constants.SaleChannelCardShow, "2026-06-18", true},
		{"local inside window", constants.SaleChannelLocal, "2026-06-18", true},
		{"ebay inside window not forced", constants.SaleChannelEbay, "2026-06-18", false},
		{"website inside window not forced", constants.SaleChannelWebsite, "2026-06-18", false},
		{"bad sale date", constants.SaleChannelInPerson, "not-a-date", false},
		{"no invoices", constants.SaleChannelInPerson, "2026-06-18", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invs := invoices
			if tt.name == "no invoices" {
				invs = nil
			}
			if got := IsForcedLiquidation(tt.channel, tt.saleDate, invs); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
