package config

import (
	"os"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	// Verify sensible defaults
	if cfg.Mode.WebPort != 8081 {
		t.Errorf("expected default web port 8081, got %d", cfg.Mode.WebPort)
	}
	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Errorf("expected default read timeout 15s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected default log level 'info', got %s", cfg.Logging.Level)
	}
}

func TestFromEnv(t *testing.T) {
	// Save original env vars
	origLogLevel := os.Getenv("LOG_LEVEL")
	origListenAddr := os.Getenv("HTTP_LISTEN_ADDR")
	origRateLimit := os.Getenv("RATE_LIMIT_REQUESTS")

	// Clean up
	defer func() {
		_ = os.Setenv("LOG_LEVEL", origLogLevel)
		_ = os.Setenv("HTTP_LISTEN_ADDR", origListenAddr)
		_ = os.Setenv("RATE_LIMIT_REQUESTS", origRateLimit)
	}()

	// Set test env vars
	_ = os.Setenv("LOG_LEVEL", "debug")
	_ = os.Setenv("HTTP_LISTEN_ADDR", ":9090")
	_ = os.Setenv("RATE_LIMIT_REQUESTS", "200")

	cfg := FromEnv(Default())

	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level 'debug' from env, got %s", cfg.Logging.Level)
	}
	if cfg.Server.ListenAddr != ":9090" {
		t.Errorf("expected listen addr ':9090' from env, got %s", cfg.Server.ListenAddr)
	}
	if cfg.Mode.RateLimitRequests != 200 {
		t.Errorf("expected rate limit 200 from env, got %d", cfg.Mode.RateLimitRequests)
	}
}

func TestFromFlags(t *testing.T) {
	base := Default()
	args := []string{
		"--log-level", "debug",
	}

	cfg, err := FromFlags(base, args)
	if err != nil {
		t.Fatalf("FromFlags failed: %v", err)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level 'debug', got %s", cfg.Logging.Level)
	}
}

func TestFromFlags_WebMode(t *testing.T) {
	base := Default()
	args := []string{
		"--web",
		"--port", "3000",
	}

	cfg, err := FromFlags(base, args)
	if err != nil {
		t.Fatalf("FromFlags failed: %v", err)
	}

	if !cfg.Mode.WebMode {
		t.Error("expected web mode enabled")
	}
	if cfg.Mode.WebPort != 3000 {
		t.Errorf("expected web port 3000, got %d", cfg.Mode.WebPort)
	}
	if cfg.Server.ListenAddr != "0.0.0.0:3000" {
		t.Errorf("expected listen addr '0.0.0.0:3000', got %s", cfg.Server.ListenAddr)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := Default()
	cfg.Mode.WebPort = 99999

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for invalid port")
	}
}

func TestValidate_InvalidRateLimit(t *testing.T) {
	cfg := Default()
	cfg.Mode.RateLimitRequests = 0

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for invalid rate limit")
	}

	cfg.Mode.RateLimitRequests = 20000
	err = cfg.Validate()
	if err == nil {
		t.Error("expected validation error for rate limit too high")
	}
}

func TestLoad_FullFlow(t *testing.T) {
	// Set env var
	_ = os.Setenv("LOG_LEVEL", "debug")
	defer func() { _ = os.Unsetenv("LOG_LEVEL") }()

	// Provide CLI args
	args := []string{}

	cfg, err := Load(args)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check env var applied
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level 'debug' from env, got %s", cfg.Logging.Level)
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"yes", true},
		{"on", true},
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"0", false},
		{"no", false},
		{"off", false},
		{"invalid", false}, // default
		{"", false},        // default
	}

	for _, tc := range tests {
		result := parseBool(tc.input, false)
		if result != tc.expected {
			t.Errorf("parseBool(%q) = %v, expected %v", tc.input, result, tc.expected)
		}
	}
}

func TestPrintConfig(t *testing.T) {
	// This is a smoke test - just ensure it doesn't panic
	cfg := Default()
	cfg.Mode.WebMode = true

	// Redirect stdout to avoid cluttering test output
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	cfg.PrintConfig()

	// Restore stdout
	_ = w.Close()
	os.Stdout = oldStdout
}

func TestPrintVersion(t *testing.T) {
	// This is a smoke test - just ensure it doesn't panic
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	PrintVersion()

	_ = w.Close()
	os.Stdout = oldStdout
}

func TestFromEnv_DurationParsing(t *testing.T) {
	_ = os.Setenv("HTTP_READ_TIMEOUT", "30s")
	_ = os.Setenv("HTTP_WRITE_TIMEOUT", "45s")
	defer func() {
		_ = os.Unsetenv("HTTP_READ_TIMEOUT")
		_ = os.Unsetenv("HTTP_WRITE_TIMEOUT")
	}()

	cfg := FromEnv(Default())

	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected read timeout 30s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 45*time.Second {
		t.Errorf("expected write timeout 45s, got %v", cfg.Server.WriteTimeout)
	}
}

func TestFromEnv_InvalidDuration(t *testing.T) {
	base := Default()
	originalTimeout := base.Server.ReadTimeout

	_ = os.Setenv("HTTP_READ_TIMEOUT", "invalid")
	defer func() { _ = os.Unsetenv("HTTP_READ_TIMEOUT") }()

	cfg := FromEnv(base)

	// Should keep default value on parse error
	if cfg.Server.ReadTimeout != originalTimeout {
		t.Errorf("expected timeout to remain at default %v on parse error, got %v",
			originalTimeout, cfg.Server.ReadTimeout)
	}
}

func TestDatabaseConfig(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		cfg := Default()

		if cfg.Database.Path != "data/slabledger.db" {
			t.Errorf("expected default database path 'data/slabledger.db', got %s", cfg.Database.Path)
		}
		if cfg.Database.MigrationsPath != "" {
			t.Errorf("expected default migrations path to be empty (use embedded), got %s", cfg.Database.MigrationsPath)
		}
	})

	t.Run("environment variable override", func(t *testing.T) {
		// Save original env vars
		origDBPath := os.Getenv("DATABASE_PATH")
		origMigPath := os.Getenv("MIGRATIONS_PATH")

		// Clean up
		defer func() {
			if origDBPath != "" {
				_ = os.Setenv("DATABASE_PATH", origDBPath)
			} else {
				_ = os.Unsetenv("DATABASE_PATH")
			}
			if origMigPath != "" {
				_ = os.Setenv("MIGRATIONS_PATH", origMigPath)
			} else {
				_ = os.Unsetenv("MIGRATIONS_PATH")
			}
		}()

		// Set test env vars
		_ = os.Setenv("DATABASE_PATH", "/custom/path/test.db")
		_ = os.Setenv("MIGRATIONS_PATH", "/custom/migrations")

		cfg := FromEnv(Default())

		if cfg.Database.Path != "/custom/path/test.db" {
			t.Errorf("expected database path '/custom/path/test.db' from env, got %s", cfg.Database.Path)
		}
		if cfg.Database.MigrationsPath != "/custom/migrations" {
			t.Errorf("expected migrations path '/custom/migrations' from env, got %s", cfg.Database.MigrationsPath)
		}
	})

	t.Run("CLI flag override", func(t *testing.T) {
		args := []string{"--db-path", "/cli/database.db", "--migrations-path", "/cli/migrations"}

		cfg, err := FromFlags(Default(), args)
		if err != nil {
			t.Fatalf("unexpected error from FromFlags: %v", err)
		}

		if cfg.Database.Path != "/cli/database.db" {
			t.Errorf("expected database path '/cli/database.db' from CLI flag, got %s", cfg.Database.Path)
		}
		if cfg.Database.MigrationsPath != "/cli/migrations" {
			t.Errorf("expected migrations path '/cli/migrations' from CLI flag, got %s", cfg.Database.MigrationsPath)
		}
	})
}

func TestMaintenanceConfig(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		cfg := Default()

		if cfg.Maintenance.AccessLogRetentionDays != 30 {
			t.Errorf("expected default retention days 30, got %d", cfg.Maintenance.AccessLogRetentionDays)
		}
		if cfg.Maintenance.AccessLogCleanupInterval != 24*time.Hour {
			t.Errorf("expected default cleanup interval 24h, got %v", cfg.Maintenance.AccessLogCleanupInterval)
		}
		if !cfg.Maintenance.AccessLogCleanupEnabled {
			t.Error("expected cleanup to be enabled by default")
		}
	})

	t.Run("environment variable override", func(t *testing.T) {
		// Save original env vars
		origRetention := os.Getenv("ACCESS_LOG_RETENTION_DAYS")
		origInterval := os.Getenv("ACCESS_LOG_CLEANUP_INTERVAL")
		origEnabled := os.Getenv("ACCESS_LOG_CLEANUP_ENABLED")

		// Clean up
		defer func() {
			if origRetention != "" {
				_ = os.Setenv("ACCESS_LOG_RETENTION_DAYS", origRetention)
			} else {
				_ = os.Unsetenv("ACCESS_LOG_RETENTION_DAYS")
			}
			if origInterval != "" {
				_ = os.Setenv("ACCESS_LOG_CLEANUP_INTERVAL", origInterval)
			} else {
				_ = os.Unsetenv("ACCESS_LOG_CLEANUP_INTERVAL")
			}
			if origEnabled != "" {
				_ = os.Setenv("ACCESS_LOG_CLEANUP_ENABLED", origEnabled)
			} else {
				_ = os.Unsetenv("ACCESS_LOG_CLEANUP_ENABLED")
			}
		}()

		// Set test env vars
		_ = os.Setenv("ACCESS_LOG_RETENTION_DAYS", "90")
		_ = os.Setenv("ACCESS_LOG_CLEANUP_INTERVAL", "12h")
		_ = os.Setenv("ACCESS_LOG_CLEANUP_ENABLED", "false")

		cfg := FromEnv(Default())

		if cfg.Maintenance.AccessLogRetentionDays != 90 {
			t.Errorf("expected retention days 90 from env, got %d", cfg.Maintenance.AccessLogRetentionDays)
		}
		if cfg.Maintenance.AccessLogCleanupInterval != 12*time.Hour {
			t.Errorf("expected cleanup interval 12h from env, got %v", cfg.Maintenance.AccessLogCleanupInterval)
		}
		if cfg.Maintenance.AccessLogCleanupEnabled {
			t.Error("expected cleanup to be disabled from env")
		}
	})
}
