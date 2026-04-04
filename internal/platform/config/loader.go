package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Load loads configuration from defaults, environment, and CLI flags
func Load(args []string) (Config, error) {
	// Start with defaults
	cfg := Default()

	// Overlay environment variables
	cfg = FromEnv(cfg)

	// Overlay CLI flags
	var err error
	cfg, err = FromFlags(cfg, args)
	if err != nil {
		return cfg, err
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	// Create required directories
	if err := EnsureDirectories(cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// FromEnv overlays environment variables onto the base config
func FromEnv(base Config) Config {
	cfg := base

	// Logging
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("LOG_JSON"); v != "" {
		cfg.Logging.JSON = parseBool(v, false)
	}

	// Server
	if v := os.Getenv("HTTP_LISTEN_ADDR"); v != "" {
		cfg.Server.ListenAddr = v
	}
	if v := os.Getenv("HTTP_READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ReadTimeout = d
		}
	}
	if v := os.Getenv("HTTP_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.WriteTimeout = d
		}
	}
	if v := os.Getenv("HTTP_IDLE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.IdleTimeout = d
		}
	}

	// Rate limiting
	if v := os.Getenv("RATE_LIMIT_REQUESTS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Mode.RateLimitRequests = i
		}
	}
	if v := os.Getenv("RATE_LIMIT_TRUST_PROXY"); v != "" {
		cfg.Mode.TrustProxy = parseBool(v, false)
	}

	// Database configuration
	if v := os.Getenv("DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("MIGRATIONS_PATH"); v != "" {
		cfg.Database.MigrationsPath = v
	}

	// Maintenance configuration
	if v := os.Getenv("ACCESS_LOG_RETENTION_DAYS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Maintenance.AccessLogRetentionDays = i
		}
	}
	if v := os.Getenv("ACCESS_LOG_CLEANUP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Maintenance.AccessLogCleanupInterval = d
		}
	}
	if v := os.Getenv("ACCESS_LOG_CLEANUP_ENABLED"); v != "" {
		cfg.Maintenance.AccessLogCleanupEnabled = parseBool(v, true)
	}

	// Auth configuration
	cfg.Auth.EncryptionKey = os.Getenv("ENCRYPTION_KEY")
	if v := os.Getenv("ADMIN_EMAILS"); v != "" {
		emails := strings.Split(v, ",")
		for i, e := range emails {
			emails[i] = strings.TrimSpace(e)
		}
		cfg.Auth.AdminEmails = emails
	}

	// Price refresh scheduler configuration
	if v := os.Getenv("PRICE_REFRESH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.PriceRefresh.RefreshInterval = d
		}
	}
	if v := os.Getenv("PRICE_BATCH_SIZE"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.PriceRefresh.BatchSize = i
		}
	}
	if v := os.Getenv("PRICE_BATCH_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.PriceRefresh.BatchDelay = d
		}
	}
	if v := os.Getenv("PRICE_MAX_BURST_CALLS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.PriceRefresh.MaxBurstCalls = i
		}
	}
	if v := os.Getenv("PRICE_MAX_CALLS_PER_HOUR"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.PriceRefresh.MaxCallsPerHour = i
		}
	}
	if v := os.Getenv("PRICE_BURST_PAUSE_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.PriceRefresh.BurstPauseDuration = d
		}
	}

	// Cache warmup configuration
	if v := os.Getenv("CACHE_WARMUP_ENABLED"); v != "" {
		cfg.CacheWarmup.Enabled = parseBool(v, true)
	}
	if v := os.Getenv("CACHE_WARMUP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CacheWarmup.Interval = d
		}
	}
	if v := os.Getenv("CACHE_WARMUP_RATE_LIMIT_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CacheWarmup.RateLimitDelay = d
		}
	}

	// Price refresh scheduler enabled
	if v := os.Getenv("PRICE_REFRESH_ENABLED"); v != "" {
		cfg.PriceRefresh.Enabled = parseBool(v, true)
	}

	// Session cleanup configuration
	if v := os.Getenv("SESSION_CLEANUP_ENABLED"); v != "" {
		cfg.SessionCleanup.Enabled = parseBool(v, true)
	}
	if v := os.Getenv("SESSION_CLEANUP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.SessionCleanup.Interval = d
		}
	}

	// Fusion provider configuration
	if v := os.Getenv("FUSION_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Fusion.CacheTTL = d
		}
	}
	if v := os.Getenv("FUSION_PRICECHARTING_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Fusion.PriceChartingTimeout = d
		}
	}
	if v := os.Getenv("FUSION_SECONDARY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Fusion.SecondarySourceTimeout = d
		}
	}

	// CardHedger scheduler configuration
	if v := os.Getenv("CARD_HEDGER_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CardHedger.PollInterval = d
		}
	}
	if v := os.Getenv("CARD_HEDGER_BATCH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CardHedger.BatchInterval = d
		}
	}
	if v := os.Getenv("CARD_HEDGER_MAX_CARDS_PER_RUN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.CardHedger.MaxCardsPerRun = n
		}
	}
	if v := os.Getenv("CARD_HEDGER_ENABLED"); v != "" {
		cfg.CardHedger.Enabled = parseBool(v, true)
	}

	// JustTCG scheduler configuration
	if v := os.Getenv("JUSTTCG_DAILY_BUDGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.JustTCG.DailyBudget = n
		}
	}
	if v := os.Getenv("JUSTTCG_RATE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.JustTCG.RateInterval = d
		}
	}
	if v := os.Getenv("JUSTTCG_RUN_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.JustTCG.RunInterval = d
		}
	}
	if v := os.Getenv("JUSTTCG_REFRESH_ENABLED"); v != "" {
		cfg.JustTCG.Enabled = parseBool(v, true)
	}

	// Inventory refresh scheduler configuration
	if v := os.Getenv("INVENTORY_REFRESH_ENABLED"); v != "" {
		cfg.InventoryRefresh.Enabled = parseBool(v, true)
	}
	if v := os.Getenv("INVENTORY_REFRESH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.InventoryRefresh.Interval = d
		}
	}
	if v := os.Getenv("INVENTORY_REFRESH_STALE_THRESHOLD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.InventoryRefresh.StaleThreshold = d
		}
	}
	if v := os.Getenv("INVENTORY_REFRESH_BATCH_SIZE"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.InventoryRefresh.BatchSize = i
		}
	}
	if v := os.Getenv("INVENTORY_REFRESH_BATCH_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.InventoryRefresh.BatchDelay = d
		}
	}

	// Snapshot enrichment scheduler configuration
	if v := os.Getenv("SNAPSHOT_ENRICH_ENABLED"); v != "" {
		cfg.SnapshotEnrich.Enabled = parseBool(v, true)
	}
	if v := os.Getenv("SNAPSHOT_ENRICH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.SnapshotEnrich.Interval = d
		}
	}
	if v := os.Getenv("SNAPSHOT_ENRICH_BATCH_SIZE"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.SnapshotEnrich.BatchSize = i
		}
	}
	if v := os.Getenv("SNAPSHOT_ENRICH_RETRY_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.SnapshotEnrich.RetryInterval = d
		}
	}
	if v := os.Getenv("SNAPSHOT_ENRICH_MAX_RETRIES"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.SnapshotEnrich.MaxRetries = i
		}
	}

	// Snapshot history scheduler configuration
	if v := os.Getenv("SNAPSHOT_HISTORY_ENABLED"); v != "" {
		cfg.SnapshotHistory.Enabled = parseBool(v, true)
	}
	if v := os.Getenv("SNAPSHOT_HISTORY_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.SnapshotHistory.Interval = d
		}
	}

	// Advisor refresh scheduler
	if v := os.Getenv("ADVISOR_REFRESH_ENABLED"); v != "" {
		cfg.AdvisorRefresh.Enabled = parseBool(v, true)
	}
	if v := os.Getenv("ADVISOR_REFRESH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.AdvisorRefresh.Interval = d
		}
	}
	if v := os.Getenv("ADVISOR_REFRESH_INITIAL_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			cfg.AdvisorRefresh.InitialDelay = d
		}
	}
	if v := os.Getenv("ADVISOR_REFRESH_HOUR"); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h >= -1 && h <= 23 {
			cfg.AdvisorRefresh.RefreshHour = h
		}
	}
	if v := os.Getenv("ADVISOR_MAX_TOOL_ROUNDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.AdvisorRefresh.MaxToolRounds = n
		}
	}

	// Social content scheduler
	if v := os.Getenv("SOCIAL_CONTENT_ENABLED"); v != "" {
		cfg.SocialContent.Enabled = parseBool(v, false)
	}
	if v := os.Getenv("SOCIAL_CONTENT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.SocialContent.Interval = d
		}
	}
	if v := os.Getenv("SOCIAL_CONTENT_INITIAL_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.SocialContent.InitialDelay = d
		}
	}
	if v := os.Getenv("SOCIAL_CONTENT_HOUR"); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h >= -1 && h <= 23 {
			cfg.SocialContent.ContentHour = h
		}
	}

	// Picks refresh scheduler
	if v := os.Getenv("PICKS_REFRESH_ENABLED"); v != "" {
		cfg.PicksRefresh.Enabled = parseBool(v, true)
	}
	if v := os.Getenv("PICKS_REFRESH_HOUR"); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h >= -1 && h <= 23 {
			cfg.PicksRefresh.ContentHour = h
		}
	}

	// Card Ladder scheduler configuration
	if v := os.Getenv("CARDLADDER_REFRESH_ENABLED"); v != "" {
		cfg.CardLadder.Enabled = parseBool(v, false)
	}
	if v := os.Getenv("CARDLADDER_REFRESH_HOUR"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i >= -1 && i <= 23 {
			cfg.CardLadder.RefreshHour = i
		}
	}

	// Adapter API keys and tokens
	cfg.Adapters.PriceChartingToken = os.Getenv("PRICECHARTING_TOKEN")
	cfg.Adapters.CardHedgerKey = os.Getenv("CARD_HEDGER_API_KEY")
	cfg.Adapters.CardHedgerClientID = os.Getenv("CARD_HEDGER_CLIENT_ID")
	cfg.Adapters.PSAToken = os.Getenv("PSA_ACCESS_TOKEN")
	cfg.Adapters.PSAImageToken = os.Getenv("PAO_API")
	cfg.Adapters.PricingAPIKey = os.Getenv("PRICING_API_KEY")
	cfg.Adapters.AzureAIEndpoint = os.Getenv("AZURE_AI_ENDPOINT")
	cfg.Adapters.AzureAIKey = os.Getenv("AZURE_AI_API_KEY")
	cfg.Adapters.AzureAIDeployment = os.Getenv("AZURE_AI_DEPLOYMENT")
	if cfg.Adapters.AzureAIDeployment == "" {
		cfg.Adapters.AzureAIDeployment = "gpt-5.4"
	}
	cfg.Adapters.SocialAIDeployment = os.Getenv("SOCIAL_AI_DEPLOYMENT")
	if cfg.Adapters.SocialAIDeployment == "" {
		cfg.Adapters.SocialAIDeployment = cfg.Adapters.AzureAIDeployment
	}
	cfg.Adapters.ImageAIDeployment = os.Getenv("IMAGE_AI_DEPLOYMENT")
	cfg.Adapters.ImageAIQuality = os.Getenv("IMAGE_AI_QUALITY")
	if cfg.Adapters.ImageAIQuality == "" {
		cfg.Adapters.ImageAIQuality = "medium"
	}
	cfg.Adapters.ImageAIEnabled = parseBool(os.Getenv("IMAGE_AI_ENABLED"), false)
	cfg.Adapters.JustTCGKey = os.Getenv("JUSTTCG_API_KEY")
	cfg.Adapters.DHKey = os.Getenv("DH_INTEGRATION_API_KEY")
	cfg.Adapters.DHEnterpriseKey = os.Getenv("DH_ENTERPRISE_API_KEY")
	if v := os.Getenv("DH_API_BASE_URL"); v != "" {
		cfg.Adapters.DHBaseURL = v
	}
	if v := os.Getenv("DH_CACHE_TTL_HOURS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.DH.CacheTTLHours = i
		}
	}
	if v := os.Getenv("DH_RATE_LIMIT_RPS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			cfg.DH.RateLimitRPS = i
		}
	}
	cfg.DH.Enabled = parseBool(os.Getenv("DH_ENABLED"), cfg.DH.Enabled)
	if v := os.Getenv("DH_ORDERS_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.DH.OrdersPollInterval = d
		}
	}
	if v := os.Getenv("DH_INVENTORY_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.DH.InventoryPollInterval = d
		}
	}

	cfg.JustTCG.ApplyDefaults()

	// Auto-enable JustTCG if API key is present and not explicitly disabled
	if cfg.Adapters.JustTCGKey != "" && os.Getenv("JUSTTCG_REFRESH_ENABLED") == "" {
		cfg.JustTCG.Enabled = true
	}

	return cfg
}

// FromFlags overlays CLI flags onto the base config
func FromFlags(base Config, args []string) (Config, error) {
	cfg := base

	// Create a new FlagSet for testability
	fs := flag.NewFlagSet("slabledger", flag.ContinueOnError)

	// Mode flags
	fs.BoolVar(&cfg.Mode.WebMode, "web", cfg.Mode.WebMode, "Start web server mode instead of CLI")
	fs.IntVar(&cfg.Mode.WebPort, "port", cfg.Mode.WebPort, "Web server port")
	fs.IntVar(&cfg.Mode.RateLimitRequests, "rate-limit", cfg.Mode.RateLimitRequests, "API requests allowed per minute")
	fs.BoolVar(&cfg.Mode.TrustProxy, "trust-proxy", cfg.Mode.TrustProxy, "Trust X-Forwarded-For and X-Real-IP proxy headers")

	// Logging flags
	fs.StringVar(&cfg.Logging.Level, "log-level", cfg.Logging.Level, "Log level: debug, info, warn, error")
	fs.BoolVar(&cfg.Logging.JSON, "log-json", cfg.Logging.JSON, "Output JSON logs instead of text")

	// Cache flags
	fs.StringVar(&cfg.Cache.Path, "cache", cfg.Cache.Path, "Cache file location")

	// Database flags
	fs.StringVar(&cfg.Database.Path, "db-path", cfg.Database.Path, "Path to SQLite database file")
	fs.StringVar(&cfg.Database.MigrationsPath, "migrations-path", cfg.Database.MigrationsPath, "Path to migrations directory (empty = use embedded)")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}

	// Update server listen address from port if in web mode
	// Only override if not explicitly set via HTTP_LISTEN_ADDR env var
	if cfg.Mode.WebMode {
		if envAddr := os.Getenv("HTTP_LISTEN_ADDR"); envAddr != "" {
			cfg.Server.ListenAddr = envAddr
		} else {
			cfg.Server.ListenAddr = fmt.Sprintf("0.0.0.0:%d", cfg.Mode.WebPort)
		}
	}

	return cfg, nil
}

// parseBool parses a string as a boolean with a default fallback
func parseBool(s string, defaultVal bool) bool {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return defaultVal
	}
}
