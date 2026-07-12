# Invoice Due-Date Population + Forced-Liquidation Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Guarantee every PSA invoice carries a correct, non-empty `due_date` (= `invoice_date + 7` calendar days) so the forced-liquidation heuristic flags new forced-channel sales, and backfill the 7 legacy empty-due-date rows.

**Architecture:** Extract the due-date arithmetic in `service_import_psa.go` into a pure helper, correct the payment term `15 → 7`, and fill `due_date` in both branches of `autoDetectInvoices` (create, and update-when-empty). A one-off era-aware Postgres script backfills history. No schema migration; no frontend change. `IsForcedLiquidation` itself is unchanged — it was correct but starved of due-date data.

**Tech Stack:** Go 1.26 (hexagonal), Postgres (`due_date TEXT NOT NULL DEFAULT ''`), table-driven tests with `internal/testutil/mocks`.

**Design:** `docs/specs/2026-07-07-invoice-due-date-design.md` (operator-approved 2026-07-07).

**Worktree:** `.worktrees/invoice-due-date`, branch `fix/invoice-due-date`. All paths below are relative to that worktree root.

## Global Constraints

- Payment term is **7 calendar days** (July 2026 PSA portal; operator-confirmed). Constant `defaultPSAPaymentTermDays = 7`.
- All dates are strings in `YYYY-MM-DD` format; use `time.Parse("2006-01-02", ...)` / `AddDate` (date-only, no TZ math), matching `IsForcedLiquidation`.
- Backfill era history (for the script only): `invoice_date < 2026-05-15` → **+14 calendar days**; `2026-05-15 ≤ invoice_date ≤ 2026-06-30` → **+1 business day**; `invoice_date ≥ 2026-07-01` → **+7 calendar days**.
- Non-empty existing `due_date` values must never be overwritten (overridability).
- Backfill is a reviewed one-off SQL script (`scripts/`), **not** a migration. Dry-run preview → operator review → `BEGIN/COMMIT` against prod.
- Run `go test -race ./...` and `golangci-lint run` before the final commit. Never merge to main directly — open a PR.
- Unexported symbols (`dueDateFromInvoiceDate`) are tested in `package inventory`; exported-API tests live in `package inventory_test`.

---

### Task 1: Extract due-date helper, correct term to 7, use it on create

**Files:**
- Modify: `internal/domain/inventory/service_import_psa.go` (const at line 14; create branch at lines 368-380)
- Test (create): `internal/domain/inventory/service_import_psa_internal_test.go` (new, `package inventory`)

**Interfaces:**
- Produces: `func dueDateFromInvoiceDate(invoiceDate string) string` — returns `invoiceDate + 7` calendar days as `YYYY-MM-DD`, or `""` when `invoiceDate` is empty or unparseable. Consumed by Task 2.
- Produces: `const defaultPSAPaymentTermDays = 7`.

- [ ] **Step 1: Write the failing helper test**

Create `internal/domain/inventory/service_import_psa_internal_test.go`:

```go
package inventory

import "testing"

func TestDueDateFromInvoiceDate(t *testing.T) {
	tests := []struct {
		name        string
		invoiceDate string
		want        string
	}{
		{"plus 7 days", "2026-07-01", "2026-07-08"},
		{"crosses month boundary", "2026-07-28", "2026-08-04"},
		{"crosses year boundary", "2026-12-29", "2027-01-05"},
		{"empty input", "", ""},
		{"malformed input", "not-a-date", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dueDateFromInvoiceDate(tt.invoiceDate); got != tt.want {
				t.Errorf("dueDateFromInvoiceDate(%q) = %q, want %q", tt.invoiceDate, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd .worktrees/invoice-due-date && go test ./internal/domain/inventory/ -run TestDueDateFromInvoiceDate`
Expected: FAIL — `undefined: dueDateFromInvoiceDate`.

- [ ] **Step 3: Change the constant and add the helper**

In `internal/domain/inventory/service_import_psa.go`, change line 14 from `const defaultPSAPaymentTermDays = 15` to:

```go
// defaultPSAPaymentTermDays is the standard net-payment term for PSA invoices
// (July 2026 portal terms: due 7 calendar days after issue).
const defaultPSAPaymentTermDays = 7
```

Add the helper immediately above `func (s *service) autoDetectInvoices` (before line 303):

```go
// dueDateFromInvoiceDate returns invoiceDate advanced by defaultPSAPaymentTermDays
// as a YYYY-MM-DD string, or "" if invoiceDate is empty or unparseable.
func dueDateFromInvoiceDate(invoiceDate string) string {
	t, err := time.Parse("2006-01-02", invoiceDate)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, defaultPSAPaymentTermDays).Format("2006-01-02")
}
```

- [ ] **Step 4: Refactor the create branch to use the helper**

In `autoDetectInvoices`, replace the create-branch block (currently lines 368-380):

```go
		dueDate := ""
		if t, err := time.Parse("2006-01-02", invoiceDate); err == nil {
			dueDate = t.AddDate(0, 0, defaultPSAPaymentTermDays).Format("2006-01-02")
		}
		inv := &Invoice{
			ID:          s.idGen(),
			InvoiceDate: invoiceDate,
			TotalCents:  totalCents,
			DueDate:     dueDate,
			Status:      "unpaid",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
```

with:

```go
		inv := &Invoice{
			ID:          s.idGen(),
			InvoiceDate: invoiceDate,
			TotalCents:  totalCents,
			DueDate:     dueDateFromInvoiceDate(invoiceDate),
			Status:      "unpaid",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
```

- [ ] **Step 5: Run the helper test to verify it passes**

Run: `cd .worktrees/invoice-due-date && go test ./internal/domain/inventory/ -run TestDueDateFromInvoiceDate -v`
Expected: PASS (all 5 subtests).

- [ ] **Step 6: Run the existing import tests to check for regressions**

Run: `cd .worktrees/invoice-due-date && go test ./internal/domain/inventory/ -run TestService_ImportPSAExportGlobal`
Expected: PASS (existing behavior preserved; term change does not affect these tests, which assert totals not due dates).

- [ ] **Step 7: Commit**

```bash
cd .worktrees/invoice-due-date
git add internal/domain/inventory/service_import_psa.go internal/domain/inventory/service_import_psa_internal_test.go
git commit -m "fix(inventory): correct PSA payment term to 7d, extract dueDateFromInvoiceDate helper

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Populate due_date on the update branch when empty (heal)

**Files:**
- Modify: `internal/domain/inventory/service_import_psa.go` (update branch, lines 342-361)
- Test: `internal/domain/inventory/service_test.go` (add to existing `package inventory_test`)

**Interfaces:**
- Consumes: `dueDateFromInvoiceDate` (Task 1).
- Behavior change: in `autoDetectInvoices`, an existing invoice is written back when **either** its total changed **or** its `DueDate` is empty and now computable. `updated` increments once per written invoice.

- [ ] **Step 1: Write the failing heal test**

Add to `internal/domain/inventory/service_test.go` (external `inventory_test` package):

```go
// A re-import over an existing invoice with an empty due_date must populate the
// due_date (invoice_date + 7) even when the invoice total is unchanged — this is
// how legacy empty rows self-heal on the next import.
func TestService_ImportPSAExportGlobal_HealsEmptyDueDate(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// First import creates the invoice (now with a +7 due date after Task 1).
	rows := []inventory.PSAExportRow{
		{CertNumber: "HEAL001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 200, Date: "2026-07-01", InvoiceDate: "2026-07-01", Category: "Pokemon"},
	}
	if _, err := svc.ImportPSAExportGlobal(ctx, rows); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Simulate a legacy row: blank out the due date, leaving the total intact.
	var invID string
	for id, inv := range repo.Invoices {
		if inv.InvoiceDate == "2026-07-01" {
			// A freshly-created invoice must already carry a non-empty +7 due date.
			if inv.DueDate != "2026-07-08" {
				t.Errorf("created invoice DueDate = %q, want 2026-07-08 (creation must populate due_date)", inv.DueDate)
			}
			inv.DueDate = ""
			invID = id
		}
	}
	if invID == "" {
		t.Fatal("expected an invoice for 2026-07-01 after first import")
	}

	// Re-import the identical row: total is unchanged, but the empty due date must heal.
	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if result.InvoicesUpdated != 1 {
		t.Errorf("InvoicesUpdated = %d, want 1 (heal writes the invoice)", result.InvoicesUpdated)
	}
	if got := repo.Invoices[invID].DueDate; got != "2026-07-08" {
		t.Errorf("healed DueDate = %q, want 2026-07-08", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd .worktrees/invoice-due-date && go test ./internal/domain/inventory/ -run TestService_ImportPSAExportGlobal_HealsEmptyDueDate`
Expected: FAIL — `InvoicesUpdated = 0, want 1` and `healed DueDate = "", want 2026-07-08` (the update branch never touches due_date and skips the write when the total is unchanged).

- [ ] **Step 3: Add the heal logic to the update branch**

In `autoDetectInvoices`, replace the update-branch loop body (currently lines 345-358):

```go
			for _, inv := range existing {
				if inv.TotalCents != totalCents {
					inv.TotalCents = totalCents
					inv.UpdatedAt = time.Now()
					if err := s.finance.UpdateInvoice(ctx, inv); err != nil {
						if s.logger != nil {
							s.logger.Warn(ctx, "autoDetectInvoices: failed to update invoice",
								observability.String("invoiceDate", invoiceDate),
								observability.Err(err))
						}
					} else {
						updated++
					}
				}
			}
```

with:

```go
			for _, inv := range existing {
				needsWrite := false
				if inv.TotalCents != totalCents {
					inv.TotalCents = totalCents
					needsWrite = true
				}
				// Heal legacy rows: fill an empty due date so the forced-liquidation
				// heuristic has data. Never overwrite a due date that is already set.
				if inv.DueDate == "" {
					if dd := dueDateFromInvoiceDate(inv.InvoiceDate); dd != "" {
						inv.DueDate = dd
						needsWrite = true
					}
				}
				if needsWrite {
					inv.UpdatedAt = time.Now()
					if err := s.finance.UpdateInvoice(ctx, inv); err != nil {
						if s.logger != nil {
							s.logger.Warn(ctx, "autoDetectInvoices: failed to update invoice",
								observability.String("invoiceDate", invoiceDate),
								observability.Err(err))
						}
					} else {
						updated++
					}
				}
			}
```

- [ ] **Step 4: Run the heal test to verify it passes**

Run: `cd .worktrees/invoice-due-date && go test ./internal/domain/inventory/ -run TestService_ImportPSAExportGlobal_HealsEmptyDueDate -v`
Expected: PASS.

- [ ] **Step 5: Run the full invoice test group to confirm no regression**

Run: `cd .worktrees/invoice-due-date && go test ./internal/domain/inventory/ -run TestService_ImportPSAExportGlobal`
Expected: PASS — including the existing `InvoiceUpdatesOnReimport` (total-change path still writes and counts as one update).

- [ ] **Step 6: Commit**

```bash
cd .worktrees/invoice-due-date
git add internal/domain/inventory/service_import_psa.go internal/domain/inventory/service_test.go
git commit -m "fix(inventory): heal empty invoice due_date on re-import

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: End-to-end forced-liquidation verification test

**Files:**
- Test: `internal/domain/inventory/service_test.go` (add to `package inventory_test`)

**Interfaces:**
- Consumes: `inventory.CreateSale` (exported), `IsForcedLiquidation` (exercised transitively), `mocks.NewInMemoryCampaignStore`.
- No production code changes — this task guards the wired path end-to-end.

Note: `time.Now()` is used to build a due date 5 days out so the sale falls inside the 0–6 day window. Go tests may call `time.Now()` freely.

- [ ] **Step 1: Write the end-to-end test**

Add to `internal/domain/inventory/service_test.go`:

```go
// End-to-end: a forced-channel (inperson) sale dated within 6 days before an
// unpaid invoice's due date must persist ForcedLiquidation = true via CreateSale.
func TestService_CreateSale_FlagsForcedLiquidation(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	today := time.Now()
	saleDate := today.Format("2006-01-02")
	dueDate := today.AddDate(0, 0, 5).Format("2006-01-02") // 5 days ahead → inside the 0..6 window

	// Unpaid invoice due in 5 days.
	if err := repo.CreateInvoice(ctx, &inventory.Invoice{
		ID: "inv-forced", InvoiceDate: today.AddDate(0, 0, -2).Format("2006-01-02"),
		TotalCents: 100000, DueDate: dueDate, Status: "unpaid",
	}); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}

	// Purchase dated well before the sale so CreateSale's date checks pass.
	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "FORCED01",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: today.AddDate(0, 0, -30).Format("2006-01-02"),
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup purchase: %v", err)
	}

	s := &inventory.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    inventory.SaleChannelInPerson,
		SalePriceCents: 60000,
		SaleDate:       saleDate,
	}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	if !s.ForcedLiquidation {
		t.Errorf("ForcedLiquidation = false, want true (inperson sale %s, invoice due %s)", saleDate, dueDate)
	}

	// Control: an ebay sale on the same date must NOT be flagged.
	p2 := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Pikachu", CertNumber: "FORCED02",
		GradeValue: 10, BuyCostCents: 30000, PurchaseDate: today.AddDate(0, 0, -30).Format("2006-01-02"),
	}
	if err := svc.CreatePurchase(ctx, p2); err != nil {
		t.Fatalf("setup purchase 2: %v", err)
	}
	s2 := &inventory.Sale{
		PurchaseID:     p2.ID,
		SaleChannel:    inventory.SaleChannelEbay,
		SalePriceCents: 40000,
		SaleDate:       saleDate,
	}
	if err := svc.CreateSale(ctx, s2, c, p2); err != nil {
		t.Fatalf("CreateSale (control): %v", err)
	}
	if s2.ForcedLiquidation {
		t.Errorf("control ForcedLiquidation = true, want false (ebay is not a forced channel)")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `cd .worktrees/invoice-due-date && go test ./internal/domain/inventory/ -run TestService_CreateSale_FlagsForcedLiquidation -v`
Expected: PASS. (This guards the full wiring — invoice due date → `ListInvoices` → `IsForcedLiquidation` → persisted flag. It passes because the heuristic is correct once a due date exists; a failure would signal a wiring break.)

- [ ] **Step 3: Commit**

```bash
cd .worktrees/invoice-due-date
git add internal/domain/inventory/service_test.go
git commit -m "test(inventory): end-to-end forced-liquidation flag on forced-channel sale

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Era-aware backfill script for legacy empty due dates

**Files:**
- Create: `scripts/backfill-invoice-due-dates-2026-07-07.sql`

**Interfaces:**
- Standalone Postgres (`psql`) script. Targets only `WHERE due_date = ''`. Idempotent (a second run matches nothing). Not wired into migrations.

- [ ] **Step 1: Write the backfill script**

Create `scripts/backfill-invoice-due-dates-2026-07-07.sql`:

```sql
-- =============================================================================
-- Backfill invoice due dates — 2026-07-07
-- =============================================================================
--
-- Context:
--   Historical invoices were created without a due_date (pre-cutover code, and
--   autoDetectInvoices' update branch never populated it). The forced-liquidation
--   heuristic (IsForcedLiquidation) keys on Invoice.DueDate. This one-off script
--   backfills the empty rows. All affected rows are paid & historical, so this is
--   a finance-reporting cleanup; it does not change detection on new sales.
--
--   Era-aware terms (what PSA actually applied when each invoice was issued):
--     invoice_date <  2026-05-15            -> +14 calendar days
--     2026-05-15 .. 2026-06-30 (inclusive)  -> +1 business day
--     invoice_date >= 2026-07-01            -> +7 calendar days
--
-- Target DB: Postgres (Supabase in prod, local Postgres in dev). Dates are TEXT
--   in YYYY-MM-DD; cast to ::date for arithmetic, format back with to_char.
--
-- Usage:
--   # DRY RUN — preview only, no writes (Step 0 runs outside any transaction):
--   psql "$DATABASE_URL" -f scripts/backfill-invoice-due-dates-2026-07-07.sql
--   # ...review Step 0 output with the operator, THEN uncomment Step 1's
--   #    BEGIN/UPDATE/COMMIT block and re-run to apply.
--
-- Rollback:
--   UPDATE invoices SET due_date = '' WHERE id IN (<ids listed in the preview>);
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Step 0: Preview — rows that WILL be changed and their computed due dates
-- ---------------------------------------------------------------------------
SELECT
    id,
    invoice_date,
    status,
    CASE
        WHEN invoice_date::date < DATE '2026-05-15' THEN 'pre-05-15 (+14d)'
        WHEN invoice_date::date <= DATE '2026-06-30' THEN 'mid (+1 business day)'
        ELSE 'post-07-01 (+7d)'
    END AS era,
    to_char(
        CASE
            WHEN invoice_date::date < DATE '2026-05-15'
                THEN invoice_date::date + 14
            WHEN invoice_date::date <= DATE '2026-06-30'
                THEN invoice_date::date + CASE EXTRACT(DOW FROM invoice_date::date)
                                              WHEN 5 THEN 3  -- Fri -> Mon
                                              WHEN 6 THEN 2  -- Sat -> Mon
                                              ELSE 1          -- Sun/Mon-Thu -> next day
                                          END
            ELSE invoice_date::date + 7
        END, 'YYYY-MM-DD') AS computed_due_date
FROM invoices
WHERE due_date = ''
ORDER BY invoice_date;

-- ---------------------------------------------------------------------------
-- Step 1: Apply — UNCOMMENT after the Step 0 preview is reviewed
-- ---------------------------------------------------------------------------
-- BEGIN;
--
-- UPDATE invoices
-- SET due_date = to_char(
--         CASE
--             WHEN invoice_date::date < DATE '2026-05-15'
--                 THEN invoice_date::date + 14
--             WHEN invoice_date::date <= DATE '2026-06-30'
--                 THEN invoice_date::date + CASE EXTRACT(DOW FROM invoice_date::date)
--                                               WHEN 5 THEN 3
--                                               WHEN 6 THEN 2
--                                               ELSE 1
--                                           END
--             ELSE invoice_date::date + 7
--         END, 'YYYY-MM-DD'),
--     updated_at = now()
-- WHERE due_date = '';
--
-- COMMIT;

-- ---------------------------------------------------------------------------
-- Step 2: Verify — after applying, this should return zero rows
-- ---------------------------------------------------------------------------
SELECT count(*) AS remaining_empty_due_dates FROM invoices WHERE due_date = '';
```

- [ ] **Step 2: Validate the SQL against local Postgres (dry run)**

Run: `cd .worktrees/invoice-due-date && psql "$DATABASE_URL" -f scripts/backfill-invoice-due-dates-2026-07-07.sql`
Expected: the Step 0 preview and Step 2 count both execute without a SQL error (0 rows locally is fine — the point is syntactic validity and that the CASE/DOW expressions parse). If `$DATABASE_URL` is unset or unreachable in the dev environment, note that and defer validation to the operator-run dry run against prod.

- [ ] **Step 3: Commit**

```bash
cd .worktrees/invoice-due-date
git add scripts/backfill-invoice-due-dates-2026-07-07.sql
git commit -m "chore(scripts): era-aware backfill for legacy empty invoice due dates

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 5: Full verification, lint, and PR

**Files:** none (verification + PR only)

- [ ] **Step 1: Run the full test suite with race detection**

Run: `cd .worktrees/invoice-due-date && go test -race ./...`
Expected: PASS (all packages).

- [ ] **Step 2: Lint**

Run: `cd .worktrees/invoice-due-date && golangci-lint run ./internal/domain/inventory/...`
Expected: no findings. (If `make check` is preferred, run it — it also runs the import/file-size checks.)

- [ ] **Step 3: Push the branch and open a PR to main**

```bash
cd .worktrees/invoice-due-date
git push -u origin fix/invoice-due-date
gh pr create --base main --title "fix(inventory): populate invoice due_date so forced-liquidation flags new sales" --body "$(cat <<'EOF'
## Summary
- Correct the PSA payment term to **7 calendar days** (`defaultPSAPaymentTermDays` 15→7) and extract a pure `dueDateFromInvoiceDate` helper.
- Populate `due_date` on invoice **create** and **heal it on re-import** when an existing row's due date is empty — closing the gap where `autoDetectInvoices`' update branch never set it.
- Era-aware one-off Postgres backfill script for the 7 legacy empty rows (+14 / +1BD / +7), dry-run gated on operator review.
- Tests: +7 arithmetic (table-driven), created invoice carries due_date, empty due_date self-heals, and an end-to-end forced-liquidation flag on a forced-channel sale.

## Context
Investigation found the reported premise was partly inaccurate: the create path already set a due date, but at +15, and the only empty rows are historical *paid* invoices — so the backfill is reporting cleanup, while the durable code fix is the term change + update-branch heal. See `docs/specs/2026-07-07-invoice-due-date-design.md`.

## Verification
- `go test -race ./...` — green.
- `golangci-lint run` — clean.
- Backfill script: **not run against prod in this PR** — operator reviews the Step 0 dry-run before applying.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR created against `main`. Do **not** merge — leave for review.

- [ ] **Step 4: Report the PR URL and the backfill next-step**

Surface the PR URL, and flag that the backfill script still needs an operator-reviewed dry run against prod before it is applied.

---

## Self-Review

**Spec coverage:**
- Populate at creation paths → Task 1 (create branch + term) ✓. Only one production creation path (`autoDetectInvoices`); confirmed no POST route.
- Overridable if already supplied → create sets only new rows; heal fills only when empty (Task 2) ✓.
- Backfill historical rows, one-off reviewed script, era-aware → Task 4 ✓.
- Test: created invoice carries non-empty due_date → Task 2's heal test asserts `DueDate == "2026-07-08"` immediately after the first import (before blanking), and Task 1 proves the arithmetic ✓.
- Test: +7d arithmetic → Task 1 ✓.
- Verify forced_liquidation=true end-to-end → Task 3 ✓.
- Worktree, PR to main, never merge → Task 5 ✓.

**Placeholder scan:** No TBD/TODO; all code blocks are complete and reference real symbols (`ImportPSAExportGlobal`, `InvoicesUpdated`, `repo.Invoices`, `SaleChannelInPerson`, `CreateInvoice`).

**Type consistency:** `dueDateFromInvoiceDate(string) string` used identically in Tasks 1 and 2. `defaultPSAPaymentTermDays = 7` single source. Mock/service construction (`inventory.NewService(repo×7, withTestIDGen())`) matches existing tests verbatim.
