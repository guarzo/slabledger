package tcgdex

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/platform/storage"
)

// PersistedSet represents a set and its cards persisted to disk
type PersistedSet struct {
	Set      domainCards.Set    `json:"set"`
	Cards    []domainCards.Card `json:"cards"`
	Metadata StorageMetadata    `json:"metadata"`
}

// StorageMetadata tracks when data was stored
type StorageMetadata struct {
	FetchedAt time.Time `json:"fetchedAt"`
	Version   string    `json:"version"`
}

// SetStore manages persistent storage of sets and cards
type SetStore struct {
	store    storage.FileStore
	cacheDir string
}

// NewSetStore creates a new set store
func NewSetStore(store storage.FileStore, cacheDir string) *SetStore {
	return &SetStore{
		store:    store,
		cacheDir: cacheDir,
	}
}

// setPath returns the file path for a set
func (s *SetStore) setPath(setID string) string {
	return filepath.Join(s.cacheDir, fmt.Sprintf("%s.json", setID))
}

// LoadSet loads a set and its cards from disk
func (s *SetStore) LoadSet(ctx context.Context, setID string) (*PersistedSet, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	path := s.setPath(setID)

	if !s.store.Exists(path) {
		return nil, apperrors.NotFoundError("set", setID)
	}

	var persistedSet PersistedSet
	if err := s.store.Read(path, &persistedSet); err != nil {
		return nil, apperrors.StorageError("load set "+setID, err)
	}

	return &persistedSet, nil
}

// SaveSet saves a set and its cards to disk
func (s *SetStore) SaveSet(ctx context.Context, setID string, set domainCards.Set, cards []domainCards.Card) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	persistedSet := PersistedSet{
		Set:   set,
		Cards: cards,
		Metadata: StorageMetadata{
			FetchedAt: time.Now(),
			Version:   "1.0",
		},
	}

	path := s.setPath(setID)
	if err := s.store.Write(path, persistedSet); err != nil {
		return apperrors.StorageError("save set "+setID, err)
	}

	return nil
}

// DeleteSet removes a set from persistent storage
func (s *SetStore) DeleteSet(ctx context.Context, setID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	path := s.setPath(setID)
	if err := s.store.Delete(path); err != nil {
		return apperrors.StorageError("delete set "+setID, err)
	}
	return nil
}

// SetExists checks if a set exists in persistent storage
func (s *SetStore) SetExists(setID string) bool {
	return s.store.Exists(s.setPath(setID))
}
