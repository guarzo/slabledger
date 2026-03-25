// Package storage provides simple file-based persistence for structured data.
// This is a platform/infrastructure layer component.
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileStore provides simple JSON file persistence
type FileStore interface {
	// Read reads JSON data from a file into the provided struct
	Read(path string, into any) error

	// Write writes data as JSON to a file
	Write(path string, data any) error

	// Exists checks if a file exists
	Exists(path string) bool

	// Delete removes a file
	Delete(path string) error

	// EnsureDir ensures a directory exists
	EnsureDir(dir string) error
}

// JSONFileStore implements FileStore for JSON files
type JSONFileStore struct{}

// NewJSONFileStore creates a new JSON file store
func NewJSONFileStore() *JSONFileStore {
	return &JSONFileStore{}
}

// Read reads JSON data from a file
func (s *JSONFileStore) Read(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", path)
		}
		return fmt.Errorf("failed to read file %s: %w", path, err)
	}

	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("failed to parse JSON from %s: %w", path, err)
	}

	return nil
}

// Write writes data as JSON to a file
func (s *JSONFileStore) Write(path string, data any) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := s.EnsureDir(dir); err != nil {
		return err
	}

	// Marshal to compact JSON (no indentation) for performance
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Write to file with atomic write pattern (write to temp, then rename)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			// Best effort cleanup - log but don't fail if cleanup fails
			// Original rename error is more important to return
		}
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Exists checks if a file exists
func (s *JSONFileStore) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Delete removes a file
func (s *JSONFileStore) Delete(path string) error {
	if !s.Exists(path) {
		return nil // Already deleted
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", path, err)
	}

	return nil
}

// EnsureDir ensures a directory exists
func (s *JSONFileStore) EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return nil
}
