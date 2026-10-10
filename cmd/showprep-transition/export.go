package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
)

type snapshotReader interface {
	GetList(context.Context, string) (sp.List, error)
	GetItems(context.Context, string) ([]sp.Item, error)
}

// Export only ordinary stored reads. Fingerprint and private DTO derive from the
// same list/items, never evaluated public projections. Apply checks atomically.
func exportSnapshot(ctx context.Context, listID, path string, store snapshotReader) error {
	if !canonicalID(listID) || path == "" {
		return errors.New("export requires canonical list UUID and private snapshot path")
	}
	list, err := store.GetList(ctx, listID)
	if err != nil {
		return commandError(err)
	}
	if list.ID != listID {
		return errors.New("export list identity mismatch")
	}
	items, err := store.GetItems(ctx, listID)
	if err != nil {
		return commandError(err)
	}
	p := capturePrivateSnapshot(list, items)
	if err := validateSnapshot(p.detail(), "confirm-existing"); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil || len(b) > maxSnapshotBytes {
		return errors.New("private snapshot cannot be serialized or is too large")
	}
	if err := ctx.Err(); err != nil {
		return errors.New("snapshot export cancelled")
	}
	// Never truncate or follow an existing path/symlink: captured consent must stay
	// immutable across uncertain apply responses. Each export needs a new path.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("cannot create private snapshot; choose a new private file path")
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return errors.New("cannot enforce private snapshot permissions; discard incomplete file")
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return errors.New("snapshot export write failed; discard incomplete file")
	}
	if err := f.Close(); err != nil {
		return errors.New("snapshot export close failed; discard incomplete file")
	}
	return nil
}
