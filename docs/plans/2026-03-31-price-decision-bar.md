# PriceDecisionBar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract a shared `PriceDecisionBar` component to replace 4 inconsistent price selection UIs across eBay export, Shopify sync, inventory review, and price override screens.

**Architecture:** A single React component (`PriceDecisionBar`) renders inline quick-pick source buttons (CL, Market, Cost, Last Sold) + custom $ input + Confirm/Skip actions. Backend APIs are extended with additive fields so the frontend has all price sources available. Consumers build a `sources` array and pass it to the shared component.

**Tech Stack:** React + TypeScript (frontend), Go (backend), @testing-library/react (frontend tests), Go stdlib testing (backend tests)

**Spec:** `docs/specs/2026-03-31-price-decision-bar-design.md`

---

### Task 1: Backend — Add fields to EbayExportItem

**Files:**
- Modify: `internal/domain/campaigns/ebay_types.go:23-39`
- Modify: `internal/domain/campaigns/service_ebay_export.go:27-58`
- Modify: `internal/domain/campaigns/service_export_ebay_test.go`

- [ ] **Step 1: Write tests for the new EbayExportItem fields**

Add a test that verifies `CostBasisCents`, `LastSoldCents`, `ReviewedPriceCents`, and `ReviewedAt` are populated on the export item. Add to `service_export_ebay_test.go`:

```go
func TestListEbayExportItems_IncludesNewPriceFields(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	repo.campaigns["c1"] = &Campaign{ID: "c1", Phase: PhaseActive}
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CampaignID: "c1", CertNumber: "111",
		CardName: "Charizard", SetName: "Base Set",
		CardNumber: "4", CardYear: "1999",
		GradeValue: 8, Grader: "PSA",
		CLValueCents:       25000,
		BuyCostCents:       10000,
		PSASourcingFeeCents: 300,
		ReviewedPriceCents: 24000,
		ReviewedAt:         "2026-03-30T10:00:00Z",
		EbayExportFlaggedAt: &now,
		MarketSnapshotData: MarketSnapshotData{
			MedianCents:   27500,
			LastSoldCents: 26000,
		},
	}

	svc := &service{repo: repo, idGen: func() string { return "id" }}
	resp, err := svc.ListEbayExportItems(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	item := resp.Items[0]
	if item.CostBasisCents != 10300 {
		t.Errorf("CostBasisCents = %d, want 10300", item.CostBasisCents)
	}
	if item.LastSoldCents != 26000 {
		t.Errorf("LastSoldCents = %d, want 26000", item.LastSoldCents)
	}
	if item.ReviewedPriceCents != 24000 {
		t.Errorf("ReviewedPriceCents = %d, want 24000", item.ReviewedPriceCents)
	}
	if item.ReviewedAt != "2026-03-30T10:00:00Z" {
		t.Errorf("ReviewedAt = %q, want 2026-03-30T10:00:00Z", item.ReviewedAt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/campaigns/ -run TestListEbayExportItems_IncludesNewPriceFields -v`
Expected: FAIL — `item.CostBasisCents undefined` (field doesn't exist yet)

- [ ] **Step 3: Add fields to EbayExportItem struct**

In `internal/domain/campaigns/ebay_types.go`, add 4 fields to `EbayExportItem` after `BackImageURL`:

```go
type EbayExportItem struct {
	PurchaseID          string  `json:"purchaseId"`
	CertNumber          string  `json:"certNumber"`
	CardName            string  `json:"cardName"`
	SetName             string  `json:"setName"`
	CardNumber          string  `json:"cardNumber"`
	CardYear            string  `json:"cardYear"`
	GradeValue          float64 `json:"gradeValue"`
	Grader              string  `json:"grader"`
	CLValueCents        int     `json:"clValueCents"`
	MarketMedianCents   int     `json:"marketMedianCents"`
	SuggestedPriceCents int     `json:"suggestedPriceCents"`
	HasCLValue          bool    `json:"hasCLValue"`
	HasMarketData       bool    `json:"hasMarketData"`
	FrontImageURL       string  `json:"frontImageUrl,omitempty"`
	BackImageURL        string  `json:"backImageUrl,omitempty"`
	CostBasisCents      int     `json:"costBasisCents"`
	LastSoldCents       int     `json:"lastSoldCents"`
	ReviewedPriceCents  int     `json:"reviewedPriceCents,omitempty"`
	ReviewedAt          string  `json:"reviewedAt,omitempty"`
}
```

- [ ] **Step 4: Populate new fields in ListEbayExportItems**

In `internal/domain/campaigns/service_ebay_export.go`, update the `items = append(items, EbayExportItem{...})` block (lines 41-57) to include the new fields:

```go
		items = append(items, EbayExportItem{
			PurchaseID:          p.ID,
			CertNumber:          p.CertNumber,
			CardName:            p.CardName,
			SetName:             p.SetName,
			CardNumber:          p.CardNumber,
			CardYear:            p.CardYear,
			GradeValue:          p.GradeValue,
			Grader:              p.Grader,
			CLValueCents:        p.CLValueCents,
			MarketMedianCents:   p.MedianCents,
			SuggestedPriceCents: suggested,
			HasCLValue:          hasCL,
			HasMarketData:       hasMarket,
			FrontImageURL:       p.FrontImageURL,
			BackImageURL:        p.BackImageURL,
			CostBasisCents:      p.BuyCostCents + p.PSASourcingFeeCents,
			LastSoldCents:       p.LastSoldCents,
			ReviewedPriceCents:  p.ReviewedPriceCents,
			ReviewedAt:          p.ReviewedAt,
		})
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/domain/campaigns/ -run TestListEbayExportItems -v`
Expected: All `TestListEbayExportItems*` tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/campaigns/ebay_types.go internal/domain/campaigns/service_ebay_export.go internal/domain/campaigns/service_export_ebay_test.go
git commit -m "feat: add cost basis, last sold, reviewed fields to eBay export"
```

---

### Task 2: Backend — Add LastSoldCents to ShopifyPriceSyncMatch

**Files:**
- Modify: `internal/domain/campaigns/analytics_types.go:157-179`
- Modify: `internal/domain/campaigns/service_sell_sheet.go:264-284`

- [ ] **Step 1: Add `LastSoldCents` field to the Go struct**

In `internal/domain/campaigns/analytics_types.go`, add `LastSoldCents` to `ShopifyPriceSyncMatch` after `MarketPriceCents`:

```go
	MarketPriceCents      int            `json:"marketPriceCents"`
	LastSoldCents         int            `json:"lastSoldCents"`
	Recommendation        string         `json:"recommendation"`
```

- [ ] **Step 2: Populate the field in MatchShopifyPrices**

In `internal/domain/campaigns/service_sell_sheet.go`, inside the `if sellItem.CurrentMarket != nil` block (line 282), add `LastSoldCents`:

```go
		if sellItem.CurrentMarket != nil {
			match.MarketPriceCents = sellItem.CurrentMarket.MedianCents
			match.LastSoldCents = sellItem.CurrentMarket.LastSoldCents
		}
```

- [ ] **Step 3: Run all backend tests**

Run: `go test ./internal/domain/campaigns/ -v -count=1`
Expected: All tests PASS (additive field, no breakage)

- [ ] **Step 4: Commit**

```bash
git add internal/domain/campaigns/analytics_types.go internal/domain/campaigns/service_sell_sheet.go
git commit -m "feat: add lastSoldCents to Shopify price sync response"
```

---

### Task 3: Frontend — Update TypeScript types

**Files:**
- Modify: `web/src/types/campaigns/core.ts`

- [ ] **Step 1: Add fields to EbayExportItem**

In `web/src/types/campaigns/core.ts`, add 4 fields to `EbayExportItem` after `backImageUrl`:

```typescript
export interface EbayExportItem {
  purchaseId: string;
  certNumber: string;
  cardName: string;
  setName: string;
  cardNumber: string;
  cardYear: string;
  gradeValue: number;
  grader: string;
  clValueCents: number;
  marketMedianCents: number;
  suggestedPriceCents: number;
  hasCLValue: boolean;
  hasMarketData: boolean;
  frontImageUrl?: string;
  backImageUrl?: string;
  costBasisCents: number;
  lastSoldCents: number;
  reviewedPriceCents?: number;
  reviewedAt?: string;
}
```

- [ ] **Step 2: Add `lastSoldCents` to ShopifyPriceSyncMatch**

In the same file, add `lastSoldCents` to `ShopifyPriceSyncMatch` after `marketPriceCents`:

```typescript
export interface ShopifyPriceSyncMatch {
  certNumber: string;
  cardName: string;
  setName?: string;
  cardNumber?: string;
  grade: number;
  grader?: string;
  currentPriceCents: number;
  suggestedPriceCents: number;
  minimumPriceCents: number;
  costBasisCents: number;
  clValueCents: number;
  marketPriceCents: number;
  lastSoldCents: number;
  recommendation: string;
  priceDeltaPct: number;
  hasMarketData: boolean;
  overridePriceCents?: number;
  overrideSource?: string;
  aiSuggestedPriceCents?: number;
  recommendedPriceCents: number;
  recommendedSource: string;
  reviewedAt?: string;
}
```

- [ ] **Step 3: Verify the frontend builds**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add web/src/types/campaigns/core.ts
git commit -m "feat: add price source fields to eBay export and Shopify sync types"
```

---

### Task 4: Frontend — Create PriceDecisionBar component

**Files:**
- Create: `web/src/react/ui/PriceDecisionBar.tsx`
- Create: `web/src/react/ui/PriceDecisionBar.test.tsx`
- Modify: `web/src/react/ui/index.ts`

- [ ] **Step 1: Write failing tests for PriceDecisionBar**

Create `web/src/react/ui/PriceDecisionBar.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PriceDecisionBar from './PriceDecisionBar';
import type { PriceSource } from './PriceDecisionBar';

const sources: PriceSource[] = [
  { label: 'CL', priceCents: 28500, source: 'cl' },
  { label: 'Market', priceCents: 26000, source: 'market' },
  { label: 'Cost', priceCents: 14250, source: 'cost_basis' },
  { label: 'Last Sold', priceCents: 27000, source: 'last_sold' },
];

describe('PriceDecisionBar', () => {
  it('renders all source buttons with formatted prices', () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} />);
    expect(screen.getByText(/CL/)).toBeInTheDocument();
    expect(screen.getByText(/\$285\.00/)).toBeInTheDocument();
    expect(screen.getByText(/Market/)).toBeInTheDocument();
    expect(screen.getByText(/\$260\.00/)).toBeInTheDocument();
    expect(screen.getByText(/Cost/)).toBeInTheDocument();
    expect(screen.getByText(/\$142\.50/)).toBeInTheDocument();
    expect(screen.getByText(/Last Sold/)).toBeInTheDocument();
    expect(screen.getByText(/\$270\.00/)).toBeInTheDocument();
  });

  it('disables buttons with 0 price and shows dash', () => {
    const withZero: PriceSource[] = [
      { label: 'CL', priceCents: 0, source: 'cl' },
      { label: 'Cost', priceCents: 14250, source: 'cost_basis' },
    ];
    render(<PriceDecisionBar sources={withZero} onConfirm={() => {}} />);
    const clButton = screen.getByRole('button', { name: /CL/ });
    expect(clButton).toBeDisabled();
    expect(clButton).toHaveTextContent('—');
  });

  it('pre-selects the specified source on mount', () => {
    render(<PriceDecisionBar sources={sources} preSelected="cl" onConfirm={() => {}} />);
    const input = screen.getByPlaceholderText('0.00') as HTMLInputElement;
    expect(input.value).toBe('285.00');
  });

  it('calls onConfirm with selected source price', async () => {
    const onConfirm = vi.fn();
    render(<PriceDecisionBar sources={sources} preSelected="cl" onConfirm={onConfirm} />);
    await userEvent.click(screen.getByRole('button', { name: /Confirm/ }));
    expect(onConfirm).toHaveBeenCalledWith(28500, 'cl');
  });

  it('calls onConfirm with custom value as manual source', async () => {
    const onConfirm = vi.fn();
    render(<PriceDecisionBar sources={sources} onConfirm={onConfirm} />);
    const input = screen.getByPlaceholderText('0.00');
    await userEvent.clear(input);
    await userEvent.type(input, '300.00');
    await userEvent.click(screen.getByRole('button', { name: /Confirm/ }));
    expect(onConfirm).toHaveBeenCalledWith(30000, 'manual');
  });

  it('clicking a source button syncs the dollar input', async () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} />);
    await userEvent.click(screen.getByRole('button', { name: /Market/ }));
    const input = screen.getByPlaceholderText('0.00') as HTMLInputElement;
    expect(input.value).toBe('260.00');
  });

  it('typing in input clears source selection', async () => {
    const onConfirm = vi.fn();
    render(<PriceDecisionBar sources={sources} preSelected="cl" onConfirm={onConfirm} />);
    const input = screen.getByPlaceholderText('0.00');
    await userEvent.clear(input);
    await userEvent.type(input, '999.00');
    await userEvent.click(screen.getByRole('button', { name: /Confirm/ }));
    expect(onConfirm).toHaveBeenCalledWith(99900, 'manual');
  });

  it('shows Skip button when onSkip is provided', () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} onSkip={() => {}} />);
    expect(screen.getByRole('button', { name: /Skip/ })).toBeInTheDocument();
  });

  it('does not show Skip button when onSkip is not provided', () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} />);
    expect(screen.queryByRole('button', { name: /Skip/ })).not.toBeInTheDocument();
  });

  it('shows Flag Price Issue button when onFlag is provided', () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} onFlag={() => {}} />);
    expect(screen.getByRole('button', { name: /Flag Price Issue/ })).toBeInTheDocument();
  });

  it('renders accepted state with locked price and Change button', () => {
    const onReset = vi.fn();
    render(
      <PriceDecisionBar sources={sources} preSelected="cl" status="accepted" onConfirm={() => {}} onReset={onReset} />
    );
    expect(screen.getByText(/\$285\.00/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Change/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Confirm/ })).not.toBeInTheDocument();
  });

  it('Change button calls onReset', async () => {
    const onReset = vi.fn();
    render(
      <PriceDecisionBar sources={sources} preSelected="cl" status="accepted" onConfirm={() => {}} onReset={onReset} />
    );
    await userEvent.click(screen.getByRole('button', { name: /Change/ }));
    expect(onReset).toHaveBeenCalled();
  });

  it('renders skipped state with Undo button', () => {
    const onReset = vi.fn();
    render(
      <PriceDecisionBar sources={sources} status="skipped" onConfirm={() => {}} onSkip={() => {}} onReset={onReset} />
    );
    expect(screen.getByText(/Skipped/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Undo/ })).toBeInTheDocument();
  });

  it('Undo button calls onReset', async () => {
    const onReset = vi.fn();
    render(
      <PriceDecisionBar sources={sources} status="skipped" onConfirm={() => {}} onSkip={() => {}} onReset={onReset} />
    );
    await userEvent.click(screen.getByRole('button', { name: /Undo/ }));
    expect(onReset).toHaveBeenCalled();
  });

  it('disables all controls when disabled prop is true', () => {
    render(<PriceDecisionBar sources={sources} preSelected="cl" disabled onConfirm={() => {}} />);
    const buttons = screen.getAllByRole('button');
    buttons.forEach(btn => expect(btn).toBeDisabled());
    expect(screen.getByPlaceholderText('0.00')).toBeDisabled();
  });

  it('Enter in input triggers confirm', async () => {
    const onConfirm = vi.fn();
    render(<PriceDecisionBar sources={sources} onConfirm={onConfirm} />);
    const input = screen.getByPlaceholderText('0.00');
    await userEvent.type(input, '500.00{Enter}');
    expect(onConfirm).toHaveBeenCalledWith(50000, 'manual');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/react/ui/PriceDecisionBar.test.tsx`
Expected: FAIL — module `./PriceDecisionBar` not found

- [ ] **Step 3: Implement PriceDecisionBar**

Create `web/src/react/ui/PriceDecisionBar.tsx`:

```tsx
import { useState, useEffect } from 'react';
import { Button } from '../ui';
import { formatCents, dollarsToCents, centsToDollars } from '../utils/formatters';

export interface PriceSource {
  label: string;
  priceCents: number;
  source: string;
}

export interface PriceDecisionBarProps {
  sources: PriceSource[];
  preSelected?: string;
  onConfirm: (priceCents: number, source: string) => void;
  onSkip?: () => void;
  onFlag?: () => void;
  status?: 'pending' | 'accepted' | 'skipped';
  disabled?: boolean;
  isSubmitting?: boolean;
  confirmLabel?: string;
  /** Called when user clicks "Change" (accepted) or "Undo" (skipped) to return to pending */
  onReset?: () => void;
}

export default function PriceDecisionBar({
  sources,
  preSelected,
  onConfirm,
  onSkip,
  onFlag,
  status = 'pending',
  disabled = false,
  isSubmitting = false,
  confirmLabel = 'Confirm',
  onReset,
}: PriceDecisionBarProps) {
  const [selectedSource, setSelectedSource] = useState<string | null>(null);
  const [customValue, setCustomValue] = useState('');

  // Pre-select on mount or when preSelected changes
  useEffect(() => {
    if (preSelected) {
      const match = sources.find(s => s.source === preSelected && s.priceCents > 0);
      if (match) {
        setSelectedSource(match.source);
        setCustomValue(centsToDollars(match.priceCents));
      }
    }
  }, [preSelected, sources]);

  const handleSourceClick = (src: PriceSource) => {
    setSelectedSource(src.source);
    setCustomValue(centsToDollars(src.priceCents));
  };

  const handleCustomChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setCustomValue(e.target.value);
    setSelectedSource(null);
  };

  const getConfirmValues = (): { priceCents: number; source: string } | null => {
    if (selectedSource) {
      const match = sources.find(s => s.source === selectedSource);
      if (match && match.priceCents > 0) {
        return { priceCents: match.priceCents, source: match.source };
      }
    }
    const cents = dollarsToCents(customValue);
    if (cents > 0) {
      return { priceCents: cents, source: 'manual' };
    }
    return null;
  };

  const handleConfirm = () => {
    const values = getConfirmValues();
    if (values) {
      onConfirm(values.priceCents, values.source);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      handleConfirm();
    }
  };

  const hasSelection = selectedSource !== null || (customValue !== '' && dollarsToCents(customValue) > 0);
  const allDisabled = disabled || isSubmitting;

  // --- Accepted state ---
  if (status === 'accepted') {
    const values = getConfirmValues();
    const displayCents = values?.priceCents ?? 0;
    return (
      <div className="flex items-center gap-3 flex-wrap opacity-60">
        <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>
        {sources.map(src => (
          <button
            key={src.source}
            type="button"
            disabled
            className={`text-xs px-2.5 py-1.5 rounded-md border transition-colors ${
              selectedSource === src.source
                ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]'
                : 'border-[var(--border)] text-[var(--text-muted)]'
            } disabled:cursor-not-allowed`}
          >
            {src.label} {src.priceCents > 0 ? formatCents(src.priceCents) : '\u2014'}
          </button>
        ))}
        <span className="text-xs px-2.5 py-1.5 rounded-md bg-[var(--success)]/15 text-[var(--success)] font-medium border border-[var(--success)]/30">
          &#10003; {formatCents(displayCents)}
        </span>
        {onReset && (
          <Button variant="ghost" size="sm" onClick={onReset} disabled={disabled}>
            Change
          </Button>
        )}
      </div>
    );
  }

  // --- Skipped state ---
  if (status === 'skipped') {
    return (
      <div className="flex items-center gap-3 flex-wrap opacity-50">
        <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>
        {sources.map(src => (
          <button key={src.source} type="button" disabled
            className="text-xs px-2.5 py-1.5 rounded-md border border-[var(--border)] text-[var(--text-muted)] disabled:cursor-not-allowed">
            {src.label} {src.priceCents > 0 ? formatCents(src.priceCents) : '\u2014'}
          </button>
        ))}
        <span className="text-xs text-[var(--text-muted)] italic">Skipped</span>
        {onReset && (
          <Button variant="ghost" size="sm" onClick={onReset} disabled={disabled}>
            Undo
          </Button>
        )}
      </div>
    );
  }

  // --- Pending state (default) ---
  return (
    <div className="flex items-center gap-3 flex-wrap">
      <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>

      {sources.map(src => (
        <button
          key={src.source}
          type="button"
          onClick={() => handleSourceClick(src)}
          disabled={allDisabled || src.priceCents === 0}
          className={`text-xs px-2.5 py-1.5 rounded-md border transition-colors ${
            selectedSource === src.source
              ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]'
              : 'border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]'
          } disabled:opacity-40 disabled:cursor-not-allowed`}
        >
          {src.label} {src.priceCents > 0 ? formatCents(src.priceCents) : '\u2014'}
        </button>
      ))}

      <div className="flex items-center gap-1.5">
        <span className="text-[var(--text-muted)] text-xs">$</span>
        <input
          type="text"
          inputMode="decimal"
          placeholder="0.00"
          value={customValue}
          onChange={handleCustomChange}
          onKeyDown={handleKeyDown}
          disabled={allDisabled}
          className="w-20 px-2 py-1.5 text-xs rounded-md border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:border-[var(--accent)] disabled:opacity-40"
        />
      </div>

      <Button
        variant="success"
        size="sm"
        onClick={handleConfirm}
        disabled={!hasSelection || allDisabled}
        loading={isSubmitting}
      >
        {confirmLabel}
      </Button>

      {onSkip && (
        <Button variant="ghost" size="sm" onClick={onSkip} disabled={allDisabled}>
          Skip
        </Button>
      )}

      {onFlag && (
        <div className="ml-auto">
          <Button variant="danger" size="sm" onClick={onFlag} disabled={allDisabled}>
            Flag Price Issue
          </Button>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Export from ui/index.ts**

Add to `web/src/react/ui/index.ts` in the "Interactive" section:

```typescript
// Price Decision
export { default as PriceDecisionBar } from './PriceDecisionBar';
export type { PriceSource, PriceDecisionBarProps } from './PriceDecisionBar';
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd web && npx vitest run src/react/ui/PriceDecisionBar.test.tsx`
Expected: All tests PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/react/ui/PriceDecisionBar.tsx web/src/react/ui/PriceDecisionBar.test.tsx web/src/react/ui/index.ts
git commit -m "feat: create PriceDecisionBar shared component"
```

---

### Task 5: Frontend — Integrate PriceDecisionBar into Inventory Review

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx`
- Delete: `web/src/react/pages/campaign-detail/inventory/ReviewActionBar.tsx`

- [ ] **Step 1: Replace ReviewActionBar with PriceDecisionBar in ExpandedDetail**

Replace the full contents of `web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx`:

```tsx
import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../../types/campaigns';
import { api } from '../../../../js/api';
import { useToast } from '../../../contexts/ToastContext';
import { queryKeys } from '../../../queries/queryKeys';
import PriceSignalCard from './PriceSignalCard';
import PriceDecisionBar from '../../../ui/PriceDecisionBar';
import type { PriceSource } from '../../../ui/PriceDecisionBar';

interface ExpandedDetailProps {
  item: AgingItem;
  onReviewed?: () => void;
  campaignId?: string;
  onOpenFlagDialog?: () => void;
}

export default function ExpandedDetail({ item, onReviewed, campaignId, onOpenFlagDialog }: ExpandedDetailProps) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [isSubmitting, setIsSubmitting] = useState(false);

  const snap = item.currentMarket;
  const purchase = item.purchase;
  const costBasis = purchase.buyCostCents + purchase.psaSourcingFeeCents;

  const clCents = purchase.clValueCents;
  const marketCents = snap?.medianCents ?? 0;
  const lastSoldCents = snap?.lastSoldCents ?? 0;

  const sources: PriceSource[] = [
    { label: 'CL', priceCents: clCents, source: 'cl' },
    { label: 'Market', priceCents: marketCents, source: 'market' },
    { label: 'Cost', priceCents: costBasis, source: 'cost_basis' },
    { label: 'Last Sold', priceCents: lastSoldCents, source: 'last_sold' },
  ];

  // Pre-selection priority: reviewed > cl > market > cost
  let preSelected: string | undefined;
  if (purchase.reviewedPriceCents && purchase.reviewedPriceCents > 0) {
    const matchingSource = sources.find(s => s.priceCents === purchase.reviewedPriceCents && s.priceCents > 0);
    preSelected = matchingSource?.source;
  }
  if (!preSelected) {
    if (clCents > 0) preSelected = 'cl';
    else if (marketCents > 0) preSelected = 'market';
    else if (costBasis > 0) preSelected = 'cost_basis';
  }

  const invalidateQueries = () => {
    if (campaignId) {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.inventory(campaignId) });
    } else {
      queryClient.invalidateQueries({
        predicate: (query) => query.queryKey[0] === 'campaigns' && query.queryKey[2] === 'inventory',
      });
    }
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
  };

  const handleConfirm = async (priceCents: number, source: string) => {
    setIsSubmitting(true);
    try {
      await api.setReviewedPrice(purchase.id, priceCents, source);
      toast.success('Reviewed price saved');
      invalidateQueries();
      onReviewed?.();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to save reviewed price';
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleCardSelect = (source: string) => {
    // PriceSignalCard click still highlights — but the PriceDecisionBar
    // handles its own selection state internally now.
    // This handler is kept for the visual feedback on PriceSignalCards.
  };

  return (
    <div className="glass-vrow-expanded px-6 py-4 border-t border-[rgba(255,255,255,0.05)]">
      {/* 3x2 price signal grid */}
      <div className="grid grid-cols-3 gap-3 mb-4">
        <PriceSignalCard label="Cost Basis" valueCents={costBasis} />
        <PriceSignalCard label="Card Ladder" valueCents={clCents} />
        <PriceSignalCard
          label="Market (Median)"
          valueCents={marketCents}
          highlight={marketCents > 0 && marketCents > costBasis ? 'success' : marketCents > 0 && marketCents < costBasis ? 'danger' : undefined}
        />
        <PriceSignalCard label="Last Sold" valueCents={lastSoldCents} />
        <PriceSignalCard label="Lowest eBay Listing" valueCents={snap?.lowestListCents ?? 0} />
        <PriceSignalCard
          label="Current Override"
          valueCents={purchase.overridePriceCents ?? 0}
          highlight={purchase.overridePriceCents ? 'warning' : 'muted'}
        />
      </div>

      {/* Price decision bar */}
      <PriceDecisionBar
        sources={sources}
        preSelected={preSelected}
        onConfirm={handleConfirm}
        onFlag={onOpenFlagDialog}
        isSubmitting={isSubmitting}
      />
    </div>
  );
}
```

Key changes vs. current:
- Removed `ReviewActionBar` import, replaced with `PriceDecisionBar`
- Added `Cost` to the sources array (was missing before)
- Added pre-selection priority logic
- Removed `selectedPick` state — `PriceDecisionBar` manages its own selection state
- Removed `onClick` and `selected` props from `PriceSignalCard` — the decision bar handles selection now, avoiding dual-state confusion

- [ ] **Step 2: Delete ReviewActionBar**

Delete `web/src/react/pages/campaign-detail/inventory/ReviewActionBar.tsx`

- [ ] **Step 3: Check for any other imports of ReviewActionBar**

Run: `grep -r "ReviewActionBar" web/src/` — should only find the deleted file reference in git. If any other file imports it, update that import to use `PriceDecisionBar` instead.

- [ ] **Step 4: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx web/src/react/ui/index.ts
git rm web/src/react/pages/campaign-detail/inventory/ReviewActionBar.tsx
git commit -m "refactor: replace ReviewActionBar with PriceDecisionBar in inventory"
```

---

### Task 6: Frontend — Integrate PriceDecisionBar into eBay Export

**Files:**
- Modify: `web/src/react/pages/tools/EbayExportTab.tsx`

- [ ] **Step 1: Rewrite EbayExportTab to use PriceDecisionBar**

Replace the full contents of `web/src/react/pages/tools/EbayExportTab.tsx`:

```tsx
import { useState, useCallback, useEffect, useRef, useMemo } from 'react';
import { api } from '@/js/api';
import type { EbayExportItem, EbayExportGenerateItem } from '@/types/campaigns/core';
import { centsToDollars } from '@/react/utils/formatters';
import PriceDecisionBar from '@/react/ui/PriceDecisionBar';
import type { PriceSource } from '@/react/ui/PriceDecisionBar';

type Decision = { action: 'accept'; priceCents: number; source: string } | { action: 'skip' };
type Phase = 'review' | 'export';

function buildSources(item: EbayExportItem): PriceSource[] {
  return [
    { label: 'CL', priceCents: item.clValueCents, source: 'cl' },
    { label: 'Market', priceCents: item.marketMedianCents, source: 'market' },
    { label: 'Cost', priceCents: item.costBasisCents, source: 'cost_basis' },
    { label: 'Last Sold', priceCents: item.lastSoldCents, source: 'last_sold' },
  ];
}

function preSelectSource(item: EbayExportItem): string | undefined {
  const sources = buildSources(item);
  // Priority: reviewed > cl > market > cost
  if (item.reviewedPriceCents && item.reviewedPriceCents > 0) {
    const match = sources.find(s => s.priceCents === item.reviewedPriceCents && s.priceCents > 0);
    if (match) return match.source;
  }
  if (item.clValueCents > 0) return 'cl';
  if (item.marketMedianCents > 0) return 'market';
  if (item.costBasisCents > 0) return 'cost_basis';
  return undefined;
}

export default function EbayExportTab() {
  const [phase, setPhase] = useState<Phase>('review');
  const [items, setItems] = useState<EbayExportItem[]>([]);
  const [decisions, setDecisions] = useState<Map<string, Decision>>(new Map());
  const [flaggedOnly, setFlaggedOnly] = useState(true);
  const [loading, setLoading] = useState(false);
  const [exportCount, setExportCount] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const fetchControllerRef = useRef<AbortController | null>(null);

  const fetchItems = useCallback(async () => {
    fetchControllerRef.current?.abort();
    const controller = new AbortController();
    fetchControllerRef.current = controller;

    setLoading(true);
    setError(null);
    try {
      const resp = await api.listEbayExportItems(flaggedOnly);
      if (controller.signal.aborted) return;
      setItems(resp.items);
      setDecisions(new Map());
    } catch (err) {
      if (controller.signal.aborted) return;
      setError(err instanceof Error ? err.message : 'Failed to load items');
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [flaggedOnly]);

  useEffect(() => {
    fetchControllerRef.current?.abort();
    setItems([]);
    setDecisions(new Map());
    setError(null);
  }, [flaggedOnly]);

  const setDecision = (purchaseId: string, decision: Decision) => {
    setDecisions(prev => new Map(prev).set(purchaseId, decision));
  };

  const acceptAll = () => {
    const next = new Map(decisions);
    for (const item of items) {
      const existing = next.get(item.purchaseId);
      if (existing?.action === 'skip') continue;
      const sources = buildSources(item);
      const preKey = preSelectSource(item);
      const source = sources.find(s => s.source === preKey && s.priceCents > 0);
      if (source) {
        next.set(item.purchaseId, { action: 'accept', priceCents: source.priceCents, source: source.source });
      }
    }
    setDecisions(next);
  };

  const skipAll = () => {
    const next = new Map(decisions);
    for (const item of items) {
      next.set(item.purchaseId, { action: 'skip' });
    }
    setDecisions(next);
  };

  const handleExport = async () => {
    const exportItems: EbayExportGenerateItem[] = [];
    for (const [purchaseId, decision] of decisions) {
      if (decision.action === 'accept') {
        exportItems.push({ purchaseId, priceCents: decision.priceCents });
      }
    }
    if (exportItems.length === 0) return;

    setLoading(true);
    setError(null);
    try {
      const blob = await api.generateEbayCSV(exportItems);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'ebay_import.csv';
      a.click();
      URL.revokeObjectURL(url);
      setExportCount(exportItems.length);
      setPhase('export');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to generate CSV');
    } finally {
      setLoading(false);
    }
  };

  const acceptedCount = useMemo(() =>
    Array.from(decisions.values()).filter(d => d.action === 'accept').length,
    [decisions]
  );

  if (phase === 'export') {
    return (
      <div className="rounded border border-green-700 bg-green-900/20 p-6 text-center">
        <h3 className="text-lg font-medium text-green-300">Export Complete</h3>
        <p className="mt-2 text-sm text-gray-400">
          {exportCount} items exported to ebay_import.csv
        </p>
        <button
          onClick={() => { setPhase('review'); setItems([]); setDecisions(new Map()); }}
          className="mt-4 rounded bg-gray-700 px-4 py-2 text-sm text-gray-200 hover:bg-gray-600"
        >
          Start Over
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <label className="flex items-center gap-2 text-sm text-gray-300">
          <input
            type="checkbox"
            checked={flaggedOnly}
            onChange={e => setFlaggedOnly(e.target.checked)}
            className="rounded border-gray-600"
          />
          Flagged for export only
        </label>
        <button
          onClick={fetchItems}
          disabled={loading}
          className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
        >
          {loading ? 'Loading...' : items.length > 0 ? 'Refresh' : 'Load Items'}
        </button>
      </div>

      {error && (
        <div className="rounded border border-red-700 bg-red-900/30 p-3 text-sm text-red-300">
          {error}
        </div>
      )}

      {items.length > 0 && (
        <>
          <div className="flex items-center justify-between">
            <div className="flex gap-2">
              <button onClick={acceptAll} className="rounded bg-green-700 px-3 py-1 text-xs text-white hover:bg-green-600">
                Accept All
              </button>
              <button onClick={skipAll} className="rounded bg-gray-700 px-3 py-1 text-xs text-gray-200 hover:bg-gray-600">
                Skip All
              </button>
            </div>
            <div className="text-sm text-gray-400">
              {items.length} items · {acceptedCount} accepted
            </div>
          </div>

          <div className="space-y-2">
            {items.map(item => {
              const decision = decisions.get(item.purchaseId);
              const status: 'pending' | 'accepted' | 'skipped' =
                decision?.action === 'accept' ? 'accepted' :
                decision?.action === 'skip' ? 'skipped' : 'pending';

              return (
                <div key={item.purchaseId} className="rounded border border-[var(--border)] bg-[var(--surface-1)] p-3">
                  <div className="flex items-center gap-4 mb-2 text-sm">
                    <span className="font-medium text-[var(--text)]">{item.cardName}</span>
                    <span className="text-[var(--text-muted)]">{item.setName}</span>
                    <span className="text-[var(--text-muted)]">#{item.cardNumber}</span>
                    <span className="text-[var(--text)]">PSA {item.gradeValue}</span>
                    <span className="font-mono text-xs text-[var(--text-muted)]">{item.certNumber}</span>
                  </div>
                  <PriceDecisionBar
                    sources={buildSources(item)}
                    preSelected={preSelectSource(item)}
                    status={status}
                    onConfirm={(priceCents, source) => {
                      setDecision(item.purchaseId, { action: 'accept', priceCents, source });
                    }}
                    onSkip={() => setDecision(item.purchaseId, { action: 'skip' })}
                    onReset={() => {
                      setDecisions(prev => {
                        const next = new Map(prev);
                        next.delete(item.purchaseId);
                        return next;
                      });
                    }}
                  />
                </div>
              );
            })}
          </div>

          <div className="flex justify-end">
            <button
              onClick={handleExport}
              disabled={loading || acceptedCount === 0}
              className="rounded bg-green-600 px-6 py-2 text-sm font-medium text-white hover:bg-green-500 disabled:opacity-50"
            >
              {loading ? 'Generating...' : `Export eBay CSV (${acceptedCount} items)`}
            </button>
          </div>
        </>
      )}

      {!loading && items.length === 0 && (
        <p className="text-sm text-gray-500">
          Click &quot;Load Items&quot; to see inventory available for export.
        </p>
      )}
    </div>
  );
}
```

Key changes:
- Replaced `Accept/Edit/Skip` buttons and inline number input with `PriceDecisionBar`
- Removed `editingId`, `editPrice`, `confirmEdit`, `handleEdit` state — all handled by `PriceDecisionBar`
- Changed from table layout to card layout (each item is a card with the `PriceDecisionBar` below the card info) — this gives the quick-pick buttons horizontal space
- Decision type now tracks `source` alongside `priceCents`
- `acceptAll` respects pre-selection priority per item
- "Change" action (via `__change__` sentinel) clears the decision for re-editing

- [ ] **Step 2: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/src/react/pages/tools/EbayExportTab.tsx
git commit -m "refactor: replace eBay export inline editing with PriceDecisionBar"
```

---

### Task 7: Frontend — Integrate PriceDecisionBar into Shopify Sync

**Files:**
- Modify: `web/src/react/pages/ShopifySyncPage.tsx`

- [ ] **Step 1: Update ReviewRow to use PriceDecisionBar**

In `web/src/react/pages/ShopifySyncPage.tsx`, replace the `ReviewRow` function (lines 234-298) with:

```tsx
function ReviewRow({ match, decision, onDecide }: {
  match: ShopifyPriceSyncMatch;
  decision: ItemDecision | undefined;
  onDecide: (d: ItemDecision) => void;
}) {
  const sources: PriceSource[] = [
    { label: 'CL', priceCents: match.clValueCents, source: 'cl' },
    { label: 'Market', priceCents: match.marketPriceCents, source: 'market' },
    { label: 'Cost', priceCents: match.costBasisCents, source: 'cost_basis' },
    { label: 'Last Sold', priceCents: match.lastSoldCents ?? 0, source: 'last_sold' },
  ];

  // Pre-selection: reviewed > cl > market > cost
  let preSelected: string | undefined;
  if (match.recommendedSource === 'user_reviewed' && match.recommendedPriceCents > 0) {
    const matchingSrc = sources.find(s => s.priceCents === match.recommendedPriceCents && s.priceCents > 0);
    preSelected = matchingSrc?.source;
  }
  if (!preSelected) {
    if (match.clValueCents > 0) preSelected = 'cl';
    else if (match.marketPriceCents > 0) preSelected = 'market';
    else if (match.costBasisCents > 0) preSelected = 'cost_basis';
  }

  const status: 'pending' | 'accepted' | 'skipped' =
    decision?.action === 'update' ? 'accepted' :
    decision?.action === 'skip' ? 'skipped' : 'pending';

  return (
    <tr className={`border-b border-[var(--surface-2)]/50 ${
      status === 'accepted' ? 'bg-[var(--success-bg)]/30' :
      status === 'skipped' ? 'bg-[var(--surface-2)]/30' : ''
    }`}>
      <td className="py-2 px-2">
        <div className="text-sm font-medium text-[var(--text)]">{match.cardName}</div>
        {match.setName && (
          <div className="text-[10px] text-[var(--text-muted)]">
            {match.setName}{match.cardNumber ? ` #${match.cardNumber}` : ''}
          </div>
        )}
      </td>
      <td className="py-2 px-2 text-xs text-center text-[var(--text)]">
        {match.grader ? `${match.grader} ` : ''}{match.grade}
      </td>
      <td className="py-2 px-2 text-right text-sm text-[var(--text)]">{formatCents(match.currentPriceCents)}</td>
      <td className="py-2 px-2" colSpan={4}>
        <PriceDecisionBar
          sources={sources}
          preSelected={preSelected}
          status={status}
          confirmLabel="Update"
          onConfirm={(priceCents) => onDecide({ action: 'update', priceCents })}
          onSkip={() => onDecide({ action: 'skip' })}
          onReset={() => onDecide(undefined as unknown as ItemDecision)}
        />
      </td>
    </tr>
  );
}
```

Also add the imports at the top of the file:

```tsx
import PriceDecisionBar from '@/react/ui/PriceDecisionBar';
import type { PriceSource } from '@/react/ui/PriceDecisionBar';
```

- [ ] **Step 2: Update SectionTable headers**

Replace the `SectionTable` function's `<thead>` (lines 319-327) to match the new column layout:

```tsx
<thead>
  <tr className="border-b-2 border-[var(--surface-2)]">
    <th className="text-left py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Card</th>
    <th className="text-center py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Grade</th>
    <th className="text-right py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Store Price</th>
    <th className="text-left py-2 px-2 text-[var(--text-muted)] font-medium text-xs" colSpan={4}>Price Decision</th>
  </tr>
</thead>
```

- [ ] **Step 3: Update the `updateAll` callback**

Replace the `updateAll` callback (lines 437-446) to use pre-selection logic:

```tsx
  const updateAll = useCallback(() => {
    const next = new Map(decisions);
    for (const m of allMismatches) {
      const existing = next.get(m.certNumber);
      if (existing?.action === 'skip') continue;
      // Use pre-selection priority: reviewed > cl > market > cost
      let priceCents = 0;
      if (m.recommendedSource === 'user_reviewed' && m.recommendedPriceCents > 0) {
        priceCents = m.recommendedPriceCents;
      } else if (m.clValueCents > 0) {
        priceCents = m.clValueCents;
      } else if (m.marketPriceCents > 0) {
        priceCents = m.marketPriceCents;
      } else if (m.costBasisCents > 0) {
        priceCents = m.costBasisCents;
      }
      if (priceCents > 0) {
        next.set(m.certNumber, { action: 'update', priceCents });
      }
    }
    setDecisions(next);
  }, [allMismatches, decisions]);
```

- [ ] **Step 4: Handle "Change" action in onDecide**

The `onReset` callback in `ReviewRow` passes `undefined` to `onDecide`, so `setDecisionFor` needs to handle `undefined` by deleting the decision. Update the `setDecisionFor` callback (lines 428-434):

```tsx
  const setDecisionFor = useCallback((certNumber: string, decision: ItemDecision) => {
    setDecisions(prev => {
      const next = new Map(prev);
      if (!decision) {
        next.delete(certNumber);
      } else {
        next.set(certNumber, decision);
      }
      return next;
    });
  }, []);
```

- [ ] **Step 5: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/ShopifySyncPage.tsx
git commit -m "refactor: replace Shopify sync Update/Skip buttons with PriceDecisionBar"
```

---

### Task 8: Verify and clean up

**Files:** All modified files

- [ ] **Step 1: Run all backend tests**

Run: `go test ./internal/domain/campaigns/ -v -count=1`
Expected: All tests PASS

- [ ] **Step 2: Run all frontend tests**

Run: `cd web && npm test`
Expected: All tests PASS

- [ ] **Step 3: Verify build**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 4: Verify no stale imports of ReviewActionBar**

Run: `grep -r "ReviewActionBar" web/src/`
Expected: No results (file deleted, no remaining imports)

- [ ] **Step 5: Run Go linting**

Run: `make check`
Expected: All checks pass

- [ ] **Step 6: Final commit if any cleanup was needed**

```bash
git add -A
git commit -m "chore: final cleanup after PriceDecisionBar migration"
```
