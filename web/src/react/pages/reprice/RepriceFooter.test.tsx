import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import RepriceFooter from './RepriceFooter';
import type { LiquidationPreviewItem } from '../../../types/liquidation';

function makeItem(overrides: Partial<LiquidationPreviewItem> = {}): LiquidationPreviewItem {
  return {
    purchaseId: 'p1',
    certNumber: '',
    cardName: 'Test',
    setName: '',
    cardNumber: '',
    grade: 10,
    buyCostCents: 1000,
    clValueCents: 0,
    compPriceCents: 0,
    compCount: 0,
    mostRecentCompDate: '',
    confidenceLevel: 'low',
    gapPct: 0,
    currentReviewedPriceCents: 0,
    suggestedPriceCents: 0,
    belowCost: false,
    ...overrides,
  };
}

describe('RepriceFooter', () => {
  it('renders nothing when items is empty', () => {
    const { container } = render(
      <RepriceFooter items={[]} selectedCount={0} applyableCount={0} onAcceptBucket={vi.fn()} onDeselectAll={vi.fn()} onApply={vi.fn()} />
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders three buckets with correct counts', () => {
    const items = [
      makeItem({ purchaseId: 'a', belowCost: true }),
      makeItem({ purchaseId: 'b', belowCost: true }),
      makeItem({ purchaseId: 'c', belowCost: false, compCount: 3 }),
      makeItem({ purchaseId: 'd', belowCost: false, compCount: 0 }),
    ];
    render(
      <RepriceFooter items={items} selectedCount={0} applyableCount={0} onAcceptBucket={vi.fn()} onDeselectAll={vi.fn()} onApply={vi.fn()} />
    );
    expect(screen.getByRole('button', { name: /Accept 2 below cost/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Accept 1 with comps/ })).toBeInTheDocument();
    expect(screen.getByText(/1 skipped \(no data\)/)).toBeInTheDocument();
  });

  it('disables a bucket button when its count is zero', () => {
    const items = [makeItem({ belowCost: false, compCount: 5 })];
    render(
      <RepriceFooter items={items} selectedCount={0} applyableCount={0} onAcceptBucket={vi.fn()} onDeselectAll={vi.fn()} onApply={vi.fn()} />
    );
    expect(screen.getByRole('button', { name: /Accept 0 below cost/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: /Accept 1 with comps/ })).toBeEnabled();
  });

  it('clicking a bucket calls onAcceptBucket with the bucket name', async () => {
    const onAcceptBucket = vi.fn();
    const items = [makeItem({ belowCost: true })];
    render(
      <RepriceFooter items={items} selectedCount={0} applyableCount={0} onAcceptBucket={onAcceptBucket} onDeselectAll={vi.fn()} onApply={vi.fn()} />
    );
    await userEvent.click(screen.getByRole('button', { name: /Accept 1 below cost/ }));
    expect(onAcceptBucket).toHaveBeenCalledWith('belowCost');
  });

  it('shows selected count', () => {
    render(
      <RepriceFooter items={[makeItem()]} selectedCount={7} applyableCount={5} onAcceptBucket={vi.fn()} onDeselectAll={vi.fn()} onApply={vi.fn()} />
    );
    expect(screen.getByText(/Selected: 7/)).toBeInTheDocument();
  });

  it('Apply Prices is disabled when applyableCount is zero', () => {
    render(
      <RepriceFooter items={[makeItem()]} selectedCount={3} applyableCount={0} onAcceptBucket={vi.fn()} onDeselectAll={vi.fn()} onApply={vi.fn()} />
    );
    expect(screen.getByRole('button', { name: /Apply Prices/ })).toBeDisabled();
  });

  it('Deselect All calls onDeselectAll', async () => {
    const onDeselectAll = vi.fn();
    render(
      <RepriceFooter items={[makeItem()]} selectedCount={3} applyableCount={2} onAcceptBucket={vi.fn()} onDeselectAll={onDeselectAll} onApply={vi.fn()} />
    );
    await userEvent.click(screen.getByRole('button', { name: /Deselect All/ }));
    expect(onDeselectAll).toHaveBeenCalledTimes(1);
  });

  it('Apply Prices calls onApply', async () => {
    const onApply = vi.fn();
    render(
      <RepriceFooter items={[makeItem()]} selectedCount={3} applyableCount={2} onAcceptBucket={vi.fn()} onDeselectAll={vi.fn()} onApply={onApply} />
    );
    await userEvent.click(screen.getByRole('button', { name: /Apply Prices/ }));
    expect(onApply).toHaveBeenCalledTimes(1);
  });
});
