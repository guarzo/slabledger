package sqlite

import (
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

var _ inventory.DHRepository = (*DHStore)(nil)
