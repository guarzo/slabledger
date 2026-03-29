# Example Client: CardHedger

A real API client in the codebase demonstrating all key patterns. Located at `internal/adapters/clients/cardhedger/`.

## File Structure

```
cardhedger/
├── client.go           # Client struct, constructor, API methods
├── source_adapters.go  # Fusion source adapter (implements SecondaryPriceSource)
├── cert_resolver.go    # Batch cert→card_id resolution
├── types.go            # API response types
└── client_test.go      # Tests with httptest.NewServer
```

## Key Patterns

### Constructor with functional options
```go
type Client struct {
    apiKey      string
    clientID    string
    httpClient  *httpx.Client
    rateLimiter *rate.Limiter
    logger      observability.Logger
}

type ClientOption func(*Client)

func WithLogger(logger observability.Logger) ClientOption {
    return func(c *Client) { c.logger = logger }
}

func NewClient(apiKey, clientID string, opts ...ClientOption) *Client {
    config := httpx.DefaultConfig("CardHedger")
    config.DefaultTimeout = 15 * time.Second
    c := &Client{
        apiKey:      apiKey,
        clientID:    clientID,
        httpClient:  httpx.NewClient(config),
        rateLimiter: rate.NewLimiter(rate.Every(700*time.Millisecond), 1),
    }
    for _, opt := range opts {
        opt(c)
    }
    return c
}
```

### Available() for graceful degradation
```go
func (c *Client) Available() bool { return c.apiKey != "" && c.clientID != "" }
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
return nil, 0, nil, apperrors.ConfigMissing("card_hedger_api_key", "CARD_HEDGER_API_KEY")
```

### Card name normalization before API calls
```go
cleanName := cardutil.NormalizePurchaseName(cardName)
cleanName = cardutil.SimplifyForSearch(cleanName)
normalizedSet := cardutil.NormalizeSetNameForSearch(setName)
```
Always normalize card/set names before sending to external APIs. The `cardutil` package handles PSA abbreviation expansion, variant stripping, and set code removal.

## Additional Patterns in CardHedger

Beyond the basics, CardHedger demonstrates:
- Multiple API endpoints (search, details-by-certs, batch)
- Cert-to-card-ID resolution with caching via `CardIDMappingRepository`
- 3-tier query fallback in `source_adapters.go`
- Delta polling for price updates
