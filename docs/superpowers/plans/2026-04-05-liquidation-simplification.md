# Liquidation Analysis Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate 8 sale channels to 3 (eBay, Website, In Person), trim the liquidation AI tool set from 18 to 8, and update prompts to fix `ERR_ADVISOR_MAX_ROUNDS` timeouts.

**Architecture:** Domain types and fee logic change in `internal/domain/campaigns/`. A `NormalizeChannel` function maps legacy DB values to the 3 new channels at display time — no DB migration needed. AI advisor prompts and tool sets update in `internal/domain/advisor/`. Frontend shows only the 3 new channels for new sales but renders legacy channels correctly via normalization.

**Tech Stack:** Go 1.26, React/TypeScript, SQLite

---

### Task 1: Update Domain Types and Add NormalizeChannel

**Files:**
- Modify: `internal/domain/campaigns/types.go:17-29`
- Modify: `internal/domain/campaigns/channel_fees.go:1-42`

- [ ] **Step 1: Update SaleChannel constants in types.go**

Replace the channel const block at lines 20-29:

```go
const (
	SaleChannelEbay     SaleChannel = "ebay"
	SaleChannelWebsite  SaleChannel = "website"
	SaleChannelInPerson SaleChannel = "inperson"
)

// Legacy channel values — kept for backward-compatible DB reads.
const (
	SaleChannelTCGPlayer  SaleChannel = "tcgplayer"
	SaleChannelLocal      SaleChannel = "local"
	SaleChannelOther      SaleChannel = "other"
	SaleChannelGameStop   SaleChannel = "gamestop"
	SaleChannelCardShow   SaleChannel = "cardshow"
	SaleChannelDoubleHolo SaleChannel = "doubleholo"
)
```

- [ ] **Step 2: Update channel_fees.go — remove GameStop constants, update CalculateSaleFee, add NormalizeChannel**

Replace the entire file content:

```go
package campaigns

import "math"

// DefaultMarketplaceFeePct is the default fee percentage for eBay (12.35%).
const DefaultMarketplaceFeePct = 0.1235

// DefaultWebsiteFeePct is the fee percentage for website/online store sales (3% credit card processing).
const DefaultWebsiteFeePct = 0.03

// grossModeFee signals enrichSellSheetItem to skip fee deduction, returning gross prices.
const grossModeFee = -1.0

// CalculateSaleFee computes marketplace fees for a given channel and sale price.
func CalculateSaleFee(channel SaleChannel, salePriceCents int, campaign *Campaign) int {
	switch NormalizeChannel(channel) {
	case SaleChannelEbay:
		feePct := campaign.EbayFeePct
		if feePct == 0 {
			feePct = DefaultMarketplaceFeePct
		}
		return int(math.Round(float64(salePriceCents) * feePct))
	case SaleChannelWebsite:
		return int(math.Round(float64(salePriceCents) * DefaultWebsiteFeePct))
	default:
		return 0
	}
}

// CalculateNetProfit computes net profit for a sale.
// netProfit = salePrice - buyCost - sourcingFee - saleFee
func CalculateNetProfit(salePriceCents, buyCostCents, sourcingFeeCents, saleFeeCents int) int {
	return salePriceCents - buyCostCents - sourcingFeeCents - saleFeeCents
}

// NormalizeChannel maps legacy channel values to the three active channels.
// Used for display, analytics, and fee calculations. Old DB values are preserved.
func NormalizeChannel(ch SaleChannel) SaleChannel {
	switch ch {
	case SaleChannelEbay, SaleChannelTCGPlayer:
		return SaleChannelEbay
	case SaleChannelWebsite:
		return SaleChannelWebsite
	case SaleChannelInPerson, SaleChannelLocal, SaleChannelOther,
		SaleChannelGameStop, SaleChannelCardShow, SaleChannelDoubleHolo:
		return SaleChannelInPerson
	default:
		return SaleChannelInPerson
	}
}
```

- [ ] **Step 3: Run tests to verify compilation**

Run: `cd /workspace/.worktrees/aireviewz && go build ./internal/domain/campaigns/`
Expected: Build errors in files that reference removed constants — that's expected, we'll fix them in subsequent tasks.

- [ ] **Step 4: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add internal/domain/campaigns/types.go internal/domain/campaigns/channel_fees.go
git commit -m "refactor: consolidate sale channels to 3, add NormalizeChannel"
```

---

### Task 2: Update Channel Fee Tests

**Files:**
- Modify: `internal/domain/campaigns/channel_fees_test.go`

- [ ] **Step 1: Rewrite channel_fees_test.go with new channel tests and NormalizeChannel tests**

```go
package campaigns

import "testing"

func TestCalculateSaleFee(t *testing.T) {
	campaign := &Campaign{EbayFeePct: 0.1235}

	tests := []struct {
		name           string
		channel        SaleChannel
		salePriceCents int
		wantFee        int
	}{
		{"ebay 100 dollars", SaleChannelEbay, 10000, 1235},
		{"legacy tcgplayer maps to ebay", SaleChannelTCGPlayer, 10000, 1235},
		{"website 3pct", SaleChannelWebsite, 10000, 300},
		{"inperson no fee", SaleChannelInPerson, 10000, 0},
		{"legacy local maps to inperson", SaleChannelLocal, 10000, 0},
		{"legacy gamestop maps to inperson", SaleChannelGameStop, 10000, 0},
		{"legacy other maps to inperson", SaleChannelOther, 10000, 0},
		{"legacy cardshow maps to inperson", SaleChannelCardShow, 10000, 0},
		{"legacy doubleholo maps to inperson", SaleChannelDoubleHolo, 10000, 0},
		{"ebay 500 dollars", SaleChannelEbay, 50000, 6175},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateSaleFee(tt.channel, tt.salePriceCents, campaign)
			if got != tt.wantFee {
				t.Errorf("CalculateSaleFee(%s, %d) = %d, want %d", tt.channel, tt.salePriceCents, got, tt.wantFee)
			}
		})
	}
}

func TestCalculateSaleFee_DefaultEbayPct(t *testing.T) {
	campaign := &Campaign{EbayFeePct: 0}
	got := CalculateSaleFee(SaleChannelEbay, 10000, campaign)
	if got != 1235 {
		t.Errorf("CalculateSaleFee with default fee = %d, want 1235", got)
	}
}

func TestCalculateNetProfit(t *testing.T) {
	net := CalculateNetProfit(75000, 50000, 300, 9263)
	want := 75000 - 50000 - 300 - 9263
	if net != want {
		t.Errorf("CalculateNetProfit = %d, want %d", net, want)
	}
}

func TestNormalizeChannel(t *testing.T) {
	tests := []struct {
		input SaleChannel
		want  SaleChannel
	}{
		{SaleChannelEbay, SaleChannelEbay},
		{SaleChannelTCGPlayer, SaleChannelEbay},
		{SaleChannelWebsite, SaleChannelWebsite},
		{SaleChannelInPerson, SaleChannelInPerson},
		{SaleChannelLocal, SaleChannelInPerson},
		{SaleChannelGameStop, SaleChannelInPerson},
		{SaleChannelCardShow, SaleChannelInPerson},
		{SaleChannelOther, SaleChannelInPerson},
		{SaleChannelDoubleHolo, SaleChannelInPerson},
		{SaleChannel("unknown"), SaleChannelInPerson},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := NormalizeChannel(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeChannel(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests**

Run: `cd /workspace/.worktrees/aireviewz && go test ./internal/domain/campaigns/ -run "TestCalculateSaleFee|TestCalculateNetProfit|TestNormalizeChannel" -v`
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add internal/domain/campaigns/channel_fees_test.go
git commit -m "test: update channel fee tests for 3-channel model"
```

---

### Task 3: Update Validation and Remove GameStop Warnings

**Files:**
- Modify: `internal/domain/campaigns/validation.go:42-51` (validSaleChannels map)
- Modify: `internal/domain/campaigns/validation.go:173-190` (ValidateSaleWarnings)

- [ ] **Step 1: Update validSaleChannels map**

Replace lines 42-51:

```go
var validSaleChannels = map[SaleChannel]bool{
	SaleChannelEbay:     true,
	SaleChannelWebsite:  true,
	SaleChannelInPerson: true,
	// Legacy channels accepted for backward compatibility with existing DB records.
	SaleChannelTCGPlayer:  true,
	SaleChannelLocal:      true,
	SaleChannelOther:      true,
	SaleChannelGameStop:   true,
	SaleChannelCardShow:   true,
	SaleChannelDoubleHolo: true,
}
```

- [ ] **Step 2: Remove GameStop-specific warning logic**

Replace the `ValidateSaleWarnings` function (lines 173-190) with:

```go
// ValidateSaleWarnings returns soft warnings (not errors) for a sale.
func ValidateSaleWarnings(_ *Sale, _ *Purchase) []SaleWarning {
	return nil
}
```

- [ ] **Step 3: Run validation tests**

Run: `cd /workspace/.worktrees/aireviewz && go test ./internal/domain/campaigns/ -run "TestValidate" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add internal/domain/campaigns/validation.go
git commit -m "refactor: update validation for 3-channel model, remove GameStop warnings"
```

---

### Task 4: Update Suggestion Rules and Sell Sheet

**Files:**
- Modify: `internal/domain/campaigns/suggestions.go:191-193` (isMarketplaceChannel)
- Modify: `internal/domain/campaigns/suggestion_rules.go:185-253` (GameStop buy terms logic)
- Modify: `internal/domain/campaigns/service_sell_sheet.go:67,96-107` (recommendChannel, fee deduction)

- [ ] **Step 1: Update isMarketplaceChannel in suggestions.go**

Replace lines 191-193:

```go
// isMarketplaceChannel returns true for channels that charge marketplace fees.
func isMarketplaceChannel(ch SaleChannel) bool {
	return NormalizeChannel(ch) == SaleChannelEbay
}
```

- [ ] **Step 2: Remove GameStop-specific buy terms logic in suggestion_rules.go**

In `suggestChannelInformedBuyTerms`, replace lines 185-253 (from `isGameStop := ...` to the end of the `for` loop body) with simplified logic that treats all channels uniformly:

```go
	for _, c := range campaigns {
		if c.Phase != PhaseActive {
			continue
		}

		var feePct float64
		if isMarketplaceChannel(bestChannel.Channel) {
			feePct = c.EbayFeePct
			if feePct == 0 {
				feePct = DefaultMarketplaceFeePct
			}
		}

		targetMargin := suggTargetMargin

		maxBuy := bestMargin - targetMargin - feePct
		if maxBuy <= 0 {
			continue
		}

		if c.BuyTermsCLPct > maxBuy+suggBuyTermsBuffer {
			confidence := confidenceLabelWithAge(bestChannel.SaleCount, "", now)

			suggestions = append(suggestions, CampaignSuggestion{
				Type:  "adjust",
				Title: fmt.Sprintf("Lower buy terms on %s", c.Name),
				Rationale: fmt.Sprintf("Best channel (%s) margin is %.0f%%. With %.0f%% fees and 10%% target margin, max buy should be ~%.0f%% CL. Current: %.0f%%.",
					bestChannel.Channel, bestMargin*100, feePct*100, maxBuy*100, c.BuyTermsCLPct*100),
				Confidence: confidence,
				DataPoints: bestChannel.SaleCount,
				SuggestedParams: CampaignSuggestionParams{
					Name:          c.Name,
					BuyTermsCLPct: maxBuy,
				},
				ExpectedMetrics: ExpectedMetrics{
					ExpectedMarginPct: targetMargin,
					DataConfidence:    confidence,
				},
			})
		}
	}
```

This removes the entire `isGameStop` branch and `GameStopPayoutMinPct`/`GameStopPayoutMaxPct` references.

- [ ] **Step 3: Update recommendChannel in service_sell_sheet.go**

Replace the `recommendChannel` function (lines 96-107):

```go
// recommendChannel determines the best exit channel for a sell-sheet item.
func recommendChannel(grade float64, _ int, mkt *MarketSnapshot) (SaleChannel, string) {
	if grade == 7 {
		return SaleChannelInPerson, "In Person"
	}
	if mkt != nil && mkt.Trend30d > 0.05 {
		return SaleChannelInPerson, "In Person"
	}
	return SaleChannelEbay, "eBay"
}
```

- [ ] **Step 4: Update fee deduction check in enrichSellSheetItem**

Replace line 67 in service_sell_sheet.go:

Old:
```go
	if ebayFeePct != grossModeFee && item.TargetSellPrice > 0 && (item.RecommendedChannel == SaleChannelEbay || item.RecommendedChannel == SaleChannelTCGPlayer) {
```

New:
```go
	if ebayFeePct != grossModeFee && item.TargetSellPrice > 0 && NormalizeChannel(item.RecommendedChannel) == SaleChannelEbay {
```

- [ ] **Step 5: Run domain tests**

Run: `cd /workspace/.worktrees/aireviewz && go test ./internal/domain/campaigns/ -v`
Expected: Some test failures from tests using old channel constants — we fix those in Task 6.

- [ ] **Step 6: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add internal/domain/campaigns/suggestions.go internal/domain/campaigns/suggestion_rules.go internal/domain/campaigns/service_sell_sheet.go
git commit -m "refactor: update suggestions and sell sheet for 3-channel model"
```

---

### Task 5: Update DH Orders Poll Scheduler

**Files:**
- Modify: `internal/adapters/scheduler/dh_orders_poll.go:215-226` (mapDHChannel)

- [ ] **Step 1: Update mapDHChannel**

Replace the `mapDHChannel` function (lines 215-226):

```go
// mapDHChannel converts a DH channel string to a campaigns.SaleChannel.
func mapDHChannel(channel string) campaigns.SaleChannel {
	switch channel {
	case "ebay":
		return campaigns.SaleChannelEbay
	case "shopify":
		return campaigns.SaleChannelEbay
	case "dh":
		return campaigns.SaleChannelWebsite
	default:
		return campaigns.SaleChannelInPerson
	}
}
```

- [ ] **Step 2: Run scheduler tests**

Run: `cd /workspace/.worktrees/aireviewz && go test ./internal/adapters/scheduler/ -v`
Expected: PASS (or test failures related to the old channel mapping — fix accordingly)

- [ ] **Step 3: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add internal/adapters/scheduler/dh_orders_poll.go
git commit -m "refactor: update DH orders poll channel mapping"
```

---

### Task 6: Update Advisor Tool Set and Prompts

**Files:**
- Modify: `internal/domain/advisor/service_impl.go:56-64` (OpLiquidation tool list)
- Modify: `internal/domain/advisor/prompts.go:4-37` (baseSystemPrompt exit channels)
- Modify: `internal/domain/advisor/prompts.go:106-153` (liquidation prompts)

- [ ] **Step 1: Trim OpLiquidation tool list in service_impl.go**

Replace lines 56-64:

```go
	OpLiquidation: {
		"get_dashboard_summary", "get_global_inventory", "get_sell_sheet",
		"get_suggestion_stats", "get_inventory_alerts",
		"get_expected_values_batch", "suggest_price_batch",
		"get_crack_opportunities",
	},
```

- [ ] **Step 2: Update baseSystemPrompt exit channels in prompts.go**

Replace lines 9-13 (the exit channels section):

```
### Exit Channels & Fees
- **eBay** (primary): 12.35% total seller fees. Cards typically sell at CL value. Net = sale × 87.65%
- **Website**: Listed at market price. ~3% credit card processing fees.
- **In Person** (card shows, local stores): No platform fees. Cards sell at 80-85% of market price.
```

- [ ] **Step 3: Update liquidationSystemPrompt in prompts.go**

Replace the liquidation system prompt (lines 106-138) with:

```go
// liquidationSystemPrompt is used for liquidation analysis.
const liquidationSystemPrompt = baseSystemPrompt + `

## Your Task: Liquidation Analysis
Identify cards where selling now (even below market) is better than holding.
Consider: credit pressure, carrying costs (5% annual), days held, market trend, liquidity, and EV.
A card with negative EV or declining market that ties up capital should be liquidated.
Prioritize by capital freed relative to markdown cost.

Also check get_crack_opportunities for cards where cracking and selling raw outperforms holding the graded slab. Crack candidates are a form of liquidation.

When you identify cards that should be repriced, use the suggest_price_batch tool
to save your recommended prices. The user will review your suggestions
in the inventory UI and can accept or dismiss each one.

Before making new suggestions, call get_suggestion_stats to see how your
previous recommendations performed. If acceptance rate is low, adjust your
pricing strategy — you may be suggesting prices that are too aggressive.

## Exit Channels
Recommend one of three channels for each card:
- **eBay**: Best for most cards. 12.35% fees, sells at CL value.
- **Website**: Good for unique or high-demand cards. 3% fees.
- **In Person** (card shows, local stores): Best for quick liquidation. 0% fees, sells at 80-85% of market.

## Tool Strategy
You have a **3-round tool budget** and 8 tools. Plan your calls carefully:

**Round 1**: Call get_dashboard_summary, get_global_inventory, get_sell_sheet,
get_suggestion_stats, and get_inventory_alerts together. These give you credit health, inventory aging, and pricing data.

**Round 2**: Call get_expected_values_batch (one call, all campaigns or specific ones) for EV data,
suggest_price_batch (one call) for all cards you want to reprice, and get_crack_opportunities for crack candidates.

**Round 3**: Escape hatch only if absolutely needed. Prefer completing the analysis after Round 2.

After your tool rounds, write your analysis with the data you have.`
```

- [ ] **Step 4: Update liquidationUserPrompt**

Replace the liquidation user prompt (lines 140-153):

```go
const liquidationUserPrompt = `Run a liquidation analysis across my entire portfolio.

For each liquidation candidate, provide:
- **Card name, grade, cert** (if available)
- **Cost basis** and **days held**
- **Current market** (median, trend, velocity)
- **Recommended action**: sell at [price] on [eBay / Website / In Person], or hold
- **Reasoning**: why sell now vs hold (credit pressure, declining trend, low liquidity, etc.)
- **Capital freed** if sold

Sort by urgency: credit-critical first, then declining-trend cards, then low-EV holds.
End with a summary: total capital that could be freed and net cost of liquidation.`
```

- [ ] **Step 5: Run advisor tests**

Run: `cd /workspace/.worktrees/aireviewz && go test ./internal/domain/advisor/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add internal/domain/advisor/service_impl.go internal/domain/advisor/prompts.go
git commit -m "refactor: trim liquidation tools to 8, update prompts for 3-channel model"
```

---

### Task 7: Fix Domain Test Files

**Files:**
- Modify: `internal/domain/campaigns/service_test.go`
- Modify: `internal/domain/campaigns/suggestions_test.go`
- Modify: `internal/domain/campaigns/tuning_test.go`
- Modify: `internal/adapters/storage/sqlite/campaigns_repository_test.go`

- [ ] **Step 1: Update service_test.go**

Replace all `campaigns.SaleChannelLocal` with `campaigns.SaleChannelInPerson` in test data (lines 188, 388, 1047, 1081). Replace `campaigns.SaleChannelGameStop` with `campaigns.SaleChannelInPerson` (lines 1046, 1074).

- [ ] **Step 2: Update suggestions_test.go**

Replace `SaleChannelLocal` references with `SaleChannelInPerson` (lines 95, 99, 123). Replace `SaleChannelGameStop` with `SaleChannelInPerson` (line 253). Remove or update the `TestGameStopPayoutRange` test (line 250) — replace it with an equivalent "In Person" channel test that uses the simplified non-GameStop buy terms logic.

- [ ] **Step 3: Update tuning_test.go**

Replace `SaleChannelLocal` with `SaleChannelInPerson` (line 95). Replace `SaleChannelTCGPlayer` with `SaleChannelEbay` (lines 343, 365).

- [ ] **Step 4: Update campaigns_repository_test.go**

Replace `campaigns.SaleChannelGameStop` with `campaigns.SaleChannelInPerson` (lines 286, 287, 317). Replace `campaigns.SaleChannelLocal` with `campaigns.SaleChannelInPerson` (lines 288, 302, 323).

- [ ] **Step 5: Run all domain and adapter tests**

Run: `cd /workspace/.worktrees/aireviewz && go test ./internal/domain/campaigns/ ./internal/adapters/storage/sqlite/ ./internal/adapters/scheduler/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add internal/domain/campaigns/service_test.go internal/domain/campaigns/suggestions_test.go internal/domain/campaigns/tuning_test.go internal/adapters/storage/sqlite/campaigns_repository_test.go
git commit -m "test: update test files for 3-channel model"
```

---

### Task 8: Update Frontend Types and Constants

**Files:**
- Modify: `web/src/types/campaigns/core.ts:6`
- Modify: `web/src/react/utils/campaignConstants.ts`

- [ ] **Step 1: Update SaleChannel type in core.ts**

Replace line 6:

```typescript
export type SaleChannel = 'ebay' | 'website' | 'inperson' | 'tcgplayer' | 'local' | 'other' | 'gamestop' | 'cardshow';
```

Note: Legacy values kept in the union for backward compatibility with historical data from the API.

- [ ] **Step 2: Update campaignConstants.ts**

Replace the entire file:

```typescript
import type { Phase, SaleChannel, CreateCampaignInput } from '../../types/campaigns';

export const DEFAULT_SALE_CHANNEL: SaleChannel = 'ebay';

/** Channels available for recording new sales. */
export const activeSaleChannels: SaleChannel[] = ['ebay', 'website', 'inperson'];

/** Maps any channel (including legacy) to its display label. */
export const saleChannelLabels: Record<SaleChannel, string> = {
  ebay: 'eBay',
  website: 'Website',
  inperson: 'In Person',
  // Legacy channels — displayed for historical data
  tcgplayer: 'eBay',
  local: 'In Person',
  other: 'In Person',
  gamestop: 'In Person',
  cardshow: 'In Person',
};

/** Normalizes a legacy channel to one of the 3 active channels. */
export function normalizeChannel(ch: SaleChannel): SaleChannel {
  switch (ch) {
    case 'ebay':
    case 'tcgplayer':
      return 'ebay';
    case 'website':
      return 'website';
    default:
      return 'inperson';
  }
}

export const saleChannelColors: Record<SaleChannel, string> = {
  ebay: 'bg-blue-500',
  website: 'bg-indigo-500',
  inperson: 'bg-green-500',
  // Legacy channels map to their normalized color
  tcgplayer: 'bg-blue-500',
  local: 'bg-green-500',
  other: 'bg-green-500',
  gamestop: 'bg-green-500',
  cardshow: 'bg-green-500',
};

export const phaseColors: Record<Phase, string> = {
  pending: 'bg-amber-500',
  active: 'bg-green-500',
  closed: 'bg-gray-400',
};

export const campaignTabs = [
  { id: 'overview', label: 'Overview' },
  { id: 'transactions', label: 'Transactions' },
  { id: 'tuning', label: 'Tuning' },
  { id: 'settings', label: 'Settings' },
] as const;

export type CampaignTabId = typeof campaignTabs[number]['id'];

export const phaseOptions = [
  { value: 'pending', label: 'Pending' },
  { value: 'active', label: 'Active' },
  { value: 'closed', label: 'Closed' },
] as const;

export const defaultCampaignInput: CreateCampaignInput = {
  name: '',
  sport: 'Pokemon',
  yearRange: '',
  gradeRange: '',
  priceRange: '',
  clConfidence: '',
  buyTermsCLPct: 0.78,
  dailySpendCapCents: 50000,
  inclusionList: '',
  exclusionMode: false,
  psaSourcingFeeCents: 300,
  ebayFeePct: 0.1235,
};
```

- [ ] **Step 3: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add web/src/types/campaigns/core.ts web/src/react/utils/campaignConstants.ts
git commit -m "feat(frontend): update types and constants for 3-channel model"
```

---

### Task 9: Update RecordSaleModal

**Files:**
- Modify: `web/src/react/pages/campaign-detail/RecordSaleModal.tsx`

- [ ] **Step 1: Remove GameStop functions and update channel dropdown**

Remove the `gameStopWarnings` function (lines 70-82) and `gameStopPayout` function (lines 84-91).

Update the channel `<Select>` (line 229) to only show active channels:

```tsx
import { saleChannelLabels, DEFAULT_SALE_CHANNEL, activeSaleChannels } from '../../utils/campaignConstants';
```

Replace line 229:

```tsx
options={activeSaleChannels.map(ch => ({ value: ch, label: saleChannelLabels[ch] }))}
```

Remove the GameStop payout estimate block (lines 232-247 — the whole `{isSingle && items[0] && (() => {` IIFE after the Select).

Remove the GameStop warnings block (lines 260-267).

In the multi-item list (line 345), remove the `gameStopWarnings` call and the warnings display (lines 345-357 warning div).

- [ ] **Step 2: Verify frontend builds**

Run: `cd /workspace/.worktrees/aireviewz/web && npm run build`
Expected: Build succeeds with no errors

- [ ] **Step 3: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add web/src/react/pages/campaign-detail/RecordSaleModal.tsx
git commit -m "feat(frontend): simplify RecordSaleModal to 3 channels"
```

---

### Task 10: Update InsightsSection Chart Colors

**Files:**
- Modify: `web/src/react/components/insights/InsightsSection.tsx:43-46`

- [ ] **Step 1: Update channelColors to use normalizeChannel**

Replace the channelColors object (lines 43-46):

```tsx
const channelColors: Record<string, string> = {
  ebay: 'var(--channel-ebay)', website: 'var(--channel-website)', inperson: 'var(--channel-inperson)',
  // Legacy fallbacks
  tcgplayer: 'var(--channel-ebay)', local: 'var(--channel-inperson)', other: 'var(--channel-inperson)',
  gamestop: 'var(--channel-inperson)', cardshow: 'var(--channel-inperson)',
};
```

Note: The CSS variables `--channel-inperson` may not exist yet. Check the theme CSS and add if needed, or fall back to an existing variable like `--channel-local`. The implementer should check `web/src/index.css` or equivalent for CSS variable definitions and add `--channel-inperson` if needed.

- [ ] **Step 2: Verify frontend builds**

Run: `cd /workspace/.worktrees/aireviewz/web && npm run build`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add web/src/react/components/insights/InsightsSection.tsx
git commit -m "feat(frontend): update InsightsSection chart colors for 3-channel model"
```

---

### Task 11: Update Campaign Analysis Skill

**Files:**
- Modify: `.claude/skills/campaign-analysis/SKILL.md`

- [ ] **Step 1: Update channel references in SKILL.md**

Replace lines 89-91 (Data conventions section):

Old:
```
- **Margin at 80% terms:** CL x 7.65% - $3 per card on eBay (12.35% fees)
- **PSA 7 cards have NO GameStop exit** (PSA 8-10 only, $1,500 cash cap)
```

New:
```
- **Margin at 80% terms:** CL x 7.65% - $3 per card on eBay (12.35% fees)
- **Exit channels:** eBay (12.35% fees), Website (3% fees), In Person/card shows (0% fees, 80-85% of market)
```

- [ ] **Step 2: Commit**

```bash
cd /workspace/.worktrees/aireviewz
git add .claude/skills/campaign-analysis/SKILL.md
git commit -m "docs: update campaign analysis skill for 3-channel model"
```

---

### Task 12: Run Full Test Suite and Fix Any Remaining Issues

- [ ] **Step 1: Run full Go test suite**

Run: `cd /workspace/.worktrees/aireviewz && go test ./... 2>&1 | tail -50`
Expected: All PASS

- [ ] **Step 2: Run frontend tests**

Run: `cd /workspace/.worktrees/aireviewz/web && npm test 2>&1 | tail -30`
Expected: All PASS

- [ ] **Step 3: Run quality checks**

Run: `cd /workspace/.worktrees/aireviewz && make check 2>&1 | tail -20`
Expected: PASS (no lint errors, no architecture violations, no oversized files)

- [ ] **Step 4: Fix any remaining failures**

If any tests reference old channel constants or GameStop-specific logic, update them to use the new 3-channel model.

- [ ] **Step 5: Final commit if fixes were needed**

```bash
cd /workspace/.worktrees/aireviewz
git add -A
git commit -m "fix: resolve remaining test failures from channel consolidation"
```
