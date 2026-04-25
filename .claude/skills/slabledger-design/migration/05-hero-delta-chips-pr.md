# Prompt: Finish HeroStatsBar (delta chips + ROI magnitude tier)

**Status:** the HeroStatsBar refactor from PR #3 is **partially on main**. The structural bones are in — money/time groups, divider, ROI gradient-text hook via `data-tone`, StatusPill alerts slot. What's missing is the **delta chips** and a couple of polish passes. This is the short follow-up to close the loop.

## What's already shipped ✅

- `HeroStatsBar.tsx` takes `health: PortfolioHealth` and `capital: CapitalSummary` (typed from the domain, not generic `Stat[]`)
- ROI block + `data-tone="pos|neg"` wired up
- Money group: Deployed / Recovered / At Risk
- Time group: Wks to Cover / Outstanding / 30d Recovery (with `TrendArrow`)
- Alerts slot with `StatusPill` + unpaid-invoices link
- Empty state + no-data guard
- Typed `tone` prop on internal `Stat` (`warn | neg | muted | success`)

Nice — the typed domain props are actually **better than the spec**. Keep them.

## What's still missing 🟡

### 1. Delta chips next to values

The ROI value and the money stats should each support an optional delta indicator ("▲ 2.4 vs last wk"). The `PortfolioHealth` / `CapitalSummary` types need a delta field or we need a sibling `PortfolioDeltas` type — **check with the backend team which shape they prefer** before wiring.

Suggested shape (add to `types/campaigns.ts`):

```ts
export interface PortfolioDelta {
  /** Signed number. Percentage points for ROI, dollars for money fields. */
  value: number;
  /** Optional human label, e.g. "vs last wk", "30d". */
  label?: string;
  /** "pct" renders with %, "cents" renders through formatCents. Defaults to "pct". */
  unit?: 'pct' | 'cents';
}

export interface PortfolioHealth {
  // ...existing
  realizedROIDelta?: PortfolioDelta;
  totalRecoveredDelta?: PortfolioDelta;   // optional — only show if meaningful
}
```

Component-side `DeltaChip`:

```tsx
function DeltaChip({ delta, small }: { delta: PortfolioDelta; small?: boolean }) {
  const pos = delta.value >= 0;
  const formatted =
    delta.unit === 'cents' ? formatCents(Math.abs(delta.value))
    : `${Math.abs(delta.value).toFixed(1)}`;
  const suffix = delta.unit === 'cents' ? '' : '%';
  return (
    <span className={clsx(styles.delta, pos ? styles.dPos : styles.dNeg, small && styles.dSmall)}>
      {pos ? '▲' : '▼'} {formatted}{suffix}
      {delta.label && <span className={styles.dMeta}> {delta.label}</span>}
    </span>
  );
}
```

Render inline in the ROI row and inside `Stat` when present:

```tsx
<div className={styles.roiRow}>
  <span className={styles.roiValue}>{formatPct(roi)}</span>
  {health.realizedROIDelta && <DeltaChip delta={health.realizedROIDelta} />}
</div>
```

CSS (append to `HeroStatsBar.module.css`):

```css
.delta {
  display: inline-flex; align-items: center; gap: 3px;
  font-size: 11px; font-weight: 700;
  padding: 2px 7px; border-radius: 999px;
  font-variant-numeric: tabular-nums;
}
.dSmall { font-size: 10px; padding: 1px 5px; }
.dPos { background: rgba(16,185,129,0.14); color: var(--success); border: 1px solid rgba(16,185,129,0.25); }
.dNeg { background: rgba(239,68,68,0.14);  color: var(--danger);  border: 1px solid rgba(239,68,68,0.25); }
.dMeta { font-weight: 500; opacity: 0.8; }
```

### 2. ROI magnitude tier (optional but high-impact)

Right now a +3% ROI and a +47% ROI look identical — same gradient, same size. Add a data attribute that scales the ROI with its magnitude:

```tsx
const magnitude =
  Math.abs(roi) >= 0.5 ? 'huge'
  : Math.abs(roi) >= 0.2 ? 'big'
  : 'normal';

<section ... data-tone={...} data-mag={magnitude}>
```

```css
.hero[data-mag="big"]  .roiValue { font-size: calc(var(--hero-roi-size) * 1.15); }
.hero[data-mag="huge"] .roiValue { font-size: calc(var(--hero-roi-size) * 1.30); text-shadow: 0 0 24px currentColor; }
```

Small touch, makes the dashboard feel alive when ROI actually pops.

### 3. Micro-polish

- [ ] Verify `formatPct` prepends `+` on positive values. If not, branch: `{roi >= 0 ? '+' : ''}{formatPct(roi)}`.
- [ ] The empty state currently uses `icon="📊"` — you've historically avoided emoji. Consider swapping to a small inline SVG or just dropping the icon and leaning on the headline.
- [ ] Reduced-motion: the future gradient-text animation from PR #4 isn't here, but if you ever add a shimmer, gate it behind `@media (prefers-reduced-motion: no-preference)`.

## Test checklist

- [ ] ROI with delta renders chip inline, positive delta is green ▲, negative is red ▼
- [ ] `unit: 'cents'` deltas format through `formatCents` (e.g. "+$8,240 vs last wk")
- [ ] `data-mag="huge"` triggers only when `|roi| ≥ 0.5`
- [ ] Dashboard still renders with no deltas provided (backwards-compatible)
- [ ] Negative ROI still flips to red gradient via existing `data-tone="neg"`
- [ ] Empty state unaffected

## Scope

- `web/src/react/components/portfolio/HeroStatsBar.tsx` — add `DeltaChip`, render in ROI row + `Stat`, add `data-mag`
- `web/src/react/components/portfolio/HeroStatsBar.module.css` — append delta styles, magnitude rules
- `web/src/types/campaigns.ts` — add `PortfolioDelta` interface, extend `PortfolioHealth` (optional fields)
- **Backend:** wire up `realizedROIDelta` if you want it live; the UI no-ops when it's absent, so this is non-blocking.

Single PR. Keeps the contract backwards-compatible — no call-site changes required on the Dashboard unless you want to opt in to deltas immediately.
