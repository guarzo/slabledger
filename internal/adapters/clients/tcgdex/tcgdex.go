// Package tcgdex provides a TCGdex.dev API adapter for card and set data.
// This is an ADAPTER that implements the domain cards.CardProvider interface.
//
// Architecture: Hexagonal (Ports & Adapters)
//   - Domain: internal/domain/cards (defines CardProvider interface)
//   - Adapter: internal/adapters/clients/tcgdex (implements CardProvider interface)
//
// This adapter wraps the TCGdex REST API (https://api.tcgdex.net/v2) and
// translates responses into domain types defined in internal/domain/cards.
// No API key is required. Supports multiple languages (English + Japanese).
package tcgdex

import (
	"context"
	"os"
	"time"

	"golang.org/x/time/rate"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/platform/storage"
)

// tcgdexTimeout is the HTTP timeout for TCGdex API requests.
const tcgdexTimeout = 30 * time.Second

// tcgdexMaxRetries is the maximum number of retries for TCGdex API requests.
const tcgdexMaxRetries = 3

// defaultCacheDir is the default directory for persistent set cache data.
const defaultCacheDir = "data/cache/tcgdex-sets"

// DefaultNewSetDiscoveryInterval is the default interval between set list refreshes.
const DefaultNewSetDiscoveryInterval = 7 * 24 * time.Hour

// defaultLanguages are the languages fetched by default.
var defaultLanguages = []string{"en", "ja"}

// Compile-time interface check
var _ domainCards.CardProvider = (*TCGdex)(nil)

// TCGdex implements cards.CardProvider using the TCGdex.dev API.
type TCGdex struct {
	languages               []string
	cache                   cache.Cache
	httpClient              httpx.HTTPClient
	registryMgr             *SetRegistryManager
	setStore                *SetStore
	enablePersist           bool
	logger                  observability.Logger
	newSetDiscoveryInterval time.Duration
	baseURL                 string
	fileStore               storage.FileStore
	cacheDir                string
	rateLimiter             *rate.Limiter
}

// TCGdexOption is a functional option for configuring TCGdex.
type TCGdexOption func(*TCGdex)

// WithFileStore sets the file store used for persistent storage.
func WithFileStore(fs storage.FileStore) TCGdexOption {
	return func(t *TCGdex) {
		t.fileStore = fs
	}
}

// WithCacheDir sets the directory for persistent cache data.
func WithCacheDir(dir string) TCGdexOption {
	return func(t *TCGdex) {
		t.cacheDir = dir
	}
}

// WithEnablePersist enables or disables persistent storage.
func WithEnablePersist(enable bool) TCGdexOption {
	return func(t *TCGdex) {
		t.enablePersist = enable
	}
}

// WithTCGdexLogger sets the logger for the TCGdex adapter.
func WithTCGdexLogger(l observability.Logger) TCGdexOption {
	return func(t *TCGdex) {
		if l != nil {
			t.logger = l
		}
	}
}

// WithBaseURL sets the base URL for TCGdex API requests (useful for testing).
func WithBaseURL(url string) TCGdexOption {
	return func(t *TCGdex) {
		t.baseURL = url
	}
}

// NewTCGdex creates a new TCGdex adapter with default HTTP client and persistent storage.
func NewTCGdex(c cache.Cache, log observability.Logger) *TCGdex {
	if log == nil {
		log = observability.NewNoopLogger()
	}

	config := httpx.DefaultConfig("TCGdex")
	config.DefaultTimeout = tcgdexTimeout
	config.RetryPolicy.MaxRetries = tcgdexMaxRetries
	httpClient := httpx.NewClient(config)

	fileStore := storage.NewJSONFileStore()
	cacheDir := os.Getenv("TCGDEX_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = defaultCacheDir
	}

	if err := fileStore.EnsureDir(cacheDir); err != nil {
		log.Warn(context.Background(), "Failed to create persistent cache directory, persistence disabled",
			observability.String("cache_dir", cacheDir), observability.Err(err))
		return NewTCGdexWithClientAndStorage(c, httpClient, WithTCGdexLogger(log))
	}

	return NewTCGdexWithClientAndStorage(c, httpClient,
		WithFileStore(fileStore), WithCacheDir(cacheDir), WithEnablePersist(true), WithTCGdexLogger(log))
}

// newTCGdexWithClient creates a TCGdex adapter with injectable HTTP client.
// Persistent storage is disabled (for testing).
func newTCGdexWithClient(c cache.Cache, httpClient httpx.HTTPClient) *TCGdex {
	return NewTCGdexWithClientAndStorage(c, httpClient)
}

// NewTCGdexWithClientAndStorage creates a TCGdex adapter with optional persistent storage.
func NewTCGdexWithClientAndStorage(c cache.Cache, httpClient httpx.HTTPClient, opts ...TCGdexOption) *TCGdex {
	t := &TCGdex{
		languages:     defaultLanguages,
		cache:         c,
		httpClient:    httpClient,
		logger:        observability.NewNoopLogger(),
		enablePersist: false,
		baseURL:       defaultBaseURL,
		rateLimiter:   rate.NewLimiter(rate.Limit(2), 2), // 2 req/sec
	}

	for _, opt := range opts {
		opt(t)
	}

	if t.enablePersist && t.fileStore != nil && t.cacheDir != "" {
		t.registryMgr = NewSetRegistryManager(t.fileStore, t.cacheDir, t.logger)
		t.setStore = NewSetStore(t.fileStore, t.cacheDir)
	}

	t.newSetDiscoveryInterval = DefaultNewSetDiscoveryInterval

	return t
}

// RegistryManager returns the set registry manager for external wiring (e.g., scheduler).
// Returns a typed nil when registryMgr is nil so that callers doing provider != nil
// see the real nil state (not a non-nil interface wrapping a nil pointer).
func (t *TCGdex) RegistryManager() domainCards.NewSetIDsProvider {
	if t.registryMgr == nil {
		return nil
	}
	return t.registryMgr
}

// Available implements domainCards.CardProvider interface.
// TCGdex is a public API with no authentication, so it's always available.
func (t *TCGdex) Available() bool {
	return true
}

// languagesForSet returns the language order to try when fetching a set.
// If the registry knows which language the set was discovered in, that language
// is tried first (and exclusively, since a set belongs to one language).
// Falls back to trying all configured languages if the registry has no record.
func (t *TCGdex) languagesForSet(ctx context.Context, setID string) []string {
	if t.enablePersist && t.registryMgr != nil {
		registry, err := t.registryMgr.LoadRegistry(ctx)
		if err == nil {
			if entry, ok := registry.Sets[setID]; ok && entry.Language != "" {
				return []string{entry.Language}
			}
		}
	}
	return t.languages
}
