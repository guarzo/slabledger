// Package testutil provides utilities for testing, including test data factories,
// configuration helpers, and environment variable management.
//
// This package helps standardize test setup across the codebase by providing
// common test utilities and mock data generation capabilities.
package testutil

import (
	"os"
)

// GetTestToken returns a test token from environment variable or default value.
// This helper function provides consistent token retrieval across all tests.
//
// Parameters:
//   - envVar: Name of the environment variable to check
//   - defaultValue: Value to return if environment variable is not set
//
// Returns:
//   - string: Token from environment or default value
//
// Example:
//
//	token := GetTestToken("MY_TEST_TOKEN", "default-value")
//	// Returns value of MY_TEST_TOKEN env var, or "default-value" if not set
func GetTestToken(envVar, defaultValue string) string {
	if token := os.Getenv(envVar); token != "" {
		return token
	}
	return defaultValue
}
