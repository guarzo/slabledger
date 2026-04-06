package mediafs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// --- resolve ---

func TestResolve_NormalPaths(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{"simple file", "photo.jpg", filepath.Join(root, "photo.jpg")},
		{"subdirectory", "cards/front.png", filepath.Join(root, "cards", "front.png")},
		{"nested path", "a/b/c/file.txt", filepath.Join(root, "a", "b", "c", "file.txt")},
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
	root := t.TempDir()
	store := NewStore(root)

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
	root := t.TempDir()
	store := NewStore(root)

	got, err := store.resolve(".")
	if err != nil {
		t.Fatalf("resolve(\".\") error: %v", err)
	}
	if got != filepath.Clean(root) {
		t.Errorf("resolve(\".\") = %q, want %q", got, filepath.Clean(root))
	}
}

// --- EnsureDir ---

func TestEnsureDir(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		setup   func(root string)
		wantErr bool
	}{
		{
			name: "creates directory",
			path: "subdir/nested",
		},
		{
			name: "already exists",
			path: "existing",
			setup: func(root string) {
				os.MkdirAll(filepath.Join(root, "existing"), 0o755)
			},
		},
		{
			name:    "path traversal rejected",
			path:    "../escape",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(root)

			if tt.setup != nil {
				tt.setup(root)
			}

			err := store.EnsureDir(context.Background(), tt.path)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("EnsureDir error: %v", err)
			}

			info, err := os.Stat(filepath.Join(root, tt.path))
			if err != nil {
				t.Fatalf("directory not created: %v", err)
			}
			if !info.IsDir() {
				t.Error("expected directory, got file")
			}
		})
	}
}

// --- WriteFile ---

func TestWriteFile(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		data        []byte
		setup       func(root string)
		wantErr     bool
		wantErrIs   error
		wantContent string
		wantSize    int
	}{
		{
			name:        "writes content",
			path:        "test.txt",
			data:        []byte("hello world"),
			wantContent: "hello world",
		},
		{
			name:      "missing parent dir fails",
			path:      "subdir/file.txt",
			data:      []byte("data"),
			wantErr:   true,
			wantErrIs: os.ErrNotExist,
		},
		{
			name:    "path traversal rejected",
			path:    "../../evil.txt",
			data:    []byte("hack"),
			wantErr: true,
		},
		{
			name: "overwrite",
			path: "file.txt",
			data: []byte("updated"),
			setup: func(root string) {
				os.WriteFile(filepath.Join(root, "file.txt"), []byte("original"), 0o644)
			},
			wantContent: "updated",
		},
		{
			name:     "empty content",
			path:     "empty.txt",
			data:     []byte{},
			wantSize: 0,
		},
		{
			name:     "binary content",
			path:     "image.png",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // PNG header
			wantSize: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(root)

			if tt.setup != nil {
				tt.setup(root)
			}

			err := store.WriteFile(context.Background(), tt.path, tt.data)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("expected errors.Is(%v), got: %v", tt.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("WriteFile error: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(root, tt.path))
			if err != nil {
				t.Fatalf("reading written file: %v", err)
			}

			if tt.wantContent != "" {
				if string(got) != tt.wantContent {
					t.Errorf("content = %q, want %q", string(got), tt.wantContent)
				}
			}
			if tt.wantSize > 0 && len(got) != tt.wantSize {
				t.Errorf("size = %d, want %d", len(got), tt.wantSize)
			}
			// empty content case
			if len(tt.data) == 0 && len(got) != 0 {
				t.Errorf("expected empty file, got %d bytes", len(got))
			}
		})
	}
}
