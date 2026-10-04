package inventory

import "context"

// PurchaseMutationScope owns a bounded READ COMMITTED transaction. Nested work
// on the same purchase shares ownership; it must use the callback context.
type PurchaseMutationScope interface {
	WithPurchaseMutation(context.Context, string, func(context.Context) error) error
}
