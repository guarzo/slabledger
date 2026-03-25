// Package storage defines interfaces for caching and data persistence.
//
// The core interface is Cache, which provides a generic key-value storage
// abstraction with TTL support. Implementations handle different storage
// backends (memory, file, Redis, etc.) transparently.
//
// # Usage
//
//	cache := filecache.New(config)
//	defer cache.Close()
//
//	// Store data with TTL
//	err := cache.Set(ctx, "key", data, 1*time.Hour)
//
//	// Retrieve data
//	var result MyType
//	found, err := cache.Get(ctx, "key", &result)
//
// # Cache Strategy
//
// The application uses a multi-layer cache hierarchy:
//
//  1. In-memory cache (fastest, session-scoped)
//  2. Persistent file cache (for immutable data like sets)
//  3. API fetch (slowest, only when needed)
//
// # Performance
//
// Type-safe cache implementations provide 50-70% better performance than
// generic alternatives. See ADR-002 (Cache Performance Optimization).
//
// # TTL Guidelines
//
//   - Immutable data (card sets): No expiration
//   - Price data: 4-24 hours
//   - Population data: 24-48 hours
//   - Market trends: 1-6 hours
package storage
