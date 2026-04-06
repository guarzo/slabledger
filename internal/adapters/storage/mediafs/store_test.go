package mediafs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// --- resolve ---

func TestResolve_NormalPaths(t *testing.T) {
	store := NewStore("/tmp/media")

	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{"simple file", "photo.jpg", "/tmp/media/photo.jpg"},
		{"subdirectory", "cards/front.png", "/tmp/media/cards/front.png"},
		{"nested path", "a/b/c/file.txt", "/tmp/media/a/b/c/file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.resolve(tt.path)
			if err != nil {
				t.Fatalf("resolve(%q) error: %v", tt.path, err)
			}
			if got != tt.wantPath {
				t.Errorf("resolve(%q) = %q, want %q", tt.path, got, tt.wantPath)
			}
		})
	}
}

func TestResolve_PathTraversalRejected(t *testing.T) {
	store := NewStore("/tmp/media")

	tests := []struct {
		name string
		path string
	}{
		{"parent directory", "../etc/passwd"},
		{"deep traversal", "../../../etc/shadow"},
		{"mixed traversal", "cards/../../etc/passwd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.resolve(tt.path)
			if err == nil {
				t.Errorf("resolve(%q) should have been rejected", tt.path)
			}
		})
	}
}

func TestResolve_RootPath(t *testing.T) {
	store := NewStore("/tmp/media")

	// "." resolves to the root itself, which should be allowed.
	got, err := store.resolve(".")
	if err != nil {
		t.Fatalf("resolve(\".\") error: %v", err)
	}
	if got != filepath.Clean("/tmp/media") {
		t.Errorf("resolve(\".\") = %q, want %q", got, filepath.Clean("/tmp/media"))
	}
}

// --- EnsureDir ---

func TestEnsureDir_CreatesDirectory(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	err := store.EnsureDir(context.Background(), "subdir/nested")
	if err != nil {
		t.Fatalf("EnsureDir error: %v", err)
	}

	info, err := os.Stat(filepath.Join(root, "subdir", "nested"))
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory, got file")
	}
}

func TestEnsureDir_AlreadyExists(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	// Create it first.
	if err := os.MkdirAll(filepath.Join(root, "existing"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Should succeed without error.
	err := store.EnsureDir(context.Background(), "existing")
	if err != nil {
		t.Fatalf("EnsureDir on existing dir should succeed: %v", err)
	}
}

func TestEnsureDir_PathTraversalRejected(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	err := store.EnsureDir(context.Background(), "../escape")
	if err == nil {
		t.Fatal("EnsureDir should reject path traversal")
	}
}

// --- WriteFile ---

func TestWriteFile_WritesContent(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	data := []byte("hello world")
	err := store.WriteFile(context.Background(), "test.txt", data)
	if err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "test.txt"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", string(got), "hello world")
	}
}

func TestWriteFile_MissingParentDirFails(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	// os.WriteFile does not create parent directories.
	err := store.WriteFile(context.Background(), "subdir/file.txt", []byte("data"))
	if err == nil {
		t.Fatal("expected error when parent directory does not exist")
	}
}

func TestWriteFile_PathTraversalRejected(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	err := store.WriteFile(context.Background(), "../../evil.txt", []byte("hack"))
	if err == nil {
		t.Fatal("WriteFile should reject path traversal")
	}
}

func TestWriteFile_Overwrite(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	// Write initial content.
	if err := store.WriteFile(context.Background(), "file.txt", []byte("original")); err != nil {
		t.Fatal(err)
	}
	// Overwrite.
	if err := store.WriteFile(context.Background(), "file.txt", []byte("updated")); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(filepath.Join(root, "file.txt"))
	if string(got) != "updated" {
		t.Errorf("content = %q, want %q", string(got), "updated")
	}
}

func TestWriteFile_EmptyContent(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	err := store.WriteFile(context.Background(), "empty.txt", []byte{})
	if err != nil {
		t.Fatalf("WriteFile empty error: %v", err)
	}

	info, err := os.Stat(filepath.Join(root, "empty.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("file size = %d, want 0", info.Size())
	}
}

func TestWriteFile_BinaryContent(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header
	err := store.WriteFile(context.Background(), "image.png", data)
	if err != nil {
		t.Fatalf("WriteFile binary error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(root, "image.png"))
	if len(got) != len(data) {
		t.Errorf("binary content length = %d, want %d", len(got), len(data))
	}
}
