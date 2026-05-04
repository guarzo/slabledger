# SlabLedger — Slab Terminal Redesign

**Date:** 2026-05-04
**Branch:** `ui/slab-terminal`
**Author:** Daisy + Claude
**Status:** Phase 1 in progress

---

## Premise

The functional UX is in good shape. 22 friction-log iterations have resolved every 🔴 finding — empty-state copy, color discipline, mobile parity, banner truthfulness, drill-in chips, danger-variant restraint. The remaining gap is **identity**: the product reads as competent generic dark-mode SaaS rather than something unmistakably SlabLedger.

The premise of the product (a Bloomberg-style terminal for a graded-card portfolio business with real money on the line every day, second brand: Card Yeti, third brand: the slab itself) gives us a strong, distinctive direction that the current chrome doesn't lean into. `tokens.css` already has 500+ lines of design intent (Pokémon gradients, AI accents, slab-grade ramps, rec-badge gradients) — almost none of which surfaces in the screenshots.

## Aesthetic direction

**"The Slab Terminal"** — Bloomberg discipline meets the physical card slab. Quiet authority for the operator running real capital, with one or two restrained moments of card-collector character so it doesn't feel like a bank screen.

### Three principles

1. **Money is the product — type it like the product.** Every dollar / percent / serial is in **JetBrains Mono** with `font-variant-numeric: tabular-nums`. Hero figures stay in Fraunces (already loaded). Body text stays `ui-sans-serif`. That single move differentiates ~80% of the screens because most pixels on screen *are* numbers.
2. **The slab is the container.** A subtle thin/double-line border treatment (echoing the PSA holder's flange) becomes the canonical "this number matters" frame — applied to hero metrics, the empty campaign-detail hero, and the expanded inventory reprice panel. Currently every container is the same softly-rounded dark card; visual hierarchy is doing all its work through size alone.
3. **Restrained character, one surprise per page.** PSA 10 chips already shimmer faintly — push that. CL/MM price lines pulse on refresh. The dashboard ROI tickers in once on mount. Everything else stays still. The friction log shows the team is disciplined about not over-coloring; the same discipline applies to motion.

### Color discipline

Current state: success-green is doing four jobs (ROI, recovery, sell CTA, status dot), brand-indigo is doing three (active nav pill, primary buttons, focus ring), and they collide visually.

Target:

- `--success` (emerald) → reserved for **realized** money: net-positive P&L, completed sales.
- `--brand` (indigo) → primary brand and interactive accent: active nav, primary CTAs, focus.
- `--warning` (amber) → intent-to-cover, partial-progress invoices.
- `--danger` (red) → genuinely below-cost, hard errors.
- Status dot stays green only when *all sources are healthy*. Sell CTA becomes brand-indigo or neutral-with-ring (separating it from "+P&L" emerald).

## Plan

### Phase 1 — Foundation (system-level, no page redesigns)

Cheapest, highest leverage. Each of these is a one-PR-or-less change.

| # | Change | Files |
|---|---|---|
| 1.1 | Load JetBrains Mono via Google Fonts; add `--font-numeric` token | `web/src/index.html`, `web/src/css/tokens.css` |
| 1.2 | Sweep monetary cells onto the new numeric font (`tabular-nums`, mono) | Dashboard hero, Campaigns table, Inventory table, Reprice triplets, Insights metrics |
| 1.3 | Define `.slab-frame` utility (thin double-line border, optional inset corner ticks) and apply to dashboard ROI hero, campaign-detail empty hero, expanded reprice panel | `web/src/css/tokens.css` (or new `slab.css`) + 3 component touchpoints |
| 1.4 | Drop the 2px solid coloured left-border on every inventory row (`base.css:174-182`); keep the below-cost row tint subtle, keep the P&L cell as the single signal-carrier | `web/src/css/base.css` |
| 1.5 | Resolve emerald/indigo overlap — pick the canonical green/indigo placements; reassign nav-active, sell CTA, status dot | Multiple components; mostly className swaps |
| 1.6 | Align Opportunities (`PsaExchangePage`) chrome with the rest of the app — page padding, breadcrumb spacing, error-card treatment | `web/src/react/pages/PsaExchangePage.tsx` |
| 1.7 | Brighten login wordmark + `POWERED BY · Card Yeti` mark (already 🟢 in friction log) | `web/src/react/pages/LoginPage.tsx` + `LoginPage.css` |

**Acceptance:** all 14 desktop + 14 mobile screenshots regenerate. Money in tabular mono. Three slab-framed surfaces visible. Inventory row indicator is gone. Opportunities page reads as part of the same product. Friction-log convention table updated.

### Phase 2 — Hero pages (opinionated redesigns)

The three pages that do 80% of daily work.

#### Dashboard

- Promote `+21.0%` to a true display-scale headline (Fraunces, ~96px desktop, ~64px mobile).
- Demote the six adjacent KPIs to a single tabular strip beneath, in JetBrains Mono.
- Add an "as of HH:MM · since last login +X" line under the headline.
- "NEXT MOVES" reads like a captain's log: monospace counter (`01 02 03`) on the left, action label center, action chip right; hover reveals a one-sentence rationale; topmost item carries an animated 1px underscore.
- Weekly Review section gets a 90-day sparkline.

#### Inventory

- Slim 60-80px chart strip above the table — unrealized P&L over the last 90 days + cost-vs-market diverging bar.
- Larger card thumbnails framed in slab borders.
- Action column compresses to one Sell + a kebab on desktop (recovers ~80px for card name on mid-width screens).
- Expanded reprice panel restructures to a single tabular row of four large mono numbers (Cost · CL · MM · Sug) with the input + Confirm pinned right.

#### Insights

- Lead with 3 large recommendation cards ("Tighten Modern grade range to 9+ — sell-through 79% on a $5K cap is the bottleneck"), each with a one-click action.
- Demote the 5-column tuning matrix to a "All campaigns" expandable below.
- Page becomes advice-first instead of checkup-first.

### Phase 3 — Long-tail polish

| Surface | Move |
|---|---|
| Campaign-detail empty | Replace bare "Awaiting first sale" wedge with a slab-framed placeholder showing the next two recommended actions |
| Reprice | Visually differentiate the two sliders (with-comps vs without-comps) — same shape, very different semantics today |
| Scan | Collapse the 4-step strip after the first successful scan; style cert input as a real scanner field with optional scan-line during inflight |
| Sell Sheet | Each preset shows total ask as the hero number on its tile, with count + range below and a 3-line mini-list of top items |
| Admin / Integrations | Per-integration pulse indicator ("last call N seconds ago") + inline Test Connection; promote DH error to page-level banner when present |
| Campaigns | Group rows by phase (Active / Pending / Closed); per-row 7-day P&L sparkline |

## What we're explicitly skipping

- **Don't relayout the navigation.** It works.
- **Don't touch friction-log conventions.** Empty-state branching, semantic state tokens, banner-must-agree-with-table, neutral-zero-activity coloring — earned and load-bearing.
- **Don't introduce illustrated/playful brand language.** This is a money tool. Character comes from typography + slab geometry + restraint, not mascots.
- **Don't ship a light theme.** Out of scope.

## Risks

- **Numeric font load**: JetBrains Mono adds another Google Fonts request. Mitigated by `display=swap` and only loading the regular + medium + semibold weights.
- **Colour reassignment regressions**: `--success` is referenced widely. Phase 1.5 is the highest-risk Phase-1 step; do it last in Phase 1, run the test suite + regenerate screenshots, eyeball every page.
- **Slab-frame visual noise**: if applied broadly, the double-line treatment becomes wallpaper. Stay disciplined: three placements maximum in Phase 1; one more in Phase 3 (campaign-detail empty).

## Tracking

- Friction log: `web/screenshots/friction-log.md` — append per-cycle as we land changes.
- Screenshot regen: `make screenshots` after each phase.
- Worktree: `.worktrees/slab-terminal` on branch `ui/slab-terminal`.

## Open questions

- Should the slab-frame carry a faux serial number ("SLR-001 · Q-205X") on hero placements? Decorative but on-character. Default: no for Phase 1, revisit in Phase 3.
- Sparkline library: install `recharts` (heavy) vs hand-rolled SVG (light). Default: hand-rolled until we need >2 chart shapes.
