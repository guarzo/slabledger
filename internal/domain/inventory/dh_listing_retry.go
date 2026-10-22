package inventory

import "context"

// DHListingPatchRetryError identifies a repeatable same-target listing PATCH,
// not proof that DH made no changes. It must never wrap a channel-sync failure.
type DHListingPatchRetryError struct{ Cause error }

func (e *DHListingPatchRetryError) Error() string { return e.Cause.Error() }
func (e *DHListingPatchRetryError) Unwrap() error { return e.Cause }

// DHListingRetryRepository saves permission to resume an immutable listing.
type DHListingRetryRepository interface {
	EnableDHListingRetry(context.Context, string, *DHMutationAttempt) error
}
