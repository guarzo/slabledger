# Design

> Source of truth: `web/src/css/tokens.css` and `web/src/css/base.css`. This document mirrors them. When tokens change, update this file.

## Visual Theme

**Dark-mode only.** Light mode was explicitly removed. The page lives on a layered radial+linear background — `radial-gradient(ellipse at top, #131b2e, transparent)`, `radial-gradient(ellipse at bottom, #0a0f1a, transparent)`, and a vertical fade from `#0f172a` to `#0b1120`, all `background-attachment: fixed`. Content scrolls; the bg does not.

The product feel is *operator terminal, not consumer SaaS.* Dense tabular data, tabular-num everywhere, hairline row dividers, indigo for interaction, gold for PSA 10. Glass appears on exactly two surfaces: the sticky header and the login card. Pokémon-themed gradients exist as tokens but are reserved for premium moments — recommendation badges, the occasional gold-glow PSA 10 — never decorative section fills.

Mood sentence: *the operator opens this at 7am with coffee, glances at ROI on a 27-inch monitor, and acts on three things before standing up.*

## Color System

OKLCH was not used in the source; the existing palette is sRGB hex. Tint-toward-brand is implicit in the surface ramp (cool blue-gray neutrals).

**Strategy: Restrained.** Tinted neutrals carry ~85% of the surface; indigo (`--brand-500`) is the single accent for interaction; semantic green/yellow/red carry money-state meaning; grade ramp + channel hues are the one piece of branded polychrome — and only used functionally, never decoratively.

### Brand

```
--brand-50  #eef2ff   --brand-500 #5a5de8  ← interaction color
--brand-100 #e0e7ff   --brand-600 #4f46e5
--brand-200 #c7d2fe   --brand-700 #4338ca
--brand-300 #a5b4fc   --brand-800 #3730a3
--brand-400 #818cf8   --brand-900 #312e81
```

`--primary: #2563eb` is the richer blue used on primary CTA fills. `--brand-500` is the atmospheric/glow indigo (focus rings, selected tabs, links, login orbs). Both are valid — pick per surface.

### Surfaces (elevation ramp, dark)

```
--bg            #0f172a   page background
--surface-0     #0b1220   flat / lowest (used as card border — receding edge)
--surface-1     #111827   default card fill
--surface-2     #1f2937   raised card, inputs, hover
--surface-3     #374151   higher elevation, default border
--surface-4     #4b5563   highest elevation
--surface-overlay  rgba(0,0,0,0.5)   modal backdrop
```

### Text

```
--text          #e5e7eb   primary
--text-muted    #b0b8c4   secondary (WCAG AA)
--text-subtle   #8b95a5   tertiary / hints
--color-heading #f9fafb
--color-paragraph #d1d5db
```

### Semantic (bright-on-dark for AA)

```
--success #34d399  --success-bg rgba(16,185,129,0.10)  --success-border rgba(16,185,129,0.20)
--warning #fbbf24  --warning-bg rgba(245,158,11,0.10)  --warning-border rgba(245,158,11,0.20)
--danger  #f87171  --danger-bg  rgba(239,68,68,0.10)   --danger-border  rgba(239,68,68,0.20)
--info    #22d3ee  --info-bg    rgba(34,211,238,0.10)  --info-border    rgba(34,211,238,0.20)
```

### Money-state aliases (use these for capital/inventory states)

```
--state-waiting    →  --text-muted   (Outstanding, Awaiting Intake/Receipt)
--state-at-risk    →  --warning      (At Risk capital)
--state-problem    →  --danger       (Below Cost, Unrealized loss)
```

### Grade quality ramp (1–10) + special

Each grade has `-bg` (≈22% alpha), `-bd` (≈45%), `-fg` (full color text). Grade 10 is a gradient + gold glow; BGS Black Label is a separate dark variant.

```
1–2   reds        9     emerald  #6ee7b7
3–4   oranges    10     gold gradient + --grade-10-glow
5–6   yellows           PSA 10 #fde68a fg
7     lime
8     green       black label  #e5e7eb fg on near-black fill
```

Half-grades (CGC 9.5, BGS 8.5) snap up to the next integer.

### Channel hues (for chart legends, channel pills)

```
--channel-ebay       #3b82f6
--channel-website    #f59e0b
--channel-inperson   #10b981
--channel-tcgplayer  #8b5cf6   (legacy)
--channel-local      #10b981   (legacy)
--channel-gamestop   #ef4444   (legacy)
--channel-cardshow   #ec4899   (legacy)
--channel-other      #6b7280
```

### Pokémon gradients (premium moments only)

```
--gradient-charizard   #FF3C00 → #FFD700
--gradient-pikachu     #FFD700 → #FFA500
--gradient-lugia       #2563eb → #60a5fa
--gradient-mewtwo      #764ba2 → #9333ea
--gradient-rayquaza    #10b981 → #34d399
```

Use sparingly — recommendation badges, occasional accent strips. Never fill a section.

### Recommendation badge tiers

Each tier is a 135° gradient + matching border + colored drop-shadow.

```
MUST BUY        #047857 → #059669   (green, premium glow)
STRONG BUY      #059669 → #10b981
BUY             #10b981 → #34d399
BUY WITH CAUTION #f59e0b → #fbbf24  (amber)
WATCH           #6b7280 → #9ca3af   (neutral)
AVOID           #dc2626 → #ef4444   (red)
```

### Price-quality indicators

```
excellent  #22c55e   good       #4ade80
acceptable #eab308   above      #facc15
expensive  #ef4444
```

### Intelligence indicators (Sprint 5)

```
velocity      high #10b981   moderate #f59e0b   stagnant #ef4444
momentum      up #10b981     flat #6b7280       down #ef4444
confidence    high #10b981   medium #f59e0b     low #ef4444
```

## Typography

**No webfonts.** Platform UI stack only.

```
--font-sans  ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont,
             "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif
--font-mono  ui-monospace, "SF Mono", Menlo, Monaco, "Cascadia Code",
             "Roboto Mono", Consolas, "Courier New", monospace
```

### Scale (Tailwind-aligned)

```
2xs  10px  --font-size-2xs   tiny labels
xs   12px  --font-size-xs    table headers, meta
sm   14px  --font-size-sm    UI default — labels, form controls
base 16px  --font-size-base
lg   18px  --font-size-lg
xl   20px  --font-size-xl
2xl  24px  --font-size-2xl
3xl  30px  --font-size-3xl   h2
4xl  36px  --font-size-4xl   h1
```

### Weights

```
medium    500    body UI
semibold  600    headings (h1–h6 default)
bold      700    stat values
extrabold 800    hero ROI, login wordmark
```

### Headings

`h1 36 / h2 30 / h3 24 / h4 20 / h5 18 / h6 16`. Line-height `1.25`. Color `--color-heading #f9fafb`.

### Signature treatment

UPPERCASE + `tracking-wider` (0.05em) at `2xs/xs` is the stat-label / table-header voice. Reserved for that role; not for body or buttons.

### Tabular numerals

`font-variant-numeric: tabular-nums` on every price, percentage, count, and cert. Class: `.tabular-nums`.

### Hero ROI

`--hero-roi-size: clamp(32px, 4vw, 44px)` at `extrabold 800`. The HeroStatsBar's signature.

### Body line length

Cap at 65–75ch on prose surfaces. Almost no surface in this app is prose; this rule mostly governs the login card and AI insight summaries.

## Spacing

4px base scale + half-steps for dense tables.

```
--space-0      0
--space-0-5    2px
--space-1      4px
--space-1-5    6px
--space-2      8px
--space-2-5   10px
--space-3     12px
--space-3-5   14px   ← card-to-card gap
--space-4     16px
--space-5     20px   ← default card padding
--space-6     24px
--space-8     32px
--space-10    40px
--space-12    48px
--space-16    64px
```

Card padding tiers: `sm 14px / default 20px / lg 28px`.
Header chrome: `py-2.5 px-6` (10×24).
Page content: `max-w-6xl mx-auto px-4 py-8`.
Top chrome max-width: `1920px`.

## Corner Radii

```
--radius-sm    10px   compact pills, inputs
--radius-md    14px   most buttons, nav items, inline badges
--radius-lg    18px   cards (default)
--radius-xl    22px   login card, feature surfaces
--radius-full  9999px avatars, capsule pills, Google sign-in button
--btn-radius   10px   --btn-radius-sm 8px   --btn-radius-lg 12px
```

## Elevation & Shadows

```
--shadow-1  0 6px 16px rgba(0,0,0,0.24)    card
--shadow-2  0 10px 30px rgba(0,0,0,0.35)   raised
--shadow-3  0 20px 40px rgba(0,0,0,0.45)   modal
```

### Glows (premium moments only)

```
--glow-gold        0 0 20px rgba(255,215,0,0.5)     PSA 10
--glow-brand       0 0 20px rgba(99,102,241,0.4)    selected/featured
--glow-brand-soft  0 0 20px rgba(99,102,241,0.3)
--glow-violet      0 0 40px rgba(139,92,246,0.5)    AI / insights
```

### Nav

```
--shadow-nav-inset  inset 0 1px 2px rgba(0,0,0,0.10), 0 2px 8px rgba(0,0,0,0.08)
--shadow-nav-active 0 4px 16px rgba(99,102,241,0.4), inset 0 1px 1px rgba(255,255,255,0.10)
--shadow-brand-scroll 0 8px 32px rgba(99,102,241,0.15)
```

### Button chrome (operator-grade affordance)

```
--btn-top-highlight  inset 0 1px 0 rgba(255,255,255,0.25)
--btn-bottom-edge    0 1px 0 rgba(0,0,0,0.4)
--btn-glow-brand     0 6px 20px -4px rgba(99,102,241,0.55)
--btn-glow-brand-h   0 10px 28px -4px rgba(99,102,241,0.70)   hover
--btn-glow-success   0 6px 18px -4px rgba(16,185,129,0.45)
--btn-glow-gold      0 6px 16px -2px rgba(251,191,36,0.50)
--btn-glow-gold-h    0 10px 24px -4px rgba(251,191,36,0.70)
--btn-glow-ai        0 4px 16px -4px rgba(167,139,250,0.40)
--btn-gold-edge      0 2px 0 #b45309
```

## Borders

- Default: `var(--surface-3)` (#374151).
- Card: 1px solid `--surface-0` (darker than fill — receding edge on dark bg).
- Glass: `rgba(139,152,207,0.10)`.
- Hairline rows: `border-bottom: 1px solid rgba(255,255,255,0.03)` — intentionally under-contrast.
- P&L row strips: 2px `border-left` keyed to `--success` / `--danger` / transparent neutral.
- Severity strips (Insights / Tuning): `data-severity="act|kill"` red, `tune` warning, `ok` success.

## Glassmorphism (rare and deliberate)

```
--glass-bg     rgba(22,27,34,0.7)
--glass-border rgba(139,152,207,0.1)
--glass-blur   blur(20px) saturate(180%)
```

Only on the sticky header (`bg-[var(--surface-1)]/80 backdrop-blur-xl`) and the login card. Don't stack glass-on-glass.

Backdrop-blur scale: `xs 2 / sm 4 / md 12 / lg 16 / xl 24 / 2xl 40 / 3xl 64` (px).

## Motion

**Easing:** `cubic-bezier(0.4, 0, 0.2, 1)` for everything UI.

```
--transition-fast  150ms cubic-bezier(0.4, 0, 0.2, 1)
--transition-base  200ms cubic-bezier(0.4, 0, 0.2, 1)
--transition-slow  300ms cubic-bezier(0.4, 0, 0.2, 1)
```

**Entries:** `fadeIn`, `fadeInUp`, `scaleIn`, `bounceIn` (overshoot `cubic-bezier(0.68,-0.55,0.265,1.55)`). 250–400ms.

**Glow pulses:** `pulse-glow`, `pulse-glow-blue`, `titleGlow` — 2–3s ease-in-out infinite. Login wordmark and occasional premium cards. They breathe; they don't bounce.

**Buttons:** `useRipple` hook — white ripple on filled (primary/success/danger/warning), dark ripple on secondary/ghost/outline. 600ms.

**Loader:** Pokéball spinner for route transitions.

**Reduced motion:** all durations collapse to `0.01ms` via `prefers-reduced-motion: reduce`. Confirmed in `base.css`.

**Don't animate layout properties.** Transform and opacity only.

## States

**Hover.**
- Buttons: `hover:-translate-y-0.5 hover:shadow-sm` + brightness boost on solid fills.
- Chrome links: `text-muted → text` + faint `bg-surface-2/10` panel.
- Interactive cards: `-translate-y-0.5` lift + `shadow-1 → shadow-2` + bg → `--surface-hover`.
- Glass table rows: `bg: rgba(255,255,255,0.02)` — almost invisible, just enough to track the pointer.

**Press.**
- Buttons return to baseline + inner shadow; primary keeps its fill.
- Ripple fires on press, fades during release.

**Focus.**
- 3px outline `var(--color-focus)` (#60a5fa), offset 3px, plus `box-shadow: 0 0 0 4px var(--color-focus-ring)` halo.
- Buttons/links/inputs use offset 2px + `--color-focus-ring-subtle` halo.
- Interactive cards (`[data-interactive="true"]`) use offset 4px + 6px halo for clear distinction from focusable controls inside them.

## Components

### Card (six variants)

```
default   bg #111827   border #0b1220   shadow-1
elevated  bg #1f2937                    shadow-2
glass     bg rgba(22,27,34,0.7)   border rgba(139,152,207,0.1)   blur(20px) saturate(180%)
premium   linear-gradient(135deg, #111827 0%, #1f2937 100%)   border rgba(99,102,241,0.30)   glow-brand
ai        radial(120% 100% at 0% 0%, rgba(99,102,241,0.10), transparent 60%) on #111827
          + gradient border (mask trick): #6366f1 → #22d3ee → #a855f7
data      bg #0f1623   border rgba(255,255,255,0.04)   inset 0 1px 0 rgba(255,255,255,0.03)
```

Default padding `--space-5` (20px). Default radius `--radius-lg` (18px). Interactive variant adds `tabIndex=0`, ARIA, lift on hover, full focus ring.

### Button

`--btn-radius 10px`. Sizes `sm 8px / lg 12px`. Top-highlight inset, bottom-edge shadow, variant-specific glow. Ripple on press.

Variants by glow token:
- **Primary** (indigo): `--btn-glow-brand` → `-h` on hover.
- **Success** (green): `--btn-glow-success`.
- **Gold** (PSA 10 / premium): `--btn-glow-gold` + `--btn-gold-edge`.
- **AI** (violet): `--btn-glow-ai`.
- **Secondary / ghost / outline**: dark ripple, no glow.

### Badge / capsule (semantic pills)

`rounded-full`, `padding 2px 8px (sm) → 6px 12px (lg)`. Fill 10% alpha, border 30% alpha, full-color text. Tokens: `color-badge-{primary|success|warning|danger|info|neutral}` + `-border`.

```
sm  font-size-2xs   space-0-5 / space-2
md  font-size-xs    space-1   / space-2-5
lg  font-size-sm    space-1-5 / space-3
radius var(--radius-md)
```

### GradeBadge

Capsule keyed to grader+grade. Uses the `--grade-{1..10}-{bg|bd|fg}` ramp; Grade 10 adds `--grade-10-glow`. Black Label is a black-fill variant. Half-grades snap up.

### RecommendationBadge

135° gradient + matching border + colored drop-shadow per tier (MUST BUY → AVOID).

### HeroStatsBar (signature dashboard layout)

One oversize ROI on the left (`--hero-roi-size`, extrabold), supporting stats in a flex-wrap row to the right, separated by a hairline `--hero-divider: 1px solid rgba(255,255,255,0.08)`. **No cards.** Just typography arranged on the page.

### Glass table (data tables)

```
container  border-radius 12px, bg rgba(255,255,255,0.02), border rgba(255,255,255,0.06)
header     transparent, border-bottom rgba(255,255,255,0.06)
th         padding 0.75rem, font-size 11px, uppercase, tracking 0.05em, weight 600, text-muted
row        border-bottom rgba(255,255,255,0.03), transition 150ms
row:hover  bg rgba(255,255,255,0.02)
td         padding 0.75rem
```

Virtualized variant uses `data-stripe`, `data-selected`, `data-belowcost`, `data-pl` attributes instead of `:nth-child`.

P/L strips: 2px `border-left` keyed to `data-pl="positive|negative|neutral"`.

Below-cost rows: `bg --danger-tint` (5% alpha red).

Selected: `--table-row-selected` (8% indigo) → `--table-row-selected-hover` (12%).

Expanded: `glass-vrow-expanded` — `bg rgba(255,255,255,0.015)` + `inventory-expand-in 200ms ease-out`.

### Header (sticky chrome)

`top-0 z-50`, `bg-[var(--surface-1)]/80 backdrop-blur-xl`. Gains `shadow-md` after 10px scroll. Logo 28×28 `rounded-lg`. Nav items use `--shadow-nav-inset` and `--shadow-nav-active` for the active pill.

### Login card (the one brand moment)

`rgba(22,27,34,0.6)` + `blur(20px)` glass card. Two animated colored orbs (400px brand-primary + 300px success→cyan) at 80px blur, opacity 0.4, 8s float loop. Brand wordmark with `titleGlow` pulse. Card Yeti logo at 180×120. Google sign-in: official multi-color "G" mark, full pill, `--shadow-button-light` and `--shadow-button-focus`.

### EmptyState

Optional icon, short title, one-sentence description, numbered `steps` array. Example: `['Create a campaign', 'Import PSA purchases', 'Record sales as you go']`. No "Welcome!" copy.

### Pokéball loader

Themed spinner for route transitions. Defined in `pokeball-loader.css`. Honors `prefers-reduced-motion`.

## Iconography

**No icon library imported.** Every icon is a hand-inlined SVG, traced from Feather/Lucide. Style: `stroke-width: 2`, `stroke-linecap: round`, `stroke-linejoin: round`, `fill: none`, `currentColor`, 24×24 viewBox, rendered ~14–20px.

For prototypes/specimens, load Lucide from CDN — closest 1:1 match. Lift the Google "G" verbatim for the sign-in card.

PSA / grade labels are *not* icons — they're `GradeBadge` capsules with literal text (`PSA 10`, `BGS 10`).

## Charts

```
--chart-line-primary       #2563eb
--chart-line-secondary     #8b5cf6
--chart-line-spend         #ef4444
--chart-line-recovery      #22c55e
--chart-line-outstanding   #3b82f6
--chart-area-fill          rgba(37,99,235,0.20)
--chart-grid               rgba(255,255,255,0.10)
--chart-axis-text          #9ca3af
--chart-tooltip-bg         rgba(22,27,34,0.95)
--chart-tooltip-border     rgba(255,255,255,0.10)
--chart-tooltip-text       #e5e7eb
--chart-cursor             #2563eb
```

## Layout

- Sticky header at `top-0 z-50`, glass.
- Main content: `max-w-6xl (1280px) mx-auto px-4 py-8`.
- Top chrome max-width: `1920px`.
- HeroStatsBar is the dashboard signature — typography on the page, not a card grid.
- Don't wrap everything in a container. Most things don't need one.
- Cards are not the lazy answer — use them only when they're the best affordance. Never nest cards.

## Imagery

- App working surfaces: **no imagery.** Solid surfaces with the body radial gradient peeking through.
- Login: two animated colored orbs behind a glass card. Single brand moment.
- Logos: `slabledger-card-logo.png` (AI-styled PSA-slabbed Emolga, comic-book burst), `card-yeti-logo.png` (icy-blue gradient yeti wordmark + mountain range), `favicon.ico`.
- New imagery: saturated, product-true (real Pokemon cards in slabs). Never generic stock photography. Ask before inventing.

## Z-Index Layers

```
--z-dropdown          1000
--z-sticky            1020
--z-fixed             1030
--z-modal-backdrop    1040
--z-modal             1050
--z-popover           1060
--z-tooltip           1070
```

## Print

Working surfaces flip to `background: white !important; color: black !important`. Header, nav, `.no-print`, `.print:hidden` are hidden. `@page { margin: 0.4in 0.35in; size: portrait }`. Used by sell-sheet exports.

## Banned Patterns

Per impeccable absolute bans, plus SlabLedger-specific:

- **Side-stripe colored borders** as accents on cards/callouts. P&L 2px left-strips and severity strips are the only sanctioned uses (functional, keyed to data, never decorative).
- **Gradient text** as decoration. The Pokémon `--gradient-*` tokens fill recommendation badges and accent strips, never headlines or body. `.text-gradient-premium` exists in CSS but is reserved for the login wordmark.
- **Glassmorphism by default.** Header + login card only.
- **Hero-metric template** (big number + label + supporting stats + gradient accent in identical cards). HeroStatsBar avoids this on purpose — typography on the page, hairline divider, no cards.
- **Identical card grids.** Vary card variant by content semantics (default / elevated / glass / premium / ai / data).
- **Modal as first thought.** Prefer drawers (slide-in from right), inline expansion (`glass-vrow-expanded`), or progressive disclosure.
- **Em dashes in copy.** Use commas, colons, semicolons, periods, parentheses.
- **Decorative whole-section gradients.** Pokémon gradients are for premium moments only.
- **New grade colors.** The grade ramp is functional and fixed.
