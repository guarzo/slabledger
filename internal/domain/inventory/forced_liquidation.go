package inventory

import (
	"time"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

// forcedChannels are the exit channels used for invoice-driven bulk liquidation
// (validated against the 2026-06-25 W-005 analysis).
var forcedChannels = map[SaleChannel]bool{
	constants.SaleChannelInPerson: true,
	constants.SaleChannelLocal:    true,
	constants.SaleChannelCardShow: true,
}

// IsForcedLiquidation reports whether a sale looks like invoice-driven forced
// liquidation: a forced-channel sale dated within the 6 days before (or on) an
// invoice due date. Heuristic only — the persisted flag is operator-overridable.
func IsForcedLiquidation(channel SaleChannel, saleDate string, invoices []Invoice) bool {
	if !forcedChannels[channel] {
		return false
	}
	sold, err := time.Parse("2006-01-02", saleDate)
	if err != nil {
		return false
	}
	for _, inv := range invoices {
		due, err := time.Parse("2006-01-02", inv.DueDate)
		if err != nil {
			continue
		}
		days := due.Sub(sold).Hours() / 24
		if days >= 0 && days <= 6 {
			return true
		}
	}
	return false
}
