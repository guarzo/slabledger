package sqlite

import (
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

var _ inventory.PricingRepository = (*PricingStore)(nil)
