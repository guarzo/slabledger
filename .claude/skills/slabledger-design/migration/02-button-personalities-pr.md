# PR: Button personalities

Replace the 5-variant Button with 8 purpose-built personalities. Each earns its look from a specific moment in the app (primary action, money-in, archive, AI insight, MUST BUY, etc.) instead of being a generic "primary/secondary/danger" trio.

**Files touched:** 3

- `web/src/css/tokens.css` — new button elevation + glow tokens
- `web/src/react/ui/Button.tsx` — expanded `variant` union + keyboard-hint slot
- `web/src/react/ui/Button.module.css` — per-variant styling
- `web/src/react/ui/Segmented.tsx` — **new** component (segmented toggle, extracted from old filter chips)

---

## 1. `tokens.css` — add a button-chrome section

```css
/* ============ BUTTON CHROME ============ */
--btn-radius: 10px;
--btn-radius-sm: 8px;
--btn-radius-lg: 12px;

/* Inset highlight that sells the "soft plastic" primary */
--btn-top-highlight: inset 0 1px 0 rgba(255,255,255,0.25);
--btn-bottom-edge:   0 1px 0 rgba(0,0,0,0.4);

/* Ambient glows per personality */
--btn-glow-brand:   0 6px 20px -4px rgba(99,102,241,0.55);
--btn-glow-brand-h: 0 10px 28px -4px rgba(99,102,241,0.70);
--btn-glow-success: 0 6px 18px -4px rgba(16,185,129,0.45);
--btn-glow-gold:    0 6px 16px -2px rgba(251,191,36,0.50);
--btn-glow-gold-h:  0 10px 24px -4px rgba(251,191,36,0.70);
--btn-glow-ai:      0 4px 16px -4px rgba(167,139,250,0.40);

/* Gold CTA uses a 2px "pressable" hard shadow */
--btn-gold-edge: 0 2px 0 #b45309;
```

---

## 2. `Button.tsx`

```tsx
import { clsx } from 'clsx';
import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react';
import styles from './Button.module.css';

export type ButtonVariant =
  | 'primary'       // indigo-glow — default CTA (Create campaign, Save)
  | 'success'       // green ledger — money-in moments (Record sale, Mark paid)
  | 'secondary'     // surface chip — neutral dismiss (Cancel)
  | 'danger'        // tinted ghost — destructive (Archive, Delete)
  | 'ghost'         // no chrome until hover — tertiary / navigation
  | 'ai'            // violet→teal border — insight generation
  | 'gold'          // MUST BUY CTA — the single loudest button in the app
  | 'fab';          // floating + icon only

export type ButtonSize = 'sm' | 'md' | 'lg';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Optional leading icon (any ReactNode; a 14px SVG works best). */
  icon?: ReactNode;
  /** Optional keyboard hint, e.g. "Esc" or "⌘K". Renders a small kbd chip. */
  kbd?: string;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = 'primary', size = 'md', icon, kbd, children, className, ...rest }, ref) => {
    return (
      <button
        ref={ref}
        className={clsx(styles.btn, styles[`v-${variant}`], styles[`s-${size}`], className)}
        {...rest}
      >
        {icon}
        {children}
        {kbd && <span className={styles.kbd}>{kbd}</span>}
      </button>
    );
  },
);
Button.displayName = 'Button';
```

---

## 3. `Button.module.css`

```css
.btn {
  display: inline-flex; align-items: center; justify-content: center;
  gap: 8px;
  font-family: inherit; font-weight: 600;
  border: 1px solid transparent;
  cursor: pointer; white-space: nowrap;
  transition: transform 180ms cubic-bezier(.4,0,.2,1),
              box-shadow 180ms cubic-bezier(.4,0,.2,1),
              background 180ms ease, filter 180ms ease;
  letter-spacing: -0.005em;
  position: relative; overflow: hidden;
}
.btn:active { transform: translateY(1px) scale(0.99); }
.btn:focus-visible {
  outline: 3px solid var(--color-focus);
  outline-offset: 3px;
}

/* sizes */
.s-sm { font-size: 12px; padding: 6px 12px; border-radius: var(--btn-radius-sm); }
.s-md { font-size: 13px; padding: 9px 16px; border-radius: var(--btn-radius); }
.s-lg { font-size: 14px; padding: 11px 20px; border-radius: var(--btn-radius-lg); }

/* keyboard chip */
.kbd {
  display: inline-flex; align-items: center; justify-content: center;
  font-size: 10px; font-family: var(--font-mono); font-weight: 600;
  min-width: 16px; padding: 1px 5px; border-radius: 4px;
  background: rgba(255,255,255,0.10); color: rgba(255,255,255,0.75);
  margin-left: 2px;
}

/* ========== PRIMARY · indigo-glow ========== */
.v-primary {
  background: linear-gradient(180deg, var(--brand-500) 0%, var(--brand-600) 100%);
  color: #fff;
  border-color: rgba(255,255,255,0.12);
  box-shadow: var(--btn-top-highlight), var(--btn-bottom-edge),
              var(--btn-glow-brand), 0 0 0 1px rgba(99,102,241,0.25);
}
.v-primary:hover {
  transform: translateY(-1px);
  box-shadow: var(--btn-top-highlight), var(--btn-bottom-edge),
              var(--btn-glow-brand-h), 0 0 0 1px rgba(99,102,241,0.40);
}

/* ========== SUCCESS · money ledger ========== */
.v-success {
  background: linear-gradient(180deg, #10b981 0%, #059669 100%);
  color: #022c22;
  border-color: rgba(255,255,255,0.15);
  font-weight: 700;
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.35), var(--btn-bottom-edge),
              var(--btn-glow-success);
}
.v-success:hover { transform: translateY(-1px); filter: brightness(1.05); }

/* ========== SECONDARY · surface chip ========== */
.v-secondary {
  background: linear-gradient(180deg, var(--surface-2) 0%, var(--surface-1) 100%);
  color: var(--text);
  border-color: rgba(255,255,255,0.07);
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.04), 0 2px 6px rgba(0,0,0,0.3);
}
.v-secondary:hover {
  border-color: rgba(255,255,255,0.15);
  background: linear-gradient(180deg, #273244 0%, var(--surface-1) 100%);
}

/* ========== DANGER · tinted ghost ========== */
.v-danger {
  background: rgba(248,113,113,0.08);
  color: #fca5a5;
  border-color: rgba(248,113,113,0.25);
}
.v-danger:hover {
  background: rgba(248,113,113,0.14);
  border-color: rgba(248,113,113,0.50);
  color: #fecaca;
}

/* ========== GHOST ========== */
.v-ghost {
  background: transparent;
  color: var(--text-muted);
  border: 1px solid rgba(255,255,255,0.06);
}
.v-ghost:hover {
  color: var(--text);
  border-color: rgba(255,255,255,0.15);
  background: rgba(255,255,255,0.03);
}

/* ========== AI · violet→teal gradient border ========== */
.v-ai {
  color: #c4b5fd;
  background:
    linear-gradient(rgba(17,24,39,0.95), rgba(17,24,39,0.95)) padding-box,
    linear-gradient(135deg, #a78bfa 0%, #22d3ee 100%) border-box;
  border: 1px solid transparent;
  box-shadow: var(--btn-glow-ai);
}
.v-ai:hover { color: #e9d5ff; transform: translateY(-1px); }

/* ========== GOLD · MUST BUY ========== */
.v-gold {
  background: linear-gradient(135deg, #f59e0b 0%, #fbbf24 50%, #f59e0b 100%);
  color: #422006;
  border-color: rgba(0,0,0,0.15);
  font-weight: 800;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  font-size: 12px;  /* overrides size default */
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.5),
              var(--btn-gold-edge),
              var(--btn-glow-gold);
}
.v-gold:hover {
  filter: brightness(1.06);
  transform: translateY(-1px);
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.5),
              var(--btn-gold-edge),
              var(--btn-glow-gold-h);
}
.v-gold:active {
  transform: translateY(1px);
  box-shadow: inset 0 2px 4px rgba(0,0,0,0.2), 0 0 0 #b45309;
}

/* ========== FAB · floating + rotating plus ========== */
.v-fab {
  width: 40px; height: 40px; padding: 0; border-radius: 999px;
  background: linear-gradient(135deg, var(--brand-500), var(--brand-600));
  color: #fff;
  border: 1px solid rgba(255,255,255,0.15);
  box-shadow: var(--btn-glow-brand), inset 0 1px 0 rgba(255,255,255,0.3);
}
.v-fab:hover {
  transform: translateY(-2px) rotate(90deg);
  box-shadow: var(--btn-glow-brand-h), inset 0 1px 0 rgba(255,255,255,0.3);
}
```

---

## 4. `Segmented.tsx` — new component

Extract the channel/tenor toggle pattern. This was previously hand-rolled from ghost buttons.

```tsx
import { clsx } from 'clsx';
import styles from './Segmented.module.css';

interface SegmentedProps<T extends string> {
  options: { value: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
  ariaLabel?: string;
}

export function Segmented<T extends string>({ options, value, onChange, ariaLabel }: SegmentedProps<T>) {
  return (
    <div className={styles.seg} role="radiogroup" aria-label={ariaLabel}>
      {options.map((o) => (
        <button
          key={o.value}
          role="radio"
          aria-checked={value === o.value}
          className={clsx(styles.item, value === o.value && styles.on)}
          onClick={() => onChange(o.value)}
          type="button"
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
```

```css
/* Segmented.module.css */
.seg {
  display: inline-flex; gap: 2px;
  background: var(--surface-0);
  border: 1px solid rgba(255,255,255,0.06);
  border-radius: 10px;
  padding: 3px;
}
.item {
  background: transparent; border: 0;
  color: var(--text-muted);
  font-family: inherit;
  font-size: 12px; font-weight: 600;
  padding: 6px 12px; border-radius: 7px;
  cursor: pointer;
  transition: background 150ms ease, color 150ms ease;
}
.item:hover { color: var(--text); }
.on {
  background: var(--surface-2); color: var(--text);
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.05);
}
```

---

## Usage guide — which variant when

| Variant | Use for | Examples |
|---|---|---|
| `primary` | The default CTA on any form or page | Save campaign · Create · Apply filters |
| `success` | Money-in / ledger actions | Record sale · Mark invoice paid · Confirm payout |
| `secondary` | Neutral dismiss or alt | Cancel (with `kbd="Esc"`) · Close · Back |
| `danger` | Destructive but reversible | Archive · Remove from batch · Soft delete |
| `ghost` | Tertiary / in-nav | Skip (with `kbd="⌘K"`) · Dismiss banner · Clear filter |
| `ai` | AI/insight generation only | Generate insight · Auto-price · Explain trend |
| `gold` | **One per screen, max.** The MUST BUY moment. | Lock in MUST BUY · Claim high-confidence deal |
| `fab` | Global "new" in the header | + new campaign |

**Rule of thumb**: if you want to add a `gold` button, first check if there's already one on the page. If yes, this one should be `primary`. Gold is scarce by design.

---

## Migration

Most existing call sites are already `variant="primary|secondary|danger|ghost"` — those are no-ops. New work:

1. Grep for success-y primary buttons (`Record sale`, `Confirm payment`, `Mark paid`) → switch to `variant="success"`.
2. Grep for AI/insight CTAs (`Generate`, `Explain`, `Ask AI`) → switch to `variant="ai"`.
3. Find the `MUST BUY` / lock-in CTAs in the recommendations UI → switch to `variant="gold"`.
4. Header + empty-state "+new" → `variant="fab"` (they're currently custom circles).
5. Replace any hand-rolled filter chip row with `<Segmented>`.

## Test checklist

- [ ] Only one `gold` button renders per screen (lint with a `data-testid` sweep if helpful)
- [ ] `ai` variant's border gradient renders in Safari (it's `background-clip: border-box` — supported ≥ Safari 14; confirm on your target)
- [ ] `fab` rotation animates smoothly, not jumpy, on hover (GPU-accelerated by default)
- [ ] `kbd` chip doesn't overflow in the `sm` size
- [ ] `success`'s dark `#022c22` text stays legible when the gradient is desaturated (`prefers-contrast: more` users)
- [ ] All variants show the 3px focus ring on keyboard focus
- [ ] Reduced-motion: the `-1px` hover lift is decorative; add `@media (prefers-reduced-motion: reduce)` to zero the `transform` transitions

---

That's both PRs. Ship `01-grade-badge-pr.md` first — lower risk, immediate visual win. This one is broader and touches call sites, so plan a grep + codemod session.

Let me know if you want me to also diff the other preview cards (the `HeroStatsBar`, the inventory row with recommendation badges) — those are the next biggest perception wins.
