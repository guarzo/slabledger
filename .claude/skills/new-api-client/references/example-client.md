# Example Client: PokemonPrice

The simplest real API client in the codebase. Located at `internal/adapters/clients/pokemonprice/`.

## File Structure

```
pokemonprice/
├── client.go      # Client struct, constructor, API methods (308 LOC)
├── types.go       # API response types (CardsResponse, CardPriceData, EbayGradeData)
└── client_test.go # Tests with httptest.NewServer
```

## Key Patterns

### Constructor with functional options
```go
type Client struct {
    apiKey      string
    httpClient  *httpx.Client
    rateLimiter *rate.Limiter
    logger      observability.Logger
}

type ClientOption func(*Client)

func WithLogger(logger observability.Logger) ClientOption {
    return func(c *Client) { c.logger = logger }
}

func NewClient(apiKey string, opts ...ClientOption) *Client {
    config := httpx.DefaultConfig("PokemonPriceTracker")
    config.DefaultTimeout = 15 * time.Second
    c := &Client{
        apiKey:      apiKey,
        httpClient:  httpx.NewClient(config),
        rateLimiter: rate.NewLimiter(rate.Limit(8), 4), // 8/sec, burst 4
    }
    for _, opt := range opts {
        opt(c)
    }
    return c
}
```

### Available() for graceful degradation
```go
func (c *Client) Available() bool { return c.apiKey != "" }
```
When the API key isn't configured, the client reports itself as unavailable. The fusion engine and schedulers check this before making calls.

### Rate limiting with golang.org/x/time/rate
```go
if err := c.rateLimiter.Wait(ctx); err != nil {
    return nil, 0, nil, err
}
```
Called before every API request. Respects context cancellation.

### httpx.Client for retry + circuit breaker
The client doesn't implement retry or circuit breaker logic — `httpx.Client` provides both automatically. Just call:
```go
resp, err := c.httpClient.Get(ctx, fullURL, headers, 15*time.Second)
```

### Error wrapping with domain error types
```go
import apperrors "github.com/guarzo/slabledger/internal/domain/errors"

// Not found
return nil, statusCode, headers, apperrors.ProviderNotFound(c.Name(), description)

// Unavailable (network, timeout, etc.)
return nil, statusCode, headers, apperrors.ProviderUnavailable(c.Name(), err)

// Bad response
return nil, statusCode, headers, apperrors.ProviderInvalidResponse(c.Name(), err)

// Missing config
return nil, 0, nil, apperrors.ConfigMissing("pokemonprice_api_key", "POKEMONPRICE_TRACKER_API_KEY")
```

### Card name normalization before API calls
```go
cleanName := cardutil.NormalizePurchaseName(cardName)
cleanName = cardutil.SimplifyForSearch(cleanName)
normalizedSet := cardutil.NormalizeSetNameForSearch(setName)
```
Always normalize card/set names before sending to external APIs. The `cardutil` package handles PSA abbreviation expansion, variant stripping, and set code removal.

## More Complex Example: CardHedger

For a more complex client with batch operations and cert resolution, see `internal/adapters/clients/cardhedger/`. Key differences:
- Multiple API endpoints (search, details-by-certs, batch)
- Cert-to-card-ID resolution with caching via `CardIDMappingRepository`
- 3-tier query fallback in `source_adapters.go`
- Delta polling for price updates
