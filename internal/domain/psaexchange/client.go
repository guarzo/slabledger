package psaexchange

import "context"

// CatalogClient is the port the domain uses to talk to PSA-Exchange.
// Implementations live in adapters; tests use mocks under testutil/mocks.
type CatalogClient interface {
	// FetchCatalog returns the full /api/catalog response.
	FetchCatalog(ctx context.Context) (Catalog, error)

	// FetchCardLadder returns the per-cert CardLadder enrichment.
	FetchCardLadder(ctx context.Context, cert string) (CardLadder, error)

	// CategoryURL returns the public catalog URL for the given category.
	// Returns "" when no buyer token is configured.
	CategoryURL(category string) string
}
