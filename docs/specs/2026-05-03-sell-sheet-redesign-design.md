# Sell Sheet Redesign — Slice-Based Print Menu

**Date:** 2026-05-03
**Status:** Draft (awaiting user review)

## Problem

The current sell sheet flow asks the user to select inventory items, "add" them to a sheet, then print one consolidated list. This does not match how the seller actually uses the sheet. The real use cases are:

1. **Card-show vendor handoff** — a printed sheet a vendor at a card show flips through to find what interests them. Needs to be browsable by lens (PSA 10s, modern, vintage, high-value).
2. **Local card store handoff** — a flat list sorted by grade.

In both cases, the seller wants to print **everything currently in hand** (i.e., received, unsold), not a curated subset. The "select / add to sheet" step is wasted work.

## Goals

- One global page that always operates on every received-but-unsold purchase across all campaigns.
- A menu of pre-defined print "slices," each with a count + total ask preview.
- Each slice opens a print-ready view using the existing print-row component.
- Remove the per-campaign sell-sheet flow and "add to sell sheet" UX entirely.

## Non-goals

- No change to the printed row layout (card name, set/number, grade, cert + barcode, CL value, blank "Agreed Price" column). The redesign is about *which rows appear in which sheet*, not row shape.
- No change to pricing/recommendation logic in `enrichSellSheetItem`.
- No change to which purchases qualify (still `received_at IS NOT NULL` AND not sold).
- No persisted "sell sheet" state. Slices are computed per render.

## Design

### Page

- Single global route: `/sell-sheet`.
- Per-campaign sell-sheet route(s) and the in-page `SellSheetView` toggle on campaign detail are removed.

### Slice menu (on screen)

```
Sell Sheet — All Inventory (147 in hand · $40,600 ask)

[Print]  PSA 10s                      — 23 · $14,200
[Print]  Modern (2020+)               — 41 · $8,900
[Print]  Vintage (pre-2020)           — 38 · $22,400
[Print]  High-Value ($1,000+)         — 12 · $18,500
[Print]  Under $1,000                 — 135 · $26,100
[Print]  By Grade (local card store)  — 147 · $40,600
[Print]  Full List                    — 147 · $40,600
```

Slices are **overlapping lenses**, not mutually exclusive buckets. A PSA 10 from 2022 worth $1,500 shows up in PSA 10s, Modern, and High-Value. The vendor flips to whichever section interests them.

### Slice definitions

| Slice | Filter | Sort |
|---|---|---|
| PSA 10s | `grader == "PSA"` AND `gradeValue == 10` | target sell price desc |
| Modern (2020+) | parsed year from `cardYear` ≥ 2020 | set name asc, card number asc |
| Vintage (pre-2020) | parsed year from `cardYear` < 2020 | set name asc, card number asc |
| High-Value ($1,000+) | `targetSellPrice ≥ 100000` (cents) | target desc |
| Under $1,000 | `targetSellPrice < 100000` (cents) | target desc |
| By Grade | all | gradeValue desc, target desc |
| Full List | all | set asc, card number asc |

**Year parsing:** `Purchase.CardYear` is a free-form string (e.g. `"1999"`, `"1999-2000"`). Parser extracts the leading 4-digit run with a regex `^(\d{4})`. If parsing fails, the item appears in **neither** Modern nor Vintage (it does still appear in Full List, By Grade, and the price/grade slices). Items with no parseable year are surfaced in a footer note: "N items have no year and were excluded from era slices."

### Print view

- Reuses `SellSheetPrintRow` unchanged (row layout: #, card, grade, cert+barcode, CL value, agreed price column).
- Slice page header: `Sell Sheet — <Slice Name> · <count> cards · $<total ask>`.
- Generated-at timestamp in the footer.
- Triggers `window.print()` on a `[Print]` click. Standard browser print dialog; user picks PDF or paper.

### Local card store mode

The "By Grade" slice IS the local-card-store handoff. No separate mode needed — same page, same UI, just a different slice button.

### Backend changes

- **Keep** `service.GenerateGlobalSellSheet(ctx)` — returns all received+unsold cross-campaign. This is the only sell-sheet endpoint going forward.
- **Remove**:
  - `service.GenerateSellSheet(ctx, campaignID, purchaseIDs)` (per-campaign, selection-driven)
  - `service.GenerateSelectedSellSheet(ctx, purchaseIDs)` (cross-campaign, selection-driven)
  - The corresponding handlers and routes in `internal/adapters/httpserver/`.
  - `sell_sheet_items` repository, table, and migration **if** they were only consumed by the old "add to sheet" UX (verify in plan phase — likely yes given file naming).

All slice computation happens **client-side** from the single global response. Backend stays dumb.

### Frontend changes

- New page: `web/src/react/pages/SellSheet.tsx` (or rework existing global sell sheet entry point if one exists).
- Replace `SellSheetView.tsx` content: remove "select / add / remove" UI, render slice menu and per-slice print view.
- Remove all "Add to sell sheet" buttons from inventory/aging tables.
- New helper: `web/src/react/utils/sellSheetSlices.ts` — pure functions that take `AgingItem[]` and return each slice's filtered+sorted list and totals. Heavily unit-tested.
- Keep: `SellSheetPrintRow.tsx`, `print-sell-sheet.css`, `MobileSellSheetRow.tsx` (mobile preview row).
- `useSellSheet.ts` is refit: drops selection state, fetches the global sheet once, exposes the slice helpers' output.

### Mobile

The slice menu works as-is on mobile (just a list of buttons). Printing on mobile is uncommon for this workflow but degrades to the browser's share/print sheet without special handling.

## Testing

- **Backend:** existing `GenerateGlobalSellSheet` tests stay. Tests for the removed methods are deleted.
- **Frontend (slice helpers):** table-driven tests for each slice — boundary cases (year exactly 2020, price exactly $1,000, missing CardYear, missing grade, non-PSA grader). One unit test file per slice or one consolidated `sellSheetSlices.test.ts`.
- **Frontend (page):** smoke test that the menu renders all 7 slices with non-zero counts when fixture data covers each.

## Migration / rollout

- Single PR. No data migration needed (no schema changes assuming `sell_sheet_items` removal is included).
- If `sell_sheet_items` table is dropped, ship the down migration alongside.

## Open questions resolved during brainstorm

- Categorization axis: overlapping lenses (PSA 10, era, price), not mutually exclusive — confirmed with user.
- Vintage cutoff: 2020 (per user) — modern card-show convention.
- High-value threshold: $1,000.
- Per-campaign sell sheets: removed entirely.
- Print row layout: unchanged.
- Cost basis / profit / signals on print: never were on the print row; remain off.
