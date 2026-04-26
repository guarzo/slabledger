# Sell Sheet Print Redesign

**Date:** 2026-04-26
**Branch:** `sell-sheet-print`
**Scope:** Print-only changes to the campaign Sell Sheet view. No on-screen changes, no backend changes.

---

## Problem

The current printed sell sheet (sample: `/workspace/tmp/SlabLedger.pdf`) is built for internal review, not for handing to a vendor. Specific issues:

1. **Cost** column is shown — irrelevant to the vendor and reveals our margin.
2. **List / Rec price** is shown — anchors the vendor to our number when they want to set their own.
3. **No CL price**, **no last-sale comp**, **no last-sale date** — exactly the data the vendor needs to evaluate each card.
4. **No barcode** — cert numbers must be typed manually; vendor with a scanner can't sweep through fast.
5. Repeated `External` chip on every row is visual noise.
6. Toast (`✓ Added 16 items to sell sheet`) leaks into the printed output.
7. Card names print in raw all-caps (`MEW EX SPECIAL ART RARE`) — `formatCardName()` exists but isn't applied to the print row.
8. No place for the vendor to write their per-card price or do the lot-offer math.

## Use case

User hands the printed sheet to a vendor / card store. Vendor reviews each card, writes their own price per row, then offers a single percentage of the agreed-price total for the entire lot.

## Goals

- Give the vendor the data they need per card: identity, grade, scannable cert, CL price, last-sale comp + date.
- Provide a blank **Agreed $** column for the vendor to fill in per row.
- Provide a totals strip at the bottom for the lot-offer math (Agreed total × Offer % = Offer $).
- Strip everything internal-only (cost, our list/rec, campaign, internal chips).
- Stay client-side; no backend or API changes.

## Non-goals

- No changes to the on-screen sell sheet view.
- No changes to the Go backend, domain types, or sell-sheet data model.
- No new API endpoints. All required fields already exist on `SellSheetItem` / `MarketSnapshot`.
- No PDF export tooling — relies on browser print as today.

---

## Design

### Layout (landscape letter, ~10–11 rows per page)

```
┌──┬──────────────────────────────────────────┬─────┬──────────────┬────────┬──────────┬──────────┐
│ #│ Card                                     │Grade│ Cert         │CL Price│Last Sale │Agreed $  │
├──┼──────────────────────────────────────────┼─────┼──────────────┼────────┼──────────┼──────────┤
│ 1│ Golem (Holo) · Masaki Promo              │ 4   │ 133487731    │ $279   │ $265     │          │
│  │ Pokémon Japanese Vending · #76           │     │ ▍▌▍▌▌▍▌▌▍▍▌  │        │ 03/12/26 │          │
├──┼──────────────────────────────────────────┼─────┼──────────────┼────────┼──────────┼──────────┤
│ 5│ Squirtle · Reverse Foil                  │ 5   │ 139108108    │ ~$185  │          │          │
│  │ Pokémon Expedition · #132                │     │ ▍▌▍▌▌▍▌▌▍▍▌  │        │          │          │
└──┴──────────────────────────────────────────┴─────┴──────────────┴────────┴──────────┴──────────┘
```

### Columns

| # | Column      | Source                                                    | Notes |
|---|-------------|-----------------------------------------------------------|-------|
| 1 | **#**       | Sequential row index, computed at render time             | Lets the vendor reference cards verbally ("row 14") |
| 2 | **Card**    | `formatCardName(item.cardName)` + `cardSubtitle(item)`    | Two lines: title-cased name, then `Set · #Number` |
| 3 | **Grade**   | `gradeDisplay(item)` rendered as just the number+grader   | E.g. `PSA 10` or just `10` if grader is PSA |
| 4 | **Cert**    | `item.certNumber` (top) + Code 128 SVG barcode (bottom)   | Barcode ~120×24px via `jsbarcode`; cert text remains human-readable |
| 5 | **CL Price**| `item.clValueCents` if > 0, else `~` + `item.targetSellPrice` | If both are 0/missing → render `—` |
| 6 | **Last Sale**| `item.currentMarket.lastSoldCents` + `lastSoldDate`      | Two lines: price, then date `MM/DD/YY`. Blank cell if no last sale. |
| 7 | **Agreed $**| Empty cell, ~70px wide                                    | Vendor writes in. White background, thin border. |

### CL Price fallback rule

```
if (clValueCents > 0)        "$" + format(clValueCents)
else if (targetSellPrice > 0) "~$" + format(targetSellPrice)
else                          "—"
```

The `~` prefix signals "estimated, not a real CL comp" so the vendor knows the difference.

### Sort order (print-only)

**Grade DESC, then CL Price DESC.** Highest-grade and biggest-ticket items first so the vendor's eye lands on the value drivers immediately. This is independent of whatever sort is active on screen.

### Page header (top of page 1, repeated abbreviated on continuation pages)

```
SlabLedger Sell Sheet                                     Generated: 4/26/2026
21 cards · CL prices as of 4/26/2026                                  Page 1/2
```

Implemented via `thead { display: table-header-group }` so the column header row repeats on each printed page; the SlabLedger title strip prints once at the top.

### Page footer (last page only)

```
─────────────────────────────────────────────────────────────────────────────
Totals       CL Price total: $12,345        Agreed total: $ ___________
                                            Offer % :     ____ %
                                            Offer $ :     $ __________
─────────────────────────────────────────────────────────────────────────────
21 items · CL price reflects most-recent CardLadder market value (~ = estimate from our recommended price).
Last Sale shows our most recent realized sale where available.
```

The CL Price total sums whatever is shown in the column — real CL values + `~` fallback estimates. Footer note explains the `~`.

### Barcode

- **Library:** `jsbarcode` (~10KB, MIT, no dependencies). Already-rendered SVG, prints crisp.
- **Format:** `CODE128` (the same family used by PSA).
- **Value:** raw `certNumber` string.
- **Dimensions:** width ≈ 120px, height ≈ 24px, no displayed value (cert number text is rendered separately above it).
- **Render mode:** SVG inline in the row — no images, no canvas, no extra HTTP requests.

### Visual style

- Alternating row bands `#f5f5f5` for scanning ease (print-safe).
- Each row block uses `break-inside: avoid` so a card never splits across pages.
- 8pt body font for data, 9pt for card name (already close to current style).
- No color other than black and the light grey banding.

### Missing-data handling

| Field            | Value missing → render |
|------------------|------------------------|
| CL Price + targetSellPrice both 0 | `—` |
| Last sale price 0 / missing | blank cell |
| Last sale date missing but price present | price only, no date |
| `certNumber` missing | no barcode, blank space (rare; cert is required for sell sheet items) |

---

## Files touched

All changes are client-side under `web/`:

| File | Change |
|------|--------|
| `web/package.json` | Add `jsbarcode` dep (~10KB) |
| `web/src/styles/print-sell-sheet.css` | Rewrite for new column structure: hide cost/list/rec/external/toast, add print header strip + footer totals, alternating bands, repeating thead, totals layout |
| `web/src/react/pages/campaign-detail/SellSheetView.tsx` (or whichever component owns the inventory print) | Inject the print header and print footer; in print mode swap to the new column set; apply the print sort |
| `web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.tsx` *(new)* | Focused component for one printed row with the barcode SVG. Accepts a `SellSheetItem` + row index. |
| `web/src/react/utils/sellSheetHelpers.tsx` | Add `formatLastSaleDate(iso?: string): string` and `clPriceDisplayCents(item): { cents: number; estimated: boolean }` |

**Not touched:** Go backend, domain types, on-screen sell sheet, sell-sheet API responses, any non-print CSS.

---

## Component boundaries

- **`SellSheetPrintRow`** — pure presentation. Inputs: `{ item: SellSheetItem; rowNumber: number }`. Owns the barcode `<svg>` and applies `formatCardName` + `cardSubtitle`. No state, no fetching.
- **`sellSheetHelpers`** — pure helpers, already the home for `formatCardName`, `cardSubtitle`, `gradeDisplay`. New helpers fit cleanly here.
- **`print-sell-sheet.css`** — the only place with `@media print` rules; everything is scoped via `.sell-sheet-print-*` class names so we can't accidentally affect the screen view.
- **`SellSheetView.tsx`** — orchestration only: chooses which row/column set to render based on `isPrinting`.

This keeps each unit small, testable, and inspectable in isolation.

---

## Testing

- Unit test `clPriceDisplayCents` and `formatLastSaleDate` with the obvious branches (CL present, CL missing fallback, both missing, ISO date present/missing).
- Manual print preview verification on a campaign with ~20 items: check column alignment, barcode renders, last-sale dates parse, totals math, page break behavior, footer appears only on the last page.
- Confirm screen view is unchanged (no `@media screen` rules touched).

## Out of scope (explicit)

- A vendor-name field on the printed sheet (kept generic).
- Per-vendor price suggestion / AI advisor in the print view.
- A separate "vendor PDF" backend export endpoint.
- Restoring or modifying the cost column in any view.
- Changes to the mobile sell sheet view.

## Open follow-ups (not blocking)

- Consider a print-button option to swap sort (grade DESC vs. CL DESC vs. card name) — defer until requested.
- Consider toggle for showing our recommended price as a faint anchor — explicitly rejected for now.
