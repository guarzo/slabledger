# PR: Inventory & Dashboard foundations

A single PR that lands four things in one ship:

1. **`StatusPill`** — net-new shared UI (currently only a local fn in `CardIntakeTab.tsx`)
2. **`RecommendationBadge`** — net-new shared UI, 6-tier gradient set
3. **`HeroStatsBar`** refactor — money/time groups, ROI gradient, alerts slot
4. **`InventoryRow`** — net-new component for the Inventory page refresh

**Depends on:** PR #1 (grade ramp tokens + `GradeBadge` update).

**Files touched:** 8 new + 2 edits + 1 token addition.

- `web/src/react/ui/StatusPill.tsx` + `.module.css` · **NEW**
- `web/src/react/ui/RecommendationBadge.tsx` + `.module.css` · **NEW**
- `web/src/react/components/inventory/InventoryRow.tsx` + `.module.css` · **NEW**
- `web/src/react/components/portfolio/HeroStatsBar.tsx` · edit
- `web/src/react/components/portfolio/HeroStatsBar.module.css` · **NEW**
- `web/src/css/tokens.css` · add hero tokens
- `web/src/react/components/intake/CardIntakeTab.tsx` · swap local `StatusPill` for shared import (optional cleanup in same PR)

---

## 1. `tokens.css` additions

```css
/* hero */
--hero-divider: 1px solid rgba(255,255,255,0.08);
--hero-roi-size: clamp(32px, 4vw, 44px);

/* recommendation tiers (one row per tier: from / to / border / glow) */
--rec-must-buy-from:       #047857; --rec-must-buy-to:       #059669; --rec-must-buy-bd:       #065f46; --rec-must-buy-glow:       0 4px 16px rgba(5,150,105,0.5);
--rec-strong-buy-from:     #059669; --rec-strong-buy-to:     #10b981; --rec-strong-buy-bd:     #047857;
--rec-buy-from:            #10b981; --rec-buy-to:            #34d399; --rec-buy-bd:            #059669;
--rec-buy-caution-from:    #f59e0b; --rec-buy-caution-to:    #fbbf24; --rec-buy-caution-bd:    #d97706;
--rec-watch-from:          #6b7280; --rec-watch-to:          #9ca3af; --rec-watch-bd:          #4b5563;
--rec-avoid-from:          #dc2626; --rec-avoid-to:          #ef4444; --rec-avoid-bd:          #b91c1c;
```

---

## 2. `StatusPill` — shared

`ui/StatusPill.tsx`

```tsx
import { clsx } from 'clsx';
import type { ReactNode } from 'react';
import styles from './StatusPill.module.css';

export type StatusTone = 'success' | 'warning' | 'danger' | 'info' | 'brand' | 'neutral';

export function StatusPill({ tone = 'info', children, className }: {
  tone?: StatusTone; children: ReactNode; className?: string;
}) {
  return <span className={clsx(styles.pill, styles[`t-${tone}`], className)}>{children}</span>;
}
```

`ui/StatusPill.module.css`

```css
.pill {
  display: inline-flex; align-items: center; gap: 4px;
  font-size: 11px; font-weight: 600;
  padding: 3px 9px; border-radius: 999px;
  border: 1px solid;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.03em;
}
.t-success { background: rgba(16,185,129,0.10); color: #34d399; border-color: rgba(16,185,129,0.30); }
.t-warning { background: rgba(245,158,11,0.10); color: #fbbf24; border-color: rgba(245,158,11,0.30); }
.t-danger  { background: rgba(239,68,68,0.10);  color: #f87171; border-color: rgba(239,68,68,0.30); }
.t-info    { background: rgba(34,211,238,0.10); color: #22d3ee; border-color: rgba(34,211,238,0.30); }
.t-brand   { background: rgba(99,102,241,0.10); color: #a5b4fc; border-color: rgba(99,102,241,0.30); }
.t-neutral { background: rgba(107,114,128,0.10); color: #9ca3af; border-color: rgba(107,114,128,0.30); }
```

**Cleanup (same PR):** delete the local `StatusPill` function in `CardIntakeTab.tsx`, import from `@/react/ui/StatusPill`. Same API, zero call-site changes in that file.

---

## 3. `RecommendationBadge` — shared

`ui/RecommendationBadge.tsx`

```tsx
import { clsx } from 'clsx';
import styles from './RecommendationBadge.module.css';

export type RecTier =
  | 'MUST BUY' | 'STRONG BUY' | 'BUY'
  | 'BUY WITH CAUTION' | 'WATCH' | 'AVOID';

const slug: Record<RecTier, string> = {
  'MUST BUY': 'must-buy', 'STRONG BUY': 'strong-buy', 'BUY': 'buy',
  'BUY WITH CAUTION': 'buy-caution', 'WATCH': 'watch', 'AVOID': 'avoid',
};

export function RecommendationBadge({ tier, className }: { tier: RecTier; className?: string }) {
  return <span className={clsx(styles.rec, styles[`t-${slug[tier]}`], className)}>{tier}</span>;
}
```

`ui/RecommendationBadge.module.css`

```css
.rec {
  display: inline-flex; align-items: center;
  font-size: 10px; font-weight: 800;
  padding: 4px 9px; border-radius: 6px;
  color: #fff; letter-spacing: 0.06em; text-transform: uppercase;
  border: 1px solid;
}
.t-must-buy    { background: linear-gradient(135deg, var(--rec-must-buy-from),    var(--rec-must-buy-to));    border-color: var(--rec-must-buy-bd);    box-shadow: var(--rec-must-buy-glow); }
.t-strong-buy  { background: linear-gradient(135deg, var(--rec-strong-buy-from),  var(--rec-strong-buy-to));  border-color: var(--rec-strong-buy-bd); }
.t-buy         { background: linear-gradient(135deg, var(--rec-buy-from),         var(--rec-buy-to));         border-color: var(--rec-buy-bd); }
.t-buy-caution { background: linear-gradient(135deg, var(--rec-buy-caution-from), var(--rec-buy-caution-to)); border-color: var(--rec-buy-caution-bd); color: #422006; }
.t-watch       { background: linear-gradient(135deg, var(--rec-watch-from),       var(--rec-watch-to));       border-color: var(--rec-watch-bd); }
.t-avoid       { background: linear-gradient(135deg, var(--rec-avoid-from),       var(--rec-avoid-to));       border-color: var(--rec-avoid-bd); }
```

---

## 4. `HeroStatsBar` refactor

`components/portfolio/HeroStatsBar.tsx`

```tsx
import { clsx } from 'clsx';
import { StatusPill, type StatusTone } from '@/react/ui/StatusPill';
import styles from './HeroStatsBar.module.css';

interface Delta { value: number; label?: string; }
interface Stat  { label: string; value: string; tone?: 'neutral' | 'warn' | 'neg'; delta?: Delta; }
interface Alert { label: string; href: string; tone?: StatusTone; }

interface HeroStatsBarProps {
  roi: { value: string; delta?: Delta; negative?: boolean };
  money: Stat[];   // Deployed, Recovered, At Risk
  time:  Stat[];   // Wks to Cover, 30d Recovery
  alerts?: Alert[];
}

export function HeroStatsBar({ roi, money, time, alerts }: HeroStatsBarProps) {
  return (
    <section className={styles.hero} data-tone={roi.negative ? 'neg' : 'pos'} aria-label="Portfolio summary">
      <div className={styles.roiBlock}>
        <div className={styles.roiLabel}>Realized ROI</div>
        <div className={styles.roiRow}>
          <span className={styles.roiValue}>{roi.value}</span>
          {roi.delta && <DeltaChip delta={roi.delta} />}
        </div>
      </div>

      <Group stats={money} />
      <div className={styles.divider} aria-hidden />
      <Group stats={time} />

      {alerts && alerts.length > 0 && (
        <div className={styles.alerts}>
          {alerts.map((a) => (
            <a key={a.href} href={a.href} className={styles.alertLink}>
              <StatusPill tone={a.tone ?? 'warning'}>{a.label} →</StatusPill>
            </a>
          ))}
        </div>
      )}
    </section>
  );
}

function Group({ stats }: { stats: Stat[] }) {
  return (
    <div className={styles.group}>
      {stats.map((s) => (
        <div key={s.label} className={styles.stat}>
          <div className={styles.statLabel}>{s.label}</div>
          <div className={clsx(styles.statValue, s.tone && styles[`t-${s.tone}`])}>
            {s.value}
            {s.delta && <DeltaChip delta={s.delta} small />}
          </div>
        </div>
      ))}
    </div>
  );
}

function DeltaChip({ delta, small }: { delta: Delta; small?: boolean }) {
  const pos = delta.value >= 0;
  return (
    <span className={clsx(styles.delta, pos ? styles.dPos : styles.dNeg, small && styles.dSmall)}>
      {pos ? '▲' : '▼'} {Math.abs(delta.value).toFixed(1)}
      {delta.label && <span className={styles.dMeta}> {delta.label}</span>}
    </span>
  );
}
```

`HeroStatsBar.module.css`

```css
.hero { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 32px; padding-bottom: 24px; border-bottom: var(--hero-divider); margin-bottom: 24px; }
.roiBlock { display: flex; flex-direction: column; gap: 6px; }
.roiLabel { font-size: 11px; font-weight: 600; color: var(--brand-400); text-transform: uppercase; letter-spacing: 0.12em; }
.roiRow { display: flex; align-items: baseline; gap: 12px; }
.roiValue {
  font-size: var(--hero-roi-size); font-weight: 800;
  letter-spacing: -0.03em; line-height: 1; font-variant-numeric: tabular-nums;
  background: linear-gradient(180deg, var(--success) 0%, color-mix(in srgb, var(--success) 70%, #fff) 100%);
  -webkit-background-clip: text; background-clip: text; color: transparent;
}
.hero[data-tone="neg"] .roiValue {
  background: linear-gradient(180deg, var(--danger) 0%, color-mix(in srgb, var(--danger) 70%, #fff) 100%);
  -webkit-background-clip: text; background-clip: text;
}
.group { display: flex; flex-wrap: wrap; gap: 24px; padding-bottom: 4px; }
.stat { display: flex; flex-direction: column; gap: 4px; min-width: 80px; }
.statLabel { font-size: 10px; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.08em; font-weight: 500; }
.statValue { font-size: 18px; font-weight: 600; color: var(--text); font-variant-numeric: tabular-nums; display: flex; align-items: baseline; gap: 6px; }
.t-warn { color: var(--warning); }
.t-neg  { color: var(--danger); }
.divider { width: 1px; align-self: stretch; background: rgba(255,255,255,0.08); margin: 4px 0; }
.delta { display: inline-flex; align-items: center; gap: 3px; font-size: 11px; font-weight: 700; padding: 2px 7px; border-radius: 999px; font-variant-numeric: tabular-nums; }
.dSmall { font-size: 10px; padding: 1px 5px; }
.dPos { background: rgba(16,185,129,0.14); color: var(--success); border: 1px solid rgba(16,185,129,0.25); }
.dNeg { background: rgba(239,68,68,0.14); color: var(--danger); border: 1px solid rgba(239,68,68,0.25); }
.dMeta { font-weight: 500; opacity: 0.8; }
.alerts { display: flex; flex-direction: column; gap: 4px; align-self: center; }
.alertLink { text-decoration: none; }
.alertLink:hover { filter: brightness(1.15); }
```

### Call-site diff (Dashboard page)

```tsx
// Before
<HeroStatsBar roi="+18.4%" stats={[{ label: 'Deployed', value: '$62,480' }, ...]} />

// After
<HeroStatsBar
  roi={{ value: '+18.4%', delta: { value: 2.4, label: 'vs last wk' } }}
  money={[
    { label: 'Deployed',  value: '$62,480' },
    { label: 'Recovered', value: '$73,990', delta: { value: 8.1 } },
    { label: 'At Risk',   value: '$4,220', tone: 'warn' },
  ]}
  time={[
    { label: 'Wks to Cover', value: '3.1' },
    { label: '30d Recovery', value: '$18,240' },
  ]}
  alerts={[{ label: '3 unpaid invoices', href: '/invoices?filter=unpaid' }]}
/>
```

---

## 5. `InventoryRow` — net-new

`components/inventory/InventoryRow.tsx`

```tsx
import { clsx } from 'clsx';
import { GradeBadge } from '@/react/ui/GradeBadge';
import { RecommendationBadge, type RecTier } from '@/react/ui/RecommendationBadge';
import styles from './InventoryRow.module.css';

type Direction = 'rising' | 'falling' | 'stable';

interface InventoryRowProps {
  name: string; set: string;
  grader: 'PSA' | 'BGS' | 'CGC'; grade: number; blackLabel?: boolean;
  deployedCents: number; marketCents: number;
  direction: Direction; marketDeltaPct: number;
  daysHeld: number;
  rec: RecTier;
  onClick?: () => void;
}

const fmt = (cents: number) => `$${(cents / 100).toLocaleString('en-US', { minimumFractionDigits: 2 })}`;
const ageTone = (d: number) => d > 90 ? 'danger' : d > 60 ? 'warning' : d < 30 ? 'fresh' : 'neutral';

export function InventoryRow(p: InventoryRowProps) {
  const Tag = p.onClick ? 'button' : 'div';
  return (
    <Tag
      className={clsx(styles.row, p.onClick && styles.interactive)}
      onClick={p.onClick} type={p.onClick ? 'button' : undefined}
    >
      <div className={styles.gradeCol}>
        <GradeBadge grader={p.grader} grade={p.grade} blackLabel={p.blackLabel} size="md" />
      </div>
      <div className={styles.nameCol}>
        <div className={styles.name}>{p.name}</div>
        <div className={styles.set}>{p.set}</div>
      </div>
      <div className={styles.numCol}>
        <div className={styles.numLabel}>Deployed</div>
        <div className={styles.numValue}>{fmt(p.deployedCents)}</div>
      </div>
      <div className={clsx(styles.numCol, styles[`dir-${p.direction}`])}>
        <div className={styles.numLabel}>Market</div>
        <div className={styles.marketRow}>
          <span className={styles.numValue}>{fmt(p.marketCents)}</span>
          <span className={styles.dirChip}>
            {p.direction === 'rising' ? '↗' : p.direction === 'falling' ? '↘' : '→'}
            {' '}{p.marketDeltaPct >= 0 ? '+' : ''}{p.marketDeltaPct.toFixed(1)}%
          </span>
        </div>
      </div>
      <div className={clsx(styles.ageCol, styles[`age-${ageTone(p.daysHeld)}`])}>
        <div className={styles.numLabel}>Age</div>
        <div className={styles.numValue}>
          <span className={styles.ageBar}><span style={{ width: `${Math.min(100, (p.daysHeld / 120) * 100)}%` }} /></span>
          {p.daysHeld}d
        </div>
      </div>
      <div className={styles.recCol}><RecommendationBadge tier={p.rec} /></div>
    </Tag>
  );
}
```

`InventoryRow.module.css`

```css
.row {
  display: grid; grid-template-columns: 80px 1.8fr 120px 180px 120px 150px;
  align-items: center; gap: 16px;
  padding: 14px 18px;
  background: var(--surface-1); border: 1px solid var(--surface-0);
  border-radius: 14px; box-shadow: var(--shadow-1);
  text-align: left; width: 100%;
  font-family: inherit; color: inherit;
}
.interactive { cursor: pointer; transition: all 180ms cubic-bezier(.4,0,.2,1); }
.interactive:hover { transform: translateY(-1px); border-color: var(--brand-500); box-shadow: var(--shadow-2); }
.gradeCol { display: flex; }
.nameCol { min-width: 0; }
.name { font-size: 14px; font-weight: 600; color: var(--color-heading); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.set { font-size: 11px; color: var(--text-muted); margin-top: 2px; }
.numCol, .ageCol { display: flex; flex-direction: column; gap: 2px; }
.numLabel { font-size: 9px; color: var(--text-subtle); text-transform: uppercase; letter-spacing: 0.08em; font-weight: 600; }
.numValue { font-size: 14px; font-weight: 600; color: var(--text); font-variant-numeric: tabular-nums; display: flex; align-items: center; gap: 6px; }
.marketRow { display: flex; align-items: baseline; gap: 8px; }
.dirChip { font-size: 11px; font-weight: 700; font-variant-numeric: tabular-nums; padding: 1px 6px; border-radius: 999px; display: inline-flex; align-items: center; gap: 3px; }
.dir-rising  .dirChip { background: rgba(16,185,129,0.14); color: var(--success); }
.dir-falling .dirChip { background: rgba(239,68,68,0.14);  color: var(--danger); }
.dir-stable  .dirChip { background: rgba(107,114,128,0.14); color: var(--text-muted); }
.ageBar { display: inline-block; width: 40px; height: 4px; background: rgba(255,255,255,0.06); border-radius: 2px; overflow: hidden; }
.ageBar > span { display: block; height: 100%; background: currentColor; }
.age-fresh   { color: var(--success); }
.age-neutral { color: var(--text-muted); }
.age-warning { color: var(--warning); }
.age-danger  { color: var(--danger); }
.age-warning .numValue, .age-danger .numValue { color: currentColor; }
.recCol { display: flex; justify-content: flex-end; }
```

---

## Test checklist

- [ ] `StatusPill` tones all render correctly; `CardIntakeTab` still renders after local copy is replaced
- [ ] `RecommendationBadge` all 6 tiers render; MUST BUY has the extra glow shadow
- [ ] `HeroStatsBar` — positive ROI gets green gradient-text; set `negative: true` flips to red
- [ ] `HeroStatsBar` — money/time groups wrap independently on narrow viewports; alerts column sticks rightmost
- [ ] `InventoryRow` — grade column (PR#1's ramp) leads; market delta signed correctly; age bar fills proportionally and caps at 100%
- [ ] `InventoryRow` with `onClick` renders as `<button>` and has a 3px focus ring
- [ ] Long card names truncate with ellipsis
- [ ] Row shadow & hover lift match the rest of the card system

---

## Ship order

1. PR #1 · grade ramp
2. PR #2 · button personalities
3. **PR #3 · this one** — requires #1 for the grade chip

Independent of #2, so can ship in parallel.

---

This is the last migration PR. The rest of the preview cards (semantic pills, card shells, type scale) are either already covered here (`StatusPill` = semantic pills) or match your existing tokens (type scale). Say the word if you want a `CardShell` variant pass specifically for AI/premium surfaces.
