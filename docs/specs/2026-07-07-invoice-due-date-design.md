# Invoice due-date population + forced-liquidation fix

**Date:** 2026-07-07
**Branch:** `fix/invoice-due-date` (worktree `.worktrees/invoice-due-date`)
**Status:** Design — pending operator review

## Problem (as reported)

PSA invoices are due 7 days after issue (July 2026 portal term, operator-confirmed
2026-07-07). The report stated invoice rows are created with an empty `due_date`, so
the live forced-liquidation heuristic (`IsForcedLiquidation`, `forced_liquidation.go`,
added in PR #458) — which keys on `Invoice.DueDate` — silently flags nothing on new
sales.

## Verified reality (investigation, 2026-07-07)

The reported premise is partly inaccurate. Ground truth gathered from the production API
(`GET https://slabledger.dpao.la/api/credit/invoices`) and the code on `main`:

| Reported | Verified |
|---|---|
| Invoices created with empty `due_date` | The only production creation path — `autoDetectInvoices` (`service_import_psa.go`) — **already sets** `DueDate = invoice_date + defaultPSAPaymentTermDays`. There is no POST-create route; invoice routes are GET + PUT only. |
| Due 7 days after issue | Code constant is **`defaultPSAPaymentTermDays = 15`**, not 7. |
| Rows with empty `due_date` to backfill | Production has **8 invoices**. The 7 historical ones (2026-03-15 → 2026-06-15) have empty `due_date` **and are all `paid`**. The single **unpaid** invoice (`2026-07-01`) already has `dueDate=2026-07-08` = **+7, non-empty**. |

### Root cause of the empty historical rows

Three compounding factors, all confirmed:

1. **`autoDetectInvoices` has two branches; only the create branch sets `due_date`.**
   The update branch (`service_import_psa.go:342-359`) mutates only `TotalCents` /
   `UpdatedAt` and **never populates `due_date`**. Once a row exists empty, no import
   ever fills it — it is stuck empty forever.
2. **The historical rows were born under pre-cutover code.** Their `createdAt`
   timestamps run 2026-03-19 → 2026-06-16; the earliest predates even the squashed
   initial commit (2026-03-25). Production ran SQLite-era code that did not populate
   `due_date`; after cutover, the update branch kept skipping them.
3. **The one populated row was not produced by `main`.** `2026-07-01 → 2026-07-08`
   is **+7**, but `main` computes **+15**. That due date was set out-of-band (a manual
   PUT via `/api/credit/invoices`, or the unmerged `feat/psa-portal-import` harvester),
   consistent with the July portal transition to +7 terms.

### Consequence for the heuristic

`IsForcedLiquidation` matches a forced-channel sale dated 0–6 days **before or on** an
invoice due date. All 7 empty rows are **paid and historical**, so:

- **New-sale detection is unaffected by the backfill.** New forced-channel sales are
  dated ~now and match against the current *unpaid* invoice, which already carries its
  +7 due date.
- **The backfill only affects retroactive reclassification** of past sales and
  finance-reporting accuracy.

The durable *code* fix is therefore: (a) correct the term `15 → 7`, and (b) make the
update branch populate `due_date` when empty, so no row can get stuck empty again.

## Goals

1. New invoices carry a correct, non-empty `due_date` (= `invoice_date + 7` calendar days),
   overridable when a value is already supplied.
2. No invoice row can remain with an empty `due_date` across a subsequent PSA import.
3. Historical empty rows are backfilled via a reviewed one-off script (not a migration).
4. Regression tests lock in both the non-empty guarantee and the +7 arithmetic.
5. Forced-liquidation detection verified end-to-end for a new forced-channel sale.

## Non-goals

- No schema migration (the `due_date` column already exists: `TEXT NOT NULL DEFAULT ''`).
- No frontend changes (no public interface or JSON-shape change).
- No change to `IsForcedLiquidation`'s logic — it is correct; it was starved of data.
- Not merging or depending on `feat/psa-portal-import`.

## Design

### Component 1 — Term correction + pure helper

`internal/domain/inventory/service_import_psa.go`

- Change `defaultPSAPaymentTermDays` from `15` to `7`.
- Extract the arithmetic into a pure, testable helper:

```go
// dueDateFromInvoiceDate returns invoiceDate + defaultPSAPaymentTermDays as
// YYYY-MM-DD, or "" if invoiceDate is empty/unparseable.
func dueDateFromInvoiceDate(invoiceDate string) string {
    t, err := time.Parse("2006-01-02", invoiceDate)
    if err != nil {
        return ""
    }
    return t.AddDate(0, 0, defaultPSAPaymentTermDays).Format("2006-01-02")
}
```

### Component 2 — Populate on create AND on empty-update

Same file, `autoDetectInvoices`:

- **Create branch:** replace the inline block with `dueDate := dueDateFromInvoiceDate(invoiceDate)`.
  Behavior identical except the term is now 7.
- **Update branch:** currently only runs when `inv.TotalCents != totalCents`. Change so
  that when `inv.DueDate == ""`, we also set `inv.DueDate = dueDateFromInvoiceDate(inv.InvoiceDate)`
  and persist — even if the total is unchanged. Concretely, an existing row is written
  back when *either* the total changed *or* its due date is empty and now computable.

This guarantees goal #2: any empty-due-date row is healed the next time PSA data is imported.

**Overridability:** the create branch only sets `due_date` for brand-new rows, and the
update branch only fills it when *empty* — a non-empty due date (e.g. an operator's
manual PUT, or a future portal-harvester value) is never overwritten. That satisfies
"overridable if a value is already supplied."

### Component 3 — One-off backfill script (Postgres)

`scripts/backfill-invoice-due-dates-2026-07-07.sql`

- **Postgres** dialect (`psql`), matching current persistence — the existing precedent
  script (`lcs-bulk-sale-2026-04-11.sql`) is SQLite and predates the cutover; do not
  copy its dialect, only its structure (preview → transaction → verify → documented
  rollback).
- Targets only rows `WHERE due_date = ''`.
- **Era-aware** due-date computation (default — see Decision 2), matching documented PSA
  term history:
  - `invoice_date < 2026-05-15`  → `+14` calendar days
  - `2026-05-15 <= invoice_date <= 2026-06-30` → `+1 business day`
  - `invoice_date >= 2026-07-01` → `+7` calendar days
- Structure: header comment (context/usage/rollback), a `SELECT` preview of affected
  rows with computed due dates, then `BEGIN; UPDATE ...; COMMIT;`, then a verification
  `SELECT`. Reviewed by the operator before running against prod.
- Rollback: `UPDATE invoices SET due_date = '' WHERE id IN (<the 7 ids>);`

Note: because all 7 rows are paid, this is a finance-reporting cleanup; it does not alter
live forced-liquidation detection.

### Component 4 — Tests

`internal/domain/inventory/service_import_psa_test.go` (new or extended) and existing
import tests:

1. **`TestDueDateFromInvoiceDate`** — table-driven: `2026-07-01 → 2026-07-08`,
   month-boundary (`2026-07-28 → 2026-08-04`), leap-ish/quarter boundary, empty input `→ ""`,
   malformed input `→ ""`.
2. **Created invoice carries non-empty due date** — drive `autoDetectInvoices` (or the
   public `ImportPSA` entry) with a row that produces a new invoice; assert the persisted
   invoice's `DueDate != ""` and equals `invoice_date + 7`.
3. **Update branch heals an empty due date** — seed an existing invoice with `DueDate == ""`,
   run an import for that invoice date, assert `DueDate` is now populated (+7) even when
   the total is unchanged.
4. **End-to-end forced-liquidation** (`service_crud` path) — seed an unpaid invoice with
   `DueDate = today+5`; create an `inperson` sale dated today; assert the persisted sale's
   `ForcedLiquidation == true`. Uses the in-memory store / mocks; no production mutation.

All table-driven per project convention; mocks from `internal/testutil/mocks`.

## Data flow (end-to-end)

```
ImportPSA(rows)
  └─ autoDetectInvoices
       ├─ create branch  → due_date = invoice_date + 7   (new rows)
       └─ update branch  → due_date = invoice_date + 7    (when empty)
                                    │
new forced-channel Sale ───────────┤
  service_crud.RecordSale / service_import_orders
       └─ ListInvoices → IsForcedLiquidation(channel, saleDate, invoices)
             └─ true when 0 <= (dueDate - saleDate) <= 6 days on an unpaid invoice
                  └─ Sale.ForcedLiquidation persisted
                       └─ portfolio PNL: Forced vs Discretionary split
```

## Decisions (operator was away at design time — confirm at review gate)

1. **Update-branch fix: YES.** Populate `due_date` in the update branch when empty. This
   is the durable fix; without it, rows stay stuck empty. *(Recommended; assumed.)*
2. **Backfill: era-aware** (`+14` / `+1BD` / `+7`) rather than uniform `+7`. Only ~7 rows;
   reporting correctness is cheap. Uniform `+7` is a one-line fallback if preferred.
   *(Assumed; low-stakes since all rows are paid.)*

## Verification plan

- `go test ./internal/domain/inventory/...` — new + existing tests pass.
- `go test -race ./...` — full suite, race-clean (per project rule before commit).
- `golangci-lint run` + file-size check (`make check`).
- Backfill script: run its dry-run preview against a prod snapshot / read-only, share
  output for operator review **before** any `COMMIT` against prod.
- Manual confirmation that a new forced-channel sale within 6 days of the unpaid
  invoice's due date yields `forced_liquidation = true` (covered by the E2E Go test;
  optional live spot-check post-deploy).

## Risks / edge cases

- **Timezone / day-boundary:** arithmetic uses date-only `time.Parse("2006-01-02")` and
  `AddDate`, matching the existing code and `IsForcedLiquidation`. No TZ drift.
- **Duplicate invoice dates:** `autoDetectInvoices` already handles multiple invoices per
  date via a slice map; the empty-due-date fill applies per-row, preserving that.
- **Non-empty due dates never clobbered:** update branch fills only when empty, so manual
  operator PUTs and any future harvester values survive.
- **Backfill idempotency:** script targets `due_date = ''` only; re-running is a no-op
  after the first successful run.
