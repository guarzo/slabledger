# Friction log template

The friction log (`web/screenshots/friction-log.md`) is appended once per cycle. Use this template so successive cycles can read prior state without ambiguity.

## Per-iteration template

```markdown
## Iteration <N> — <YYYY-MM-DD HH:MM>

### Fixed this cycle
- ✅ <title> — `path/to/file.tsx` — user story or structural sentence — regression: clean|side-effect — kind: cosmetic|structural|tier-c

### Structural attempt this cycle
- <one-line confirmation that fix-1 was structural or that a Tier C finding was investigated; OR a note that this cycle was intentionally all-cosmetic with UI_CLEAN blocked>

### Deferred / intentional
- <title> — reason — first raised: iter <N>

### Re-examined deferred items
- <title originally from iter N> — outcome: re-raised | promoted to epic | retired — one-line justification

### Epics (structural findings too large for one cycle)
- <title> — files/flow sketch — estimated scope — last touched: iter <N>

### Retired (no longer considered friction)
- <title> — justification

### Recurring (raised but not fully resolved across 2+ cycles)
- <title> — last attempt — next move

### Design conventions ratified this cycle
- <one-line convention>

### Outstanding 🔴 (blocking friction not yet resolved)
- <title> — user story
```

## Field guidance

- `kind` is required on every fixed entry. Use `cosmetic` for label/copy/spacing/variant swaps, `structural` for layout/flow changes, `tier-c` for systemic findings (nav, onboarding, missing destinations, product coherence, system-level surfaces).
- `first raised: iter N` is required on every deferred entry. Items hit their re-examination threshold three cycles after this tag.
- The "Structural attempt this cycle" section is the gate for UI_CLEAN. If it records `none — all cosmetic`, UI_CLEAN cannot be emitted this cycle regardless of 🔴 count.

## Worked example 1 — shipping cycle with one structural fix

```markdown
## Iteration 5 — 2026-04-21 09:00

### Fixed this cycle
- ✅ Inventory flat-table reshape — `web/src/react/pages/global-inventory/InventoryTable.tsx` — user trying to find aging cards ends up scanning 12 columns to answer one question because the table has no faceted filters. Fix: introduced a left-rail filter panel grouping by set/age-bucket/status with live counts. Regression: clean — kind: structural
- ✅ Campaigns empty-progress-bar copy — `web/src/react/pages/campaigns/PortfolioSummary.tsx` — user reading a new-campaign card ends up guessing what 0% means because there's no helper text. Fix: added "No sales yet — sell-through starts counting when the first purchase lands." Regression: clean — kind: cosmetic
- ✅ Dashboard tilde on "~5" wks — `web/src/react/pages/dashboard/CapitalStats.tsx` — user glancing at wks-to-cover ends up reading the tilde as a minus on mobile. Fix: replaced tilde with `≈`. Regression: clean — kind: cosmetic

### Structural attempt this cycle
- Fix 1 (Inventory flat-table reshape) was structural.

### Deferred / intentional
- Admin Stats overview card not collapsible on mobile — cramped but readable — first raised: iter 5

### Outstanding 🔴
- (none)
```

## Worked example 2 — all-cosmetic cycle (UI_CLEAN blocked)

```markdown
## Iteration 6 — 2026-04-22 09:00

### Fixed this cycle
- ✅ Campaigns chip tint — `web/src/react/pages/campaigns/StatusChip.tsx` — user skimming the list ends up misreading "paused" as an error because the tint leans red. Fix: shifted to slate. Regression: clean — kind: cosmetic
- ✅ Dashboard caption size — `web/src/react/pages/dashboard/StatGrid.tsx` — user reading captions ends up squinting because they're 11px. Fix: 13px. Regression: clean — kind: cosmetic
- ✅ Inventory row hover — `web/src/react/ui/Row.tsx` — user hovering a row ends up uncertain whether it's clickable because there's no affordance. Fix: added border-left accent. Regression: clean — kind: cosmetic

### Structural attempt this cycle
- none — this cycle was intentionally all-cosmetic. UI_CLEAN cannot be emitted until a structural attempt lands in the three-cycle window.

### Outstanding 🔴
- (none)
```

## Rollup rule

If `web/screenshots/friction-log.md` exceeds 500 lines, collapse the oldest iterations into one-line summaries of the shape:

```markdown
- Iter 1 (2026-04-17): 3 fixed, 5 deferred → details archived
```

Keep the 5 most recent iterations in full. Move the rest to a `## Archive` section at the bottom.
