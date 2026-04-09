package campaigns

import (
	"testing"
)

func TestPendingItemRepository_InterfaceDefined(t *testing.T) {
	// Compile-time interface satisfaction check.
	var _ PendingItemRepository = nil
	_ = PendingItem{}
	_ = ErrPendingItemNotFound
	_ = IsPendingItemNotFound(nil)
}
