# Capital Exposure Card Redesign

Replace the budget/limit-based capital exposure card with a velocity-based design that shows capital exposure as a function of actual revenue recovery.

## Problem

The current card is built around a user-configured "capital budget" — an arbitrary ceiling. Exposure % is `outstanding / budget`, and alert levels fire at 80%/90% of that number. If no budget is set, exposure is 0%. This doesn't reflect actual business health.

## Design

### Data Model

Replace `CapitalSummary` with velocity-based fields:

| Field | Type | Description |
|---|---|---|
| `OutstandingCents` | int | Invoiced purchases minus paid amounts (unchanged) |
| `RecoveryRate30dCents` | int | Sum of `sale_price_cents` from last 30 days |
| `RecoveryRate30dPriorCents` | int | Sum of `sale_price_cents` from days 31-60 |
| `WeeksToCover` | float64 | `outstanding / (recoveryRate30d / 4.3)` |
| `RecoveryTrend` | string | `"improving"`, `"declining"`, `"stable"` (>10% delta = directional) |
| `AlertLevel` | string | Based on `weeksToCover`: <6 ok, 6-12 warning, >12 critical |
| `UnpaidInvoiceCount` | int | Kept |
| `RefundedCents` | int | Kept |
| `PaidCents` | int | Kept |

**Removed fields:** `CapitalBudgetCents`, `ExposurePct`, `ProjectedExposureCents`, `DaysToNextInvoice`.

`CashflowConfig` table and endpoints remain (cash buffer field still used). `credit_limit_cents` column stays but is no longer consumed by the card.

### Backend Query

New query added to `GetCapitalSummary` in `finance_repository.go`:

```sql
SELECT
  COALESCE(SUM(CASE WHEN sale_date >= date('now', '-30 days') THEN sale_price_cents ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN sale_date >= date('now', '-60 days') AND sale_date < date('now', '-30 days') THEN sale_price_cents ELSE 0 END), 0)
FROM campaign_sales
```

Derivation in Go:
- `weeklyRate = recoveryRate30d / 4.3`
- `weeksToCover = outstanding / weeklyRate` (capped at 99.0 when weekly rate is 0)
- `trend`: 30d vs prior 30d — >10% higher = improving, >10% lower = declining, else stable

Existing outstanding and invoice queries unchanged. `avgDailySpend` calculation and projected exposure logic removed.

### Alert Level Thresholds

| WeeksToCover | Level | Condition |
|---|---|---|
| < 6 | `ok` | Green |
| 6 - 12 | `warning` | Yellow |
| > 12 | `critical` | Red |
| N/A (no recovery) | Based on outstanding: >$5K = warning, >$10K = critical | Fallback when recovery rate is 0 |

### Frontend: CapitalExposurePanel

Rebuilt layout (top to bottom):

1. **Headline: Outstanding amount** — large, prominent number (e.g., "$12,450"). Always visible.

2. **Weeks-to-cover badge** — colored pill:
   - Green: "~4 wks" (under 6)
   - Yellow: "~8 wks" (6-12)
   - Red: "~16 wks" (over 12, capped display at "20+ wks")
   - Gray: "No sales data" (recovery rate is 0)

3. **Recovery rate + trend** — smaller text: "$3,200/mo recovered" with trend arrow (up/down/flat).

4. **Supporting details** — unpaid invoice count, refunded amount if nonzero.

**Removed:** Progress bar, "X% of Y budget" text, projected exposure line.

### Frontend: HeroStatsBar

- "Capital Exposure" percentage stat replaced with "Wks to Cover" showing the colored badge value
- "Outstanding" stat unchanged

### Edge Cases

- **No sales ever:** Card shows outstanding + "No recovery data yet" in gray, no weeks-to-cover badge
- **Zero outstanding:** "$0 outstanding" in green, recovery stats still visible for context
- **Very small recovery rate:** Weeks-to-cover caps display at "20+ wks" in red
- **Zero outstanding + zero recovery:** Card shows "$0 outstanding" in green, "No recovery data yet"

### TypeScript Types

Update `CapitalSummary` interface in `web/src/types/campaigns/market.ts`:

```typescript
export interface CapitalSummary {
  outstandingCents: number;
  recoveryRate30dCents: number;
  recoveryRate30dPriorCents: number;
  weeksToCover: number;         // 99 = no recovery data
  recoveryTrend: 'improving' | 'declining' | 'stable';
  alertLevel: 'ok' | 'warning' | 'critical';
  unpaidInvoiceCount: number;
  refundedCents: number;
  paidCents: number;
}
```

**Removed fields:** `capitalBudgetCents`, `exposurePct`, `projectedExposureCents`, `daysToNextInvoice`.

### Advisor Tool Updates

Three advisor touchpoints consume `CapitalSummary` and need to reflect the new fields:

#### `get_capital_summary` tool (`internal/adapters/advisortool/tools_portfolio.go`)

Update tool description from budget-centric to velocity-centric:
> "Get capital exposure: outstanding balance, 30-day recovery rate, weeks to cover, recovery trend, and alert level."

The tool already calls `svc.GetCapitalSummary()` and serializes the result — the struct change propagates automatically.

#### `get_dashboard_summary` tool (`internal/adapters/advisortool/tools_portfolio.go`)

The `dashboardSummary.Capital` inline struct maps old fields. Update to:

```go
Capital struct {
    BalanceCents         int     `json:"balanceCents"`
    RecoveryRate30dCents int     `json:"recoveryRate30dCents"`
    WeeksToCover         float64 `json:"weeksToCover"`
    RecoveryTrend        string  `json:"recoveryTrend"`
    AlertLevel           string  `json:"alertLevel"`
} `json:"capital"`
```

Update the mapping code (lines ~148-155) to populate the new fields from the updated `CapitalSummary`.

Update tool description:
> "Get a compact portfolio overview: weekly performance, capital velocity, campaign statuses, and channel velocity."

#### LLM Prompts (`internal/domain/advisor/prompts.go`)

5 edits to align prompt language with the new data model:

1. **`baseSystemPrompt` line 22**: "Outstanding balance and projected exposure matter for capital allocation" → "Outstanding balance, recovery rate, and weeks-to-cover drive capital allocation — how much cash is tied up and how fast it cycles back"

2. **`digestUserPrompt` line 66**: "Cash flow (outstanding balance, projected exposure, payment status)" → "Cash flow (outstanding balance, recovery rate, weeks to cover, payment status)"

3. **`digestUserPrompt` line 74**: "Cash Flow — outstanding balance, projected exposure, unpaid invoices, days to next invoice" → "Cash Flow — outstanding balance, 30d recovery rate, weeks to cover, recovery trend, unpaid invoices"

4. **`liquidationSystemPrompt` lines 139-141**: "if outstanding balance is high relative to projected revenue, lower the bar for all liquidation actions" → "if weeks-to-cover exceeds 12 (critical), lower the bar for all liquidation actions. The higher the weeks-to-cover, the more aggressively capital should be freed."

5. **`liquidationUserPrompt` line 174**: "Capital Position — outstanding balance, projected exposure, capital tied up in stale inventory" → "Capital Position — outstanding balance, recovery rate, weeks to cover, recovery trend, capital tied up in stale inventory"

### Files Changed

**Backend:**
- `internal/domain/campaigns/types.go` — update `CapitalSummary` struct
- `internal/adapters/storage/sqlite/finance_repository.go` — rewrite `GetCapitalSummary` query logic
- `internal/adapters/advisortool/tools_portfolio.go` — update `get_capital_summary` description, `dashboardSummary.Capital` struct and mapping
- `internal/domain/advisor/prompts.go` — 5 prompt text edits
- Tests for the above

**Frontend:**
- `web/src/types/campaigns/market.ts` — update `CapitalSummary` interface
- `web/src/react/components/portfolio/CapitalExposurePanel.tsx` — rebuild card
- `web/src/react/components/portfolio/HeroStatsBar.tsx` — update capital exposure stat
- Tests for the above

### Not Changed

- `CashflowConfig` table/endpoints (cash buffer still used elsewhere)
- `CapitalTimelineChart` (already uses recovery data, unaffected)
- `/api/credit/config` endpoints (remain for cash buffer management)
- `credit_limit_cents` column (stays in DB, just unused by card)
