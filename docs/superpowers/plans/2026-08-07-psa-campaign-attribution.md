# PSA Campaign Attribution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Read PSA's own campaign attribution (`adjusted_description`) from the tile we already harvest, prefer it over heuristic inference for new imports, and retroactively correct existing purchases.

**Architecture:** The campaign name is already persisted raw in `psa_portal_snapshot.rows`; only `mapRow` discards it. We add the field to the mapper, two provenance columns to `campaign_purchases`, an inventory-owned resolver port implemented in the adapter layer (a direct import would create a cycle), and a reconciliation pass that runs at the end of each harvest.

**Tech Stack:** Go 1.26, hexagonal architecture, Postgres via `pgx/v5/stdlib`, `golang-migrate/v4` with embedded migrations.

**Spec:** `docs/superpowers/specs/2026-08-07-psa-campaign-attribution-design.md`

## Global Constraints

- Domain packages MUST NOT import adapter packages. Sub-packages under `internal/domain/` are flat siblings and MUST NOT import each other. `internal/domain/psacampaign` already imports `internal/domain/inventory` (`internal/domain/psacampaign/mapper.go:8`), so `inventory` importing `psacampaign` is a cycle. Enforced by `scripts/check-imports.sh` via `make check`.
- All monetary values in cents internally; USD only in API responses.
- `ctx context.Context` is always the first parameter.
- Structured logging only: `logger.Info(ctx, "msg", observability.String("key", val))`.
- Table-driven tests. Mocks come from `internal/testutil/mocks/` — never inline mocks. Sentinel errors asserted with `errors.Is`.
- Source files stay under 500 lines (`scripts/check-file-size.sh` warns at 500, fails at 600).
- Use functional options for optional dependencies (`WithPriceLookup` is the reference pattern, `internal/domain/inventory/service.go:233`).
- `go test -race ./...` and `make check` must pass before any task is called done.
- Attribution source values are exactly `psa`, `inferred`, `manual`.

---

### Task 1: Capture the campaign name in the mapper

**Files:**
- Modify: `internal/domain/inventory/import_types.go:25-39`
- Modify: `internal/adapters/clients/psaportal/mapper.go:12-24,31-43`
- Test: `internal/adapters/clients/psaportal/mapper_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `inventory.PSAExportRow.PSACampaignName string` — the raw PSA campaign name, empty when absent. Every later task reads this field.

- [ ] **Step 1: Write the failing test**

Add to `internal/adapters/clients/psaportal/mapper_test.go`:

```go
func TestMapRow_PSACampaignName(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want string
	}{
		{"present", map[string]string{"adjusted_description": "Modern"}, "Modern"},
		{"absent", map[string]string{}, ""},
		{"empty", map[string]string{"adjusted_description": ""}, ""},
		{"whitespace preserved verbatim", map[string]string{"adjusted_description": " Modern "}, " Modern "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapRow(tt.in)
			if err != nil {
				t.Fatalf("mapRow: %v", err)
			}
			if got.PSACampaignName != tt.want {
				t.Errorf("PSACampaignName = %q, want %q", got.PSACampaignName, tt.want)
			}
		})
	}
}
```

Note: the raw string is stored verbatim — trimming happens at resolution time (Task 4), not capture time, so the stored value is exactly what PSA sent.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/psaportal/ -run TestMapRow_PSACampaignName -v`
Expected: FAIL with `got.PSACampaignName undefined (type inventory.PSAExportRow has no field or method PSACampaignName)`

- [ ] **Step 3: Add the field**

In `internal/domain/inventory/import_types.go`, add to `PSAExportRow` after `BackImageURL`:

```go
	PSACampaignName string // PSA's own campaign attribution ("adjusted_description"); "" when absent
```

- [ ] **Step 4: Map the column**

In `internal/adapters/clients/psaportal/mapper.go`, add to the const block:

```go
	colCampaignName = "adjusted_description"
```

and add to the `PSAExportRow` literal in `mapRow`:

```go
		PSACampaignName: r[colCampaignName],
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapters/clients/psaportal/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/inventory/import_types.go internal/adapters/clients/psaportal/mapper.go internal/adapters/clients/psaportal/mapper_test.go
git commit -m "feat: capture PSA campaign name from itemized purchases tile"
```

---

### Task 2: Migration — provenance columns and legacy backfill

**Files:**
- Create: `internal/adapters/storage/postgres/migrations/000023_add_psa_campaign_attribution.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000023_add_psa_campaign_attribution.down.sql`

**Interfaces:**
- Consumes: nothing.
- Produces: `campaign_purchases.psa_campaign_name TEXT` and `campaign_purchases.attribution_source TEXT`, the latter non-null for all pre-existing rows.

The backfill lives here, not in reconciliation. Reconciliation only visits the ~45 snapshot rows; deferring the backfill to it would leave ~1533 rows null indefinitely.

- [ ] **Step 1: Write the up migration**

`000023_add_psa_campaign_attribution.up.sql`:

```sql
ALTER TABLE campaign_purchases ADD COLUMN IF NOT EXISTS psa_campaign_name TEXT;
ALTER TABLE campaign_purchases ADD COLUMN IF NOT EXISTS attribution_source TEXT;

-- Every pre-existing row's campaign came from FindMatchingCampaign or a hand
-- assignment we can no longer distinguish. 'inferred' is the weaker, safer claim.
UPDATE campaign_purchases SET attribution_source = 'inferred' WHERE attribution_source IS NULL;

CREATE INDEX IF NOT EXISTS idx_campaign_purchases_attribution_source
	ON campaign_purchases (attribution_source);
```

- [ ] **Step 2: Write the down migration**

`000023_add_psa_campaign_attribution.down.sql`:

```sql
DROP INDEX IF EXISTS idx_campaign_purchases_attribution_source;
ALTER TABLE campaign_purchases DROP COLUMN IF EXISTS attribution_source;
ALTER TABLE campaign_purchases DROP COLUMN IF EXISTS psa_campaign_name;
```

- [ ] **Step 3: Apply the migration against local Postgres**

Run: `go build -o slabledger ./cmd/slabledger && ./slabledger`
Expected: startup logs show migration 23 applied, no error. Stop the server with Ctrl-C.

- [ ] **Step 4: Verify the schema and backfill**

Run: `psql "$DATABASE_URL" -c "SELECT attribution_source, count(*) FROM campaign_purchases GROUP BY 1;"`
Expected: every row reports `inferred`; no NULL group.

Run: `psql "$DATABASE_URL" -c "\d campaign_purchases" | grep -E "psa_campaign_name|attribution_source"`
Expected: both columns present, type `text`.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/postgres/migrations/000023_add_psa_campaign_attribution.up.sql internal/adapters/storage/postgres/migrations/000023_add_psa_campaign_attribution.down.sql
git commit -m "feat: add psa_campaign_name and attribution_source to campaign_purchases"
```

---

### Task 3: Purchase fields and attribution persistence

**Files:**
- Modify: `internal/domain/inventory/types_core.go` (Purchase struct, ~line 223)
- Create: `internal/domain/inventory/attribution.go`
- Modify: `internal/domain/inventory/repository_purchase.go:50-60`
- Modify: `internal/adapters/storage/postgres/purchase_store.go`
- Test: `internal/adapters/storage/postgres/purchase_store_test.go`

**Interfaces:**
- Consumes: Task 2's columns.
- Produces:
  - `inventory.AttributionSourcePSA / AttributionSourceInferred / AttributionSourceManual` string constants
  - `inventory.Purchase.PSACampaignName string`, `inventory.Purchase.AttributionSource string`
  - `inventory.Reattribution` struct
  - `PurchaseRepository.ReattributePurchase(ctx context.Context, purchaseID string, r Reattribution) error`
  - `PurchaseRepository.UpdatePurchaseAttributionName(ctx context.Context, purchaseID, psaName, source string) error`

- [ ] **Step 1: Write the failing test**

Add to `internal/adapters/storage/postgres/purchase_store_test.go`:

```go
func TestReattributePurchase_RefusesWhenSaleExists(t *testing.T) {
	ps, purchaseID := newStoreWithSoldPurchase(t) // existing helper pattern in this file
	err := ps.ReattributePurchase(context.Background(), purchaseID, inventory.Reattribution{
		CampaignID:          "campaign-b",
		PSACampaignName:     "Modern High Band",
		PSASourcingFeeCents: 300,
	})
	if !errors.Is(err, inventory.ErrPurchaseHasSale) {
		t.Fatalf("err = %v, want ErrPurchaseHasSale", err)
	}
}

func TestReattributePurchase_NullsCLConfidenceWhenNil(t *testing.T) {
	ps, purchaseID := newStoreWithUnsoldPurchase(t)
	err := ps.ReattributePurchase(context.Background(), purchaseID, inventory.Reattribution{
		CampaignID:             "campaign-b",
		PSACampaignName:        "Modern",
		PSASourcingFeeCents:    300,
		CLConfidenceAtPurchase: nil,
	})
	if err != nil {
		t.Fatalf("ReattributePurchase: %v", err)
	}
	got := mustGetPurchase(t, ps, purchaseID)
	if got.CLConfidenceAtPurchase != nil {
		t.Errorf("CLConfidenceAtPurchase = %v, want nil", *got.CLConfidenceAtPurchase)
	}
	if got.AttributionSource != inventory.AttributionSourcePSA {
		t.Errorf("AttributionSource = %q, want %q", got.AttributionSource, inventory.AttributionSourcePSA)
	}
}
```

If `newStoreWithSoldPurchase` / `newStoreWithUnsoldPurchase` / `mustGetPurchase` do not exist, write them following the existing helper style in that file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/storage/postgres/ -run TestReattributePurchase -v`
Expected: FAIL — `ps.ReattributePurchase undefined`

- [ ] **Step 3: Add constants and the Reattribution type**

Create `internal/domain/inventory/attribution.go`:

```go
package inventory

// Attribution source values for Purchase.AttributionSource. Every write path
// must set one; a null value after migration 000023 is a defect, not a state.
const (
	// AttributionSourcePSA means PSA's own campaign name resolved to this campaign.
	AttributionSourcePSA = "psa"
	// AttributionSourceInferred means FindMatchingCampaign chose this campaign.
	AttributionSourceInferred = "inferred"
	// AttributionSourceManual means an operator assigned this campaign by hand.
	AttributionSourceManual = "manual"
)

// Reattribution carries a PSA-authoritative campaign correction for an unsold
// purchase. CLConfidenceAtPurchase is nil when the value cannot be vouched for
// and must be stored as NULL rather than guessed.
type Reattribution struct {
	CampaignID             string
	PSACampaignName        string
	PSASourcingFeeCents    int
	CLConfidenceAtPurchase *int
}
```

- [ ] **Step 4: Add the Purchase fields**

In `internal/domain/inventory/types_core.go`, alongside `CLConfidenceAtPurchase` (~line 223):

```go
	PSACampaignName   string `json:"psaCampaignName,omitempty"`   // raw campaign name PSA reported, verbatim
	AttributionSource string `json:"attributionSource,omitempty"` // psa | inferred | manual
```

- [ ] **Step 5: Extend the repository interface**

In `internal/domain/inventory/repository_purchase.go`, under "Field updates":

```go
	// ReattributePurchase moves a purchase to a PSA-authoritative campaign and
	// sets attribution_source='psa'. Returns ErrPurchaseHasSale if a linked sale
	// exists — sold rows carry frozen sale-side financials that this does not repair.
	ReattributePurchase(ctx context.Context, purchaseID string, r Reattribution) error
	// UpdatePurchaseAttributionName records PSA's campaign name and attribution
	// source without moving the campaign. Safe on sold purchases.
	UpdatePurchaseAttributionName(ctx context.Context, purchaseID, psaName, source string) error
```

- [ ] **Step 6: Implement in the store**

In `internal/adapters/storage/postgres/purchase_store.go`, following the existing `UpdatePurchaseCampaign` pattern (`:336-362`):

```go
func (ps *PurchaseStore) ReattributePurchase(ctx context.Context, purchaseID string, r inventory.Reattribution) error {
	// Conditional update mirrors UpdatePurchaseCampaign: refuse when a linked
	// sale exists, avoiding a TOCTOU race between checking and updating.
	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET campaign_id = $1,
		     psa_sourcing_fee_cents = $2,
		     cl_confidence_at_purchase = $3,
		     psa_campaign_name = $4,
		     attribution_source = $5,
		     updated_at = $6
		 WHERE id = $7
		   AND NOT EXISTS (SELECT 1 FROM campaign_sales WHERE purchase_id = $8)`,
		r.CampaignID, r.PSASourcingFeeCents, r.CLConfidenceAtPurchase,
		r.PSACampaignName, inventory.AttributionSourcePSA, time.Now(), purchaseID, purchaseID,
	)
	if err != nil {
		return fmt.Errorf("reattribute purchase: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("reattribute purchase: rows affected: %w", err)
	}
	if n == 0 {
		var exists int
		if qErr := ps.db.QueryRowContext(ctx,
			`SELECT 1 FROM campaign_purchases WHERE id = $1`, purchaseID,
		).Scan(&exists); qErr != nil {
			return inventory.ErrPurchaseNotFound
		}
		return inventory.ErrPurchaseHasSale
	}
	return nil
}

func (ps *PurchaseStore) UpdatePurchaseAttributionName(ctx context.Context, purchaseID, psaName, source string) error {
	return ps.execAndExpectRow(ctx, "update attribution name",
		`UPDATE campaign_purchases SET psa_campaign_name = $1, attribution_source = $2, updated_at = $3 WHERE id = $4`,
		psaName, source, time.Now(), purchaseID,
	)
}
```

- [ ] **Step 7: Add the columns to every purchase SELECT and scan**

Find the shared column list and row scanner in `purchase_store.go` (search for `cl_confidence_at_purchase`) and add `psa_campaign_name` and `attribution_source` to both, scanning into `sql.NullString` and assigning `.String` to the struct fields. Missing this is the most likely source of a silent read-back failure in Task 7.

- [ ] **Step 8: Add the methods to the mock**

In `internal/testutil/mocks/`, add Fn-field methods to `PurchaseRepositoryMock` following the existing pattern:

```go
	ReattributePurchaseFn          func(ctx context.Context, purchaseID string, r inventory.Reattribution) error
	UpdatePurchaseAttributionNameFn func(ctx context.Context, purchaseID, psaName, source string) error
```

with methods that call the Fn if set and return nil otherwise.

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./internal/adapters/storage/postgres/ ./internal/domain/inventory/ -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/domain/inventory/attribution.go internal/domain/inventory/types_core.go internal/domain/inventory/repository_purchase.go internal/adapters/storage/postgres/purchase_store.go internal/adapters/storage/postgres/purchase_store_test.go internal/testutil/mocks/
git commit -m "feat: add attribution provenance fields and reattribution persistence"
```

---

### Task 4: PSACampaignResolver port and adapter

**Files:**
- Create: `internal/domain/inventory/psa_resolver.go`
- Modify: `internal/domain/inventory/service.go` (options block, ~line 233)
- Create: `internal/adapters/clients/psaportal/campaignresolver.go`
- Test: `internal/adapters/clients/psaportal/campaignresolver_test.go`

**Interfaces:**
- Consumes: `campaigns.psa_campaign_request_id`, `psacampaign.SnapshotStore`.
- Produces:
  - `inventory.PSACampaignResolver` interface with `ResolveCampaignID(ctx context.Context, psaName string) (campaignID string, ok bool, err error)`
  - `inventory.WithPSACampaignResolver(r PSACampaignResolver) ServiceOption`
  - `psaportal.NewCampaignResolver(snap psacampaign.SnapshotStore, campaigns CampaignLister, now func() time.Time) *CampaignResolver`

The port lives in `inventory` and the implementation in the adapter layer. `inventory` must not import `psacampaign` — that is a cycle (`psacampaign/mapper.go:8` imports `inventory`).

- [ ] **Step 1: Write the failing test**

`internal/adapters/clients/psaportal/campaignresolver_test.go`:

```go
func TestCampaignResolver_ResolveCampaignID(t *testing.T) {
	portal := []psacampaign.PortalCampaign{
		{ID: "req-modern", Name: "Modern"},
		{ID: "req-crystal", Name: "Crystal"},
	}
	internal := []inventory.Campaign{
		{ID: "camp-1", Name: "Modern", PSACampaignRequestID: "req-modern"},
		{ID: "camp-2", Name: "Crystal Pokemon", PSACampaignRequestID: "req-crystal"},
	}

	tests := []struct {
		name    string
		psaName string
		wantID  string
		wantOK  bool
	}{
		{"exact match", "Modern", "camp-1", true},
		{"case drift", "modern", "camp-1", true},
		{"whitespace drift", "  Modern  ", "camp-1", true},
		{"name drift via request id", "Crystal", "camp-2", true},
		{"dead campaign name", "Brady modern", "", false},
		{"empty name", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver(t, portal, internal, time.Now())
			gotID, gotOK, err := r.ResolveCampaignID(context.Background(), tt.psaName)
			if err != nil {
				t.Fatalf("ResolveCampaignID: %v", err)
			}
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Errorf("= (%q, %v), want (%q, %v)", gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestCampaignResolver_RefusesStaleSnapshot(t *testing.T) {
	stale := time.Now().Add(-27 * time.Hour)
	r := newTestResolver(t, []psacampaign.PortalCampaign{{ID: "req-modern", Name: "Modern"}},
		[]inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}}, stale)
	_, _, err := r.ResolveCampaignID(context.Background(), "Modern")
	if !errors.Is(err, ErrStaleCampaignSnapshot) {
		t.Fatalf("err = %v, want ErrStaleCampaignSnapshot", err)
	}
}
```

`newTestResolver` builds a `CampaignResolver` over stub `SnapshotStore` / `CampaignLister` returning the given data and `fetchedAt`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/psaportal/ -run TestCampaignResolver -v`
Expected: FAIL — `undefined: newTestResolver`, `undefined: ErrStaleCampaignSnapshot`

- [ ] **Step 3: Define the domain port**

`internal/domain/inventory/psa_resolver.go`:

```go
package inventory

import "context"

// PSACampaignResolver maps PSA's own campaign name to an internal campaign ID.
//
// The two-hop lookup (portal snapshot -> psa_campaign_request_id -> campaign)
// is implemented in the adapter layer: the portal snapshot types live in
// internal/domain/psacampaign, which already imports this package, so resolving
// here would create an import cycle.
type PSACampaignResolver interface {
	// ResolveCampaignID returns the internal campaign ID for a PSA campaign name.
	// ok is false when the name is empty or names a campaign no longer in the
	// portal snapshot (e.g. deleted in the 2026-07-27/28 band restructure).
	// A non-nil error means the lookup could not be performed at all — a stale
	// snapshot, for instance — and must not be treated as "unresolved".
	ResolveCampaignID(ctx context.Context, psaName string) (campaignID string, ok bool, err error)
}
```

- [ ] **Step 4: Add the service option**

In `internal/domain/inventory/service.go`, next to `WithPriceLookup`:

```go
// WithPSACampaignResolver enables PSA-authoritative campaign attribution.
func WithPSACampaignResolver(r PSACampaignResolver) ServiceOption {
	return func(s *service) { s.psaResolver = r }
}
```

and add the `psaResolver PSACampaignResolver` field to the `service` struct.

- [ ] **Step 5: Implement the adapter**

`internal/adapters/clients/psaportal/campaignresolver.go`:

```go
package psaportal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// ErrStaleCampaignSnapshot means the portal campaign list is too old to resolve
// names against. Resolving fresh purchases through a stale campaign list would
// silently fail to resolve names, or resolve them to superseded campaigns.
var ErrStaleCampaignSnapshot = errors.New("psa campaign snapshot is stale")

// CampaignLister reads internal campaigns and their portal links.
type CampaignLister interface {
	ListCampaigns(ctx context.Context) ([]inventory.Campaign, error)
}

// CampaignResolver implements inventory.PSACampaignResolver over the portal
// campaign snapshot and the internal campaign list.
type CampaignResolver struct {
	snap      psacampaign.SnapshotStore
	campaigns CampaignLister
	now       func() time.Time // test seam
}

func NewCampaignResolver(snap psacampaign.SnapshotStore, campaigns CampaignLister, now func() time.Time) *CampaignResolver {
	if now == nil {
		now = time.Now
	}
	return &CampaignResolver{snap: snap, campaigns: campaigns, now: now}
}

func (r *CampaignResolver) ResolveCampaignID(ctx context.Context, psaName string) (string, bool, error) {
	name := strings.TrimSpace(psaName)
	if name == "" {
		return "", false, nil
	}

	portal, fetchedAt, err := r.snap.GetSnapshot(ctx)
	if err != nil {
		return "", false, fmt.Errorf("read campaign snapshot: %w", err)
	}
	if fetchedAt.IsZero() {
		return "", false, fmt.Errorf("%w: never fetched", ErrStaleCampaignSnapshot)
	}
	if age := r.now().Sub(fetchedAt); age > maxSnapshotAge {
		return "", false, fmt.Errorf("%w (fetched %s ago)", ErrStaleCampaignSnapshot, age.Round(time.Minute))
	}

	var requestID string
	for _, pc := range portal {
		if strings.EqualFold(strings.TrimSpace(pc.Name), name) {
			requestID = pc.ID
			break
		}
	}
	if requestID == "" {
		return "", false, nil // dead campaign name — expected, not an error
	}

	all, err := r.campaigns.ListCampaigns(ctx)
	if err != nil {
		return "", false, fmt.Errorf("list campaigns: %w", err)
	}
	for _, c := range all {
		if c.PSACampaignRequestID == requestID {
			return c.ID, true, nil
		}
	}
	return "", false, nil
}
```

Verify `psacampaign.PortalCampaign`'s name and ID field names against `internal/domain/psacampaign/` before writing this; adjust if they differ.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/adapters/clients/psaportal/ -run TestCampaignResolver -v`
Expected: PASS

- [ ] **Step 7: Verify the hexagonal invariant**

Run: `./scripts/check-imports.sh`
Expected: PASS. A failure here means the resolver logic leaked into `inventory`.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/inventory/psa_resolver.go internal/domain/inventory/service.go internal/adapters/clients/psaportal/campaignresolver.go internal/adapters/clients/psaportal/campaignresolver_test.go
git commit -m "feat: add PSA campaign resolver port and adapter"
```

---

### Task 5: Prefer PSA attribution on import

**Files:**
- Modify: `internal/domain/inventory/service_import_psa.go:95-115,248-266`
- Test: `internal/domain/inventory/service_import_psa_test.go`

**Interfaces:**
- Consumes: `PSAExportRow.PSACampaignName` (Task 1), `PSACampaignResolver` (Task 4), attribution constants (Task 3).
- Produces: new purchases carry `AttributionSource` and `PSACampaignName`.

- [ ] **Step 1: Write the failing test**

```go
func TestImportPSA_PrefersPSAAttribution(t *testing.T) {
	tests := []struct {
		name           string
		psaName        string
		resolveTo      string
		resolveOK      bool
		wantCampaignID string
		wantSource     string
		wantMatcherRun bool
	}{
		{"psa resolves", "Modern", "camp-psa", true, "camp-psa", inventory.AttributionSourcePSA, false},
		{"psa name dead", "Brady modern", "", false, "camp-inferred", inventory.AttributionSourceInferred, true},
		{"no psa name", "", "", false, "camp-inferred", inventory.AttributionSourceInferred, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var matcherRan bool
			// build svc with a resolver stub returning (tt.resolveTo, tt.resolveOK, nil)
			// and a campaign set whose only inferable match is "camp-inferred";
			// set matcherRan via the campaign list access, or assert on the result campaign.
			got := runSingleRowImport(t, tt.psaName)
			if got.CampaignID != tt.wantCampaignID {
				t.Errorf("CampaignID = %q, want %q", got.CampaignID, tt.wantCampaignID)
			}
			if got.AttributionSource != tt.wantSource {
				t.Errorf("AttributionSource = %q, want %q", got.AttributionSource, tt.wantSource)
			}
			if got.PSACampaignName != tt.psaName {
				t.Errorf("PSACampaignName = %q, want %q", got.PSACampaignName, tt.psaName)
			}
			_ = matcherRan
		})
	}
}
```

`runSingleRowImport` builds the service with `inventory.WithPSACampaignResolver(stub)` and an in-memory store (`mocks.NewInMemoryCampaignStore()`), imports one `PSAExportRow`, and returns the created `Purchase`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestImportPSA_PrefersPSAAttribution -v`
Expected: FAIL — attribution fields empty

- [ ] **Step 3: Resolve before inferring**

In `service_import_psa.go`, replace the `FindMatchingCampaign` call site (~:104) with:

```go
		// PSA's own attribution wins when it resolves; inference is the fallback.
		var campaign *Campaign
		attributionSource := AttributionSourceInferred

		if s.psaResolver != nil && row.PSACampaignName != "" {
			campaignID, ok, rErr := s.psaResolver.ResolveCampaignID(ctx, row.PSACampaignName)
			switch {
			case rErr != nil:
				// Lookup failed outright (e.g. stale snapshot). Fall back to
				// inference rather than dropping the row, and say so.
				if s.logger != nil {
					s.logger.Warn(ctx, "PSA campaign resolve failed, falling back to inference",
						observability.String("cert", row.CertNumber),
						observability.String("psaCampaign", row.PSACampaignName),
						observability.Err(rErr))
				}
			case ok:
				campaign = campaignMap[campaignID]
				if campaign != nil {
					attributionSource = AttributionSourcePSA
				}
			default:
				if s.logger != nil {
					s.logger.Info(ctx, "PSA campaign name did not resolve",
						observability.String("cert", row.CertNumber),
						observability.String("psaCampaign", row.PSACampaignName))
				}
			}
		}

		var match CampaignMatch
		if campaign == nil {
			match = FindMatchingCampaign(
				gradeValue,
				buyCostCents,
				meta.CardName,
				meta.SetName,
				meta.CardYear,
				matchingCampaigns,
			)
			if match.Status == "matched" {
				campaign = campaignMap[match.CampaignID]
			}
		}
```

Leave the existing downstream handling of `match` (ambiguous/unmatched → pending items) intact. When PSA resolved the campaign, `match` stays its zero value and the pending-item path is not reached because `campaign != nil`. Verify that against the code below the call site and adjust if the zero `match` is read unconditionally.

- [ ] **Step 4: Set the fields on the new purchase**

In the `&Purchase{...}` literal (~:250):

```go
			PSACampaignName:     row.PSACampaignName,
			AttributionSource:   attributionSource,
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/domain/inventory/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/inventory/service_import_psa.go internal/domain/inventory/service_import_psa_test.go
git commit -m "feat: prefer PSA campaign attribution over inference on import"
```

---

### Task 6: Mark manual reassignment

**Files:**
- Modify: `internal/domain/inventory/service_crud.go:321-334`
- Test: `internal/domain/inventory/service_crud_test.go`

**Interfaces:**
- Consumes: `AttributionSourceManual` (Task 3).
- Produces: operator-assigned purchases carry `attribution_source = 'manual'`.

- [ ] **Step 1: Write the failing test**

```go
func TestReassignPurchase_SetsManualSource(t *testing.T) {
	svc, repo := newTestServiceWithPurchase(t, "purchase-1", "camp-a")
	if err := svc.ReassignPurchase(context.Background(), "purchase-1", "camp-b"); err != nil {
		t.Fatalf("ReassignPurchase: %v", err)
	}
	got := repo.Purchases["purchase-1"]
	if got.AttributionSource != inventory.AttributionSourceManual {
		t.Errorf("AttributionSource = %q, want %q", got.AttributionSource, inventory.AttributionSourceManual)
	}
}
```

The method is `ReassignPurchase(ctx, purchaseID, newCampaignID string) error` at `service_crud.go:321`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/inventory/ -run TestReassignPurchase_SetsManualSource -v`
Expected: FAIL — `AttributionSource = "", want "manual"`

- [ ] **Step 3: Set the source in the reassignment path**

After the successful `UpdatePurchaseCampaign` call, add a `UpdatePurchaseAttributionName` call preserving any existing `psa_campaign_name` and setting source to `AttributionSourceManual`. Read the current purchase first so the PSA name is not clobbered.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/inventory/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/inventory/service_crud.go internal/domain/inventory/service_crud_test.go
git commit -m "feat: mark hand-assigned purchases as manual attribution"
```

---

### Task 7: Reconciliation service

**Files:**
- Create: `internal/domain/inventory/service_reconcile_psa.go`
- Test: `internal/domain/inventory/service_reconcile_psa_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1, 3, 4.
- Produces:
  - `inventory.ReconcileResult{Agreed, Moved, SoldSkipped, Unresolved, Failed int}`
  - `Service.ReconcilePSAAttribution(ctx context.Context, rows []PSAExportRow) (ReconcileResult, error)`

This is the core task. Keep it in its own file — `service_import_psa.go` is already large.

- [ ] **Step 1: Write the failing tests**

```go
func TestReconcilePSAAttribution(t *testing.T) {
	tests := []struct {
		name       string
		psaName    string
		resolveTo  string
		resolveOK  bool
		currentCID string
		sold       bool
		want       inventory.ReconcileResult
	}{
		{"agreement", "Modern", "camp-a", true, "camp-a", false,
			inventory.ReconcileResult{Agreed: 1}},
		{"disagreement moves", "Modern", "camp-a", true, "camp-b", false,
			inventory.ReconcileResult{Moved: 1}},
		{"sold purchase skipped", "Modern", "camp-a", true, "camp-b", true,
			inventory.ReconcileResult{SoldSkipped: 1}},
		{"dead name unresolved", "Brady modern", "", false, "camp-b", false,
			inventory.ReconcileResult{Unresolved: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newReconcileFixture(t, tt.currentCID, tt.sold, tt.resolveTo, tt.resolveOK)
			got, err := svc.ReconcilePSAAttribution(context.Background(),
				[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: tt.psaName}})
			if err != nil {
				t.Fatalf("ReconcilePSAAttribution: %v", err)
			}
			if got != tt.want {
				t.Errorf("result = %+v, want %+v", got, tt.want)
			}
			_ = repo
		})
	}
}

func TestReconcilePSAAttribution_SoldPurchaseRecordsNameWithoutMoving(t *testing.T) {
	svc, repo := newReconcileFixture(t, "camp-b", true, "camp-a", true)
	if _, err := svc.ReconcilePSAAttribution(context.Background(),
		[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Modern"}}); err != nil {
		t.Fatalf("ReconcilePSAAttribution: %v", err)
	}
	got := repo.Purchases["purchase-1"]
	if got.CampaignID != "camp-b" {
		t.Errorf("CampaignID = %q, want unchanged camp-b", got.CampaignID)
	}
	if got.PSACampaignName != "Modern" {
		t.Errorf("PSACampaignName = %q, want Modern", got.PSACampaignName)
	}
}

func TestReconcilePSAAttribution_UnresolvedEnqueuesPendingItem(t *testing.T) {
	svc, repo := newReconcileFixture(t, "camp-b", false, "", false)
	if _, err := svc.ReconcilePSAAttribution(context.Background(),
		[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Brady modern"}}); err != nil {
		t.Fatalf("ReconcilePSAAttribution: %v", err)
	}
	if len(repo.PendingItems) != 1 {
		t.Fatalf("pending items = %d, want 1", len(repo.PendingItems))
	}
}

func TestReconcilePSAAttribution_ResolvingClearsStalePendingItem(t *testing.T) {
	svc, repo := newReconcileFixture(t, "camp-b", false, "camp-a", true)
	repo.PendingItems = []inventory.PendingItem{{CertNumber: "123"}}
	if _, err := svc.ReconcilePSAAttribution(context.Background(),
		[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Modern"}}); err != nil {
		t.Fatalf("ReconcilePSAAttribution: %v", err)
	}
	if len(repo.PendingItems) != 0 {
		t.Errorf("pending items = %d, want 0 (resolved)", len(repo.PendingItems))
	}
}

func TestReconcilePSAAttribution_CLConfidenceFreezing(t *testing.T) {
	tests := []struct {
		name              string
		purchaseDate      string
		campaignUpdatedAt time.Time
		wantNil           bool
	}{
		{"campaign untouched since purchase", "2026-08-01", mustTime("2026-07-01"), false},
		{"campaign written after purchase", "2026-07-01", mustTime("2026-08-01"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newReconcileFixtureWithDates(t, tt.purchaseDate, tt.campaignUpdatedAt)
			if _, err := svc.ReconcilePSAAttribution(context.Background(),
				[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Modern"}}); err != nil {
				t.Fatalf("ReconcilePSAAttribution: %v", err)
			}
			got := repo.Purchases["purchase-1"].CLConfidenceAtPurchase
			if tt.wantNil && got != nil {
				t.Errorf("CLConfidenceAtPurchase = %d, want nil", *got)
			}
			if !tt.wantNil && got == nil {
				t.Error("CLConfidenceAtPurchase = nil, want re-derived value")
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/inventory/ -run TestReconcilePSAAttribution -v`
Expected: FAIL — `svc.ReconcilePSAAttribution undefined`

- [ ] **Step 3: Implement reconciliation**

`internal/domain/inventory/service_reconcile_psa.go`:

```go
package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ReconcileResult counts the outcomes of one reconciliation pass.
type ReconcileResult struct {
	Agreed      int // PSA agrees with our current attribution
	Moved       int // campaign corrected from PSA
	SoldSkipped int // PSA disagrees but the purchase has a linked sale
	Unresolved  int // PSA named a campaign we cannot resolve
	Failed      int // per-row error; logged and skipped
}

// ReconcilePSAAttribution corrects existing purchases against PSA's own campaign
// attribution. PSA is authoritative: where it resolves, it wins.
//
// Sold purchases are skipped rather than corrected. Sales freeze campaign-derived
// values (sale_fee_cents, channel_fee_pct_at_sale, and net_profit_cents, which
// also depends on the purchase's psa_sourcing_fee_cents) and analytics reads the
// stored net_profit_cents rather than recomputing it. Moving a sold purchase's
// campaign without repairing all of that would leave the ledger inconsistent.
func (s *service) ReconcilePSAAttribution(ctx context.Context, rows []PSAExportRow) (ReconcileResult, error) {
	var res ReconcileResult
	if s.psaResolver == nil {
		return res, nil
	}

	certs := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.CertNumber != "" {
			certs = append(certs, r.CertNumber)
		}
	}
	purchases, err := s.purchases.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", certs)
	if err != nil {
		return res, err
	}

	for _, row := range rows {
		p := purchases[row.CertNumber]
		if p == nil {
			continue // cert unknown to us; cannot occur today, must not panic
		}
		if row.PSACampaignName == "" {
			continue
		}

		campaignID, ok, rErr := s.psaResolver.ResolveCampaignID(ctx, row.PSACampaignName)
		if rErr != nil {
			// A lookup that could not run at all is a hard stop: continuing would
			// mark every remaining row unresolved and enqueue spurious pending items.
			return res, rErr
		}

		if !ok {
			res.Unresolved++
			if err := s.recordUnresolvedAttribution(ctx, p, row.PSACampaignName); err != nil {
				res.Failed++
				s.logReconcileFailure(ctx, row.CertNumber, err)
			}
			continue
		}

		if p.CampaignID == campaignID {
			res.Agreed++
			if err := s.purchases.UpdatePurchaseAttributionName(ctx, p.ID, row.PSACampaignName, AttributionSourcePSA); err != nil {
				res.Failed++
				s.logReconcileFailure(ctx, row.CertNumber, err)
			}
			s.resolveStalePendingItem(ctx, row.CertNumber, campaignID)
			continue
		}

		campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
		if err != nil || campaign == nil {
			res.Failed++
			s.logReconcileFailure(ctx, row.CertNumber, err)
			continue
		}

		err = s.purchases.ReattributePurchase(ctx, p.ID, Reattribution{
			CampaignID:             campaignID,
			PSACampaignName:        row.PSACampaignName,
			PSASourcingFeeCents:    campaign.PSASourcingFeeCents,
			CLConfidenceAtPurchase: clConfidenceForReattribution(p.PurchaseDate, campaign),
		})
		switch {
		case errors.Is(err, ErrPurchaseHasSale):
			res.SoldSkipped++
			if nErr := s.purchases.UpdatePurchaseAttributionName(ctx, p.ID, row.PSACampaignName, p.AttributionSource); nErr != nil {
				res.Failed++
				s.logReconcileFailure(ctx, row.CertNumber, nErr)
			}
			s.logger.Info(ctx, "PSA reconcile: skipped sold purchase",
				observability.String("cert", row.CertNumber),
				observability.String("psaCampaign", row.PSACampaignName))
		case err != nil:
			res.Failed++
			s.logReconcileFailure(ctx, row.CertNumber, err)
		default:
			res.Moved++
			s.resolveStalePendingItem(ctx, row.CertNumber, campaignID)
		}
	}

	s.logger.Info(ctx, "PSA attribution reconciled",
		observability.Int("agreed", res.Agreed),
		observability.Int("moved", res.Moved),
		observability.Int("soldSkipped", res.SoldSkipped),
		observability.Int("unresolved", res.Unresolved),
		observability.Int("failed", res.Failed))
	return res, nil
}

// clConfidenceForReattribution returns the campaign's current CL confidence only
// when it is provably the value that was in force at purchase time.
//
// The campaigns table carries no parameter history — only updated_at, which bumps
// on any write. The one sound predicate is "the campaign has not been written at
// all since the purchase". Everything else returns nil (stored as NULL): the 8/15
// buy-terms experiment turns on this column, and a fabricated anachronistic value
// corrupts it silently, whereas a NULL is visible.
func clConfidenceForReattribution(purchaseDate string, campaign *Campaign) *int {
	d, err := time.Parse("2006-01-02", purchaseDate)
	if err != nil {
		return nil
	}
	// Compare against end-of-day so a same-day write does not falsely disqualify.
	if campaign.UpdatedAt.After(d.AddDate(0, 0, 1)) {
		return nil
	}
	c, ok := ParseCLConfidenceMin(campaign.CLConfidence)
	if !ok {
		return nil
	}
	return &c
}

func (s *service) recordUnresolvedAttribution(ctx context.Context, p *Purchase, psaName string) error {
	// Keep the inferred campaign; record PSA's name so a portal-side deletion
	// never loses it again.
	if err := s.purchases.UpdatePurchaseAttributionName(ctx, p.ID, psaName, AttributionSourceInferred); err != nil {
		return err
	}
	// No existing code path enqueues an existing purchase: imports route existing
	// certs to handleExistingPSAPurchase and skip matching entirely.
	return s.enqueueUnresolvedPendingItem(ctx, p, psaName)
}

func (s *service) logReconcileFailure(ctx context.Context, cert string, err error) {
	s.logger.Error(ctx, "PSA reconcile: row failed",
		observability.String("cert", cert), observability.Err(err))
}
```

Implement `enqueueUnresolvedPendingItem` and `resolveStalePendingItem` against `PendingItemRepository` (`internal/domain/inventory/pending_items.go:44-56`), noting these constraints from the real types:

- `s.pendingItemRepo` is **optional and may be nil** (`service.go:212`). Both helpers must no-op when it is nil, not panic.
- `PendingItem` has no field for a PSA campaign name. Set `Status: "unmatched"` and put the unresolvable name in `Candidates` (`[]string`) so the operator can see what PSA claimed. The authoritative record of the name is `campaign_purchases.psa_campaign_name`, written just above — the pending item is only the work queue.
- `SavePendingItems` upserts by `cert_number` and skips resolved items, so re-running a harvest is idempotent.
- `ResolvePendingItem(ctx, id, campaignID)` takes the pending item's **ID**, not its cert number. `resolveStalePendingItem` must `ListPendingItems` and find the matching `CertNumber` first.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/inventory/ -run TestReconcilePSAAttribution -v`
Expected: PASS

- [ ] **Step 5: Check the file size**

Run: `./scripts/check-file-size.sh`
Expected: no failure for `service_reconcile_psa.go`. If it warns, split the pending-item helpers into `service_reconcile_psa_pending.go`.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/inventory/service_reconcile_psa.go internal/domain/inventory/service_reconcile_psa_test.go
git commit -m "feat: reconcile existing purchases against PSA campaign attribution"
```

---

### Task 8: Wire reconciliation into the harvest

**Files:**
- Modify: `cmd/psa-harvest/main.go:80-126`
- Modify: wherever the inventory service is constructed for the harvest binary

**Interfaces:**
- Consumes: `ReconcilePSAAttribution` (Task 7), `NewCampaignResolver` (Task 4).
- Produces: reconciliation runs once per harvest, after both snapshots are refreshed.

Ordering matters: reconciliation must run **after** the campaign snapshot save, so it resolves against the freshest campaign list.

- [ ] **Step 1: Dump the before-state**

Before the first reconciliation, capture the rows that will change. This is an untracked operational artifact — `docs/private/` is gitignored (`.gitignore:90-91`), so committing there is self-contradictory. Write to a path outside the repo:

```bash
psql "$SUPABASE_DB_URL" -c "\copy (SELECT id, campaign_id, attribution_source, cl_confidence_at_purchase, psa_sourcing_fee_cents, cert_number FROM campaign_purchases WHERE purchase_source IN ('psa-vault-offer','psa-grading-offer')) TO '/tmp/psa-attribution-before.csv' CSV HEADER"
```

Keep it until the reattribution is reviewed and the numbers accepted, then delete it. It contains per-purchase cost data — do not attach it to a PR.

- [ ] **Step 2: Construct the resolver and call reconciliation**

In `cmd/psa-harvest/main.go`, inside the `cfg.PSASync.CampaignSyncEnabled` block, after the snapshot save and before `DrainPushQueue`:

```go
		resolver := psaportal.NewCampaignResolver(snap, campaignStore, nil)
		rows, rowsErr := rowProvider.FetchRows(ctx)
		switch {
		case rowsErr != nil:
			// Stale or missing itemized snapshot: skip reconciliation rather than
			// reattributing on old data.
			logger.Warn(ctx, "psa-harvest: skipping attribution reconcile", observability.Err(rowsErr))
		default:
			inv := buildInventoryService(db, logger, inventory.WithPSACampaignResolver(resolver))
			res, recErr := inv.ReconcilePSAAttribution(ctx, rows)
			if recErr != nil {
				logger.Error(ctx, "psa-harvest: attribution reconcile failed", observability.Err(recErr))
			} else {
				logger.Info(ctx, "psa-harvest: attribution reconciled",
					observability.Int("moved", res.Moved),
					observability.Int("unresolved", res.Unresolved),
					observability.Int("soldSkipped", res.SoldSkipped))
			}
		}
```

Adapt `buildInventoryService` and `campaignStore` to whatever this binary already has available — read the surrounding code and reuse its existing constructors rather than adding new ones.

- [ ] **Step 3: Build and verify**

Run: `go build ./... && go vet ./...`
Expected: clean

- [ ] **Step 4: Run the full suite**

Run: `go test -race ./...`
Expected: PASS

- [ ] **Step 5: Run quality checks**

Run: `make check`
Expected: PASS — especially `scripts/check-imports.sh`, which is what catches a regression of the Task 4 boundary.

- [ ] **Step 6: Commit**

```bash
git add cmd/psa-harvest/main.go
git commit -m "feat: run PSA attribution reconciliation after each harvest"
```

---

### Task 9: Production verification

**Files:** none — this task produces evidence, not code.

**Interfaces:**
- Consumes: everything.
- Produces: a verified reconciliation run, or a defect report.

- [ ] **Step 1: Confirm the expected shape before running**

Run against production (read-only):

```bash
psql "$SUPABASE_DB_URL" -c "SELECT count(*) FROM campaign_purchases WHERE purchase_source IN ('psa-vault-offer','psa-grading-offer')"
```

Expected: 45. A different number means the coverage assumptions in the spec have drifted and the targets below need recomputing.

- [ ] **Step 2: Run the harvest and capture the reconcile log line**

Expected, per the spec's Verification section:
- `moved` ≈ 23, roughly 20 of them out of Modern PSA 10
- `unresolved` = 10 (the dead names: `Brady modern` ×7, `Modern 10` ×2, `Modern 8` ×1)
- `soldSkipped` = 0 — all 45 offer purchases are currently unsold. A non-zero value on the first run means the sale-state check is wrong.
- `failed` = 0

Numbers materially different from these mean the resolution logic is wrong. Stop and investigate rather than accepting the run.

- [ ] **Step 3: Verify no null attribution source**

```bash
psql "$SUPABASE_DB_URL" -c "SELECT count(*) FROM campaign_purchases WHERE attribution_source IS NULL"
```

Expected: 0. A non-zero result means a write path was missed.

- [ ] **Step 4: Verify the dead names were captured**

```bash
psql "$SUPABASE_DB_URL" -c "SELECT psa_campaign_name, count(*) FROM campaign_purchases WHERE attribution_source = 'inferred' AND psa_campaign_name IS NOT NULL GROUP BY 1 ORDER BY 2 DESC"
```

Expected: `Brady modern` (7), `Modern 10` (2), `Modern 8` (1). This is the irreversible capture the whole design exists to secure.

- [ ] **Step 5: Record the CL confidence split**

```bash
psql "$SUPABASE_DB_URL" -c "SELECT attribution_source, (cl_confidence_at_purchase IS NULL) AS nulled, count(*) FROM campaign_purchases WHERE purchase_source IN ('psa-vault-offer','psa-grading-offer') GROUP BY 1,2"
```

Most moved rows are expected to be nulled per §6. Report the actual split so the impact on the 8/15 buy-terms experiment's sample size is known now rather than discovered later.

- [ ] **Step 6: Delete the before-state artifact once accepted**

```bash
rm -f /tmp/psa-attribution-before.csv
```

---

## Notes for the implementer

- **The riskiest task is 4.** If `ResolveCampaignID` ends up in `internal/domain/inventory`, `make check` fails and the fix is a rewrite, not a patch. Run `./scripts/check-imports.sh` before committing it.
- **Task 3 Step 7 is easy to skip and silently breaks Task 7.** If the new columns are not added to the shared SELECT list and row scanner, every read returns empty strings and reconciliation will appear to work while comparing against nothing.
- **`s.pendingItemRepo` and `s.psaResolver` are both optional and may be nil.** Every path in Task 7 that touches them must nil-check first. The existing optional dependencies in `service.go:200-215` show the convention.
- **Do not route around `ErrPurchaseHasSale`.** It is a deliberate guard. Sold purchases keep their inferred campaign permanently until the sale-side provenance repair follow-up is designed.
- **PSA names are stored verbatim.** Trim only at resolution time. The stored string must be exactly what PSA sent so a portal-side rename or deletion is always recoverable from our data.
