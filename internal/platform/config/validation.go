package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

// hexStringPattern matches strings containing only hex characters (0-9, a-f, A-F)
var hexStringPattern = regexp.MustCompile(`^[0-9a-fA-F]+$`)

// Validate checks if the configuration is valid
func (cfg *Config) Validate() error {
	// Validate port range
	if cfg.Mode.WebPort < 1 || cfg.Mode.WebPort > 65535 {
		return apperrors.ConfigInvalid("port", cfg.Mode.WebPort, "must be between 1 and 65535")
	}

	// Validate rate limit configuration
	if cfg.Mode.RateLimitRequests < 1 {
		return apperrors.ConfigInvalid("rate-limit", cfg.Mode.RateLimitRequests, "must be at least 1")
	}
	if cfg.Mode.RateLimitRequests > 10000 {
		return apperrors.ConfigInvalid("rate-limit", cfg.Mode.RateLimitRequests, "cannot exceed 10000 requests per minute")
	}

	// Cache path syntax validation only — directory creation handled at startup
	if cfg.Cache.Path != "" {
		dir := filepath.Dir(cfg.Cache.Path)
		if dir != "" && dir != "." {
			if _, err := os.Stat(dir); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("cache directory path invalid: %w", err)
			}
		}
	}

	// Validate encryption key if provided (auth is enabled when key is set)
	if cfg.Auth.EncryptionKey != "" {
		key := cfg.Auth.EncryptionKey

		// Check if key is a hex string (only 0-9a-fA-F characters)
		if hexStringPattern.MatchString(key) {
			// Hex key: must be exactly 64 characters (representing 32 bytes)
			if len(key) != 64 {
				return fmt.Errorf("ENCRYPTION_KEY must be a 64-character hex string (32 bytes) when using hex format (current: %d chars); generate with: openssl rand -hex 32", len(key))
			}
			// Verify it actually decodes to 32 bytes
			decoded, err := hex.DecodeString(key)
			if err != nil {
				return fmt.Errorf("ENCRYPTION_KEY is not a valid hex string: %w", err)
			}
			if len(decoded) != 32 {
				return fmt.Errorf("ENCRYPTION_KEY hex string must decode to exactly 32 bytes for AES-256 (got %d bytes)", len(decoded))
			}
		} else {
			// Raw key: must be at least 32 bytes
			if len(key) < 32 {
				return fmt.Errorf("ENCRYPTION_KEY must be a 64-character hex string (32 bytes) or a raw key of at least 32 bytes (current: %d bytes); generate with: openssl rand -hex 32", len(key))
			}
		}

		// Warn if key looks weak (returned as error to prevent startup with weak key)
		if isWeakKey(key) {
			return fmt.Errorf("ENCRYPTION_KEY appears weak (all same characters or contains common patterns), use a cryptographically random key: openssl rand -hex 32")
		}
	}

	return nil
}

// EnsureDirectories creates required directories for the application.
func EnsureDirectories(cfg Config) error {
	if cfg.Cache.Path != "" {
		dir := filepath.Dir(cfg.Cache.Path)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("cache directory not writable: %w", err)
			}
		}
	}
	return nil
}

// isWeakKey detects obviously weak encryption keys
func isWeakKey(key string) bool {
	if len(key) < 16 {
		return true
	}

	b := []byte(key)

	// Check for all same byte (e.g., all zeros, all 0xFF)
	allSame := true
	for i := 1; i < len(b); i++ {
		if b[i] != b[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return true
	}

	// Check for simple ascending or descending byte sequence
	ascending := true
	descending := true
	for i := 1; i < len(b); i++ {
		if b[i] != b[i-1]+1 {
			ascending = false
		}
		if b[i] != b[i-1]-1 {
			descending = false
		}
	}
	if ascending || descending {
		return true
	}

	// Check for repeated short substrings (period 1-4)
	for period := 1; period <= 4 && period < len(b); period++ {
		repeated := true
		for i := period; i < len(b); i++ {
			if b[i] != b[i%period] {
				repeated = false
				break
			}
		}
		if repeated {
			return true
		}
	}

	// Check for known weak literals
	weakLiterals := []string{
		"1234567890123456",
		"abcdefghijklmnop",
		"passwordpassword",
	}
	lower := strings.ToLower(key)
	for _, lit := range weakLiterals {
		if len(lower) >= len(lit) && lower[:len(lit)] == lit {
			return true
		}
	}

	return false
}
