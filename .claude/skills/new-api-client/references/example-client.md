# Example Client: DH (DoubleHolo)

A real API client in the codebase demonstrating all key patterns. Located at `internal/adapters/clients/dhprice/`.

## File Structure

```
dhprice/
├── provider.go         # Provider struct, constructor, API methods
├── types.go            # API response types
└── provider_test.go    # Tests with httptest.NewServer
```

## Key Patterns

### Constructor with functional options
```go
type Provider struct {
    apiKey     string
    baseURL    string
    httpClient *httpx.Client
    logger     observability.Logger
}

type ProviderOption func(*Provider)

func WithLogger(logger observability.Logger) ProviderOption {
    return func(p *Provider) { p.logger = logger }
}

func NewProvider(apiKey, baseURL string, opts ...ProviderOption) *Provider {
    config := httpx.DefaultConfig("DoubleHolo")
    config.DefaultTimeout = 15 * time.Second
    p := &Provider{
        apiKey:     apiKey,
        baseURL:    baseURL,
        httpClient: httpx.NewClient(config),
    }
    for _, opt := range opts {
        opt(p)
    }
    return p
}
```

### Available() for graceful degradation
```go
func (p *Provider) Available() bool { return p.apiKey != "" }
```
When the API key isn't configured, the provider reports itself as unavailable. Schedulers and handlers check this before making calls.

### Rate limiting with golang.org/x/time/rate
```go
if err := p.rateLimiter.Wait(ctx); err != nil {
    return nil, 0, nil, err
}
```
Called before every API request. Respects context cancellation.

### httpx.Client for retry + circuit breaker
The client doesn't implement retry or circuit breaker logic — `httpx.Client` provides both automatically. Just call:
```go
resp, err := p.httpClient.Get(ctx, fullURL, headers, 15*time.Second)
```

### Error wrapping with domain error types
```go
import apperrors "github.com/guarzo/slabledger/internal/domain/errors"

// Not found
return nil, statusCode, headers, apperrors.ProviderNotFound(p.Name(), description)

// Unavailable (network, timeout, etc.)
return nil, statusCode, headers, apperrors.ProviderUnavailable(p.Name(), err)

// Bad response
return nil, statusCode, headers, apperrors.ProviderInvalidResponse(p.Name(), err)

// Missing config
return nil, 0, nil, apperrors.ConfigMissing("dh_enterprise_api_key", "DH_ENTERPRISE_API_KEY")
```

### Card name normalization before API calls
```go
cleanName := cardutil.NormalizePurchaseName(cardName)
cleanName = cardutil.SimplifyForSearch(cleanName)
normalizedSet := cardutil.NormalizeSetNameForSearch(setName)
```
Always normalize card/set names before sending to external APIs. The `cardutil` package handles PSA abbreviation expansion, variant stripping, and set code removal.
