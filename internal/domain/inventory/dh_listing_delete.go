package inventory

import "context"

type failedListingDeleteKey struct{}

// WithFailedListingDeleteConfirmation records acknowledgement of possible DH
// exposure. It grants no authority to settle financial work or mutate DH.
func WithFailedListingDeleteConfirmation(ctx context.Context) context.Context {
	return context.WithValue(ctx, failedListingDeleteKey{}, true)
}

func FailedListingDeleteConfirmed(ctx context.Context) bool {
	confirmed, _ := ctx.Value(failedListingDeleteKey{}).(bool)
	return confirmed
}
