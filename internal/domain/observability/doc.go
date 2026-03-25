// Package observability defines interfaces for structured logging.
//
// The package provides vendor-agnostic abstractions for observability, following
// the Dependency Inversion Principle. Domain and adapter code depends on these
// interfaces, while concrete implementations are injected at runtime.
//
// # Key Interfaces
//
//   - Logger: Structured logging with context support
//
// # Logger Usage
//
//	logger.Info(ctx, "analysis started",
//	    observability.String("set", setName),
//	    observability.Int("cards", len(cards)))
//
//	logger.Error(ctx, "price fetch failed",
//	    observability.Err(err),
//	    observability.String("card", cardName))
//
// # Structured Logging
//
// Always use structured fields instead of string formatting:
//
//	// ✅ Good
//	logger.Info(ctx, "price fetched", String("card", name), Float64("price", price))
//
//	// ❌ Bad
//	logger.Info(ctx, fmt.Sprintf("Fetched price $%.2f for %s", price, name))
//
// See ADR-007 (Logging Strategy) for complete logging guidelines.
// See ADR-003 (Observability Abstraction) for architectural patterns.
package observability
