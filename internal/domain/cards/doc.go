// Package cards defines interfaces for fetching Pokemon card metadata and set information.
//
// The core interface is CardProvider, which abstracts card data fetching from sources
// like TCGdex.dev. The package handles card search, set listing, and variant matching.
//
// # Architecture
//
// Following hexagonal architecture, this package defines the interfaces (ports) that
// domain logic depends on, while concrete implementations live in adapters/clients.
//
// # Key Features
//
//   - Set enumeration and metadata
//   - Card search with fuzzy matching
//   - Variant detection (foil, reverse foil, etc.)
//   - Set-specific card listings
//
// # Usage
//
//	cardProvider := tcgdex.NewTCGdex(cache, logger)
//	sets, err := cardProvider.ListAllSets(ctx)
//	cards, err := cardProvider.GetCards(ctx, "sv8")
//
// # Caching
//
// Card data is immutable once a set is released, so implementations should use
// persistent caching to minimize API calls. See the tcgdex adapter for an
// example of persistent set caching.
//
// See ADR-001 (Hexagonal Architecture) for architectural patterns.
package cards
