package mediafs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store implements social.MediaStore using the local filesystem.
// All paths are resolved relative to mediaRoot, and path traversal
// beyond mediaRoot is rejected.
type Store struct {
	mediaRoot string
}

// NewStore creates a new filesystem-backed media store rooted at mediaRoot.
func NewStore(mediaRoot string) *Store { return &Store{mediaRoot: mediaRoot} }

// resolve joins path with mediaRoot and rejects any result that escapes it.
func (s *Store) resolve(path string) (string, error) {
	cleaned := filepath.Clean(filepath.Join(s.mediaRoot, path))
	root := filepath.Clean(s.mediaRoot)
	if !strings.HasPrefix(cleaned, root+string(filepath.Separator)) && cleaned != root {
		return "", fmt.Errorf("path %q escapes media root", path)
	}
	return cleaned, nil
}

// EnsureDir creates a directory (and parents) if it doesn't exist.
func (s *Store) EnsureDir(_ context.Context, path string) error {
	resolved, err := s.resolve(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(resolved, 0o755)
}

// WriteFile writes data to a file, creating it if necessary.
func (s *Store) WriteFile(_ context.Context, path string, data []byte) error {
	resolved, err := s.resolve(path)
	if err != nil {
		return err
	}
	return os.WriteFile(resolved, data, 0o644)
}
