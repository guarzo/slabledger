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
	ListenAddr      string        // HTTP listen address (default: 127.0.0.1:8080)
	ReadTimeout     time.Duration // HTTP read timeout
	WriteTimeout    time.Duration // HTTP write timeout
	IdleTimeout     time.Duration // HTTP idle timeout
	ShutdownTimeout time.Duration // Graceful shutdown timeout
	BaseURL         string        // Public base URL for generating absolute links (optional)
	MediaDir        string        // Directory for media file storage (default: ./data/media)
}

// LoggingConfig controls logging behavior
type LoggingConfig struct {
	Level string // Log level: debug, info, warn, error
	JSON  bool   // Output JSON logs
}

// DatabaseConfig controls database storage
type DatabaseConfig struct {
	Path           string // Path to SQLite database file (default: data/slabledger.db)
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

// CacheWarmupConfig controls the card cache warmup scheduler
type CacheWarmupConfig struct {
	// Enable/disable warmup (default: true)
	Enabled bool
	// How often to run warmup (default: 24h)
	Interval time.Duration
	// Delay between GetCards calls to respect rate limits (default: 2s)
	RateLimitDelay time.Duration
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
	PSAToken           string // PSA_ACCESS_TOKEN - PSA cert lookup (comma-separated for rotation)
	PricingAPIKey      string // PRICING_API_KEY - Bearer token for pricing API auth
	AzureAIEndpoint    string // AZURE_AI_ENDPOINT - Azure AI Foundry endpoint URL
	AzureAIKey         string // AZURE_AI_API_KEY - Azure AI API key
	AzureAIDeployment  string // AZURE_AI_DEPLOYMENT - Model deployment name (default: gpt-5.4)
	SocialAIDeployment string // SOCIAL_AI_DEPLOYMENT - Separate model for social content (default: same as AzureAIDeployment)
	ImageAIDeployment  string // IMAGE_AI_DEPLOYMENT - Image generation model deployment name
	ImageAIQuality     string // IMAGE_AI_QUALITY - Image quality: low, medium, high (default: medium)
	ImageAIEnabled     bool   // IMAGE_AI_ENABLED - Enable AI background generation (default: false)
	DHEnterpriseKey    string // DH_ENTERPRISE_API_KEY - Bearer token for enterprise endpoints
	DHBaseURL          string // DH_API_BASE_URL
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
	Mode             ModeConfig
	Cache            CacheConfig
	Server           ServerConfig
	Logging          LoggingConfig
	Database         DatabaseConfig
	Maintenance      MaintenanceConfig
	Auth             AuthConfig
	PriceRefresh     PriceRefreshConfig
	CacheWarmup      CacheWarmupConfig
	SessionCleanup   SessionCleanupConfig
	InventoryRefresh InventoryRefreshConfig
	SnapshotEnrich   SnapshotEnrichConfig
	SnapshotHistory  SnapshotHistoryConfig
	AdvisorRefresh   AdvisorRefreshConfig
	SocialContent    SocialContentConfig
	SocialPublish    SocialPublishConfig
	MetricsPoll      MetricsPollConfig
	PicksRefresh     PicksRefreshConfig
	CardLadder       CardLadderConfig
	MarketMovers     MarketMoversConfig
	GoogleSheets     GoogleSheetsConfig
	PSASync          PSASyncConfig
	DH               DHConfig
	Adapters         AdapterConfig
}

// AdvisorRefreshConfig controls the background AI advisor analysis scheduler.
type AdvisorRefreshConfig struct {
	Enabled       bool
	Interval      time.Duration // how often to run analysis (default: 24h)
	InitialDelay  time.Duration // delay before first run (default: 2m)
	RefreshHour   int           // hour (0-23 UTC) to schedule runs; -1 = use InitialDelay (default: 4)
	MaxToolRounds int           // max LLM tool-calling rounds per analysis (default: 5)
}

// SnapshotHistoryConfig controls daily archival of market snapshots.
type SnapshotHistoryConfig struct {
	Enabled  bool
	Interval time.Duration // how often to archive (default: 24h)
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

// SocialContentConfig controls the background social content generation scheduler.
type SocialContentConfig struct {
	Enabled      bool
	Interval     time.Duration // how often to run detection (default: 24h)
	InitialDelay time.Duration // delay before first run (default: 5m)
	ContentHour  int           // hour (0-23 UTC) to schedule runs; -1 = use InitialDelay (default: 5)
}

// SocialPublishConfig controls the automated Instagram publish scheduler.
type SocialPublishConfig struct {
	// RenderServiceURL is the base URL of the Puppeteer render sidecar.
	// Empty = auto-publishing disabled; the scheduler is not started.
	RenderServiceURL string
	// StartHour is the earliest hour (0–23, server local time) for auto-publishing.
	StartHour int
	// EndHour is the latest hour (exclusive) for auto-publishing.
	EndHour int
	// IntervalMinutes controls how often the publish scheduler ticks.
	IntervalMinutes int
	// MaxDaily is the maximum number of posts auto-published per calendar day.
	MaxDaily int
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

// MetricsPollConfig controls the Instagram metrics polling scheduler.
type MetricsPollConfig struct {
	Enabled  bool
	Interval time.Duration // how often to poll (default: 6h)
	MaxAge   time.Duration // stop polling posts older than this (default: 168h / 7 days)
}

// PicksRefreshConfig controls the daily AI picks generation scheduler.
type PicksRefreshConfig struct {
	Enabled     bool
	Interval    time.Duration // how often to run (default: 24h)
	ContentHour int           // hour (0-23 UTC) to schedule runs; -1 = use default delay (default: 3)
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
func (c *PicksRefreshConfig) ApplyDefaults() {
	if c.Interval <= 0 {
		c.Interval = 24 * time.Hour
	}
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
