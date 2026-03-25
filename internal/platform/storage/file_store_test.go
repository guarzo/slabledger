package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONFileStore_WriteAndRead(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()
	store := NewJSONFileStore()

	testPath := filepath.Join(tmpDir, "test.json")

	// Test data
	type TestData struct {
		Name  string
		Value int
	}
	original := TestData{Name: "test", Value: 42}

	// Write
	if err := store.Write(testPath, original); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify file exists
	if !store.Exists(testPath) {
		t.Fatal("File should exist after write")
	}

	// Read
	var read TestData
	if err := store.Read(testPath, &read); err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify
	if read.Name != original.Name || read.Value != original.Value {
		t.Errorf("Data mismatch: got %+v, want %+v", read, original)
	}
}

func TestJSONFileStore_Read_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONFileStore()

	testPath := filepath.Join(tmpDir, "nonexistent.json")

	var data map[string]string
	err := store.Read(testPath, &data)

	if err == nil {
		t.Fatal("Expected error reading non-existent file")
	}
}

func TestJSONFileStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONFileStore()

	testPath := filepath.Join(tmpDir, "test.json")

	// Write file
	data := map[string]string{"key": "value"}
	if err := store.Write(testPath, data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify exists
	if !store.Exists(testPath) {
		t.Fatal("File should exist")
	}

	// Delete
	if err := store.Delete(testPath); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	if store.Exists(testPath) {
		t.Fatal("File should not exist after delete")
	}

	// Delete again (should not error)
	if err := store.Delete(testPath); err != nil {
		t.Fatalf("Delete of non-existent file should not error: %v", err)
	}
}

func TestJSONFileStore_EnsureDir(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONFileStore()

	nestedDir := filepath.Join(tmpDir, "a", "b", "c")

	if err := store.EnsureDir(nestedDir); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("Directory should exist: %v", err)
	}

	if !info.IsDir() {
		t.Fatal("Path should be a directory")
	}
}

func TestJSONFileStore_Write_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONFileStore()

	// Write to nested path that doesn't exist
	nestedPath := filepath.Join(tmpDir, "x", "y", "z", "test.json")
	data := map[string]string{"key": "value"}

	if err := store.Write(nestedPath, data); err != nil {
		t.Fatalf("Write should create directories: %v", err)
	}

	// Verify file was created
	if !store.Exists(nestedPath) {
		t.Fatal("File should exist")
	}

	// Verify can read back
	var read map[string]string
	if err := store.Read(nestedPath, &read); err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if read["key"] != "value" {
		t.Errorf("Data mismatch: got %v, want %v", read["key"], "value")
	}
}

func TestJSONFileStore_Write_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewJSONFileStore()

	testPath := filepath.Join(tmpDir, "test.json")

	// Try to write invalid data (channels can't be marshaled to JSON)
	invalidData := make(chan int)

	err := store.Write(testPath, invalidData)
	if err == nil {
		t.Fatal("Expected error marshaling invalid data")
	}
}
