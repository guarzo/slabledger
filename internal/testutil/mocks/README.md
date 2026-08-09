# testutil/mocks

Centralized mock implementations for all domain interfaces.

The tables below cover the mocks you reach for most often; they are **not** an exhaustive
roster, and a table that tries to be one goes stale silently. For the current set, derive
it from the tree:

```bash
grep -rho '^type [A-Za-z]*Mock[A-Za-z]*' internal/testutil/mocks | awk '{print $2}' | sort -u
```

## Pattern

Nearly all mocks use the **Fn-field pattern**: every interface method has a corresponding `Fn` field.
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

A few mocks are built by a constructor instead (`NewInMemoryCampaignStore`,
`NewCapturingLogger`, `NewMockSimplePriceProvider`) because they carry initialized state.
Use the Fn-field pattern for all new mocks; see "Usage Notes" below.

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

### inventory.FinanceRepository → `FinanceRepositoryMock`
### inventory.PendingItemRepository → `MockPendingItemRepository`

Both follow the same Fn-field pattern. Unset methods return zero values or empty slices.

The remaining inventory repository interfaces (`SaleRepository`, `AnalyticsRepository`,
`PricingRepository`, `DHRepository`) have no standalone mock — tests use
`InMemoryCampaignStore`, which implements all of them.

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
    inventory.WithIDGenerator(func() string { return "test-id" }),
)
```

`WithIDGenerator` is not optional — `NewService` panics without it. Everything else
(logger, price lookup, cert lookup, …) is a `ServiceOption`; see `WithLogger` and friends
in `internal/domain/inventory/service.go`.

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
| `MockSimplePriceProvider` | `pricing.PriceProvider` (with call tracking) |
| `MockAuthRepository` | `auth.Repository` |
| `MockCertLookup` | cert lookup interface |
| `RowScanner` | postgres package's unexported `scanner` interface (`Scan(dest ...any) error`) |
| `CapturingLogger` | `observability.Logger` (records calls instead of discarding them) |

## Error Assertions

Use sentinel errors with `errors.Is`:

```go
_, err := svc.GetCampaign(ctx, "nonexistent")
if !errors.Is(err, inventory.ErrCampaignNotFound) {
    t.Errorf("expected ErrCampaignNotFound, got %v", err)
}
```

Key sentinel errors:
- `inventory.ErrCampaignNotFound`
- `inventory.ErrPurchaseNotFound`
- `inventory.ErrSaleNotFound`
- `inventory.ErrInvoiceNotFound`
- `inventory.ErrDuplicateCertNumber`

## Usage Notes

- **Never create inline mocks** in test files. Add to this package instead.
- Mocks live in `package mocks` (not `package mocks_test`) so they export for all test packages.
- White-box tests that live in the package under test cannot import `testutil/mocks` without an
  import cycle; inline mocks are intentional there.
