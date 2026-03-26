package social

import "context"

// MediaStore abstracts file storage for generated media (backgrounds, slides).
// Domain code uses this interface instead of os.MkdirAll / os.WriteFile directly.
type MediaStore interface {
	// EnsureDir creates a directory (and parents) if it doesn't exist.
	EnsureDir(ctx context.Context, path string) error
	// WriteFile writes data to a file, creating it if necessary.
	WriteFile(ctx context.Context, path string, data []byte) error
}
