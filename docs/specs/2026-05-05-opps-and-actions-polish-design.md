# Opportunities + Inventory Polish — Design

**Date:** 2026-05-05
**Branch:** `opps-and-actions-polish`
**Author:** Claude (with Thomas)

## Goal

Two surgical UI fixes:

1. **Opportunities (`/opportunities/psa-exchange`)** — kill horizontal scroll at typical laptop widths (1280–1440) by collapsing 13 columns into 6, folding low-decision-weight numerics into a composite "Signal" cell with hover popover for the rest.
2. **Inventory (`/inventory`)** — fix the awkward, half-empty Actions column by shrinking it to content width, dropping the divider, right-aligning, and lowercasing the header.

Both changes ship in a single PR off `opps-and-actions-polish`.

---

## Part 1 — Opportunities table column collapse

### Files in scope

- `web/src/react/pages/psa-exchange/OpportunitiesTable.tsx` — main change
- `web/src/react/pages/psa-exchange/SortableHeader.tsx` — no change expected
- `web/src/react/pages/psa-exchange/utils.ts` — no change expected (bucket helpers stay)

### Current state (problem)

13 columns at full width: Image, Cert, Description, Grade, PSA Value, Target, Comp, Edge, Days/sale, Vel/mo, Conf, Pop, Score. Tier-hidden at `lg`/`xl`, but at common laptop widths every column is visible and the table forces horizontal scroll. Worse: every numeric is presented at equal weight, so the eye has nowhere to land.

### Target column set (6 visible columns)

| # | Column | Content | Width strategy | Sort key |
|---|--------|---------|----------------|----------|
| 1 | Image | 36×48 thumbnail | fixed 48px | — |
| 2 | Card | Description (truncate) + Grade pill on the same line; cert chip + optional `PSA value < target` chip below | flex-1, `min-w-0`, max ~36rem | `description` |
| 3 | PSA Value | dollar | tabular-nums, right | `listPrice` |
| 4 | Target | dollar; below it: delta vs PSA Value (e.g. `−$1,469 (−13%)`) in muted text. Negative delta = Target lower than PSA Value (good buy); use neutral muted text, not green/red, to avoid duplicating the Edge bucket color signal in the next column | tabular-nums, right | `targetOffer` |
| 5 | Signal | Edge % as the loud line; below it a 3-icon rail: Days/sale dot · Vel/mo bar · Conf glyph; whole cell gets a hover popover with the full numbers + Comp + Pop | tabular-nums, right | `edgeAtOffer` |
| 6 | Score | small numeric, top-decile glow | tabular-nums, right | `score` |

The `lg:` / `xl:` `hidden` classes on column headers and cells go away — every breakpoint shows the same 6 columns.

### What gets folded where

- **Cert** → demoted to a `text-xs font-mono text-muted` chip inside the Card cell, below or beside the description. Selectable text so users can copy.
- **Grade** → moved inline with the description on the same line (existing `GradeBadge`, just relocated). Frees the standalone Grade column.
- **Comp** → into the Signal hover popover only.
- **Days/sale, Vel/mo, Conf** → into the Signal cell as a 3-icon micro-rail under the Edge number; full numbers in the hover popover.
- **Pop** → into the Signal hover popover only.

### Signal cell anatomy

```
33.3%                        ← Edge value, font-medium, bucket-color from edgeBucketClass
●  ▮▮▯  ✓                    ← Days dot · Vel/mo bar · Conf glyph (8–10px)
  └─ on hover: popover with ─┘
     Days/sale: <1d
     Velocity: 35 / mo
     Confidence: High (0.82)
     Comp: $13,100
     Pop: 12
```

- Days dot: existing `daysBucketClass` color → small filled circle.
- Vel/mo bar: 3-segment fill (1=low, 2=mid, 3=high) using existing `velocityBucketClass` thresholds.
- Conf glyph: ✓ / ~ / ? for high/mid/low using existing `confidenceColorClass`.
- Popover: native `<details>` + `<summary>` works, but we already use Radix elsewhere — use Radix `Popover` for keyboard + click-outside behavior. Trigger on hover **and** focus (a11y).

### Sortability

- Cert, Comp, Days/sale, Vel/mo, Conf, Pop lose dedicated sort headers.
- Default sort stays whatever it is today (Score desc presumed). No new sort UI in this spec.
- Worth-it follow-up (out of scope here): a "Sort by" dropdown above the table for the demoted keys. Punted.

### Group row behavior

Unchanged. The `2 listings · $X` expand row keeps `colSpan={COLUMN_COUNT}` — update `COLUMN_COUNT` to 6.

### Acceptance criteria

- At 1280px viewport the table fits without horizontal scroll. Verified with `make screenshots` desktop output.
- All Edge / Score / Days / Vel / Conf bucket coloring preserved.
- Cert is selectable in the Card cell (manual check: triple-click selects only the cert).
- Signal popover opens on hover and on keyboard focus; contains Days/sale, Vel/mo, Conf, Comp, Pop with full values.
- Group rows still expand and show member rows with the new column count.
- No regression on existing tests; existing `OpportunitiesTableSkeleton.test.tsx` and `utils.test.ts` should pass unchanged.

---

## Part 2 — Inventory Actions column tightening

### Files in scope

- `web/src/react/pages/campaign-detail/InventoryTab.tsx` — header row
- `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx` — Actions cell
- `web/src/react/pages/campaign-detail/inventory/RowActions.tsx` — no change expected
- `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx` — out of scope (mobile uses different layout)

### Current state (problem)

The Actions column is fixed at 220px, but its content (one ~70px primary button + 14px dot menu + 6px gap = ~90px). The remaining ~120px is dead space, and the column is decorated with a left vertical divider (`ml-2 pl-3 border-l border-white/[0.14]`) that creates a visual section without anything filling it. The header reads `ACTIONS` in caps while every other header is sentence-case.

### Changes

1. **Width:** drop `width: 220px` → `width: 144px`. Sized to fit the longest contextual-primary label (`Fix DH Match`, `Restore to DH`, `List on DH` — see `ACTION_LABELS` in `rowActions.ts`) at small `Button` padding plus the 14px dot menu plus gap. `Sell`-only rows will show the button + dot hugging the right edge with empty space on the left, which is fine — the column hugs the right of the row, so the empty space sits between Status and the button, not after it. Header cell width must match.
2. **Divider:** remove `ml-2 pl-3 border-l border-white/[0.14]` from both the header and row cells. Replace `ml-2` with a small `gap` between Status and Actions cells (existing flex layout already handles spacing).
3. **Alignment:** right-align contents. Change `RowActions` outer `<div className="flex items-center gap-1.5">` → `flex items-center justify-end gap-1.5` (cell-level), and right-align the header text.
4. **Header label:** `Actions` → `Actions` (already mixed-case in the source, but rendered uppercase in screenshot — confirm it's a CSS `text-transform`, not the literal). If the header style applies `uppercase`, leave as-is for typographic consistency with other headers; the visual oddness was the wide column, not the case.

   - Investigation note for the implementer: open `InventoryTab.tsx:175` and confirm the header CSS. If `glass-table-th` applies `text-transform: uppercase`, every header is uppercase and the screenshot was misleading me; in that case skip the case change. If only `Actions` was uppercase, lowercase to match.
5. **Hover-reveal of Sell (NOT in this spec):** discussed and deferred. Option A only.

### Acceptance criteria

- Actions cell + header are 104px wide with right-aligned contents.
- No vertical divider between Status and Actions.
- Header case matches the rest of the row (i.e., uses the same CSS as `Card`, `Status`, `P/L`, etc.).
- `print-hide-actions` class behavior preserved on both header and cell so print view still hides actions.
- `RowActions` Sell button + dot menu render unchanged in style — only their container alignment + width change.
- Mobile (`MobileCard.tsx`) unchanged.

---

## Out of scope (explicit)

- Hover-reveal of the inventory Sell button (Option B) — re-evaluate after Option A ships.
- Folding actions into the Status pill (Option C) — not now.
- `Sort by` dropdown for demoted opportunities columns.
- De-duping near-identical opportunities rows (`groupDuplicates` tuning) — separate bug.
- Mobile redesign for either page.
- Any change to the underlying API shapes / types.

---

## Risks

| Risk | Mitigation |
|------|------------|
| Hidden data (Comp, Pop, Vel/mo, Days/sale, Conf) becomes "invisible" to users who used to scan it | Hover/focus popover on Signal cell exposes all five. Document in a note in the row-detail panel if one exists. |
| Card cell becomes too dense with Description + Grade pill + Cert chip | Hold to single line for description (truncate); keep Grade pill compact (existing `GradeBadge size="sm"`); cert chip on its own line below in muted text. |
| 104px Actions column too tight if a future action label is long | All current actions in `ACTION_LABELS` fit in <70px; keep an eye on regressions when adding new actions. |
| Removing the column-tier `lg:`/`xl:` classes regresses small-viewport behavior | Test at 768px, 1024px, 1280px viewports via `make screenshots`. |

---

## Verification

- `npm run build` clean
- `npm test` (frontend) green
- `make screenshots` — diff `web/screenshots/inventory.png` and `web/screenshots/opportunities-psa-exchange.png` (or whatever the path is) to confirm visual outcome
- Manual: open both pages at 1280px, verify no horizontal scroll on opportunities and tighter Actions cell on inventory
- Manual: hover Signal cell on a sample row, verify popover content matches expectations
