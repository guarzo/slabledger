# Inventory Page — Friction Analysis

**Task traced:** "Look at inventory and decide what to reprice or list."
**Entry point:** `GlobalInventoryPage` → `InventoryTab` → `InventoryHeader` + `DesktopRow`/`MobileCard` + `InlinePriceEdit`.
**Method:** Source-only trace. Every claim cited to file + line. Screenshots not used as ground truth.

---

## Surface-by-surface trace

### 1. `GlobalInventoryPage` (45 lines)

**File:** `web/src/react/pages/GlobalInventoryPage.tsx`

- Page wrapped in `max-w-6xl mx-auto px-4` (line 24) — centered ~1152px column on a wide monitor.
- Header is just a breadcrumb (line 26) + `h1` "Inventory" (line 28). No subtitle, no count, no last-updated, no orientation framing.
- Hands off to `<InventoryTab items=… showCampaignColumn />` (line 41).

**Operator wants:** "How big is the queue, what's worth doing first?"
**Gap:** the page itself says nothing — all orientation lives one component deeper in `InventoryHeader`.

### 2. `InventoryTab` (357 lines)

**File:** `web/src/react/pages/campaign-detail/InventoryTab.tsx`

- Pulls 50+ fields from `useInventoryState` in one destructure (lines 56–74).
- Two virtualizers: desktop estimate 64px (line 80), mobile 140px (line 87).
- Renders InventoryHeader → optional print block → desktop table or mobile cards → modal stack.
- **Desktop table is locked to `max-h-[600px] overflow-y-auto` (line 273)** while sitting inside the page's `max-w-6xl` outer scroll. Two scroll axes stacked.

### 3. `InventoryHeader` (227 lines)

**File:** `web/src/react/pages/campaign-detail/inventory/InventoryHeader.tsx`

Renders top-to-bottom:

1. Totals breadcrumb (lines 99–135): `N cards · Cost $X · Market $Y · ±P/L unrealized (±%)` — all `text-sm`, peers separated by middle dots, no headline emphasis.
2. "All N cards" sub-line in `text-xs text-[var(--text-subtle)]` when filter is active (lines 137–144).
3. `BulkSelectionMissingCLWarning` (line 146) — only on bulk select.
4. `SellSheetActions` (line 154) — only when something is selected.
5. `CrackCandidatesBanner` (line 170) — only when `campaignId` is set, so **never visible on `GlobalInventoryPage`**.
6. `ReviewSummaryBar` (line 174) — heavy `bg-[var(--surface-raised)]` rounded panel hosting only a search input + "Show All" toggle.
7. Filter pills (lines 183–210): two rows, primary `text-xs font-semibold`, secondary `text-[11px] font-medium`. Pills appear/disappear based on count > 0; only `Needs Attention` and `All` are always shown.

### 4. Desktop table header (`InventoryTab.tsx` lines 248–266)

| col | source line | width |
|---|---|---|
| select | 251 | 28px |
| Card | 255 | flex-1 |
| Gr | 256 | 72px |
| Cost | 257 | 72px |
| List / Rec | 258 | 140px |
| P/L | 259 | 72px |
| Days | 260 | 40px |
| (sync dot, no header) | 261 | 20px |
| DH | 262 | 64px |
| (1px hairline, `bg-white/[0.06]`) | 263 | 1px |
| Sell | 264 | 64px |
| (overflow trigger, no header) | 265 | 28px |

The "signals → actions" hairline divider at line 264 is `bg-white/[0.06]` — visually almost invisible.

### 5. `DesktopRow` (435 lines)

**File:** `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx`

Per-row signals (12+ competing for attention):

- 2px `borderLeft` colored by review status (line 109).
- 32×44 thumbnail with absolute green dot for "in hand" (lines 116–141).
- Optional campaign chip with hashed color (lines 142–151).
- ★ hot seller (line 153), ◈ on-sell-sheet (line 154), ⚠ anomaly (line 158), inline marketplace links (lines 160–169).
- Set/number/cert subtitle (lines 171–175).
- Grade badge, Cost, List/Rec column, P/L chip, Days held, sync dot, DH badge, "Re-list" badge, Sell button, `⋯` overflow.

The List/Rec column (lines 181–214) is where the reprice decision happens:
- Three idle states: "no price data" warning button / `InlinePriceEdit` / plain display.
- Below current price, a `text-[10px] text-[var(--text-muted)] cursor-help` `<span>` reads "rec $X" (lines 205–212) — **not clickable**.

### 6. `InlinePriceEdit` (101 lines)

**File:** `web/src/react/pages/campaign-detail/inventory/InlinePriceEdit.tsx`

- Idle: `<button>` showing the formatted price or muted "set price" (lines 91–100). No icon, no underline. Visually indistinguishable from the non-editable `Cost` column.
- Editing: bare `w-20` input, no `$` prefix (line 78). Below it, a `text-[10px]` P/L preview vs cost basis (lines 82–86) — but **the rec line vanishes**, because `<InlinePriceEdit>` replaces the entire flex-col that contains it.
- Commit on Enter (line 53) or `onBlur` (line 76).
- No-op (`cents <= 0` or `cents === currentCents`) silently calls `cancel()` with no feedback (lines 36–39).
- On success: global toast "Price saved" (usePricingActions line 90) + `invalidateInventory`. No row-level flash, no transient highlight, no undo.

### 7. `MobileCard` (374 lines)

**File:** `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx`

Where desktop hides actions in `⋯`, mobile renders them as **inline buttons in a wrap-strip**: `$`/`$OVR` (lines 207–219), `Fix` (220–229), `Fix DH` (230–240), `Remove DH` (241–251), `Retry DH` (252–262), `Remove` from sell sheet (263–273), then a **second** contextual primary (Fix Match / List / Set Price / Restore, lines 282–327), Dismiss (337–350), Sell (351–357), Delete (358–370).

---

## Verified findings

### Inline edit (b/d/e)

| # | Finding | Cite |
|---|---|---|
| b | Rec is not clickable to accept. The data + handler are both in scope. | DesktopRow 205–212 (`<span>` with `cursor-help`, no `onClick`) |
| d | Blur commits silently. No-op edits disappear without feedback; stray clicks commit. | InlinePriceEdit 32–50, 76; DesktopRow 107 (row container clickable) |
| e | No row-level save confirmation. Only a global toast; row re-renders identically. | usePricingActions 87–96; InlinePriceEdit 43 (input unmounts) |

### Layout confusion

1. **Two scroll axes that compete.** Page outer scroll (`max-w-6xl`, GlobalInventoryPage line 24) + inner table scroll (`max-h-[600px]`, InventoryTab line 273). On a tall monitor you scroll the page to see totals + filters, then scroll a 600px box to see rows.
2. **`ReviewSummaryBar` panel oversells two unrelated controls.** A bordered, padded `bg-[var(--surface-raised)]` rounded panel hosts only a search input + "Show All" toggle (ReviewSummaryBar.tsx 16). The visual weight implies it's a section header for the filters below, but it isn't.
3. **Filter pill set is dynamic.** Pills appear/disappear based on `count > 0` (InventoryHeader 63–75). Two rows, with 11px vs 12px size delta. Day-to-day shape shifts.
4. **Body column widths inverse-correlate with information density.** `Card` is `flex-1` and carries decoration (thumbnail, chip, 5 inline icons, 2-line subtitle). `List/Rec` is fixed 140px and carries the densest decision content (current price + 10px rec stacked).
5. **Three left-edge status systems.** 2px `borderLeft` (DesktopRow 109), green "in hand" dot on thumbnail (125–141), DH status pill in its own column. The same row state is signaled in 3 different visual languages, none of them a label.
6. **Two unlabeled columns.** Sync-dot 20px (InventoryTab 261) and `⋯` overflow 28px (line 265). Header renders empty `<div>`s.
7. **Hairline "signals → actions" divider is invisible.** `bg-white/[0.06]` at 1px (InventoryTab 264).
8. **Page header says nothing.** GlobalInventoryPage 28 — just `<h1>Inventory</h1>`. No count, no orientation.

### Action inconsistency

1. **Desktop = overflow menu; mobile = inline strip.** Same item, totally different action discoverability. DesktopRow 354–434 vs MobileCard 206–371.
2. **Set Price duplicated on mobile in `set_and_list` state.** MobileCard line 207 unconditionally renders `$`/`$OVR`; lines 308–317 also render a "Set Price" warning button. Both call `onSetPrice`.
3. **Verbs aren't parallel.**
   - "Set Price" / `$` / `$OVR`
   - "Fix Pricing" (desktop) / "Fix" (mobile)
   - "Fix DH Match" (desktop) / "Fix DH" (mobile) / "Fix Match" (mobile contextual)
   - "Remove DH Match" / "Remove DH"
   - "Retry DH Match" / "Retry DH"
   - "Remove from Sell Sheet" / "Remove"
4. **`Sell` is always the row's primary affordance, even when the item can't be sold.** DesktopRow 267–271 hardcoded. The actually-correct next action lives inside the `⋯` menu styled `ITEM_PRIMARY` (line 377). Brand-tinted "right action" treatment is one click deeper than the wrong-action button.
5. **Contextual primary uses two different visual languages.** Mobile: colored inline pill (warning/success/brand bg, MobileCard 282–327). Desktop: dropdown menu item (DesktopRow 379) — invisible until the menu is open.
6. **Confirm pattern inconsistent.** `window.confirm()` on Dismiss (DesktopRow 407, MobileCard 342) and Delete (DesktopRow 424). No confirm on Remove DH Match, Retry DH Match, Restore, List on DH, Set Price.
7. **Contextual-primary dedupe is partial.** DesktopRow 351–352 only de-dupes "Set Price" and "Fix DH Match" against the contextual primary. `List on DH` and `Restore to DH` aren't in the standard menu at all, so dedupe doesn't apply — but the model is mixed: sometimes the contextual primary pulls a standard item up; sometimes it's a state-only action that only exists when contextual.

---

## Other gaps surfaced earlier (not yet prioritized)

- Recommendation rendered at 10px gray under the current price, no Δ, no color — whisper-quiet (DesktopRow 205–212).
- `Needs Attention` is one pill among many, not a headline (InventoryHeader 63–67).
- Totals breadcrumb is flat: Cost / Market / P/L all rendered as peer `text-sm` middle-dot-separated items (InventoryHeader 99–135).
- EV is conditional and unobtrusive, easy to miss when present (lines 127–134).
- 12+ visual signals per row with no spatial grouping; signals zone and actions zone separated only by a 1px almost-invisible hairline.

---

## Not yet read

- `ExpandedDetail.tsx` (the drawer)
- `useInventoryState.ts` (state machine)
- `CompSummaryPanel.tsx` (where comparison data may live)
- Full state machine in `inventoryCalcs.ts` beyond `deriveActionIntent` / `canDismiss` / tab counts

Anything proposed should re-verify against these before claiming a structural fix.

---

## Operator-confirmed priority

1. **Layout confusing + Actions inconsistent** — start here.
2. **Inline edit (b/d/e)** — second pass.
