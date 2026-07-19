# Click-to-copy certificate number — Design

**Date:** 2026-07-19
**Status:** Approved (pending spec review)

## Problem

On the inventory list, the PSA certificate number renders as plain, non-interactive
text inside a `·`-separated subtitle line (e.g. `Base Set · #4 · 12345678`). Copying it
means hand-dragging over the digits, which is fiddly and often grabs the adjacent set
name or card number. Users typically copy one cert at a time and can already see it in
the collapsed row (no expansion needed).

## Goal

Make the certificate number copy to the clipboard in a single click, directly from the
collapsed inventory row, on both desktop and mobile surfaces.

## Scope

**In scope**
- Cert number text becomes clickable; click copies the raw cert digits to the clipboard.
- Applies to the desktop row (`DesktopRow.tsx`) and mobile card (`MobileCard.tsx`).
- Minimal affordance: pointer cursor + subtle hover state; no persistent restyle.
- Inline "copied" flash feedback (~1s), no toast.

**Out of scope**
- Fix-DH match dialog (`FixDHMatchDialog.tsx`) — rare config context.
- Bulk / multi-cert copy.
- Copy icons or chip/monospace styling.

## Approach

A single focused, reusable component (Approach 1 of the brainstorm) rather than inline
handlers, so the clipboard call, flash timing, keyboard accessibility, and event
isolation live in one tested place. This matches the CLAUDE.md preference for focused,
independently testable units.

### New component — `CopyableCert`

File: `web/src/react/pages/campaign-detail/inventory/CopyableCert.tsx`

Props:
- `certNumber: string` — the raw cert digits to copy.
- `children?: React.ReactNode` — optional display content; defaults to `certNumber`.
  Lets the mobile card show `Cert #12345678` as the label while still copying only the
  raw digits.

Behavior:
- Renders a `<button type="button">` styled to sit inline in the subtitle (inherits the
  surrounding muted text color and size; not a block button).
- `onClick`:
  1. `e.stopPropagation()` — prevents the row's own click handler from toggling/expanding
     or selecting the row. (Both `DesktopRow` and `MobileCard` are clickable; this matches
     the existing `stopPropagation` convention used by `MarketplaceLinks`.)
  2. `navigator.clipboard.writeText(certNumber)`.
  3. On success: set local `copied` state true, then clear it after ~1000ms via a
     `setTimeout` (cleaned up on unmount / re-click).
  4. On rejection: leave `copied` false (no crash). No toast per the chosen feedback model;
     the absence of the flash signals failure. (A silent failure is acceptable here since
     clipboard rejection is rare and non-destructive.)
- Keyboard accessibility: it is a real `<button>`, so Enter/Space activation and focus
  come for free. `aria-label={`Copy cert number ${certNumber}`}`; `title="Copy cert number"`.
- Flash: while `copied`, swap the visible content for a brief `Copied ✓` (or apply a
  green text color to the existing content — implementer's choice, kept to a text/color
  change only, no layout shift).
- Guard: if `certNumber` is falsy, render `null`. Callers already conditionally render,
  so this is defensive.

Styling notes:
- `cursor-pointer`, transparent background, no border/padding that would disrupt the
  inline subtitle flow.
- Hover cue: `hover:text-[var(--text)] hover:underline` (subtle, on top of the muted
  base color).

### Integration

**`DesktopRow.tsx` (~line 208)** — currently:
```tsx
{item.purchase.certNumber && <> &middot; {item.purchase.certNumber}</>}
```
becomes:
```tsx
{item.purchase.certNumber && <> &middot; <CopyableCert certNumber={item.purchase.certNumber} /></>}
```

**`MobileCard.tsx` (~line 108)** — currently:
```tsx
Cert #{item.purchase.certNumber} &middot; <GradeBadge .../>
```
becomes:
```tsx
<CopyableCert certNumber={item.purchase.certNumber}>Cert #{item.purchase.certNumber}</CopyableCert> &middot; <GradeBadge .../>
```
The child carries the full `Cert #…` label; the component still writes only the raw
`certNumber` to the clipboard.

## Testing

New `CopyableCert.test.tsx` (co-located), table-driven where practical:
1. Clicking writes the **raw** cert number to the clipboard (mock
   `navigator.clipboard.writeText`).
2. Clicking calls `stopPropagation` — assert a parent `onClick` does **not** fire.
3. Empty/falsy `certNumber` renders nothing.
4. Clipboard rejection does not throw and leaves no flash (component stays mounted/usable).
5. After a successful copy, the flash appears and clears (using fake timers).

## Risks / notes

- `navigator.clipboard` requires a secure context (HTTPS or localhost). The app already
  relies on this API in `CardIntakeRow.tsx` and `CampaignsPage.tsx`, so no new constraint.
- Mobile: a `<button>` inside a clickable card is fine as long as `stopPropagation` is in
  place; verified against the existing `MarketplaceLinks` pattern in these files.
