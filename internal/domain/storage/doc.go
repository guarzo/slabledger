// Package storage defines interfaces for caching and data persistence.
//
// The core interface is Cache, which provides a generic key-value storage
// abstraction with TTL support. Implementations handle different storage
// backends transparently.
//
// # Usage
//
//	err := cache.Set(ctx, "key", data, 1*time.Hour)
//
//	var result MyType
//	found, err := cache.Get(ctx, "key", &result)
//
// # TTL Guidelines
//
//   - Immutable data (card sets): No expiration
//   - Price data: 4-24 hours
//   - Population data: 24-48 hours
//   - Market trends: 1-6 hours
package storage
