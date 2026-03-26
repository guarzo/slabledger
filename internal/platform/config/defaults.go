package config

import "time"

// Default returns a Config with sensible default values
func Default() Config {
	return Config{
		Mode: ModeConfig{
			WebPort:           8081,
			RateLimitRequests: 300, // Increased from 100 to handle concurrent UI requests
			TrustProxy:        false,
		},
		Cache: CacheConfig{
			Path: "data/cache.json",
		},
		Server: ServerConfig{
			ListenAddr:      "127.0.0.1:8080",
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    90 * time.Second, // Sized for pricing endpoint (~30s upstream calls)
			IdleTimeout:     60 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
		Logging: LoggingConfig{
			Level: "info",
			JSON:  false,
		},
		Database: DatabaseConfig{
			Path:           "data/slabledger.db",
			MigrationsPath: "", // Empty = use embedded migrations
		},
		Maintenance: MaintenanceConfig{
			AccessLogRetentionDays:   30,             // Keep 30 days of access logs
			AccessLogCleanupInterval: 24 * time.Hour, // Run cleanup daily
			AccessLogCleanupEnabled:  true,           // Enable by default
		},
		// PriceRefresh controls the background scheduler that keeps cached prices fresh.
		// Each source (PriceCharting, PokemonPrice, CardHedger) enforces its own per-request
		// rate limit at the client level. These settings add coarser-grained scheduling
		// controls to prevent sustained bursts from saturating upstream APIs.
		PriceRefresh: PriceRefreshConfig{
			RefreshInterval: 1 * time.Hour,
			// BatchSize: max cards to refresh per scheduler tick.
			BatchSize: 100,
			// BatchDelay: minimum pause between consecutive API calls within a batch,
			// providing a baseline inter-request gap on top of each client's own limiter.
			BatchDelay: 200 * time.Millisecond,
			// MaxBurstCalls: after this many consecutive calls, insert a longer pause
			// (BurstPauseDuration) to let upstream rate-limit windows reset.
			MaxBurstCalls: 50,
			// MaxCallsPerHour: hard ceiling on scheduler-driven calls per provider per hour.
			// Once reached, the provider is skipped until the next hour window.
			MaxCallsPerHour: 400,
			// BurstPauseDuration: cooldown inserted every MaxBurstCalls calls to avoid
			// sustained request storms.
			BurstPauseDuration: 10 * time.Second,
			Enabled:            true,
		},
		CacheWarmup: CacheWarmupConfig{
			Enabled:        true,
			Interval:       24 * time.Hour,
			RateLimitDelay: 2 * time.Second,
		},
		SessionCleanup: SessionCleanupConfig{
			Enabled:  true,
			Interval: 1 * time.Hour,
		},
		Fusion: FusionConfig{
			CacheTTL:               4 * time.Hour,
			PriceChartingTimeout:   30 * time.Second,
			SecondarySourceTimeout: 20 * time.Second,
		},
		CardHedger: CardHedgerSchedulerConfig{
			PollInterval:   1 * time.Hour,
			BatchInterval:  24 * time.Hour,
			MaxCardsPerRun: 200,
			Enabled:        true,
		},
		InventoryRefresh: InventoryRefreshConfig{
			Enabled:        true,
			Interval:       1 * time.Hour,
			StaleThreshold: 12 * time.Hour,
			BatchSize:      20,
			BatchDelay:     2 * time.Second,
		},
		SnapshotEnrich: SnapshotEnrichConfig{
			Enabled:       true,
			Interval:      15 * time.Second,
			RetryInterval: 30 * time.Minute,
			BatchSize:     3,
			MaxRetries:    5,
		},
		SnapshotHistory: SnapshotHistoryConfig{
			Enabled:  true,
			Interval: 24 * time.Hour,
		},
		AdvisorRefresh: AdvisorRefreshConfig{
			Enabled:      true,
			Interval:     24 * time.Hour,
			InitialDelay: 2 * time.Minute,
			RefreshHour:  4, // 4 AM UTC
		},
		SocialContent: SocialContentConfig{
			Enabled:      false, // disabled by default
			Interval:     24 * time.Hour,
			InitialDelay: 5 * time.Minute,
			ContentHour:  5, // 5 AM UTC
		},
	}
}
