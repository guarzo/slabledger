---
name: new-api-client
description: Scaffold a new external API client following hexagonal architecture patterns
---

# New API Client Workflow

Use this skill when adding a new external API integration to SlabLedger.

## Step 1: Define the domain interface

Create or extend an interface in `internal/domain/<package>/`:

```go
// internal/domain/myfeature/provider.go
package myfeature

type Provider interface {
    GetData(ctx context.Context, query string) (*Result, error)
    Available() bool
    Name() string
}
```

Rules:
- Interface lives in domain — NO external dependencies
- Use `context.Context` as first parameter
- Include `Available() bool` for graceful degradation when not configured
- Include `Name() string` for logging and diagnostics

## Step 2: Create the adapter package

Create `internal/adapters/clients/<name>/`:

```
internal/adapters/clients/myapi/
├── client.go      # Client struct, constructor, methods
├── types.go       # API response types (if complex)
└── client_test.go # Tests with httptest.NewServer
```

Use `httpx.Client` for automatic retry + circuit breaker:

```go
package myapi

import (
    "github.com/guarzo/slabledger/internal/adapters/clients/httpx"
    "github.com/guarzo/slabledger/internal/domain/observability"
)

type Client struct {
    httpClient  *httpx.Client
    apiKey      string
    logger      observability.Logger
}

type Option func(*Client)

func WithLogger(logger observability.Logger) Option {
    return func(c *Client) { c.logger = logger }
}

func NewClient(apiKey string, opts ...Option) *Client {
    config := httpx.DefaultConfig("MyAPI")
    config.DefaultTimeout = 15 * time.Second
    c := &Client{
        apiKey:     apiKey,
        httpClient: httpx.NewClient(config),
    }
    for _, opt := range opts {
        opt(c)
    }
    return c
}

func (c *Client) Available() bool { return c.apiKey != "" }
func (c *Client) Name() string    { return "myapi" }
```

Add compile-time interface check:
```go
var _ myfeature.Provider = (*Client)(nil)
```

## Step 3: Implement the interface methods

```go
func (c *Client) GetData(ctx context.Context, query string) (*myfeature.Result, error) {
    headers := map[string]string{
        "Authorization": "Bearer " + c.apiKey,
        "Accept":        "application/json",
    }
    resp, err := c.httpClient.Get(ctx, baseURL+"/endpoint?q="+url.QueryEscape(query), headers, 15*time.Second)
    if err != nil {
        return nil, fmt.Errorf("myapi get data: %w", err)
    }
    var apiResp apiResponse
    if err := json.Unmarshal(resp.Body, &apiResp); err != nil {
        return nil, fmt.Errorf("myapi parse response: %w", err)
    }
    return convertToResult(apiResp), nil
}
```

## Step 4: Add environment variable

1. Add field to `internal/platform/config/types.go` (in `Adapters` struct)
2. Add `os.Getenv` in `internal/platform/config/loader.go`
3. Add default in `internal/platform/config/defaults.go` if needed
4. Add to `.env.example`
5. Add to `CLAUDE.md` Environment Variables section

## Step 5: Wire in main.go

In `cmd/slabledger/init.go` (price providers) or `main.go`:

```go
myClient := myapi.NewClient(cfg.Adapters.MyAPIKey, myapi.WithLogger(logger))
```

Inject via functional options:
```go
serviceOpts = append(serviceOpts, campaigns.WithMyProvider(myClient))
```

## Step 6: Write tests

Use `httptest.NewServer` to mock the external API (see `references/example-client.md`):

```go
func TestClient_GetData(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(apiResponse{...})
    }))
    defer srv.Close()
    // Create client pointing at test server...
}
```

## Step 7: Update documentation

- Update `CLAUDE.md` Architecture section with new package
- Update `docs/API.md` if new endpoints are exposed
- Run `golangci-lint run ./...` to verify

## Checklist

- [ ] Domain interface defined (no external deps)
- [ ] Adapter implements interface (compile-time check with `var _ Interface = (*Impl)(nil)`)
- [ ] Uses `httpx.Client` (gets retry + circuit breaker automatically)
- [ ] `Available()` returns false when not configured
- [ ] Rate limiting added if API has limits (use `golang.org/x/time/rate`)
- [ ] Tests use `httptest.NewServer`
- [ ] Environment variable added to config + `.env.example`
- [ ] Wired in `cmd/slabledger/init.go` or `main.go`
- [ ] `CLAUDE.md` updated

## Reference

See `references/example-client.md` for an annotated walkthrough of a simple API client example.
