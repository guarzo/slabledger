package httpx

import "context"

type noRetryKey struct{}

// WithNoRetry disables automatic retries for requests using this context.
// It does not change the client's policy or assert that a failed mutation stopped.
func WithNoRetry(ctx context.Context) context.Context {
	return context.WithValue(ctx, noRetryKey{}, true)
}

func retriesDisabled(ctx context.Context, req Request) bool {
	disabled, _ := ctx.Value(noRetryKey{}).(bool)
	return req.DisableRetry || disabled
}
