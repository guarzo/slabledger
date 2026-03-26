package mediafs

import "os"

// Store implements social.MediaStore using the local filesystem.
type Store struct{}

// NewStore creates a new filesystem-backed media store.
func NewStore() *Store { return &Store{} }

// EnsureDir creates a directory (and parents) if it doesn't exist.
func (s *Store) EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// WriteFile writes data to a file, creating it if necessary.
func (s *Store) WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
