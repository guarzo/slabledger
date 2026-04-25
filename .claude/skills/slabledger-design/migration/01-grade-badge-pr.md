# PR: Grade-aware color ramp

Swap PSA/BGS/CGC-keyed grade colors for a quality-indicative 1→10 ramp. The grader (PSA / BGS / CGC) becomes subtle metadata, not the primary signal.

**Why:** color should answer "is this card high-grade?" at a glance. Under the old scheme, a PSA 8 (dark amber) and a PSA 10 (gold) looked more alike than a PSA 9 (blue) and a PSA 10 (gold) — hue wasn't carrying quality. New scheme: red → orange → yellow → green → emerald → gold.

**Files touched:** 2

---

## 1. `web/src/css/tokens.css`

Add a new `/* ============ GRADE QUALITY RAMP ============ */` section. Keep the existing `--grade-psa-10` etc. aliases around for one release so nothing breaks; we'll delete them in a follow-up.

```css
/* ============ GRADE QUALITY RAMP ============ */
/* 1 = poor, 10 = gem mint. Each step has fill / border / text. */
--grade-1-bg:  rgba(127, 29, 29, 0.22);   --grade-1-bd:  rgba(127, 29, 29, 0.45);   --grade-1-fg:  #fca5a5;
--grade-2-bg:  rgba(153, 27, 27, 0.22);   --grade-2-bd:  rgba(153, 27, 27, 0.45);   --grade-2-fg:  #fca5a5;
--grade-3-bg:  rgba(194, 65, 12, 0.22);   --grade-3-bd:  rgba(194, 65, 12, 0.45);   --grade-3-fg:  #fdba74;
--grade-4-bg:  rgba(180, 83,  9, 0.22);   --grade-4-bd:  rgba(180, 83,  9, 0.45);   --grade-4-fg:  #fdba74;
--grade-5-bg:  rgba(161, 98,  7, 0.22);   --grade-5-bd:  rgba(161, 98,  7, 0.45);   --grade-5-fg:  #fcd34d;
--grade-6-bg:  rgba(133, 77, 14, 0.22);   --grade-6-bd:  rgba(133, 77, 14, 0.45);   --grade-6-fg:  #fde68a;
--grade-7-bg:  rgba(101,163, 13, 0.20);   --grade-7-bd:  rgba(101,163, 13, 0.40);   --grade-7-fg:  #bef264;
--grade-8-bg:  rgba( 22,163, 74, 0.20);   --grade-8-bd:  rgba( 22,163, 74, 0.45);   --grade-8-fg:  #86efac;
--grade-9-bg:  rgba(  5,150,105, 0.22);   --grade-9-bd:  rgba(  5,150,105, 0.50);   --grade-9-fg:  #6ee7b7;
--grade-10-bg: linear-gradient(135deg, rgba(251,191,36,0.28), rgba(245,158,11,0.22));
--grade-10-bd: rgba(251,191,36,0.55);
--grade-10-fg: #fde68a;
--grade-10-glow: 0 0 12px rgba(251,191,36,0.25);

/* Half-grades (CGC 9.5, BGS 8.5) snap UP to the next integer's tokens. */
/* BGS 10 Black Label: keep the old black treatment as an opt-in variant. */
--grade-black-label-bg: rgba(0,0,0,0.5);
--grade-black-label-bd: rgba(255,255,255,0.2);
--grade-black-label-fg: #e5e7eb;
```

### Deprecate (keep for one release, then remove)

```css
/* DEPRECATED — use --grade-{1..10}-* instead. Remove in next release. */
--grade-psa-10: #fbbf24;
--grade-psa-9:  #2563eb;
/* …etc */
```

---

## 2. `web/src/react/ui/GradeBadge.tsx`

Replace the grader+grade lookup with a grade-number lookup. Grader label is rendered as a small metadata chip inside the badge.

```tsx
import { clsx } from 'clsx';
import styles from './GradeBadge.module.css';

type Grader = 'PSA' | 'BGS' | 'CGC';

interface GradeBadgeProps {
  grader: Grader;
  /** 1–10, or 9.5 / 8.5 for half-grades. */
  grade: number;
  /** BGS "Black Label" 10 — renders in slate instead of gold. */
  blackLabel?: boolean;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

/** Snap half-grades up (9.5 → 10 bucket) for color; numeric label unchanged. */
const bucket = (g: number) => Math.min(10, Math.max(1, Math.ceil(g)));

export function GradeBadge({ grader, grade, blackLabel, size = 'md', className }: GradeBadgeProps) {
  const tier = blackLabel ? 'black-label' : bucket(grade);
  return (
    <span
      className={clsx(styles.badge, styles[`tier-${tier}`], styles[`size-${size}`], className)}
      aria-label={`${grader} grade ${grade}${blackLabel ? ' Black Label' : ''}`}
    >
      <span className={styles.grader}>{grader}</span>
      <span className={styles.grade}>{grade}</span>
    </span>
  );
}
```

### `GradeBadge.module.css` (new file — or inline in your existing CSS-in-JS solution)

```css
.badge {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 3px 9px 3px 5px;
  border: 1px solid;
  border-radius: 999px;
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  line-height: 1;
}
.size-sm { font-size: 11px; padding: 2px 7px 2px 4px; gap: 4px; }
.size-md { font-size: 12px; }
.size-lg { font-size: 14px; padding: 5px 11px 5px 6px; }

.grader {
  font-size: 0.72em; font-weight: 700; letter-spacing: 0.08em;
  padding: 2px 5px; border-radius: 4px;
  background: rgba(255,255,255,0.06);
  color: var(--text-muted);
  text-transform: uppercase;
}
.grade { font-size: 1em; }

/* Tiers */
.tier-1  { background: var(--grade-1-bg);  border-color: var(--grade-1-bd);  color: var(--grade-1-fg); }
.tier-2  { background: var(--grade-2-bg);  border-color: var(--grade-2-bd);  color: var(--grade-2-fg); }
.tier-3  { background: var(--grade-3-bg);  border-color: var(--grade-3-bd);  color: var(--grade-3-fg); }
.tier-4  { background: var(--grade-4-bg);  border-color: var(--grade-4-bd);  color: var(--grade-4-fg); }
.tier-5  { background: var(--grade-5-bg);  border-color: var(--grade-5-bd);  color: var(--grade-5-fg); }
.tier-6  { background: var(--grade-6-bg);  border-color: var(--grade-6-bd);  color: var(--grade-6-fg); }
.tier-7  { background: var(--grade-7-bg);  border-color: var(--grade-7-bd);  color: var(--grade-7-fg); }
.tier-8  { background: var(--grade-8-bg);  border-color: var(--grade-8-bd);  color: var(--grade-8-fg); }
.tier-9  { background: var(--grade-9-bg);  border-color: var(--grade-9-bd);  color: var(--grade-9-fg); }
.tier-10 {
  background: var(--grade-10-bg);
  border-color: var(--grade-10-bd);
  color: var(--grade-10-fg);
  box-shadow: var(--grade-10-glow);
}
.tier-black-label {
  background: var(--grade-black-label-bg);
  border-color: var(--grade-black-label-bd);
  color: var(--grade-black-label-fg);
}
```

---

## Migration

Call sites change from:

```tsx
<GradeBadge grader="PSA" grade="10" />   // string
```

to:

```tsx
<GradeBadge grader="PSA" grade={10} />   // number
<GradeBadge grader="BGS" grade={10} blackLabel />
<GradeBadge grader="CGC" grade={9.5} /> // auto-buckets to tier-10
```

A grep for `<GradeBadge` should land 8–15 call sites based on the codebase I looked at. Most are a one-character fix (quote removal).

## Test checklist

- [ ] PSA 1 through PSA 10 render in red→gold order on a dashboard row
- [ ] CGC 9.5 looks identical to a 10 (bucketed up)
- [ ] BGS 10 Black Label prop produces the slate variant, not gold
- [ ] Small size in dense tables doesn't look cramped (you may want gap: 3px in `.size-sm`)
- [ ] Dark-mode contrast — all `fg` colors hit ≥ 4.5:1 on `--surface-1` (I checked; they do)

---

Want me to also write the diff for the Button personalities (indigo-glow primary, gold MUST BUY, AI gradient-border, segmented), or ship this one first and follow up?

