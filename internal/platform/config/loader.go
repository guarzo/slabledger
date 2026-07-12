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

// envString reads an environment variable and assigns it to target if present.
func envString(key string, target *string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}

// envInt reads an environment variable, parses it as an int, and assigns it to target if valid.
func envInt(key string, target *int) {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			*target = i
		}
	}
}

// envIntPositive reads an environment variable, parses it as a positive int, and assigns it to target if valid.
func envIntPositive(key string, target *int) {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			*target = i
		}
	}
}

// envIntRange reads an environment variable, parses it as an int within [minVal, maxVal], and assigns it to target if valid.
func envIntRange(key string, target *int, minVal, maxVal int) {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i >= minVal && i <= maxVal {
			*target = i
		}
	}
}

// envDuration reads an environment variable, parses it as a duration, and assigns it to target if valid.
func envDuration(key string, target *time.Duration) {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			*target = d
		}
	}
}

// envDurationPositive reads an environment variable, parses it as a positive duration, and assigns it to target if valid.
func envDurationPositive(key string, target *time.Duration) {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			*target = d
		}
	}
}

// envBool reads an environment variable, parses it as a boolean with a default, and assigns it to target.
func envBool(key string, target *bool, defaultVal bool) {
	if v := os.Getenv(key); v != "" {
		*target = parseBool(v, defaultVal)
	}
}

// envFloat reads an environment variable, parses it as a float64, and assigns it to target if valid.
func envFloat(key string, target *float64) {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			*target = parsed
		}
	}
}

// FromEnv overlays environment variables onto the base config.
// Uses the env* helper functions above which provide type-safe, validated parsing.
// FromFlags reads configuration from CLI flags using the standard flag package.
// Both set the same Config fields; FromFlags takes precedence (applied after FromEnv).
func FromEnv(base Config) Config {
	cfg := base

	// Logging
	envString("LOG_LEVEL", &cfg.Logging.Level)
	envBool("LOG_JSON", &cfg.Logging.JSON, false)

	// Server
	envString("HTTP_LISTEN_ADDR", &cfg.Server.ListenAddr)
	envDuration("HTTP_READ_TIMEOUT", &cfg.Server.ReadTimeout)
	envDuration("HTTP_WRITE_TIMEOUT", &cfg.Server.WriteTimeout)
	envDuration("HTTP_IDLE_TIMEOUT", &cfg.Server.IdleTimeout)
	if v := os.Getenv("SHUTDOWN_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Server.SchedulerShutdownTimeout = time.Duration(n) * time.Second
		}
	}

	// Rate limiting
	envInt("RATE_LIMIT_REQUESTS", &cfg.Mode.RateLimitRequests)
	envBool("RATE_LIMIT_TRUST_PROXY", &cfg.Mode.TrustProxy, false)

	// Database
	envString("DATABASE_URL", &cfg.Database.URL)
	envString("MIGRATIONS_PATH", &cfg.Database.MigrationsPath)

	// Maintenance
	envInt("ACCESS_LOG_RETENTION_DAYS", &cfg.Maintenance.AccessLogRetentionDays)
	envDuration("ACCESS_LOG_CLEANUP_INTERVAL", &cfg.Maintenance.AccessLogCleanupInterval)
	envBool("ACCESS_LOG_CLEANUP_ENABLED", &cfg.Maintenance.AccessLogCleanupEnabled, true)
	envBool("BACKFILL_IMAGES", &cfg.Maintenance.BackfillImages, false)

	// Auth
	cfg.Auth.EncryptionKey = os.Getenv("ENCRYPTION_KEY")
	if v := os.Getenv("ADMIN_EMAILS"); v != "" {
		emails := strings.Split(v, ",")
		for i, e := range emails {
			emails[i] = strings.TrimSpace(e)
		}
		cfg.Auth.AdminEmails = emails
	}

	// Price refresh scheduler
	envDuration("PRICE_REFRESH_INTERVAL", &cfg.PriceRefresh.RefreshInterval)
	envIntPositive("PRICE_BATCH_SIZE", &cfg.PriceRefresh.BatchSize)
	envDuration("PRICE_BATCH_DELAY", &cfg.PriceRefresh.BatchDelay)
	envIntPositive("PRICE_MAX_BURST_CALLS", &cfg.PriceRefresh.MaxBurstCalls)
	envIntPositive("PRICE_MAX_CALLS_PER_HOUR", &cfg.PriceRefresh.MaxCallsPerHour)
	envDurationPositive("PRICE_BURST_PAUSE_DURATION", &cfg.PriceRefresh.BurstPauseDuration)
	envBool("PRICE_REFRESH_ENABLED", &cfg.PriceRefresh.Enabled, true)

	// Session cleanup
	envBool("SESSION_CLEANUP_ENABLED", &cfg.SessionCleanup.Enabled, true)
	envDuration("SESSION_CLEANUP_INTERVAL", &cfg.SessionCleanup.Interval)

	// Inventory refresh scheduler
	envBool("INVENTORY_REFRESH_ENABLED", &cfg.InventoryRefresh.Enabled, true)
	envDurationPositive("INVENTORY_REFRESH_INTERVAL", &cfg.InventoryRefresh.Interval)
	envDurationPositive("INVENTORY_REFRESH_STALE_THRESHOLD", &cfg.InventoryRefresh.StaleThreshold)
	envIntPositive("INVENTORY_REFRESH_BATCH_SIZE", &cfg.InventoryRefresh.BatchSize)
	envDurationPositive("INVENTORY_REFRESH_BATCH_DELAY", &cfg.InventoryRefresh.BatchDelay)

	// Snapshot enrichment scheduler
	envBool("SNAPSHOT_ENRICH_ENABLED", &cfg.SnapshotEnrich.Enabled, true)
	envDurationPositive("SNAPSHOT_ENRICH_INTERVAL", &cfg.SnapshotEnrich.Interval)
	envIntPositive("SNAPSHOT_ENRICH_BATCH_SIZE", &cfg.SnapshotEnrich.BatchSize)
	envDurationPositive("SNAPSHOT_ENRICH_RETRY_INTERVAL", &cfg.SnapshotEnrich.RetryInterval)
	envIntPositive("SNAPSHOT_ENRICH_MAX_RETRIES", &cfg.SnapshotEnrich.MaxRetries)

	// Advisor service
	envIntPositive("ADVISOR_MAX_TOOL_ROUNDS", &cfg.AdvisorRefresh.MaxToolRounds)

	// Card Ladder scheduler
	envBool("CARDLADDER_REFRESH_ENABLED", &cfg.CardLadder.Enabled, false)
	envIntRange("CARDLADDER_REFRESH_HOUR", &cfg.CardLadder.RefreshHour, 0, 23)

	// Market Movers scheduler
	envBool("MM_REFRESH_ENABLED", &cfg.MarketMovers.Enabled, true)
	envIntRange("MM_REFRESH_HOUR", &cfg.MarketMovers.RefreshHour, 0, 23)

	// Adapter API keys and tokens
	cfg.Adapters.PSAToken = os.Getenv("PSA_ACCESS_TOKEN")
	cfg.Adapters.PricingAPIKey = os.Getenv("PRICING_API_KEY")
	cfg.Adapters.GoogleOAuthEnv = os.Getenv("GOOGLE_OAUTH_ENV")
	cfg.Adapters.LocalAPIToken = os.Getenv("LOCAL_API_TOKEN")
	cfg.Adapters.AzureAIEndpoint = os.Getenv("AZURE_AI_ENDPOINT")
	cfg.Adapters.AzureAIKey = os.Getenv("AZURE_AI_API_KEY")
	cfg.Adapters.AzureAIDeployment = os.Getenv("AZURE_AI_DEPLOYMENT")
	if cfg.Adapters.AzureAIDeployment == "" {
		cfg.Adapters.AzureAIDeployment = "gpt-5.4"
	}
	cfg.Adapters.DHEnterpriseKey = os.Getenv("DH_ENTERPRISE_API_KEY")
	envString("DH_API_BASE_URL", &cfg.Adapters.DHBaseURL)
	cfg.Adapters.PSAExchangeToken = os.Getenv("PSA_EXCHANGE_TOKEN")
	cfg.Adapters.PSAExchangeBuyerCID = os.Getenv("PSA_EXCHANGE_BUYER_CID")
	envString("PSA_EXCHANGE_BASE_URL", &cfg.Adapters.PSAExchangeBaseURL)
	envIntPositive("PSA_EXCHANGE_HIGH_LIQ_VELOCITY", &cfg.Adapters.PSAExchangeHighLiqVelocity)
	envIntPositive("PSA_EXCHANGE_HIGH_LIQ_CONFIDENCE", &cfg.Adapters.PSAExchangeHighLiqConfidence)
	envFloat("PSA_EXCHANGE_HIGH_LIQ_OFFER_PCT", &cfg.Adapters.PSAExchangeHighLiqOfferPct)
	envFloat("PSA_EXCHANGE_DEFAULT_OFFER_PCT", &cfg.Adapters.PSAExchangeDefaultOfferPct)
	envIntPositive("PSA_EXCHANGE_MIN_CONFIDENCE", &cfg.Adapters.PSAExchangeMinConfidence)
	envIntPositive("PSA_EXCHANGE_MIN_QUARTER_VELOCITY", &cfg.Adapters.PSAExchangeMinQuarterVelocity)
	envDurationPositive("AZURE_AI_TIMEOUT", &cfg.Adapters.AzureAICompletionTimeout)
	envIntPositive("DH_CACHE_TTL_HOURS", &cfg.DH.CacheTTLHours)
	envIntPositive("DH_RATE_LIMIT_RPS", &cfg.DH.RateLimitRPS)
	cfg.DH.Enabled = parseBool(os.Getenv("DH_ENABLED"), cfg.DH.Enabled)
	envDuration("DH_ORDERS_POLL_INTERVAL", &cfg.DH.OrdersPollInterval)
	envDuration("DH_INVENTORY_POLL_INTERVAL", &cfg.DH.InventoryPollInterval)
	envDuration("DH_PUSH_INTERVAL", &cfg.DH.PushInterval)

	// DH demand analytics refresh (niche-opportunity leaderboard cache).
	envBool("DH_ANALYTICS_REFRESH_ENABLED", &cfg.DHAnalyticsRefresh.Enabled, true)
	envIntRange("DH_ANALYTICS_REFRESH_HOUR", &cfg.DHAnalyticsRefresh.RefreshHour, 0, 23)
	envString("DH_ANALYTICS_REFRESH_WINDOW", &cfg.DHAnalyticsRefresh.Window)

	// DH inventory reconciliation scheduler.
	envBool("DH_RECONCILE_ENABLED", &cfg.DHReconcile.Enabled, true)
	envDurationPositive("DH_RECONCILE_INTERVAL", &cfg.DHReconcile.Interval)

	// DH sold-status reconciler scheduler.
	envBool("DH_SOLD_RECONCILER_ENABLED", &cfg.DHSoldReconciler.Enabled, true)
	envDurationPositive("DH_SOLD_RECONCILER_INTERVAL", &cfg.DHSoldReconciler.Interval)

	// DH price-sync scheduler (reconciles reviewed_price_cents vs dh_listing_price_cents)
	envBool("DH_PRICE_SYNC_ENABLED", &cfg.DHPriceSync.Enabled, true)
	envDurationPositive("DH_PRICE_SYNC_INTERVAL", &cfg.DHPriceSync.Interval)

	// PSA Buyer Campaign Manager portal credentials (headless-login harvester)
	cfg.PSAPortal.Email = os.Getenv("PSA_PORTAL_EMAIL")
	cfg.PSAPortal.Password = os.Getenv("PSA_PORTAL_PASSWORD")
	// Default: enabled when credentials are present (harvester app). The
	// reader-only main app holds no credentials, so PSA_PORTAL_ENABLED lets it
	// turn on the token reader explicitly.
	cfg.PSAPortal.Enabled = cfg.PSAPortal.Email != "" && cfg.PSAPortal.Password != ""
	envBool("PSA_PORTAL_ENABLED", &cfg.PSAPortal.Enabled, cfg.PSAPortal.Enabled)

	// PSA sync scheduler
	envBool("PSA_SYNC_ENABLED", &cfg.PSASync.Enabled, false)
	envIntRange("PSA_SYNC_HOUR", &cfg.PSASync.SyncHour, -1, 23)

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
	fs.StringVar(&cfg.Database.URL, "database-url", cfg.Database.URL, "PostgreSQL connection URL")
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
