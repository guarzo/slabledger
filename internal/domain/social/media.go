package social

// MediaStore abstracts file storage for generated media (backgrounds, slides).
// Domain code uses this interface instead of os.MkdirAll / os.WriteFile directly.
type MediaStore interface {
	// EnsureDir creates a directory (and parents) if it doesn't exist.
	EnsureDir(path string) error
	// WriteFile writes data to a file, creating it if necessary.
	WriteFile(path string, data []byte) error
}
