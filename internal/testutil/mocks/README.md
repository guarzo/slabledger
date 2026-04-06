# Mock Providers Package

This package provides centralized, reusable mock implementations for all provider interfaces used in the slabledger application. These mocks are designed for comprehensive testing with configurable behaviors.

## Overview

All mocks support configurable behaviors through functional options, allowing tests to simulate various scenarios including:
- Network delays
- Timeouts
- Rate limiting
- Error conditions
- Empty responses
- Partial failures

## Available Mocks

### MockCardProvider
Mocks the card provider interface (implements `domainCards.CardProvider`).

**Methods:**
- `GetCards(ctx, setID)` - Returns mock cards for a given set
- `GetSet(ctx, setID)` - Returns mock set metadata
- `ListAllSets(ctx)` - Returns all mock card sets
- `Available()` - Returns provider availability status

**Example:**
```go
import (
    "context"
    "github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestWithCards(t *testing.T) {
    ctx := context.Background()

    // Basic usage
    cardProvider := mocks.NewMockCardProvider()
    sets, err := cardProvider.ListAllSets(ctx)
    cards, err := cardProvider.GetCards(ctx, "base")

    // With error simulation
    cardProvider := mocks.NewMockCardProvider(
        mocks.WithError(fmt.Errorf("API unavailable")),
    )

    // With empty data
    cardProvider := mocks.NewMockCardProvider(
        mocks.WithEmptyData(),
    )

    // Custom data
    cardProvider := mocks.NewMockCardProvider()
    cardProvider.SetMockCards("base", customCards)
}
```

### MockPriceProvider
Mocks the DH price provider interface.

**Methods:**
- `Available()` - Returns availability status
- `Close()` - No-op cleanup
- `LookupCard(ctx, setName, card)` - Returns mock price data
- `LookupBatch(ctx, setName, cards)` - Returns batch price data

**Example:**
```go
func TestPriceLookup(t *testing.T) {
    ctx := context.Background()
    priceProvider := mocks.NewMockPriceProvider()

    card := model.Card{Name: "Pikachu", Number: "025"}
    match, err := priceProvider.LookupCard(ctx, "Base Set", card)

    // Returns deterministic prices based on card name/number
    assert.NotZero(t, match.LoosePrice)
    assert.NotZero(t, match.ManualPrice) // PSA 10 price
}
```

**Custom match data:**
```go
func TestCustomPrices(t *testing.T) {
    priceProvider := mocks.NewMockPriceProvider()

    customMatch := &prices.PCMatch{
        LoosePrice: 1000, // $10.00 in cents
        ManualPrice: 5000, // $50.00 in cents
    }

    priceProvider.SetMockMatch("Base Set", "Charizard", "006", customMatch)
}
```

### MockPopulationProvider
Mocks the PSA population data provider interface with full interface compliance.

**Interface compliance:**
```go
var _ population.Provider = (*MockPopulationProvider)(nil)
```

**Methods:**
- `Available()` - Returns availability status
- `GetProviderName()` - Returns "Mock Population Provider"
- `IsMockMode()` - Returns true
- `LookupPopulation(ctx, card)` - Returns mock population data
- `BatchLookupPopulation(ctx, cards)` - Returns batch population data
- `GetSetPopulation(ctx, setName)` - Returns set statistics

**Example:**
```go
func TestPopulationData(t *testing.T) {
    popProvider := mocks.NewMockPopulationProvider()

    card := model.Card{Name: "Charizard", Number: "006"}
    data, err := popProvider.LookupPopulation(context.Background(), card)

    // Deterministic population based on card name/number
    assert.NotZero(t, data.PSA10Population)
    assert.NotEmpty(t, data.ScarcityLevel)
}
```

### MockHTTPClient

Mocks the httpx.HTTPClient interface for testing adapters that make HTTP requests.

**Interface compliance:**
```go
var _ httpx.HTTPClient = (*MockHTTPClient)(nil)
```

**Methods:**
- `GetJSON(ctx, url, headers, timeout, dest)` - GET request with JSON decoding
- `Get(ctx, url, headers, timeout)` - Raw GET request
- `Post(ctx, url, headers, body, timeout)` - POST request
- `PostJSON(ctx, url, headers, body, timeout, dest)` - POST with JSON encoding/decoding
- `Do(ctx, req)` - Custom request
- `GetCircuitBreakerStats()` - Returns zero values

**Key Features:**
- ✅ No retry logic (immediate responses)
- ✅ No circuit breaker overhead
- ✅ No network I/O (pure in-memory)
- ✅ URL pattern matching
- ✅ Call recording for verification
- ✅ Statistics tracking
- ✅ Thread-safe

**Example:**
```go
func TestCardProvider(t *testing.T) {
    // Pre-configured with TCGdex responses
    mockClient := mocks.NewMockHTTPClientWithTCGdexResponses()

    sets, err := provider.ListAllSets(context.Background())
    // Returns 3 mock sets immediately (no retry delays)
}
```

**Custom responses:**
```go
mockClient := mocks.NewMockHTTPClient()
mockClient.AddResponse("/v2/sets", mocks.MockHTTPResponse{
    StatusCode: 200,
    Body: `{"data": [...]}`,
})
```

**Error simulation:**
```go
// Specific error
mockClient := mocks.NewMockHTTPClientWithError(fmt.Errorf("network error"))

// HTTP error status
mockClient := mocks.NewMockHTTPClientWithStatusCode(404, "not found")

// With delay (for timeout testing)
mockClient := mocks.NewMockHTTPClient(mocks.WithDelay(5 * time.Second))
```

**Verification:**
```go
// Check calls were made
callLog := mockClient.GetCallLog()
assert.Equal(t, 2, len(callLog))
assert.Equal(t, "GET", callLog[0].Method)
assert.Contains(t, callLog[0].URL, "/v2/sets")

// Check statistics
stats := mockClient.GetStats()
fmt.Printf("Total calls: %d\n", stats.TotalCalls)
fmt.Printf("Errors: %d\n", stats.TotalErrors)
```

**Performance:**
- **Without mock:** 120+ seconds (retry backoffs: 1s, 2s, 4s, 8s...)
- **With mock:** < 5 seconds (immediate responses)
- **Speedup:** 95% faster test execution

**When to use:**
- ✅ Unit tests for HTTP adapters
- ✅ Integration tests (unless testing HTTP layer specifically)
- ✅ CI/CD pipelines (fast, deterministic)
- ❌ E2E tests (use real HTTP client)

### MockCampaignRepository
Mocks the campaigns.Repository interface for campaign persistence (implements `campaigns.Repository`).

**Interface compliance:**
```go
var _ campaigns.Repository = (*MockCampaignRepository)(nil)
```

**Methods (20 total):**
- Campaign CRUD: `CreateCampaign`, `GetCampaign`, `ListCampaigns`, `UpdateCampaign`, `DeleteCampaign`
- Purchase CRUD: `CreatePurchase`, `GetPurchase`, `ListPurchasesByCampaign`, `ListUnsoldPurchases`, `CountPurchasesByCampaign`
- Sale CRUD: `CreateSale`, `GetSaleByPurchaseID`, `ListSalesByCampaign`
- Analytics: `GetCampaignPNL`, `GetPNLByChannel`, `GetDailySpend`, `GetDaysToSellDistribution`
- Tuning: `GetPerformanceByGrade`, `GetPurchasesWithSales`

Uses the functional-field pattern: each method has a corresponding `Fn` field (e.g., `CreateCampaignFn`). If the field is nil, the method returns sensible defaults (nil error, empty slices, or stub structs).

**Example:**
```go
repo := &mocks.MockCampaignRepository{
    ListCampaignsFn: func(ctx context.Context, activeOnly bool) ([]campaigns.Campaign, error) {
        return []campaigns.Campaign{{ID: "1", Name: "Test"}}, nil
    },
    GetCampaignPNLFn: func(ctx context.Context, campaignID string) (*campaigns.CampaignPNL, error) {
        return &campaigns.CampaignPNL{CampaignID: campaignID, TotalSpentCents: 5000}, nil
    },
}
```

### MockFavoritesRepository
Mocks the favorites.Repository interface for favorites persistence (implements `favorites.Repository`).

**Interface compliance:**
```go
var _ favorites.Repository = (*MockFavoritesRepository)(nil)
```

**Methods:**
- `Add(ctx, userID, input)` - Add a card to favorites
- `Remove(ctx, userID, cardName, setName, cardNumber)` - Remove a favorite
- `List(ctx, userID, limit, offset)` - List user's favorites
- `Count(ctx, userID)` - Count user's favorites
- `IsFavorite(ctx, userID, cardName, setName, cardNumber)` - Check if a card is favorited
- `CheckMultiple(ctx, userID, cards)` - Check favorite status for multiple cards

Uses the functional-field pattern: each method has a corresponding `Fn` field (e.g., `AddFn`, `RemoveFn`). If the field is nil, the method returns sensible defaults.

**Example:**
```go
repo := &mocks.MockFavoritesRepository{
    ListFn: func(ctx context.Context, userID int64, limit, offset int) ([]favorites.Favorite, error) {
        return []favorites.Favorite{{ID: 1, UserID: userID, CardName: "Pikachu"}}, nil
    },
    IsFavoriteFn: func(ctx context.Context, userID int64, cardName, setName, cardNumber string) (bool, error) {
        return true, nil
    },
}
```

### MockCampaignService
Mocks the campaigns.Service interface (implements `campaigns.Service`).

Uses the same functional-field pattern as the repository mocks. See `campaign_service.go` for the full list of supported methods.

## Configurable Behaviors

All mocks support functional options for configuring behavior:

### Error Simulation
```go
// Return a specific error
mock := mocks.NewMockPriceProvider(
    mocks.WithError(fmt.Errorf("connection failed")),
)

// Simulate timeout
mock := mocks.NewMockPriceProvider(
    mocks.WithTimeout(),
)

// Simulate rate limiting
mock := mocks.NewMockPopulationProvider(
    mocks.WithRateLimit(),
)
```

### Performance Simulation
```go
// Add network delay
mock := mocks.NewMockCardProvider(
    mocks.WithDelay(500 * time.Millisecond),
)
```

### Partial Failures
```go
// Fail after N successful calls
mock := mocks.NewMockPriceProvider(
    mocks.WithFailAfterN(5),
)

// First 5 calls succeed, then all subsequent calls fail
```

### Empty Data
```go
// Return empty results instead of mock data
mock := mocks.NewMockPopulationProvider(
    mocks.WithEmptyData(),
)
```

### Combining Options
```go
// Multiple behaviors can be combined
mock := mocks.NewMockPriceProvider(
    mocks.WithDelay(100 * time.Millisecond),
    mocks.WithFailAfterN(10),
)
```

## Testing Patterns

### Unit Tests
Use mocks to isolate the code under test:

```go
func TestAnalysisEngine(t *testing.T) {
    // Arrange
    cardProvider := mocks.NewMockCardProvider()
    priceProvider := mocks.NewMockPriceProvider()
    popProvider := mocks.NewMockPopulationProvider()

    analyzer := analysis.NewAnalyzer(
        cardProvider,
        priceProvider,
        popProvider,
    )

    // Act
    results, err := analyzer.AnalyzeSet("base")

    // Assert
    require.NoError(t, err)
    assert.NotEmpty(t, results)
}
```

### Error Scenario Tests
Test error handling:

```go
func TestAnalysisWithProviderFailure(t *testing.T) {
    cardProvider := mocks.NewMockCardProvider()
    priceProvider := mocks.NewMockPriceProvider(
        mocks.WithError(fmt.Errorf("API unavailable")),
    )

    analyzer := analysis.NewAnalyzer(cardProvider, priceProvider)

    results, err := analyzer.AnalyzeSet("base")

    // Should handle error gracefully
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "API unavailable")
}
```

### Performance Tests
Test behavior under delays:

```go
func TestTimeoutHandling(t *testing.T) {
    priceProvider := mocks.NewMockPriceProvider(
        mocks.WithDelay(5 * time.Second),
    )

    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()

    // Should timeout before mock responds
    _, err := priceProvider.LookupCard(ctx, "Base Set", card)
    assert.Error(t, err)
}
```

### Integration Tests
Use mocks to avoid external API dependencies:

```go
func TestCompleteAnalysisFlow(t *testing.T) {
    // Use all mocks for isolated integration test
    cardProvider := mocks.NewMockCardProvider()
    priceProvider := mocks.NewMockPriceProvider()
    popProvider := mocks.NewMockPopulationProvider()

    // Run full analysis without external dependencies
    app := cli.NewApp(
        cardProvider,
        priceProvider,
        popProvider,
    )

    err := app.Run([]string{"--set", "base", "--analysis", "rank"})
    assert.NoError(t, err)
}
```

## Best Practices

1. **Use Interface Compliance Checks**
   ```go
   var _ population.Provider = (*MockPopulationProvider)(nil)
   ```

2. **Reset Mock State Between Tests**
   ```go
   provider := mocks.NewMockPriceProvider()
   // Mocks maintain state across calls - create new instances between tests
   ```

3. **Use Deterministic Data**
   - Mock data is generated deterministically based on input (e.g., card name)
   - Same inputs always produce same outputs
   - Tests are reproducible

4. **Prefer Centralized Mocks**
   - Use these instead of creating ad-hoc mocks in test files
   - Reduces duplication
   - Ensures consistency across tests

5. **Customize When Needed**
   ```go
   mockPrice := mocks.NewMockPriceProvider()
   mockPrice.SetMockMatch("Base Set", "Charizard", "006", customData)
   ```

## Migration Guide

### Replacing Old Mocks

**Before (duplicated mock):**
```go
type testMockProvider struct{}
func (t *testMockProvider) LookupPopulation(...) {...}
```

**After (centralized mock):**
```go
import "github.com/guarzo/slabledger/internal/testutil/mocks"

mockPop := mocks.NewMockPopulationProvider()
```

### Converting Tests

1. Import the mocks package
2. Replace custom mock creation with `New*` functions
3. Use functional options for behavior configuration
4. Remove old mock implementations

## Testing the Mocks

The mocks themselves have comprehensive tests in `mocks_test.go`:
- Interface compliance verification
- Basic functionality tests
- TTL expiration tests
- Error behavior tests
- Configurable behavior tests

Run mock tests:
```bash
go test ./internal/testutil/mocks/
```

## Contributing

When adding new provider interfaces:

1. Create a new mock file (e.g., `new_provider.go`)
2. Implement the provider interface
3. Add interface compliance check
4. Support `MockBehavior` configuration
5. Add tests to `mocks_test.go`
6. Document usage in this README
