# SlabLedger Design System

> Internal tool for managing the purchase and sale of graded Pokemon cards.

## What is SlabLedger?

SlabLedger is an internal portfolio-tracking application for PSA **Direct Buy** campaigns — an operator buys already-graded Pokemon cards through automated campaigns, then resells them across multiple channels (eBay, TCGPlayer, local shops, direct sales). The app's job is to keep ledgers honest: what got bought, what sold where, and whether a campaign is making money.

Core surfaces:

- **Dashboard** — top-level portfolio health (ROI, deployed/recovered/at-risk capital, weeks-to-cover)
- **Campaigns** — create/configure buy campaigns (CL%, grade range, daily spend cap); per-campaign P&L, fill rate, inventory
- **Inventory** — unsold cards across all campaigns with aging + market-direction signals (rising/falling/stable)
- **Reprice** — liquidation flow for long-held inventory
- **Insights** — AI-generated reports hub
- **Invoices** — unpaid invoice tracking
- **Tools** — price lookup, card search, CSV import helpers
- **Admin** — API usage, provider status dashboard

Stack: **Go** backend (SQLite) + **React 19 / Radix UI / Tailwind v4 / TanStack Query / Vite**. The frontend is a single dark-mode SPA protected behind Google OAuth.

It's a small-audience internal tool (PSA operators and partners), not a consumer product — which shapes the design tone: dense tables, tabular numbers everywhere, ROI front-and-center, minimal explanatory copy, lots of hotkeys and power-user affordances.

## Sources

- **GitHub codebase:** `guarzo/slabledger` — the source of truth for this design system. Key files read during construction:
  - `web/tailwind.config.cjs` — full color / shadow / animation palette
  - `web/src/css/tokens.css` — runtime CSS custom properties (the real design tokens)
  - `web/src/css/base.css` — typography, table styles, background gradients, print rules
  - `web/src/css/LoginPage.css` — auth screen motifs
  - `web/src/react/ui/*` — component primitives (Button, CardShell, Input, GradeBadge, StatCard…)
  - `web/src/react/components/Header.tsx` + `Navigation.tsx` — app chrome
  - `web/src/react/pages/DashboardPage.tsx`, `LoginPage.tsx` — representative screens
  - `web/src/react/components/portfolio/HeroStatsBar.tsx` — the signature dashboard hero
  - `web/public/logo.png`, `web/src/assets/card-yeti-business-logo.png` — brand marks
- **Related private repo:** `guarzo/slabledger-private` (docs backup; not inspected)
- **Brand neighbor:** `Double-Holo/*` repos — SlabLedger consumes the DoubleHolo enterprise pricing API, but the brands are separate

No Figma was provided. No screenshots were provided. All UI recreations are code-derived.

---

## Content Fundamentals

**Voice: operator-first, telegraphic, no hand-holding.** The user is already running a grading-and-flipping business; the UI assumes they know what PSA 10, CL%, fill rate, and days-to-sell mean. Strip explanatory copy. Let the numbers carry the page.

- **Person.** No "I". Second-person ("you") appears only in onboarding empty states and auth copy. Most surfaces are nounal labels — `Deployed`, `Recovered`, `At Risk`, `Outstanding` — not sentences.
- **Casing.** Section headers and page titles use Title Case ("Weekly Review", "Invoice Readiness"). Stat labels and table headers use UPPERCASE with `tracking-wider` (`REALIZED ROI`, `WKS TO COVER`, `30D RECOVERY`). Button text is Title Case ("Sign in with Google", "Create Campaign").
- **Numbers.** Always `tabular-nums`. Cents are formatted as dollars with `formatCents()`. Percentages through `formatPct()`. Negatives colored `var(--danger)`, positives `var(--success)`, neutrals inherit body text.
- **Abbreviation is fine.** `Wks to Cover`, `30d Recovery`, `P&L`, `ROI`, `CL%`. Never re-expand on the same screen.
- **Domain vocabulary** (use exactly these):
  - `Campaign`, `Direct Buy`, `CL%`, `Fill rate`, `Days to sell`, `Sell-through rate`
  - `PSA 10 / 9 / 8`, `BGS 10 Black Label`, `CGC`
  - Channels: `eBay`, `TCGPlayer`, `Local`, `Website`, `In-person`, `GameStop`, `Card show`, `Other`
  - Recommendation tiers (badge labels): `MUST BUY`, `STRONG BUY`, `BUY`, `BUY WITH CAUTION`, `WATCH`, `AVOID`
  - Market direction: `rising`, `falling`, `stable`
- **Emoji** are used sparsely on the login page and empty states only (`📊 Campaign tracking`, `💰 P&L analytics`, `🔍 Price lookup`, `📊 Welcome to SlabLedger`). They are *not* used in working surfaces — tables, forms, dashboards stay glyph-free.
- **Error copy is flat and instructional.** "Authentication failed. Please try again." / "Your account is not authorized. Contact an administrator for access." No exclamation points, no apologies.
- **Empty states use `EmptyState` component** with an optional icon, a short title, a one-sentence description, and a numbered `steps` array — e.g. `['Create a campaign', 'Import PSA purchases', 'Record sales as you go']`.

**Example copy pulled from the app:**

> Track PSA Direct Buy campaigns, manage card inventory across multiple sell channels, and analyze profitability with market direction signals.
>
> `SlabLedger` / `Graded Card Portfolio Tracker` (login hero)
>
> `REALIZED ROI` / `Deployed` / `Recovered` / `At Risk` / `Wks to Cover` / `Outstanding` / `30d Recovery` (HeroStatsBar)
>
> `3 unpaid invoices →` (inline warning pill)

---

## Visual Foundations

**Dark-mode only.** Light mode was explicitly removed from the codebase. Design against `--bg: #0f172a` layered with two radial gradients (top `#131b2e`, bottom `#0a0f1a`) and a vertical fade. Background is `background-attachment: fixed` — it does not scroll with content.

**Color system.**
- **Brand indigo** `#5a5de8` (`--brand-500`) is the interaction color — focus rings, selected tabs, links in UI chrome, CTA buttons' alternate. A separate `--primary: #2563eb` (richer blue) is the primary button fill in the Tailwind config; indigo wins on atmospheric/glow moments, blue wins on explicit CTAs. Both are valid — choose per surface.
- **Surfaces** step up in elevation: `--surface-0` (#0b1220) < `--surface-1` (#111827, default card) < `--surface-2` (#1f2937, raised) < `--surface-3` (#374151) < `--surface-4` (#4b5563). Card borders are `--surface-0` (darker than the card itself — gives a receding edge on a dark bg).
- **Semantic palette is bright-on-dark** to hit WCAG AA: `--success #34d399`, `--warning #fbbf24`, `--danger #f87171`, `--info #22d3ee`. Each has a `-bg` (10% alpha) and `-border` (20% alpha) token used together for pill backgrounds.
- **Grade colors** are the one piece of branded polychrome in the app: PSA 10 gold `#fbbf24`, PSA 9 blue `#2563eb`, PSA 8 dark amber `#a16207`, BGS 10 Black Label (black), CGC orange `#f59e0b`. Never invent new grade colors — these are functional.
- **Channel colors** are tokenized (`--channel-ebay #3b82f6`, `--channel-website #f59e0b`, etc) so chart legends read consistently across the app.

**Typography.**
- **No webfonts.** The app ships the platform UI stack: `ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif`. Mono is the platform mono stack. ⚠️ **Substitution flagged:** preview cards in this design system use the same stack — no Google-Font fallback needed because none was ever specified.
- Type scale is Tailwind-standard (`2xs 10` / `xs 12` / `sm 14` / `base 16` / `lg 18` / `xl 20` / `2xl 24` / `3xl 30` / `4xl 36`).
- **UI default is `sm` (14px)** — labels and form controls. Stat values step up to `base`/`lg`/`2xl`. Only the login wordmark and the HeroStatsBar ROI use larger sizes.
- Weights: `medium 500` (body UI), `semibold 600` (headings), `bold 700` (stat values), `extrabold 800` (hero ROI, login wordmark).
- **UPPERCASE + `tracking-wider` (0.05em–0.08em letter spacing) is the signature** for stat labels, table column headers, micro-labels. Use sparingly — heavy doses feel dated.
- **Tabular numerals everywhere.** Every price, percentage, count, and cert number uses `font-variant-numeric: tabular-nums`.

**Spacing.**
- 4px base (`--space-1` = 0.25rem) through 64px (`--space-16`). Half-steps at 2/6/10/14px for dense table work.
- Card padding is `--space-5` (20px), card-to-card gap `--space-3-5` (14px). Header is a tight `py-2.5 px-6` (10×24).
- Layout max-width: 1920 for top chrome, 1280 (`max-w-6xl`) for page content.

**Backgrounds & textures.**
- The app itself has **no imagery** as background — no illustrations, no photos, no repeating patterns. Chrome is solid surfaces with the body-level fixed radial gradient peeking through.
- The login page is the one exception: two floating colored blurred "orbs" (400px and 300px, 80px blur, opacity 0.4) animated on an 8s float loop, behind a glass card. Brand-orb primary uses the brand gradient; secondary uses success green → cyan.
- **Pokémon-themed gradients** (`--gradient-charizard`, `--gradient-pikachu`, `--gradient-lugia`, `--gradient-mewtwo`, `--gradient-rayquaza`) exist as tokens for "premium card" moments but are used sparingly — mostly in recommendation badges and occasional accent strips. Don't fill whole sections with them.

**Animation.**
- Easing is `cubic-bezier(0.4, 0, 0.2, 1)` for everything — fast 150ms, base 200ms, slow 300ms. Speed is the default.
- Entries: `fadeIn`, `fadeInUp`, `scaleIn`, `bounceIn` (uses an overshoot easing `cubic-bezier(0.68, -0.55, 0.265, 1.55)`). All short (~250–400ms).
- **Glow pulses** (`pulse-glow`, `pulse-glow-blue`, `titleGlow`) — 2–3s ease-in-out infinite — are used on the login wordmark and occasionally around premium cards. They breathe; they don't bounce.
- **Ripple effect on button press** via a `useRipple` hook — white ripple on primary/success/danger/warning, dark ripple on secondary/ghost/outline. 600ms duration.
- **Pokéball loader** is the themed spinner for route transitions.
- All animations respect `prefers-reduced-motion: reduce` — explicitly handled in base.css.

**Hover states.**
- Buttons: `hover:-translate-y-0.5 hover:shadow-sm` + brightness boost on solid fills. Cursor is implicit.
- Links in UI chrome: `text-[var(--text-muted)] → text-[var(--text)]` plus a subtle `bg-[var(--surface-2)]/10` panel for active/hover.
- Interactive cards: lift (`-translate-y-0.5`), elevate shadow (`shadow-1 → shadow-2`), and swap bg to `--surface-hover`.
- Rows in glass tables: `bg: rgba(255,255,255,0.02)` on hover — almost invisible, just enough to track the pointer.

**Press states.**
- Buttons return to baseline + inner shadow: `translate-y-0` + reduced shadow. Primary keeps its fill; no darkening needed.
- Ripple fires on press and fades out during the release.

**Focus states.**
- `*:focus-visible { outline: 3px solid var(--color-focus); outline-offset: 3px; box-shadow: 0 0 0 4px var(--color-focus-ring); }` — a real 3px ring, offset from the element, over a translucent halo. Uses `--color-focus: #60a5fa` (lighter blue than primary, so it reads on the indigo brand fill too).

**Borders.**
- Default border is `var(--surface-3)` — a mid-elevation gray. On the darkest surfaces it's often hairline `rgba(255,255,255,0.06)` instead.
- Table rows use `border-bottom: 1px solid rgba(255,255,255,0.03)` — intentionally under-contrast. The rhythm of rows comes from spacing, not lines.
- P&L rows add a 2px `border-left` color strip: `--success` for positive, `--danger` for negative.

**Shadow + elevation system.**
- Three elevation levels: `--shadow-1` (card), `--shadow-2` (raised card, modal-adjacent), `--shadow-3` (modal). Shadows are soft and long on dark (y=20–40px, alpha up to 0.45).
- Glow variants for premium moments: `--glow-gold` (PSA 10), `--glow-brand` (selected/featured), `--glow-violet` (AI / insights surfaces), `--glow-psa9` / `--glow-psa10`.
- `inner shadow` is used on nav containers (`--shadow-nav-inset`) for a subtle carved look.

**Capsules vs protection gradients.**
- Semantic pills are **capsules**: `rounded-full`, `padding: 2px 8px` (sm) through `6px 12px` (lg), 10% alpha fill + 30% alpha border + full-color text. This is the `color-badge-*` family in tokens.
- Recommendation badges get the **protection gradient** treatment — 135° gradient from a darker to a lighter shade of the same hue, with a matching colored border and a colored drop shadow. `MUST BUY` through `AVOID`.
- `GradeBadge` is a capsule too, but with hue keyed to grader+grade.

**Layout rules.**
- **Sticky header** at `top-0 z-50`, `bg-[var(--surface-1)]/80 backdrop-blur-xl`, scrolls under content. Gains `shadow-md` after 10px scroll.
- Page content sits in `max-w-6xl mx-auto px-4`, with `py-8` on the `<main>` element.
- **Hero stats bar** is the signature dashboard layout: one oversize metric on the left (ROI, 32px extrabold), supporting stats in a flex-wrap row to the right, separated by a hairline bottom border. No cards — just typography arranged on the page.

**Transparency & blur.**
- **Glassmorphism** is the heavy lift on two surfaces: the header (`bg-[var(--surface-1)]/80 backdrop-blur-xl`) and the login card (`rgba(22,27,34,0.6)` + `blur(20px)`). A general `glass` variant exists on `CardShell` for feature surfaces.
- Backdrop blur scale: `xs 2` / `sm 4` / `md 12` / `lg 16` / `xl 24` / `2xl 40` / `3xl 64` (px).
- Don't stack glass-on-glass more than two levels — the whole point is the single floating plane.

**Imagery vibe.**
- Bright, saturated, pop-art. The one piece of in-app hero art — the login/favicon logo — is an AI-styled PSA-slabbed Pokemon card (Emolga, "GEM MINT 10") on a comic-book halftone yellow-and-blue burst background.
- The Card Yeti logo on the login page is icy-blue gradient lettering flanked by a stylized yeti head against a mountain range — cool tones, contrasts the warm logo.
- If new imagery is needed: saturated, product-true (real Pokemon cards in slabs), never generic stock photography. Ask the user before inventing.

**Corner radii.**
- `--radius-sm: 10px` (compact pills, inputs)
- `--radius-md: 14px` (most buttons, nav items, inline badges)
- `--radius-lg: 18px` (cards — the default)
- `--radius-xl: 22px` (login card, feature surfaces)
- `--radius-full: 9999px` (avatars, capsule pills, the Google sign-in button)

**Card anatomy.**
- `bg: --surface-1` + `border: 1px solid --surface-0` (darker than fill — receding) + `border-radius: --radius-lg` + `shadow: --shadow-1` + `padding: --space-5` (default).
- Elevated variant uses `--surface-2` + `--shadow-2`. Glass variant replaces fill with `--glass-bg`.
- Interactive variant adds `-translate-y-0.5` on hover + `cursor-pointer` + full focus ring. Always `tabIndex=0` and ARIA.
- Premium variant uses a brand-gradient border tint (`--brand-500/30`) and `hover:shadow-[--glow-brand]` instead of elevating.

---

## Iconography

**No custom icon font. No icon library is imported.** Every icon in the SlabLedger codebase is a hand-inlined SVG inside the component that uses it. Stroke style: `stroke-width: 2`, `stroke-linecap: round`, `stroke-linejoin: round`, `fill: none`, `currentColor`. 24×24 viewBox is the norm; rendered around 14–20px.

**Sources of the inlined SVGs:**
- Hamburger, caret, chevron, close-X, logout — traced from **Feather / Lucide** (stroke-round, 24×24, 2px weight).
- Google "G" — the multi-color official Google sign-in mark, inlined in `LoginPage.tsx`.

**This design system's icon strategy.**
- Load **Lucide** from CDN (`https://unpkg.com/lucide@latest`) for UI kit previews — it's the closest 1:1 match to the stroke style already inlined.
- Lift the Google brand mark verbatim when recreating the sign-in card.
- **PSA / grade labels are not icons** — they are `GradeBadge` capsules with typographic content (`PSA 10`, `BGS 10`). Don't try to draw a slab icon.

**Emoji usage.** Allowed only on the login feature row (`📊 💰 🔍`) and `EmptyState` illustrations. Not in tables, forms, headers, or navigation. When in doubt, don't.

**Unicode / typographic marks.** `→` for "navigate to" pills (`3 unpaid invoices →`), `%`, `$`, `×` are used as characters — no icon substitution.

**Logos in `assets/`:**
- `slabledger-card-logo.png` — the AI-illustrated PSA slab (primary app logo, used in the sticky header at 28×28 with `rounded-lg`, and as the favicon).
- `card-yeti-logo.png` — the Card Yeti wordmark (used on the login page at 180×120). Card Yeti is the operator brand behind SlabLedger; SlabLedger-the-app is the ledger tooling *for* the Card Yeti business.
- `favicon.ico` — generated from the same source as the card logo.

---

## Index

Root of this design system:

- `README.md` — you are here
- `SKILL.md` — agent-skill entry point (Claude Code compatible)
- `colors_and_type.css` — all CSS custom properties + base typography (ready to `@import`)
- `assets/` — logos + favicon (copy out; don't link directly)
- `preview/` — ~20 small HTML cards that populate the Design System tab (swatches, type specimens, components)
- `ui_kits/slabledger-web/` — React recreations of the dark dashboard chrome + key screens. `index.html` is the clickable prototype entry point; JSX components live alongside.

Referenced but not copied in:

- `guarzo/slabledger` GitHub repo — the source of truth. Most values here are direct lifts.
- `guarzo/slabledger-private` — private docs (not inspected).
