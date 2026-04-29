package config

import "time"

// ModeConfig controls the operation mode of the application
type ModeConfig struct {
	WebMode bool // Start web server mode instead of CLI
	WebPort int  // Web server port

	// Rate limiting configuration
	RateLimitRequests int  // Requests allowed per minute (default: 300)
	TrustProxy        bool // Trust X-Forwarded-For and X-Real-IP headers (default: false)
}

// CacheConfig controls cache behavior and refresh operations
type CacheConfig struct {
	Path string // Cache file location
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	ListenAddr               string        // HTTP listen address (default: 127.0.0.1:8080)
	ReadTimeout              time.Duration // HTTP read timeout
	WriteTimeout             time.Duration // HTTP write timeout
	IdleTimeout              time.Duration // HTTP idle timeout
	ShutdownTimeout          time.Duration // Graceful HTTP server shutdown timeout
	SchedulerShutdownTimeout time.Duration // Graceful scheduler shutdown timeout (SHUTDOWN_TIMEOUT_SECONDS)
}

// LoggingConfig controls logging behavior
type LoggingConfig struct {
	Level string // Log level: debug, info, warn, error
	JSON  bool   // Output JSON logs
}

// DatabaseConfig controls database storage
type DatabaseConfig struct {
	URL            string // PostgreSQL connection URL (DATABASE_URL)
	MigrationsPath string // Path to migrations directory (empty = use embedded migrations)
}

// MaintenanceConfig controls database maintenance and cleanup operations
type MaintenanceConfig struct {
	// AccessLogRetentionDays controls how long card access log entries are retained.
	// Entries older than this are deleted by the cleanup scheduler.
	// Default: 30 days. Set to 0 to disable cleanup (not recommended for production).
	AccessLogRetentionDays int

	// AccessLogCleanupInterval controls how often the cleanup scheduler runs.
	// Default: 24 hours (daily cleanup).
	AccessLogCleanupInterval time.Duration

	// AccessLogCleanupEnabled controls whether the cleanup scheduler is active.
	// Default: true
	AccessLogCleanupEnabled bool

	// BackfillImages enqueues unsold PSA purchases with empty image URLs onto
	// the cert-enrichment queue at startup so they pick up front/back slab
	// images from PSA. Opt-in because it consumes PSA daily budget.
	BackfillImages bool
}

// PriceRefreshConfig controls the price refresh scheduler
type PriceRefreshConfig struct {
	// How often to check for stale prices
	RefreshInterval time.Duration
	// Max prices to refresh per batch
	BatchSize int
	// Delay between individual API calls
	BatchDelay time.Duration
	// Max API calls per 5-minute window per provider
	MaxBurstCalls int
	// Max API calls allowed per hour per provider (default: 50)
	MaxCallsPerHour int
	// Duration to pause after hitting burst limit (default: 30 seconds)
	BurstPauseDuration time.Duration
	// Enable scheduler
	Enabled bool
}

// SessionCleanupConfig controls session cleanup scheduling
type SessionCleanupConfig struct {
	Enabled  bool
	Interval time.Duration // How often to run cleanup (default: 1 hour)
}

// AuthConfig controls authentication settings
type AuthConfig struct {
	// EncryptionKey for encrypting OAuth tokens at rest (AES-256-GCM)
	// Must be at least 32 characters. Generate with: openssl rand -hex 32
	EncryptionKey string

	// AdminEmails is a list of email addresses that are always allowed to log in
	// and always have admin privileges. Set via ADMIN_EMAILS env var (comma-separated).
	AdminEmails []string
}

// AdapterConfig holds API keys and tokens for external service adapters.
// These are read from environment variables centrally and passed to adapter
// constructors — adapters never read env vars directly.
type AdapterConfig struct {
	PSAToken                 string        // PSA_ACCESS_TOKEN - PSA cert lookup (comma-separated for rotation)
	PricingAPIKey            string        // PRICING_API_KEY - Bearer token for pricing API auth
	GoogleOAuthEnv           string        // GOOGLE_OAUTH_ENV - controls login button visibility ("production" shows it)
	LocalAPIToken            string        // LOCAL_API_TOKEN - dev-mode bearer bypass; empty = disabled
	AzureAIEndpoint          string        // AZURE_AI_ENDPOINT - Azure AI Foundry endpoint URL
	AzureAIKey               string        // AZURE_AI_API_KEY - Azure AI API key
	AzureAIDeployment        string        // AZURE_AI_DEPLOYMENT - Model deployment name (default: gpt-5.4)
	DHEnterpriseKey          string        // DH_ENTERPRISE_API_KEY - Bearer token for enterprise endpoints
	DHBaseURL                string        // DH_API_BASE_URL
	PSAExchangeToken         string        // PSA_EXCHANGE_TOKEN - buyer access token
	PSAExchangeBuyerCID      string        // PSA_EXCHANGE_BUYER_CID - buyer CID (reserved for v2)
	PSAExchangeBaseURL       string        // PSA_EXCHANGE_BASE_URL - base URL (default: https://psa-exchange-catalog.com)
	AzureAICompletionTimeout time.Duration // AZURE_AI_TIMEOUT - Completion poll fallback timeout (default: 3m)
}

// DHConfig holds DH scheduler and rate limiting settings.
type DHConfig struct {
	Enabled               bool
	CacheTTLHours         int
	RateLimitRPS          int
	OrdersPollInterval    time.Duration // default: 30m
	InventoryPollInterval time.Duration // default: 2h
	PushInterval          time.Duration // default: 5m
}

// InventoryRefreshConfig controls the inventory snapshot refresh scheduler
type InventoryRefreshConfig struct {
	// Enable/disable scheduler (default: true)
	Enabled bool
	// How often to run refresh cycle (default: 1h)
	Interval time.Duration
	// Snapshots older than this are considered stale (default: 12h)
	StaleThreshold time.Duration
	// Max purchases to refresh per cycle (default: 20)
	BatchSize int
	// Delay between individual API calls (default: 2s)
	BatchDelay time.Duration
}

// ApplyDefaults fills in zero-valued fields with sensible defaults.
func (c *InventoryRefreshConfig) ApplyDefaults() {
	if c.Interval <= 0 {
		c.Interval = 1 * time.Hour
	}
	if c.StaleThreshold <= 0 {
		c.StaleThreshold = 12 * time.Hour
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 20
	}
	if c.BatchDelay <= 0 {
		c.BatchDelay = 2 * time.Second
	}
}

// Config holds all application configuration organized into logical groups
type Config struct {
	Mode               ModeConfig
	Cache              CacheConfig
	Server             ServerConfig
	Logging            LoggingConfig
	Database           DatabaseConfig
	Maintenance        MaintenanceConfig
	Auth               AuthConfig
	PriceRefresh       PriceRefreshConfig
	SessionCleanup     SessionCleanupConfig
	InventoryRefresh   InventoryRefreshConfig
	SnapshotEnrich     SnapshotEnrichConfig
	AdvisorRefresh     AdvisorRefreshConfig
	CardLadder         CardLadderConfig
	MarketMovers       MarketMoversConfig
	GoogleSheets       GoogleSheetsConfig
	PSASync            PSASyncConfig
	DH                 DHConfig
	DHAnalyticsRefresh DHAnalyticsRefreshConfig
	DHReconcile        DHReconcileConfig
	DHSoldReconciler   DHSoldReconcilerConfig
	DHPriceSync        DHPriceSyncConfig
	Adapters           AdapterConfig
}

// DHAnalyticsRefreshConfig controls the daily DH demand analytics refresh
// scheduler (niche-opportunity leaderboard cache). Enabled by default when the
// DH Enterprise key is configured. The scheduler wakes once per day
// (RefreshHour UTC, default 4am) and refills the dh_card_cache /
// dh_character_cache tables.
type DHAnalyticsRefreshConfig struct {
	Enabled     bool
	RefreshHour int    // UTC hour 0–23; default 3
	Window      string // demand signal window, e.g. "30d" (default)
}

// DHReconcileConfig controls the hourly DH inventory reconciliation scheduler.
// The reconciler diffs the local view of DH linkage against a fresh DH
// inventory snapshot, and resets local DH fields for purchases whose
// dh_inventory_id is no longer present on DH. The push scheduler then
// re-enrolls them as in_stock on its next tick.
type DHReconcileConfig struct {
	Enabled  bool          // default: true
	Interval time.Duration // default: 1h
}

// ApplyDefaults sets zero-valued fields to sensible defaults.
func (c *DHReconcileConfig) ApplyDefaults() {
	if c.Interval <= 0 {
		c.Interval = 1 * time.Hour
	}
}

// DHSoldReconcilerConfig controls the scheduler that reconciles dh_status for
// purchases that have a linked sale but whose dh_status is not 'sold'.
type DHSoldReconcilerConfig struct {
	Enabled  bool          // default: true
	Interval time.Duration // default: 1h
}

// ApplyDefaults sets zero-valued fields to sensible defaults.
func (c *DHSoldReconcilerConfig) ApplyDefaults() {
	if c.Interval <= 0 {
		c.Interval = 1 * time.Hour
	}
}

// DHPriceSyncConfig controls the DH price re-sync scheduler. Periodically
// reconciles drift between reviewed_price_cents and dh_listing_price_cents
// for purchases already pushed to or listed on DH. Also runs inline on
// every review-price edit; the scheduler is the safety net for
// failed/missed inline syncs.
type DHPriceSyncConfig struct {
	Enabled  bool          // default: true
	Interval time.Duration // default: 15m
}

// ApplyDefaults sets zero-valued fields to sensible defaults.
func (c *DHPriceSyncConfig) ApplyDefaults() {
	if c.Interval <= 0 {
		c.Interval = 15 * time.Minute
	}
}

// AdvisorRefreshConfig controls the background AI advisor analysis scheduler.
type AdvisorRefreshConfig struct {
	Enabled       bool
	Interval      time.Duration // how often to run analysis (default: 24h)
	InitialDelay  time.Duration // delay before first run (default: 2m)
	RefreshHour   int           // hour (0-23 UTC) to schedule runs; -1 = use InitialDelay (default: 4)
	MaxToolRounds int           // max LLM tool-calling rounds per analysis (default: 5)
}

// SnapshotEnrichConfig controls the background snapshot enrichment scheduler.
type SnapshotEnrichConfig struct {
	Enabled       bool
	Interval      time.Duration // how often to process pending snapshots (default: 15s)
	RetryInterval time.Duration // how often to retry failed snapshots (default: 30m)
	BatchSize     int           // max purchases per tick (default: 10)
	MaxRetries    int           // max retry attempts before marking exhausted (default: 5)
}

// ApplyDefaults fills in zero-valued fields with sensible defaults.
func (c *SnapshotEnrichConfig) ApplyDefaults() {
	if c.Interval <= 0 {
		c.Interval = 15 * time.Second
	}
	if c.RetryInterval <= 0 {
		c.RetryInterval = 30 * time.Minute
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 10
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 5
	}
}

// GoogleSheetsConfig holds credentials and target for Google Sheets API access.
type GoogleSheetsConfig struct {
	CredentialsJSON string // Service account JSON key content
	SpreadsheetID   string // Google Sheets document ID
	TabName         string // Sheet/tab name (empty = first sheet)
}

// PSASyncConfig controls the background PSA Google Sheets sync scheduler.
type PSASyncConfig struct {
	Enabled      bool
	Interval     time.Duration // how often to run sync (default: 24h)
	InitialDelay time.Duration // delay before first run (default: 5m)
	SyncHour     int           // hour (0-23 UTC) to schedule runs; -1 = use InitialDelay (default: 10)
}

// CardLadderConfig controls the Card Ladder value refresh scheduler.
type CardLadderConfig struct {
	Enabled     bool          // Enable CL refresh scheduler (default: false)
	Interval    time.Duration // How often to run refresh (default: 24h)
	RefreshHour int           // Hour (0-23 UTC) to schedule daily runs (default: 4)
}

// MarketMoversConfig controls the Market Movers value refresh scheduler.
type MarketMoversConfig struct {
	Enabled     bool // Enable MM refresh scheduler (default: true)
	RefreshHour int  // Hour (0-23 UTC) to schedule daily runs (default: 5)
}

// ApplyDefaults sets zero-valued fields to sensible defaults.
func (c *CardLadderConfig) ApplyDefaults() {
	if c.Interval <= 0 {
		c.Interval = 24 * time.Hour
	}
}

// ApplyDefaults sets zero-valued fields to sensible defaults.
func (c *PSASyncConfig) ApplyDefaults() {
	if c.Interval <= 0 {
		c.Interval = 24 * time.Hour
	}
	if c.InitialDelay <= 0 {
		c.InitialDelay = 5 * time.Minute
	}
}
