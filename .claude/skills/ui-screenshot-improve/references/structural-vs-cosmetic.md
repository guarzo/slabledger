# Structural vs cosmetic fixes

The diversity constraint requires at least one structural (or Tier C) fix per cycle. Use these examples to classify a candidate fix before ranking.

## Heuristic

- If the fix changes **what a user sees first** or **how they move through the page**, it's probably structural.
- If the fix changes **how pretty the thing they see is**, it's probably cosmetic.
- If the fix introduces a new surface (route, feed, empty-state flow), it's Tier C.

## Worked examples

### Cosmetic

1. Changing a chip's color class from `bg-danger` to `bg-slate-500`.
2. Swapping `variant="danger"` → `variant="secondary"` on a reporting button.
3. Adding `font-variant-numeric: tabular-nums` to a money column.
4. Adding a small label above a progress bar (borderline; escalates to structural if the label requires a new data computation).
5. Tightening vertical rhythm on a stat grid by 4px.

### Structural

6. Restructuring a progress bar + label + delta into a three-row "coverage card" with the primary number leading.
7. Converting a flat 12-column inventory table into a filter-with-faceted-counts layout (left-rail filters, live counts, sorted results).
8. Moving the primary CTA from the bottom of a mobile page to a sticky header so it's always reachable one-handed.
9. Collapsing three empty Insights sections into a single "start here" guided flow with a default Weekly Digest.
10. Splitting a single dense modal into a 3-step wizard when the user has to answer the steps sequentially anyway.

### Tier C

11. Adding a new `/finance/invoices` route so the "unpaid invoices" dashboard chip has a destination.
12. Introducing a first-run surface for users with zero campaigns that teaches intake → campaign → sell.
13. Renaming the sidebar's "Inventory" to "Intake" and restructuring the child pages to follow task order rather than data-model order.
14. Adding a global search box in the top bar that searches cards, campaigns, and intake tickets.
15. Creating a "Capital over time" system-level view that today requires assembling data from three pages.

## Gray areas

- **Adding a tooltip** is cosmetic unless the tooltip replaces a navigation step the user otherwise has to take; then it's structural.
- **Renaming a column header** is cosmetic unless the rename resolves a user-mental-model mismatch that caused repeated misclicks; then it's structural.
- **Reordering rows** is cosmetic unless the reorder changes the default action (the first-row CTA on mobile); then it's structural.

When in doubt, ask: "if I only shipped this fix and nothing else, would a user's *behavior* change, or just their impression?"
