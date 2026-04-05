# Liquidation Analysis Simplification

**Date:** 2026-04-05
**Branch:** aireviewz
**Status:** Approved

## Problem

The AI liquidation analysis consistently times out with `ERR_ADVISOR_MAX_ROUNDS` (exceeded maximum tool call rounds). Two contributing factors:

1. **Too many sale channels** (8) create complexity in prompts and AI reasoning
2. **Too many tools** (18) exposed to a 3-round budget — the AI gets distracted by tools the prompt never recommends

## Solution

### 1. Consolidate Sale Channels to Three

| Channel | DB Value | Fees | Description |
|---------|----------|------|-------------|
| eBay | `ebay` | Campaign `ebayFeePct` (default 12.35%) | Primary online marketplace. Absorbs `tcgplayer`. |
| Website | `website` | 3% (credit card processing) | Own website sales. Unchanged. |
| In Person | `inperson` | 0% | Card shows, local stores, GameStop. Assume 80-85% of market price. Absorbs `local`, `gamestop`, `cardshow`, `other`, `doubleholo`. |

**Removed constants:** `SaleChannelTCGPlayer`, `SaleChannelLocal`, `SaleChannelOther`, `SaleChannelGameStop`, `SaleChannelCardShow`, `SaleChannelDoubleHolo`

**New constant:** `SaleChannelInPerson SaleChannel = "inperson"`

**Removed:** `GameStopPayoutMinPct`, `GameStopPayoutMaxPct` constants and all GameStop-specific validation (PSA 8/9/10 restriction, $1,500 cap).

### 2. Backward-Compatible Channel Mapping

Old channel values remain in the database. A new `NormalizeChannel` function maps legacy values to the three new channels for display and analytics:

```
tcgplayer  -> ebay
local      -> inperson
gamestop   -> inperson
cardshow   -> inperson
other      -> inperson
doubleholo -> inperson
ebay       -> ebay       (unchanged)
website    -> website    (unchanged)
```

New sales can only be recorded with `ebay`, `website`, or `inperson`.

### 3. Trim Liquidation Tool Set

Reduce from 18 to 8 tools, matching what the prompt actually recommends:

**Keep:**
| Tool | Round | Purpose |
|------|-------|---------|
| `get_dashboard_summary` | 1 | Credit health, campaign overview |
| `get_global_inventory` | 1 | Inventory aging |
| `get_sell_sheet` | 1 | Current pricing data |
| `get_suggestion_stats` | 1 | Prior suggestion acceptance rate |
| `get_inventory_alerts` | 1 | Flagged items |
| `get_expected_values_batch` | 2 | Portfolio-wide EV |
| `suggest_price_batch` | 2 | Batch repricing |
| `get_crack_opportunities` | 2 | Crack-and-sell candidates |

**Remove:**
| Tool | Reason |
|------|--------|
| `list_campaigns` | Data available in `get_dashboard_summary` |
| `get_credit_summary` | Data available in `get_dashboard_summary` |
| `get_expected_values` | Replaced by batch version |
| `suggest_price` | Replaced by batch version |
| `get_inventory_aging` | Overlaps with `get_global_inventory` |
| `get_portfolio_health` | Overlaps with `get_dashboard_summary` |
| `get_cert_lookup` | Per-card lookup wastes rounds |
| `get_channel_velocity` | Not essential for liquidation decisions |
| `get_capital_timeline` | Not actionable for liquidation |
| `get_market_intelligence` | Too broad, wastes a round |

### 4. Prompt Updates

#### `baseSystemPrompt` — Exit Channels Section

Replace current 4-channel description with:

```
- **eBay** (primary): 12.35% total seller fees. Cards typically sell at CL value. Net = sale x 87.65%
- **Website**: Listed at market price. ~3% credit card processing fees.
- **In Person** (card shows, local stores): No platform fees. Cards sell at 80-85% of market price.
```

Remove GameStop-specific notes, margin formula stays the same (eBay-focused).

#### `liquidationSystemPrompt`

Update tool strategy to reference the trimmed 8-tool set. Simplify channel recommendations to three options.

#### `liquidationUserPrompt`

Update recommended action format to reference only eBay / Website / In Person channels.

### 5. Backend Changes

#### `internal/domain/campaigns/types.go`
- Remove: `SaleChannelTCGPlayer`, `SaleChannelLocal`, `SaleChannelOther`, `SaleChannelGameStop`, `SaleChannelCardShow`, `SaleChannelDoubleHolo`
- Add: `SaleChannelInPerson SaleChannel = "inperson"`

#### `internal/domain/campaigns/channel_fees.go`
- Remove `GameStopPayoutMinPct`, `GameStopPayoutMaxPct`
- Update `CalculateSaleFee`: eBay case handles only `SaleChannelEbay`, InPerson case returns 0, default returns 0
- Add `NormalizeChannel(ch SaleChannel) SaleChannel` function for legacy mapping

#### `internal/domain/campaigns/validation.go`
- Update valid channel list to `ebay`, `website`, `inperson`

#### `internal/domain/campaigns/suggestion_rules.go`
- Update any channel references to use new channels

#### `internal/domain/campaigns/service_sell_sheet.go`
- Use `NormalizeChannel` when grouping/displaying channel data

#### `internal/domain/advisor/service_impl.go`
- Trim `OpLiquidation` tool list from 18 to 8

#### `internal/domain/advisor/prompts.go`
- Update `baseSystemPrompt`, `liquidationSystemPrompt`, `liquidationUserPrompt`

#### `internal/adapters/scheduler/dh_orders_poll.go`
- Update any channel references

### 6. Frontend Changes

#### `web/src/types/campaigns/core.ts`
- Update `SaleChannel` type: `'ebay' | 'website' | 'inperson'`
- Keep legacy values accepted for display of historical data

#### `web/src/react/utils/campaignConstants.ts`
- Update channel labels: `{ ebay: 'eBay', website: 'Website', inperson: 'In Person' }`
- Update channel colors: three colors for new channels, map legacy to new
- Add `normalizeChannel()` utility for frontend display mapping

#### `web/src/react/pages/campaign-detail/RecordSaleModal.tsx`
- Show only three channel options (eBay, Website, In Person)
- Remove GameStop-specific warnings and validation

#### `web/src/react/components/insights/InsightsSection.tsx`
- Map legacy channels to new three in chart colors

### 7. Campaign Analysis Skill

Update `.claude/skills/campaign-analysis/SKILL.md`:
- Replace channel references with eBay / Website / In Person
- Remove GameStop-specific notes (PSA 8/9/10, $1,500 cap)
- Update margin context

### 8. Test Updates

- `channel_fees_test.go`: Update test cases for new channel set, add `NormalizeChannel` tests
- `service_test.go`: Update any channel references
- `suggestions_test.go`: Update channel references
- `tuning_test.go`: Update channel references
- `campaigns_repository_test.go`: Update channel references
- Mock files: Update as needed

## Out of Scope

- Database migration of existing channel values (backward-compatible mapping instead)
- Changing the 3-round budget (trimming tools should be sufficient)
- Changes to other AI operations (digest, campaign analysis, purchase assessment)
