package sqlite

import (
	"github.com/guarzo/slabledger/internal/domain/finance"
)

var _ finance.FinanceReader = (*FinanceStore)(nil)
