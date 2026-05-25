# DH Log-Noise & Functionality Fixes — Design

**Date:** 2026-05-25
**Scope:** Single PR. Three related fixes in the DH integration path.

## Problem

Production logs show three distinct error patterns dominating noise:

1. **`grading_required` HTTP 400** on every call to `GET /enterprise/cards/{id}/recent-sales`. DH now requires `grading_company` and `grade` query params; we don't send them. Breaks price refresh, DH intelligence refresh, and `recaptureMarketSnapshot` (~90% of error volume).
2. **`Couldn't find Card with 'id'=X`** 404s on `/enterprise/cards/lookup?card_id=X` for a contiguous range of DH card IDs (e.g. 82629–82672) that no longer exist on DH. We retry every cycle.
3. **`partner_card_error`** on certs where PSA returns blank `card_number` (e.g. MEP-ME "PIKACHU AT THE MUSEUM" promo). DH refuses to create a partner card. We re-push every cycle indefinitely with no operator-visible terminal state.

## Goals

- Restore price refresh + DH intelligence refresh by sending the required grade params.
- Stop hammering DH with lookups for known-dead card IDs.
- Move stuck `partner_card_error` rows to a terminal, operator-visible state.

## Non-goals

- Local card-number override path (rejected: DH supports an override field, but the user has chosen not to maintain local overrides).
- Per-ID tombstone management UI (single "Clear all" admin action is enough for now).
- Backfilling existing dead IDs into the tombstone table (next refresh cycle observes them; tombstoned within ~3 cycles).
- Telemetry/metrics on dismissal counts.

## Architecture overview

All changes land in existing locations; no new packages. Hexagonal invariant preserved (domain depends only on interfaces).

| Layer | Change |
|---|---|
| `internal/domain/pricing/` | Add `Grade int` to `Card`/`CardLookup` |
| `internal/adapters/clients/dh/` | `RecentSales` takes `gradingCompany, grade`; `MarketDataEnterprise` takes `grade`; `RecentSales` returns `ProviderInvalidRequest` if grade<=0 |
| `internal/adapters/clients/dhprice/` | Pass grade through; widen internal `dhClient` interface; consult tombstone repo |
| New domain interface | `pricing.DHCardTombstoneRepo` (or sub-package) with `IsTombstoned`, `RecordFailure`, `Clear`, `ClearAll`, `Count` |
| `internal/adapters/storage/postgres/` | New `dh_card_tombstones` table + repo impl |
| `internal/adapters/scheduler/dh_psa_import.go` | Wire `IncrementDHPushAttempts` in `partner_card_error` branch; auto-dismiss at 5 |
| `internal/adapters/storage/postgres/purchase_dh_push_store.go` | Broaden counter-reset list in `SetDHPushStatus` |
| `internal/adapters/httpserver/handlers/` | New admin endpoints: `GET /api/admin/dh-tombstones/count`, `POST /api/admin/dh-tombstones/clear` |
| `web/src/react/pages/admin/DHStatsPanel.tsx` | Add tombstone count + "Clear all tombstones" button |
| New migration | `000005_add_dh_card_tombstones` (up/down) |

## Fix #1 — Thread grade through pricing

### Interface change

Add `Grade int` to `pricing.Card` and `pricing.CardLookup`. Existing callers populate it from `Purchase.GradeValue` (rounded to int). The card-trajectory refresh scheduler passes `Grade: 10` as the default for intelligence sweeps that don't know the grade.

### Internal `dhClient` interface (in `dhprice/provider.go`)

```go
RecentSales(ctx context.Context, cardID int, gradingCompany string, grade int) ([]dh.RecentSale, error)
```

`gradingCompany` is hardcoded `"PSA"` at the call boundary (PSA-only today).

### `dh.Client.RecentSales`

```go
func (c *Client) RecentSales(ctx context.Context, cardID int, gradingCompany string, grade int) ([]RecentSale, error) {
    if grade <= 0 {
        return nil, apperrors.ProviderInvalidRequest(providerName,
            fmt.Errorf("grade required for recent-sales lookup (card_id=%d)", cardID))
    }
    fullURL := fmt.Sprintf("%s/api/v1/enterprise/cards/%d/recent-sales?grading_company=%s&grade=%d",
        c.baseURL, cardID, gradingCompany, grade)
    // ...existing decoding...
}
```

### `dh.Client.MarketDataEnterprise`

Takes a new `grade int` param; forwards to internal `RecentSales` call. Caller (card-trajectory refresh) passes `10`.

## Fix #2 — Tombstone DH card IDs after 404s

### Migration (`000005_add_dh_card_tombstones`)

```sql
-- up.sql
CREATE TABLE IF NOT EXISTS dh_card_tombstones (
  dh_card_id    BIGINT PRIMARY KEY,
  first_seen_at TIMESTAMP NOT NULL DEFAULT NOW(),
  last_seen_at  TIMESTAMP NOT NULL DEFAULT NOW(),
  attempts      INT NOT NULL DEFAULT 1,
  last_error    TEXT NOT NULL DEFAULT ''
);

-- down.sql
DROP TABLE IF EXISTS dh_card_tombstones;
```

### Domain interface

```go
type DHCardTombstoneRepo interface {
    IsTombstoned(ctx context.Context, cardID int) (bool, error)
    RecordFailure(ctx context.Context, cardID int, errMsg string) (attempts int, err error)
    Clear(ctx context.Context, cardID int) error
    ClearAll(ctx context.Context) (cleared int, err error)
    Count(ctx context.Context) (int, error)
}
```

### Tombstone threshold

**3 attempts.** On `RecordFailure` returning `attempts >= 3`, future `IsTombstoned(cardID)` returns true.

### Wiring

- `dhprice.Provider.GetPrice` and `dh.Client.MarketDataEnterprise` consult `IsTombstoned` before issuing `CardLookup`. If tombstoned, return nil (no price/no data) without an API call.
- 404 detection: match on `apperrors.ErrProvNotFound` only. Any other error type does NOT increment the counter.
- The tombstone repo is wired into both `dhprice.Provider` (via functional option) and the card-trajectory refresh scheduler (constructor injection).

### Log levels

- On `RecordFailure` increment (attempts 1–2): INFO `"dh card lookup failed, tombstone count=N"`
- On reaching threshold (attempts >= 3): INFO `"dh card tombstoned, no further lookups"`
- On subsequent skipped lookups (IsTombstoned hit): DEBUG

### Admin escape hatch

`POST /api/admin/dh-tombstones/clear` calls `ClearAll`. Frontend shows the count and a "Clear all tombstones" button on `DHStatsPanel.tsx`, mirroring the existing Reset row. Confirmation dialog, then POST, then toast.

`GET /api/admin/dh-tombstones/count` returns `{count: int}` for the panel.

## Fix #3 — Auto-dismiss after 5 partner_card_error

### Scheduler change (`internal/adapters/scheduler/dh_psa_import.go`)

In the `case dh.PSAImportStatusPartnerCardError:` branch:

```go
case dh.PSAImportStatusPartnerCardError:
    attempts, err := s.purchaseRepo.IncrementDHPushAttempts(ctx, p.ID)
    if err != nil {
        // log + skip event as today
        return processSkipped
    }
    if attempts >= 5 {
        s.logger.Info(ctx, "dh push: auto-dismissing after 5 partner_card_error attempts",
            observability.String("purchaseID", p.ID),
            observability.String("cert", p.CertNumber),
            observability.String("dhError", result.Error))
        s.pushStatusUpdater.SetDHPushStatus(ctx, p.ID, inventory.DHPushStatusDismissed,
            fmt.Sprintf("auto-dismissed: 5x partner_card_error: %s", result.Error))
        s.recordSkipEvent(ctx, p, "auto_dismissed_partner_card_error: "+result.Error)
        return processSkipped
    }
    s.logger.Debug(ctx, "dh push: partner_card_error, leaving pending",
        observability.String("purchaseID", p.ID),
        observability.String("cert", p.CertNumber),
        observability.Int("attempts", attempts),
        observability.String("dhError", result.Error))
    s.recordSkipEvent(ctx, p, "partner_card_error: "+result.Error)
    return processSkipped
```

Other failure resolutions (`psa_error`, `unknown_resolution`) are NOT included — only `partner_card_error` triggers the auto-dismiss path.

### Counter reset (`purchase_dh_push_store.go`)

In `SetDHPushStatus`, broaden the counter-reset CASE from `('pending', 'matched')` to:

```sql
CASE WHEN $2 IN ('pending', 'matched', 'unmatched_created', 'override_corrected', 'already_listed')
     THEN 0 ELSE dh_push_attempts END
```

This ensures any natural recovery resets the counter cleanly.

### Restore path

Operator clicks "Restore" in the existing "Skipped on DH Listing" tab → existing `HandleUndismissMatch` flips `dh_push_status='pending'` → counter auto-resets via the broadened reset list → next cycle re-attempts push.

## Data flow examples

### Price refresh, happy path
```
scheduler → dhprice.Provider.GetPrice(card{Name, Set, Number, Grade=9})
  → idResolver.GetExternalID → cardID=46809
  → tombstoneRepo.IsTombstoned(46809) → false
  → dhClient.RecentSales(ctx, 46809, "PSA", 9)
     GET /enterprise/cards/46809/recent-sales?grading_company=PSA&grade=9 → 200 OK
  → returns Price
```

### Intelligence refresh, tombstone hit
```
card-trajectory-refresh → for each known DH card ID 82648
  → tombstoneRepo.IsTombstoned(82648) → true
  → SKIP, log DEBUG
  → no API call issued
```

### Intelligence refresh, 404 → tombstone
```
card-trajectory-refresh → dh.Client.MarketDataEnterprise(ctx, 82648, grade=10)
  → CardLookup(82648) → 404 ErrProvNotFound
  → tombstoneRepo.RecordFailure(82648, "...") → attempts=1, INFO log
  ... 2 cycles later attempts=3 → INFO "tombstoned"
  ... future cycle: IsTombstoned returns true, skip
```

### DH push, auto-dismiss
```
dh-push scheduler → psa_import cert 150729500
  → DH returns partner_card_error "blank identity..."
  → IncrementDHPushAttempts(p.ID) → attempts=1, DEBUG log, stay pending
  ... 4 cycles later → attempts=5
  → SetDHPushStatus(p.ID, "dismissed", "auto-dismissed: 5x partner_card_error: ...")
  → INFO log, dismissed event recorded
  → row appears in "Skipped on DH Listing" UI tab
```

### Restore (counter reset)
```
operator clicks Restore in Skipped tab
  → POST /api/dh/match/undismiss → SetDHPushStatus('pending')
  → dh_push_attempts auto-reset to 0
  → next cycle re-attempts push
```

## Failure modes & edge cases

- **Tombstoned card becomes alive on DH again:** Admin UI offers "Clear all tombstones" button. No per-ID UI yet.
- **`pricing.Card.Grade == 0` reaches RecentSales:** dh.Client returns `ProviderInvalidRequest` immediately. Surfaces as a real bug rather than a silent 400 storm.
- **5-strike dismiss races with operator restore:** Worst case is one extra dismiss event on the very next cycle if the failure persists. Acceptable; same end state.
- **Existing manually-dismissed rows (e.g. cert 144121972):** Untouched. Auto-dismiss only triggers on attempts >= 5.
- **Tombstone repo unavailable:** `IsTombstoned` returning an error is treated as "not tombstoned" (fail-open). We never block a real DH call on a tombstone lookup outage.

## Testing

### Unit tests (added/modified)
- `dh.Client.RecentSales` — grade=10 builds URL with `?grading_company=PSA&grade=10`; grade=0 returns ProviderInvalidRequest with no HTTP call.
- `dhprice.Provider.GetPrice` — Grade flows from `pricing.Card` to mocked `dhClient.RecentSales`; tombstone-hit short-circuits before any API call.
- `postgres.DHCardTombstoneRepo` (in-memory mock variant in `internal/testutil/mocks/`) — `RecordFailure` increments, threshold logic correct, `Clear`/`ClearAll` reset.
- `scheduler/dh_psa_import.go` — partner_card_error increments counter; at 5 transitions to dismissed; under 5 stays pending.
- `purchase_dh_push_store.go` — `SetDHPushStatus("matched")` resets counter; same for `"unmatched_created"`, `"override_corrected"`, `"already_listed"`; non-success status does NOT reset.
- Admin handlers — count + clear endpoints return correct payloads and require admin auth.

### Manual verification before PR
- One curl against live DH `/api/v1/enterprise/cards/{id}/recent-sales?grading_company=PSA&grade=10` with a known-good card ID, confirming the 200 response shape matches our existing `dh.RecentSale` decoding.

### Quality gates
- `make check` (lint + architecture import check + file size)
- `go test -race ./...`
- `cd web && npm run build` (catches type drift on the new admin types)
- Manual curl verification of fix #1

## Out of scope (explicit YAGNI)

- Per-ID tombstone management UI
- Tombstone TTL / auto-expiry
- Backfill of existing dead IDs into the tombstone table
- Telemetry/metrics on dismissal counts
- DH override field (`PSAImportOverrides.CardNumber`) — explicitly rejected
- UI changes for the Skipped tab beyond what already exists

## Open questions

None — all clarifying decisions resolved in brainstorming.
