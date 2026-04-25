# UI Design Review — 2026-04-25

Review of `web/screenshots/` (14 desktop + 12 mobile) against the SlabLedger design
system (`web/src/css/tokens.css`, `web/src/css/base.css`, `.claude/skills/slabledger-design/README.md`).

The visual language is mostly honored — dark surfaces, tabular-num, UPPERCASE micro-labels,
semantic color — but the app reads quiet to the point of flat on the signature surfaces.
The Dashboard in particular does not feel like the "signature HeroStatsBar" the design
system describes.

## Findings

### Tier 1 — Highest impact

1. **Dashboard hero is under-weighted.** ROI (`26.4%`) is sized about the same as the
   dollar stats next to it. Spec calls for one oversize ROI (32–36px extrabold) on the
   left, supporting stats smaller. Right now ROI/Deployed/Recovered/At-Risk read as six
   equal siblings. Labels and values collide visually (`DEPLOYED $207,877.33` runs
   together). The `3 unpaid invoices →` pill is the best element on the page — give it a
   right-aligned action slot. *Files: `web/src/react/components/portfolio/HeroStatsBar.tsx`,
   `web/src/react/pages/DashboardPage.tsx`.*

2. **Campaigns progress bar is louder than the data.** Full-width yellow `Capital
   recovered 65%` bar steals focus from the table. Yellow also collides semantically
   with `--warning` ("needs attention") used elsewhere. Fix: thin 4px bar under a single
   stat line, or use `--brand-500`/`--gradient-lugia`. Reserve yellow for flags.
   *Files: `web/src/react/pages/CampaignsPage.tsx`.*

3. **Inventory row has nine actions; primary is lost.** Desktop row crams `PSA 10`,
   price chips, market badge, Set Price, Fix/Fix DH/Remove DH/Dismiss, Sell, trash. The
   eye can't find the primary action. Mobile already collapses correctly — bring that
   pattern to desktop: one primary `Sell` button pinned right, secondary actions in a
   `⋯` overflow menu. *Files: `web/src/react/pages/inventory/*` (row component).*

4. **Campaign Detail stat cards look identical but aren't.** Nine KPI cards in a 3×3
   grid all carry equal visual weight. Several are zero-state placeholders. Fix: tier
   the cards — primary 2× cards for `Total Spent` + `Net Profit`; secondary single-width
   for `Revenue/ROI/Cards Purchased/Sell-Through`; tertiary inline label/value row for
   `Sold/Unsold/Avg Days/Daily Cap`. Use `EmptyState` when all sales-related rows are zero.
   *Files: `web/src/react/pages/campaign-detail/OverviewTab.tsx` (or sibling), `web/src/react/ui/StatCard.tsx`.*

5. **Dashboard "Top Performers" is doing nothing visually.** Flat list of names + tiny
   chips + flat numbers. Convert each row into `[Rank #] [Campaign] [Profit $] [ROI %
   with TrendArrow]` using existing `TrendArrow` + `MarginBadge` primitives.
   *Files: `web/src/react/pages/DashboardPage.tsx`.*

### Tier 2 — Design-system drift

6. **Inconsistent card treatment across Admin tabs.** Stats / Pricing / Integrations
   feel like three different apps (different borders, padding, radii). Pick one
   `CardShell` variant and apply uniformly. *Files: `web/src/react/pages/admin/*`.*

7. **Login dual-logo collision.** Two brand marks stacked vertically (`SlabLedger`
   indigo wordmark + `CARD YETI` ice-blue logo) confuses first-time users. Fix:
   `SlabLedger` primary with `titleGlow`; Card Yeti as a small "powered by" wordmark at
   the bottom of the card. *Files: `web/src/react/pages/LoginPage.tsx`,
   `web/src/css/LoginPage.css`.*

8. ~~Scan page empty space.~~ **Skipped** — Scan page is intentionally roomy; in active
   use it fills with 20–30 pending rows. Don't redesign for the empty state. (See follow-up
   below for in-session improvements that make sense.)

9. **Reprice sliders disconnected from stats they affect.** `With comps`/`Without
   comps` sliders sit above raw stat cards with no visual relationship. Move
   `Current Value/Suggested Value/Below Cost` next to the sliders with a shared card
   background or connecting line. Make `Accept All` bar sticky once anything is selected.
   *Files: `web/src/react/pages/RepricePage.tsx`.*

10. **Insights urgency table has no hierarchy.** `high/high/—/low/Action` rows look
    identical; user has to read each one. Sort by urgency, apply a colored `border-left`
    strip per row using the recommendation-tier palette. Reuse `RecommendationBadge`.
    *Files: `web/src/react/pages/InsightsPage.tsx`.*

### Tier 3 — Polish

11. **Mobile nav is unlabeled / lacks breadcrumbs.** Header is just `🍔 • 🟢 D⌄`. Extend
    the breadcrumb pattern from Campaign Detail to other deep routes; add a `MENU` micro-label
    under the hamburger. *Files: `web/src/react/components/Header.tsx`,
    `web/src/react/components/Navigation.tsx`.*

12. **Empty-state copy is inconsistent.** Three different punctuation styles:
    `No pending items - all PSA imports matched or resolved.` vs.
    `Nothing needs your attention right now.` vs.
    `All clear — no open price flags.`
    Pick the em-dash variant and unify. *Files: anywhere `EmptyState` is invoked plus
    a few inline empties.*

13. **Filter-chip pattern is the best affordance in the app — propagate it.** The
    `Needs Attention 2 / Pending DH Listing 2` chips on Inventory should appear as a
    summary strip on Dashboard (`[3 unpaid invoices] [2 needs attention] [5 pending
    listings]`) and replace the `Active only` toggle on Campaigns
    (`[10 active] [0 paused]`).

14. **Tabular-num is not fully enforced.** Dashboard's `$109,989.13` and the
    `$125,180.78 pending` line below appear to use slightly different glyph widths.
    Audit every `$`-prefixed span for `font-variant-numeric: tabular-nums` (or
    Tailwind's `tabular-nums` utility).

### Scan follow-up (not a redesign — verify these in active use)

- Cert input must stick to top while pending list scrolls.
- Newly resolved row should `fadeInUp` (200ms) with a brief green border flash so user
  can confirm without looking up.
- Section header should show running count: `Pending Items · 17`.
- Bulk-resolve / Reconcile must scale to 30+ unmatched.

## Working well — preserve

- Mobile Dashboard / Campaigns layouts (KPI grid + green P&L) are exactly on-brand.
- `GradeBadge` colors. Don't touch.
- Per-row recommendation-tier left-edge strip on Campaigns (quietly excellent).
- Scan input focus ring.
- Glass header + backdrop blur.

---

# Parallel Work Prompts

Each prompt is self-contained and droppable into a fresh agent session. Tracks A–I
touch disjoint files and can run fully in parallel. Track J is cleanup — land it after
A–I to avoid merge conflicts.

Branch off clean `main` (per `feedback_branch_from_main.md`), one branch per prompt.

---

## Track A — Dashboard surface

> Rebuild the Dashboard so it reads like the "signature HeroStatsBar" described in
> `.claude/skills/slabledger-design/README.md` and surfaces actionable summary chips
> + a real Top Performers module.
>
> Scope (one PR, one branch):
>
> 1. **HeroStatsBar weight rebalance** (`web/src/react/components/portfolio/HeroStatsBar.tsx`).
>    Make ROI the dominant metric: `text-4xl font-extrabold` (36px), `tabular-nums`,
>    colored by sign (`--success`/`--danger`). Demote Deployed / Recovered / At Risk
>    / Wks to Cover / Outstanding / 30d Recovery to `text-lg font-bold` with
>    `text-xs uppercase tracking-wider text-[var(--text-muted)]` labels above each
>    value. Add a hairline divider (`border-l border-white/5`) between the ROI block
>    and the supporting-stats cluster. Stats cluster should `flex flex-wrap gap-x-8 gap-y-3`.
>
> 2. **Action chip slot.** Right-align an "alerts strip" on the same row as the hero:
>    `[3 unpaid invoices →] [N needs attention →] [N pending listings →]`. Each chip
>    is the same capsule pattern already used inline on Dashboard
>    (`rounded-full`, `--warning-bg` background, `--warning-border` border, arrow
>    suffix). Hide individual chips when their count is 0. Each chip routes to the
>    relevant filtered page.
>
> 3. **Top Performers refactor** (`web/src/react/pages/DashboardPage.tsx`, search for
>    "Top Performers"). Each row becomes a 4-column grid:
>    `[#1 rank in muted text] [Campaign name semibold] [Profit $ tabular-nums right-aligned colored] [ROI % with <TrendArrow direction=… /> right-aligned tabular-nums]`.
>    Use existing `TrendArrow` + `MarginBadge` from `web/src/react/ui/`. Limit to top 5;
>    add a `View all →` link to /campaigns sorted by profit desc.
>
> Constraints: do NOT change the Weekly Review table on the left; do NOT modify
> `StatCard.tsx` (Track D owns that). Tabular-num audit is Track J — just match
> existing usage on the page. Verify in browser at `npm run dev` desktop + iPhone-14
> viewport before opening PR.

---

## Track B — Campaigns surface

> Fix two things on `web/src/react/pages/CampaignsPage.tsx`:
>
> 1. **Capital-recovered bar.** Currently a full-width yellow bar that dominates the
>    page and color-collides with `--warning`. Replace with a thin (4px) progress strip
>    using `--brand-500` (or the `--gradient-lugia` token if you want a richer fill),
>    sitting directly under a single stat line:
>    `$148,759.14 of $227,181.37 recovered (65%)`. Stat line uses
>    `text-sm tabular-nums text-[var(--text-muted)]` with the percentage in
>    `text-[var(--text)]`. Reserve yellow for actual warnings.
>
> 2. **Filter chips replace `Active only` toggle.** Replace the toggle with a chip
>    strip following the same pattern as Inventory's Needs Attention / Pending DH
>    Listing chips: `[10 active] [N paused] [N archived]`. Selected chip uses
>    `--brand-500` ring + bold text; unselected uses muted. Chips are mutually
>    exclusive (radio-group ARIA semantics).
>
> Constraints: do NOT touch row-level rendering or the per-campaign left-edge color
> strip — those are working. Verify on desktop + mobile screenshots.

---

## Track C — Inventory rows

> Collapse the Inventory desktop row's nine action chips into one primary + an overflow
> menu, matching the cleaner pattern already shipped on mobile.
>
> Files: `web/src/react/pages/inventory/*` — find the row component (likely
> `InventoryRow.tsx` or rendered inline in the page; mobile equivalent already does
> the collapse correctly, lift its layout up).
>
> 1. **Primary action pinned right:** `<Button variant="primary" size="sm">Sell</Button>`.
>    Same affordance regardless of card state. Disabled with tooltip when the row
>    isn't sellable (no price set, no DH listing if required).
>
> 2. **Overflow menu (`⋯` icon button):** `Fix`, `Fix DH`, `Remove DH`, `Set Price`,
>    `Dismiss`, `Delete`. Use a Radix Popover or DropdownMenu — match whatever the
>    project already uses. Items are conditionally rendered based on row state
>    (don't show `Fix DH` if already DH-listed, etc.).
>
> 3. **Keep the small read-only chips** (`PSA 10`, market trend `Falling -9%`, sell
>    price, P&L) — those are signal, not actions. Just make sure they don't visually
>    blur into the action area; add a `gap-3` separator or vertical divider between
>    the signal chips and the `Sell + ⋯` cluster.
>
> Constraints: filter chips at the top of the page (`Needs Attention`, `Pending DH
> Listing`, etc.) are reference-quality — do NOT change. Mobile already works — verify
> regression with `web/screenshots/mobile/inventory.png`. Test with at least one row
> in each filter state.

---

## Track D — Campaign Detail tiered stat cards

> Replace the flat 3×3 grid of identical stat cards with a tiered hierarchy that
> reflects which numbers are summary vs. detail vs. derived.
>
> Files: `web/src/react/pages/campaign-detail/OverviewTab.tsx` (or sibling that owns
> the stat grid); `web/src/react/ui/StatCard.tsx` (extend variants).
>
> 1. **Extend `StatCard`** with a `size` prop: `'lg' | 'md' | 'sm'`. `lg` is 2× column
>    width with `text-3xl` value; `md` is single-width with `text-xl` value (current
>    default); `sm` renders as an inline label/value pair without card chrome (just
>    `flex gap-2 items-baseline` with uppercase micro-label and tabular-num value).
>
> 2. **Restructure the Overview tab grid:**
>    - Row 1 (2 cards, `lg`, full width split): `Total Spent`, `Net Profit`.
>    - Row 2 (4 cards, `md`): `Revenue`, `ROI`, `Cards Purchased`, `Sell-Through`.
>    - Row 3 (inline, `sm`): `Sold · 0    Unsold · 2    Avg Days to Sell · —    Daily Cap · $5,000.00`.
>
> 3. **Empty-state collapse.** When `Total Spent > 0` but Revenue / ROI / Sell-Through
>    are all zero, replace Row 2 with a single `<EmptyState>` card: title `No sales
>    yet`, body `Record your first sale to see P&L for this campaign.`, optional
>    `steps` array.
>
> Constraints: do NOT modify the Daily Spend table below — that's working. Other Track
> consumers of `StatCard` (Dashboard, Admin) must remain unbroken — the new `size`
> prop must default to current `md` behavior. Add unit tests for the size variants.

---

## Track E — Admin card consistency

> Three Admin tabs (Stats / Pricing / Integrations) currently render with three
> different card treatments. Unify them.
>
> Files: `web/src/react/pages/admin/*Tab.tsx` (StatsTab, PricingTab, IntegrationsTab,
> UsersTab).
>
> 1. Replace any hand-rolled `<div className="...">` wrappers used as cards with
>    `<CardShell variant="default">` (`web/src/react/ui/CardShell.tsx`).
> 2. Pull section titles into `<Section title="…">` if the project has that primitive,
>    or align hand-rolled headers to `text-sm font-semibold uppercase tracking-wider
>    text-[var(--text-muted)] mb-3`.
> 3. Healthy/error chips on Integrations (`• Healthy`, `• Not connected`) should use
>    `<StatusPill>` (`web/src/react/ui/StatusPill.tsx`) instead of inline color spans.
>
> Constraints: this is a refactor, not a redesign — no new fields, no copy changes
> other than what's needed to fit the primitives. Take before/after screenshots of
> all four tabs and put them in the PR.

---

## Track F — Reprice slider/stat coupling

> Visually tie sliders to the stats they affect.
>
> Files: `web/src/react/pages/RepricePage.tsx`, possibly `web/src/react/ui/DualRangeSlider.tsx`.
>
> 1. **Group the two sliders + the three responsive stats** (`Current Value`,
>    `Suggested Value`, `Below Cost`) inside a single `<CardShell variant="elevated">`.
>    Sliders on the left, stats on the right at lg breakpoint; stacked at sm. Title:
>    `Reprice Preview`.
> 2. **`Total Cards`, `With Comps`, `Without Comps`, `No Data`** stay as outer stat
>    cards above the preview block — they're scope counts, not slider-affected.
> 3. **Sticky action footer.** When `selected.length > 0`, render a sticky bottom bar
>    `Selected: N · [Accept All] [Deselect All]` with `position: sticky; bottom: 0;
>    backdrop-blur-xl; bg-[var(--surface-1)]/80`.
>
> Constraints: don't change the per-row interaction or the cert/price columns in the
> table below. Test on mobile — sticky footer must not occlude the last row.

---

## Track G — Insights urgency hierarchy

> Add visual urgency cues to the Campaign Tuning table on Insights.
>
> Files: `web/src/react/pages/InsightsPage.tsx`.
>
> 1. **Sort rows** by urgency (rows with `Action` status > rows with `OK` >
>    everything else). Existing column order stays.
> 2. **Left-edge color strip** per row: 2px `border-left` colored by status —
>    `--danger` for `Action`, `--success` for `OK`, `--warning` for partial signals,
>    `--surface-3` for OK-with-data.
> 3. **Reuse `RecommendationBadge`** (`web/src/react/ui/RecommendationBadge.tsx`) for
>    the `high`/`low` value chips — currently they appear to be plain text. The badge
>    handles tier-appropriate gradient + color tokens out of the box.
>
> Constraints: don't change `Health Signals` cards above the table. Don't change the
> empty-state ("Nothing needs your attention right now.") — Track J unifies copy.

---

## Track H — Login brand hierarchy

> Resolve the SlabLedger / Card Yeti dual-brand visual collision.
>
> Files: `web/src/react/pages/LoginPage.tsx`, `web/src/css/LoginPage.css`.
>
> 1. **SlabLedger wordmark stays primary** — keep the `titleGlow` animation, keep
>    indigo gradient. Increase its visual dominance by adding more `mb` between it
>    and Card Yeti.
> 2. **Card Yeti demoted** to a "powered by" treatment at the bottom of the auth card
>    (below the feature row): centered, ~50% opacity, max-height 40px, label
>    `Powered by` in `text-2xs uppercase tracking-wider text-[var(--text-muted)]`
>    above the wordmark.
> 3. The two floating colored orbs animation stays — that's the signature.
>
> Constraints: don't touch the Google sign-in button (correct as-is). Verify on
> mobile screenshot — the card already feels tight, so make sure the demoted Card
> Yeti doesn't push the button below the fold.

---

## Track I — Mobile chrome (breadcrumbs + nav labels)

> Files: `web/src/react/components/Header.tsx`, `web/src/react/components/Navigation.tsx`,
> `web/src/react/components/Breadcrumb.tsx` (`web/src/react/ui/Breadcrumb.tsx`).
>
> 1. **Hamburger micro-label.** Add a `MENU` micro-label below the hamburger icon at
>    mobile breakpoints: `text-2xs uppercase tracking-wider text-[var(--text-muted)]`.
>    Hidden at `md+`.
> 2. **Extend breadcrumbs to deep routes.** Currently only `Campaign Detail` shows the
>    `Campaigns / X` breadcrumb. Add the same pattern to: Reprice (`Inventory /
>    Reprice`), Admin sub-tabs (`Admin / Pricing` etc.), Inventory (`Inventory`).
>    Use the existing `Breadcrumb` primitive — don't roll a new one.
> 3. **Mobile menu drawer styling check.** Verify the drawer (when open) matches the
>    `glass` CardShell variant tokens.
>
> Constraints: this Track is the only one that touches `Header.tsx` and
> `Navigation.tsx` — any other Track that needs header changes must coordinate with
> this branch (rebase order). Verify no regression on every desktop screenshot's
> sticky header behavior.

---

## Track J — Cross-cutting cleanup (land last)

> **Run after Tracks A–I have merged.** This Track touches files across all surfaces;
> running it concurrently will produce conflicts.
>
> 1. **Empty-state copy unification.** Search the codebase for inline empty-state
>    strings:
>    ```
>    rg "No .* found|Nothing .* right now|All clear|all .* matched or resolved" web/src
>    ```
>    Standardize on the em-dash form: `<state> — <next-action-hint>.`. Examples:
>    - `No pending items — all PSA imports matched or resolved.`
>    - `Nothing needs attention — your campaigns are healthy.`
>    - `All clear — no open price flags.`
>    Where the inline string is inside a `<p>` rather than `<EmptyState>`, prefer
>    converting to `<EmptyState>` if the surrounding context allows. Don't invent new
>    copy; reword existing strings only.
>
> 2. **Tabular-num audit.** Find every `$`/`%` rendering site that doesn't already
>    have `tabular-nums`:
>    ```
>    rg -n '\$\{' web/src/react | grep -v 'tabular-nums\|formatCents\|formatPct'
>    ```
>    Add Tailwind's `tabular-nums` utility (or `font-variant-numeric: tabular-nums` in
>    the relevant CSS module) at every `$`-prefixed and `%`-suffixed span. Prefer
>    fixing at the formatter helper level (`formatCents`, `formatPct` in
>    `web/src/js/format.ts` or wherever) so it propagates everywhere — but verify the
>    helper output is wrapped in something that can take the class.
>
> Constraints: this is mechanical. No layout changes. Open as a single PR with a
> focused diff so reviewers can scan it in one pass.

---

## Coordination

| Track | Touches | Conflicts with |
|-------|---------|----------------|
| A — Dashboard | DashboardPage, HeroStatsBar | none |
| B — Campaigns | CampaignsPage | none |
| C — Inventory | inventory row component | none |
| D — Campaign Detail | OverviewTab, **StatCard.tsx** | A (if A reuses StatCard — it shouldn't) |
| E — Admin | admin/*Tab | none |
| F — Reprice | RepricePage | none |
| G — Insights | InsightsPage | none |
| H — Login | LoginPage, LoginPage.css | none |
| I — Mobile chrome | **Header.tsx, Navigation.tsx** | any Track touching header (none planned) |
| J — Cleanup | cross-cutting | runs **after** A–I |

Recommended merge order: A–I in any order, then J.
