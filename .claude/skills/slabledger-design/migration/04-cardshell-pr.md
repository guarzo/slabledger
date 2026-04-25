# PR: `CardShell` primitive — six variants

Ratifies the four card patterns scattered across the codebase as Tailwind one-offs into a single typed primitive, and adds two net-new variants pulled from the design-system pass: **`ai`** (gradient border for Insights / AI-generated surfaces) and **`data`** (denser inset surface for tabular content like the Inventory grid).

**Files touched:** 2 new + 1 token addition + N call-site refactors (do as a follow-up if the diff gets noisy).

- `web/src/react/ui/CardShell.tsx` + `.module.css` · **NEW**
- `web/src/css/tokens.css` · add card tokens
- Call sites (optional, same PR or follow-up): `LoginPage.tsx`, `DashboardPage.tsx`, anywhere using `bg-[var(--surface-1)]/80 backdrop-blur-xl` or hand-rolled card divs

**Independent of PRs #1, #2, #3.**

---

## 1. `tokens.css` additions

```css
/* card system */
--card-radius:       18px;
--card-radius-sm:    12px;
--card-radius-lg:    24px;
--card-padding:      20px;
--card-padding-sm:   14px;
--card-padding-lg:   28px;

--card-default-bg:   #111827;
--card-default-bd:   #0b1220;
--card-default-shadow: 0 6px 16px rgba(0,0,0,0.24);

--card-elevated-bg:  #1f2937;
--card-elevated-shadow: 0 10px 30px rgba(0,0,0,0.35);

--card-glass-bg:     rgba(22,27,34,0.70);
--card-glass-bd:     rgba(139,152,207,0.10);
--card-glass-blur:   blur(20px) saturate(180%);
--card-glass-shadow: 0 8px 32px rgba(0,0,0,0.37);

--card-premium-bg:   linear-gradient(135deg, #111827 0%, #1f2937 100%);
--card-premium-bd:   rgba(99,102,241,0.30);
--card-premium-glow: 0 0 24px rgba(99,102,241,0.35), 0 6px 16px rgba(0,0,0,0.30);

/* AI / Insights — gradient border via mask trick */
--card-ai-bg:        radial-gradient(120% 100% at 0% 0%, rgba(99,102,241,0.10) 0%, transparent 60%), #111827;
--card-ai-border-grad: linear-gradient(135deg, #6366f1 0%, #22d3ee 50%, #a855f7 100%);
--card-ai-shadow:    0 8px 24px rgba(0,0,0,0.30), inset 0 1px 0 rgba(255,255,255,0.04);

/* Data / Inventory — denser inset, no glow */
--card-data-bg:      #0f1623;
--card-data-bd:      rgba(255,255,255,0.04);
--card-data-shadow:  inset 0 1px 0 rgba(255,255,255,0.03);
```

---

## 2. `CardShell.tsx`

```tsx
import { clsx } from 'clsx';
import type { ElementType, ReactNode, HTMLAttributes } from 'react';
import styles from './CardShell.module.css';

export type CardVariant = 'default' | 'elevated' | 'glass' | 'premium' | 'ai' | 'data';
export type CardPadding = 'sm' | 'md' | 'lg' | 'none';
export type CardRadius  = 'sm' | 'md' | 'lg';

interface CardShellProps extends Omit<HTMLAttributes<HTMLElement>, 'className'> {
  variant?: CardVariant;
  padding?: CardPadding;
  radius?: CardRadius;
  as?: ElementType;
  interactive?: boolean;
  className?: string;
  children: ReactNode;
}

export function CardShell({
  variant = 'default',
  padding = 'md',
  radius = 'md',
  as: Tag = 'div',
  interactive = false,
  className,
  children,
  ...rest
}: CardShellProps) {
  return (
    <Tag
      className={clsx(
        styles.card,
        styles[`v-${variant}`],
        styles[`p-${padding}`],
        styles[`r-${radius}`],
        interactive && styles.interactive,
        className,
      )}
      {...rest}
    >
      {children}
    </Tag>
  );
}
```

---

## 3. `CardShell.module.css`

```css
.card {
  position: relative;
  color: var(--text);
  /* default radius/padding overridden by modifier classes */
}

/* radius */
.r-sm { border-radius: var(--card-radius-sm); }
.r-md { border-radius: var(--card-radius); }
.r-lg { border-radius: var(--card-radius-lg); }

/* padding */
.p-sm   { padding: var(--card-padding-sm); }
.p-md   { padding: var(--card-padding); }
.p-lg   { padding: var(--card-padding-lg); }
.p-none { padding: 0; }

/* default — workhorse surface for dashboard widgets */
.v-default {
  background: var(--card-default-bg);
  border: 1px solid var(--card-default-bd);
  box-shadow: var(--card-default-shadow);
}

/* elevated — modals, dropdowns, popovers */
.v-elevated {
  background: var(--card-elevated-bg);
  border: 1px solid var(--card-default-bd);
  box-shadow: var(--card-elevated-shadow);
}

/* glass — header, login, floating panels */
.v-glass {
  background: var(--card-glass-bg);
  backdrop-filter: var(--card-glass-blur);
  -webkit-backdrop-filter: var(--card-glass-blur);
  border: 1px solid var(--card-glass-bd);
  box-shadow: var(--card-glass-shadow);
}

/* premium — MUST BUY detail, featured recommendation */
.v-premium {
  background: var(--card-premium-bg);
  border: 1px solid var(--card-premium-bd);
  box-shadow: var(--card-premium-glow);
}

/* ai — Insights, AI-generated surfaces. gradient border via padding mask */
.v-ai {
  background: var(--card-ai-bg);
  border: 1px solid transparent;
  background-clip: padding-box;
  box-shadow: var(--card-ai-shadow);
}
.v-ai::before {
  content: '';
  position: absolute; inset: 0;
  border-radius: inherit;
  padding: 1px;
  background: var(--card-ai-border-grad);
  -webkit-mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
          mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
  -webkit-mask-composite: xor;
          mask-composite: exclude;
  pointer-events: none;
}

/* data — inventory rows, dense tabular surfaces */
.v-data {
  background: var(--card-data-bg);
  border: 1px solid var(--card-data-bd);
  box-shadow: var(--card-data-shadow);
}

/* interactive — adds hover lift + focus ring to any variant */
.interactive {
  cursor: pointer;
  transition: transform 180ms cubic-bezier(.4,0,.2,1),
              box-shadow 180ms cubic-bezier(.4,0,.2,1),
              border-color 180ms cubic-bezier(.4,0,.2,1);
}
.interactive:hover {
  transform: translateY(-2px);
  border-color: var(--brand-500);
}
.v-default.interactive:hover  { box-shadow: 0 12px 28px rgba(0,0,0,0.32); }
.v-elevated.interactive:hover { box-shadow: 0 16px 40px rgba(0,0,0,0.40); }
.v-glass.interactive:hover    { box-shadow: 0 12px 40px rgba(0,0,0,0.45); }
.v-premium.interactive:hover  { box-shadow: 0 0 32px rgba(99,102,241,0.50), 0 10px 24px rgba(0,0,0,0.35); }
.v-ai.interactive:hover       { box-shadow: 0 12px 32px rgba(0,0,0,0.35), inset 0 1px 0 rgba(255,255,255,0.06); }
.v-data.interactive:hover     { background: color-mix(in srgb, var(--card-data-bg) 90%, var(--brand-500) 10%); }

.interactive:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--brand-500);
}
```

---

## 4. Variant cheat-sheet

| Variant | Use for | Don't use for |
|---|---|---|
| `default` | Standard dashboard widgets, stat cards, list panels | Modals, AI surfaces |
| `elevated` | Modals, dropdowns, popovers, anything floating above the page surface | Inline content |
| `glass` | Header, login card, anything floating over a hero/full-bleed background | Anything stacked on another card (no glass-on-glass) |
| `premium` | MUST BUY detail card, featured recommendation, premium tier upsell | Routine widgets — overuse kills the signal |
| `ai` *(new)* | Insights surfaces, AI-generated suggestions, "recommended for you" blocks | Action-required surfaces (use `premium` for those) |
| `data` *(new)* | Inventory rows, dense table surfaces, anything inside a `default` card that needs less visual weight | Standalone surfaces — too quiet on the page bg |

---

## 5. Call-site refactors

### Login card

```tsx
// Before
<div className="rounded-3xl bg-[rgba(22,27,34,0.6)] backdrop-blur-xl border border-white/10 p-8 shadow-2xl">

// After
<CardShell variant="glass" padding="lg" radius="lg">
```

### Dashboard stat card

```tsx
// Before
<div className="bg-[var(--surface-1)] border border-[var(--surface-0)] rounded-2xl p-5 shadow-md">

// After
<CardShell variant="default">
```

### Insights / AI suggestion (NEW surface)

```tsx
<CardShell variant="ai" padding="lg">
  <div className="flex items-center gap-2 mb-3">
    <SparkleIcon className="text-brand-400" />
    <span className="text-xs uppercase tracking-wider text-brand-400 font-semibold">
      Insight
    </span>
  </div>
  <p className="text-sm text-text">
    Your PSA 9 modern stock is sitting 38 days longer than baseline.
    Consider a 5% reprice on Charizard, Pikachu VMAX, and Lugia.
  </p>
</CardShell>
```

### Inventory row (replaces the hand-rolled `.row` from PR #3)

```tsx
// In InventoryRow.tsx, swap the outer element:
<CardShell variant="data" padding="sm" radius="md" interactive={!!onClick} as={onClick ? 'button' : 'div'} onClick={onClick}>
  {/* grid contents unchanged */}
</CardShell>
```

This deletes the `.row` / `.interactive` / `.interactive:hover` blocks from `InventoryRow.module.css` — the primitive owns them now.

### MUST BUY detail card

```tsx
<CardShell variant="premium" padding="lg">
  <RecommendationBadge tier="MUST BUY" />
  <h3 className="text-lg font-semibold mt-2">PSA 10 Charizard VMAX Rainbow</h3>
  {/* ... */}
</CardShell>
```

---

## 6. Test checklist

- [ ] All 6 variants render in isolation (Storybook story or `/dev/cards` route)
- [ ] `interactive` adds hover lift + focus ring on every variant
- [ ] `as="button"` produces a real `<button type="button">` with no default browser styling bleeding through
- [ ] `ai` variant gradient border renders correctly in Safari (mask-composite is the risky bit — test in WebKit)
- [ ] `glass` variant doesn't stack with another `glass` parent — visual regression check on the login screen
- [ ] `premium` glow doesn't clip when the card is near a viewport edge (give parents `overflow: visible` where needed)
- [ ] Padding scale `sm/md/lg/none` all behave as documented
- [ ] Radius scale `sm/md/lg` all behave as documented

---

## 7. Migration order (suggested)

1. **Land the primitive** with no call-site changes — purely additive
2. **Refactor `LoginPage.tsx`** to use `variant="glass"` (highest-visibility win, single file)
3. **Refactor Dashboard stat cards** in a follow-up PR — touches more files, easier to review separately
4. **Use `ai` variant** for the next Insights feature (don't go retrofit hunting)
5. **Use `data` variant** when InventoryRow lands from PR #3

---

## Ship summary across all PRs

| PR | Scope | Token additions | Net-new components | Refactors |
|---|---|---|---|---|
| #1 | Grade ramp | `--grade-1..10` | — | `GradeBadge.tsx` |
| #2 | Button personalities | `--btn-*` per variant | `Button` variants extended | Existing call sites |
| #3 | Hero & inventory | hero + rec tier vars | `StatusPill`, `RecommendationBadge`, `InventoryRow` + `HeroStatsBar` refactor | `CardIntakeTab` (cleanup) |
| **#4** | **CardShell primitive** | **`--card-*`** | **`CardShell` (6 variants)** | **`LoginPage`, dashboard widgets, InventoryRow outer** |

That's the full migration. Anything beyond this — slide templates, marketing surfaces, mobile — is a separate workstream and probably should consume this system rather than extend it.
