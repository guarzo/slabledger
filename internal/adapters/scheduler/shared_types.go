package scheduler

import "context"

// SyncStateStore reads and writes sync state key-value pairs.
type SyncStateStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}
