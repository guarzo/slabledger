# testutil/mocks

Centralized mock implementations for all domain interfaces.

## Pattern

All mocks use the **Fn-field pattern**: every interface method has a corresponding `Fn` field.
When the field is `nil`, the method returns a sensible zero-value default.
Override any method per test by assigning a function.

```go
mock := &mocks.CampaignRepositoryMock{
    GetCampaignFn: func(ctx context.Context, id string) (*inventory.Campaign, error) {
        return &inventory.Campaign{ID: id, Name: "Test Campaign"}, nil
    },
}
```

No constructors needed for these mocks — just instantiate the struct and set the fields you care about.

## Repository Mocks

### inventory.CampaignRepository → `CampaignRepositoryMock`

```go
mock := &mocks.CampaignRepositoryMock{
    ListCampaignsFn: func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
        return []inventory.Campaign{{ID: "c1", Name: "Vintage"}}, nil
    },
}
```

Default when `Fn` is nil:
- `GetCampaign` → `inventory.ErrCampaignNotFound`
- `ListCampaigns` → `[]inventory.Campaign{}`
- mutating methods → `nil` (no-op)

### inventory.PurchaseRepository → `PurchaseRepositoryMock`

Default when `Fn` is nil:
- `GetPurchase` → `inventory.ErrPurchaseNotFound`
- list methods → empty slice

### inventory.SaleRepository → `SaleRepositoryMock`

Default when `Fn` is nil:
- `GetSaleByPurchaseID` → `inventory.ErrSaleNotFound`
- list methods → empty slice

### inventory.AnalyticsRepository → `AnalyticsRepositoryMock`
### inventory.FinanceRepository → `FinanceRepositoryMock`
### inventory.PricingRepository → `PricingRepositoryMock`
### inventory.DHRepository → `DHRepositoryMock`

All follow the same Fn-field pattern. Unset methods return zero values or empty slices.

### picks.Repository → `MockPicksRepository`
### picks.ProfitabilityProvider → `MockProfitabilityProvider`
### picks.InventoryProvider → `MockInventoryProvider`

## Service Mocks

### inventory.Service → `MockInventoryService`

```go
svc := &mocks.MockInventoryService{
    ListCampaignsFn: func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
        return []inventory.Campaign{{ID: "c1"}}, nil
    },
    GetCampaignPNLFn: func(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error) {
        return &inventory.CampaignPNL{CampaignID: campaignID}, nil
    },
}
```

### Sub-domain service mocks

| Interface | Mock type |
|-----------|-----------|
| `arbitrage.Service` | `MockArbitrageService` |
| `portfolio.Service` | `MockPortfolioService` |
| `tuning.Service` | `MockTuningService` |
| `finance.Service` | `MockFinanceService` |
| `export.Service` | `MockExportService` |
| `dhlisting.Service` | `MockDHListingService` |
| `advisor.Service` | `MockAdvisorService` |
| `social.Service` | `MockSocialService` |
| `picks.Service` | `MockPicksService` |

Each follows the same pattern: set the `*Fn` field to override a method.

## InMemoryCampaignStore

For service-layer tests that need a realistic store with actual state.

`InMemoryCampaignStore` implements all 7 inventory repository interfaces:
`CampaignRepository`, `PurchaseRepository`, `SaleRepository`, `AnalyticsRepository`,
`FinanceRepository`, `PricingRepository`, `DHRepository`.

Pass the same instance for all 7 repository slots when constructing `inventory.NewService`.

```go
store := mocks.NewInMemoryCampaignStore()

svc := inventory.NewService(
    store, // CampaignRepository
    store, // PurchaseRepository
    store, // SaleRepository
    store, // AnalyticsRepository
    store, // FinanceRepository
    store, // PricingRepository
    store, // DHRepository
    logger,
)
```

### Fn-fields on InMemoryCampaignStore

`InMemoryCampaignStore` also supports the Fn-field pattern for any method. When you need
to inject a custom error or alter a specific method's behaviour while keeping the rest of
the in-memory state:

```go
store := mocks.NewInMemoryCampaignStore()
store.CreatePurchaseFn = func(ctx context.Context, p *inventory.Purchase) error {
    return fmt.Errorf("simulated DB failure")
}
```

The default implementations provide a working in-memory store: cascade deletes, duplicate
cert detection, pagination support. **No extra Fn-fields are needed** for basic usage.

### Direct state access

Test helpers can seed data directly into the store's maps:

```go
store := mocks.NewInMemoryCampaignStore()
store.Campaigns["c1"] = &inventory.Campaign{ID: "c1", Name: "Test"}
store.Purchases["p1"] = &inventory.Purchase{ID: "p1", CampaignID: "c1"}
```

## Other Mocks

| Type | Interface |
|------|-----------|
| `MockCardProvider` | `cards.CardProvider` |
| `MockPriceProvider` | `pricing.PriceProvider` |
| `MockHTTPClient` | `httpx.Client` |
| `MockAuthRepository` | `auth.Repository` |
| `MockCertLookup` | cert lookup interface |
| `MockSocialService` | `social.Service` |

## Error Assertions

Use sentinel errors with `errors.Is`:

```go
err := svc.GetCampaign(ctx, "nonexistent")
if !errors.Is(err, inventory.ErrCampaignNotFound) {
    t.Errorf("expected ErrCampaignNotFound, got %v", err)
}
```

Key sentinel errors:
- `inventory.ErrCampaignNotFound`
- `inventory.ErrPurchaseNotFound`
- `inventory.ErrSaleNotFound`
- `inventory.ErrInvoiceNotFound`
- `inventory.ErrDuplicateCert`

## Usage Notes

- **Never create inline mocks** in test files. Add to this package instead.
- Mocks live in `package mocks` (not `package mocks_test`) so they export for all test packages.
- `picks/service_test.go` and `favorites/service_test.go` use `package picks`/`package favorites`
  (white-box tests) and therefore cannot import `testutil/mocks` — their inline mocks are intentional.
- `MockBehavior` / `MockOption` helpers in `common.go` are used only by card/HTTP mocks (legacy);
  prefer the Fn-field pattern for all new mocks.
