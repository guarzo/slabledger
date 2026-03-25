package tcgdex

import (
	"context"
	"path/filepath"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/storage"
)

// registryProvider is the provider tag written into the registry on save.
const registryProvider = "tcgdex"

// SetRegistry tracks which sets have been fetched and persisted
type SetRegistry struct {
	Version     string              `json:"version"`
	Provider    string              `json:"provider,omitempty"`
	LastUpdated time.Time           `json:"lastUpdated"`
	Sets        map[string]SetEntry `json:"sets"`
}

// SetEntry tracks metadata for a single set
type SetEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Series      string    `json:"series,omitempty"`
	Language    string    `json:"language"` // source language code ("en", "ja")
	ReleaseDate string    `json:"releaseDate"`
	TotalCards  int       `json:"totalCards"`
	FetchedAt   time.Time `json:"fetchedAt,omitempty"`
	Status      string    `json:"status"` // "discovered", "finalized"
}

// SetRegistryManager manages the set registry
type SetRegistryManager struct {
	store        storage.FileStore
	registryPath string
	logger       observability.Logger
}

// NewSetRegistryManager creates a new registry manager
func NewSetRegistryManager(store storage.FileStore, cacheDir string, logger observability.Logger) *SetRegistryManager {
	if logger == nil {
		logger = observability.NewNoopLogger()
	}
	return &SetRegistryManager{
		store:        store,
		registryPath: filepath.Join(cacheDir, "registry.json"),
		logger:       logger,
	}
}

// LoadRegistry loads the registry from disk, returns empty registry if not found
func (m *SetRegistryManager) LoadRegistry(ctx context.Context) (*SetRegistry, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if !m.store.Exists(m.registryPath) {
		return &SetRegistry{
			Version:     "1.0",
			LastUpdated: time.Now(),
			Sets:        make(map[string]SetEntry),
		}, nil
	}

	var registry SetRegistry
	if err := m.store.Read(m.registryPath, &registry); err != nil {
		m.logger.Error(ctx, "failed to load registry",
			observability.String("path", m.registryPath),
			observability.Err(err))
		return nil, apperrors.StorageError("load registry", err)
	}

	if registry.Sets == nil {
		registry.Sets = make(map[string]SetEntry)
	}

	return &registry, nil
}

// SaveRegistry saves the registry to disk
func (m *SetRegistryManager) SaveRegistry(ctx context.Context, reg *SetRegistry) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	reg.LastUpdated = time.Now()
	reg.Provider = registryProvider
	if err := m.store.Write(m.registryPath, reg); err != nil {
		m.logger.Error(ctx, "failed to save registry",
			observability.String("path", m.registryPath),
			observability.Err(err))
		return apperrors.StorageError("save registry", err)
	}
	return nil
}

// isSetCached checks if a set has been fetched and finalized.
func (m *SetRegistryManager) isSetCached(ctx context.Context, setID string) (bool, error) {
	registry, err := m.LoadRegistry(ctx)
	if err != nil {
		return false, err
	}

	entry, exists := registry.Sets[setID]
	return exists && entry.Status == "finalized", nil
}

// MarkSetDiscovered marks a set as discovered but not yet fetched.
// The language parameter records which TCGdex language endpoint this set belongs to.
func (m *SetRegistryManager) MarkSetDiscovered(ctx context.Context, setID, name, language, releaseDate string, totalCards int) error {
	registry, err := m.LoadRegistry(ctx)
	if err != nil {
		return err
	}

	if _, exists := registry.Sets[setID]; !exists {
		registry.Sets[setID] = SetEntry{
			ID:          setID,
			Name:        name,
			Language:    language,
			ReleaseDate: releaseDate,
			TotalCards:  totalCards,
			Status:      "discovered",
		}
		m.logger.Info(ctx, "set marked as discovered",
			observability.String("set_id", setID),
			observability.String("name", name),
			observability.Int("total_cards", totalCards))
		return m.SaveRegistry(ctx, registry)
	}

	return nil
}

// MarkSetFinalized marks a set as fully fetched and persisted
func (m *SetRegistryManager) MarkSetFinalized(ctx context.Context, setID string) error {
	registry, err := m.LoadRegistry(ctx)
	if err != nil {
		return err
	}

	entry, exists := registry.Sets[setID]
	if exists {
		entry.Status = "finalized"
		entry.FetchedAt = time.Now()
		registry.Sets[setID] = entry
		m.logger.Info(ctx, "set marked as finalized",
			observability.String("set_id", setID),
			observability.String("name", entry.Name))
		return m.SaveRegistry(ctx, registry)
	}

	return apperrors.NotFoundError("set", setID)
}

// RemoveSet removes a set from the registry
func (m *SetRegistryManager) RemoveSet(ctx context.Context, setID string) error {
	registry, err := m.LoadRegistry(ctx)
	if err != nil {
		return err
	}

	if _, exists := registry.Sets[setID]; !exists {
		return nil
	}

	delete(registry.Sets, setID)
	m.logger.Info(ctx, "set removed from registry",
		observability.String("set_id", setID))
	return m.SaveRegistry(ctx, registry)
}

// GetNewSetIDs returns set IDs that are not yet finalized.
func (m *SetRegistryManager) GetNewSetIDs(ctx context.Context, allSetIDs []string) ([]string, error) {
	registry, err := m.LoadRegistry(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return allSetIDs, nil
	}

	newIDs := []string{}
	for _, setID := range allSetIDs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		entry, exists := registry.Sets[setID]
		if !exists || entry.Status != "finalized" {
			newIDs = append(newIDs, setID)
		}
	}

	return newIDs, nil
}

// Clear removes the registry file
func (m *SetRegistryManager) Clear(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if err := m.store.Delete(m.registryPath); err != nil {
		m.logger.Error(ctx, "failed to clear registry",
			observability.String("path", m.registryPath),
			observability.Err(err))
		return err
	}
	m.logger.Info(ctx, "registry cleared",
		observability.String("path", m.registryPath))
	return nil
}
