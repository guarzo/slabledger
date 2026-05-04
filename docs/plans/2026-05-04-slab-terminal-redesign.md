# SlabLedger — Slab Terminal Redesign

**Date:** 2026-05-04
**Author:** Daisy + Claude
**Status:** ✅ **Complete** (Phases 1–3 shipped). Three sparkline placements deferred — see [docs/specs/2026-05-04-sparkline-api-requirements.md](../specs/2026-05-04-sparkline-api-requirements.md).

## Final shipping summary

| Phase | PR | What landed |
|---|---|---|
| 1 — Foundation | [#357](https://github.com/guarzo/slabledger/pull/357) | JetBrains Mono on every monetary cell, `.slab-frame` utility, inventory row-stripes removed, Sell button colour discipline, Opportunities chrome aligned, login Card Yeti mark legibility |
| 2.1 — Dashboard hero | [#358](https://github.com/guarzo/slabledger/pull/358) | Display-scale Fraunces ROI headline, "as of HH:MM" freshness line, captain's log NEXT MOVES with bronze topmost-row underscore |
| 2.2 — Inventory hero | [#359](https://github.com/guarzo/slabledger/pull/359) | Display-scale Fraunces unrealized headline, cost-vs-market diverging bar, slab-framed thumbnails (`.slab-frame-sm`) |
| 2.3 — Insights | [#360](https://github.com/guarzo/slabledger/pull/360) | Advice-first recommendation cards (slab-framed), derived-from-matrix fallback, tuning matrix collapsed by default |
| 3.1 — Input UX | [#361](https://github.com/guarzo/slabledger/pull/361) + [#362](https://github.com/guarzo/slabledger/pull/362) | Reprice slider differentiation (bronze/round vs amber/squarer), Scan onboarding-strip collapse, scanner-field cert input |
| 3.2 — Campaign surfaces | [#363](https://github.com/guarzo/slabledger/pull/363) | Slab-framed empty hero with tab-switching CTAs, Active/Pending/Closed phase grouping |
| Out-of-band | [#364](https://github.com/guarzo/slabledger/pull/364) | Opportunities responsive column visibility (operator friction call-out) |
| 3.3 — Preview polish | [#365](https://github.com/guarzo/slabledger/pull/365) | Sell Sheet preset previews (hero ask + 3-line top items), Admin integration "Last call X ago" pulses + DH error banner |

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

### Phase 1 — Foundation (system-level, no page redesigns) ✅ shipped (#357)

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

#### Dashboard ✅ shipped (#358)

- Promote `+21.0%` to a true display-scale headline (Fraunces, ~96px desktop, ~64px mobile).
- Demote the six adjacent KPIs to a single tabular strip beneath, in JetBrains Mono.
- Add an "as of HH:MM · since last login +X" line under the headline.
- "NEXT MOVES" reads like a captain's log: monospace counter (`01 02 03`) on the left, action label center, action chip right; hover reveals a one-sentence rationale; topmost item carries an animated 1px underscore.
- ⏸ ~~Weekly Review section gets a 90-day sparkline.~~ **Deferred — blocked on backend.** See sparkline spec.

#### Inventory ✅ shipped (#359)

- ⏸ ~~Slim 60-80px chart strip above the table — unrealized P&L over the last 90 days~~ + cost-vs-market diverging bar. **Chart strip deferred — blocked on backend.** Diverging bar shipped.
- Larger card thumbnails framed in slab borders.
- ⏭ ~~Action column compresses to one Sell + a kebab~~ — already done in friction-log iter 21; verified during review.
- ⏭ ~~Expanded reprice panel restructures…~~ — already disciplined in friction-log iter 21; no change warranted.

#### Insights ✅ shipped (#360)

- Lead with 3 large recommendation cards ("Tighten Modern grade range to 9+ — sell-through 79% on a $5K cap is the bottleneck"), each with a one-click action.
- Demote the 5-column tuning matrix to a "All campaigns" expandable below.
- Page becomes advice-first instead of checkup-first.

### Phase 3 — Long-tail polish

| Surface | Move | PR |
|---|---|---|
| Campaign-detail empty | Replace bare "Awaiting first sale" wedge with a slab-framed placeholder showing the next two recommended actions | ✅ #363 |
| Reprice | Visually differentiate the two sliders (with-comps vs without-comps) — same shape, very different semantics today | ✅ #361 |
| Scan | Collapse the 4-step strip after the first successful scan; style cert input as a real scanner field ~~with optional scan-line during inflight~~ | ✅ #361 (scan-line deferred) |
| Sell Sheet | Each preset shows total ask as the hero number on its tile, with count + range below and a 3-line mini-list of top items | ✅ #365 |
| Admin / Integrations | Per-integration pulse indicator ("last call N seconds ago") ~~+ inline Test Connection~~; promote DH error to page-level banner when present | ✅ #365 (Test Connection deferred — many integrations lack a test-only endpoint, would be net-new feature scope) |
| Campaigns | Group rows by phase (Active / Pending / Closed); ⏸ ~~per-row 7-day P&L sparkline~~ | ✅ #363 (phase grouping); sparkline deferred — blocked on backend |

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

- Friction log: `web/screenshots/friction-log.md` — appended per-cycle as we landed changes; closing entry (iteration 23) recorded after Phase 1 ship.
- Screenshot regen: `make screenshots-quick` after each phase; final cross-PR regression sweep ran during closeout.

## Open questions — resolved

- ~~Should the slab-frame carry a faux serial number ("SLR-001 · Q-205X") on hero placements?~~ **No — never adopted.** The dual-hairline treatment alone read as the slab geometry once placed; adding faux-serial would have tipped into kitsch.
- ~~Sparkline library: install `recharts` (heavy) vs hand-rolled SVG (light).~~ **N/A — no sparklines shipped.** All three placements (Weekly Review, Inventory 90d, Campaigns 7d) deferred on backend dependency. The one chart we did ship (Inventory cost-vs-market diverging bar) was hand-rolled SVG. Library decision deferred along with the sparklines themselves.

## Deferred — blocked on backend

Three sparkline placements never landed because the API doesn't currently expose a daily-snapshot series per campaign or per portfolio. The frontend work for each is straightforward once the API exists. See [docs/specs/2026-05-04-sparkline-api-requirements.md](../specs/2026-05-04-sparkline-api-requirements.md) for the full breakdown:

- Weekly Review sparkline (Dashboard) — needs daily P&L over the current week
- Inventory 90-day P&L chart strip — needs daily portfolio snapshots
- Per-campaign 7-day sparkline (Campaigns table) — needs daily P&L per campaign

All three were on the original plan and are documented in their respective phase PRs as "skipped — needs backend addition."
