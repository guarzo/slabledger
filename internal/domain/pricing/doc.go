// Package pricing defines interfaces for fetching Pokemon card prices from various sources.
//
// The core interface is PriceProvider, which abstracts price fetching from different
// marketplaces and data sources. The primary implementation is the DH price provider.
//
// # Architecture
//
// The package follows the Dependency Inversion Principle: domain logic depends on
// interfaces defined here, while concrete implementations live in adapters/clients.
//
// # Usage
//
// Price providers are typically injected into domain services:
//
//	priceProvider := dhprice.New(client, idResolver)
//
// # Key Interfaces
//
//   - PriceProvider: Fetches graded and raw card prices, provides lookup and statistics
//
// # Price Storage
//
// All prices are stored in cents (int64) for precision and to avoid floating-point errors.
// Use the helper functions ToCents() and ToDollars() for conversions.
//
// See ADR-001 (Hexagonal Architecture) for architectural patterns.
package pricing
