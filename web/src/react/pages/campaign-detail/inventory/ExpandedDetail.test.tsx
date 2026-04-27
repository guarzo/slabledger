import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import ExpandedDetail from './ExpandedDetail';
import { ToastProvider } from '../../../contexts/ToastContext';
import type { AgingItem } from '../../../../types/campaigns';

vi.mock('../../../../js/api', async () => {
  const actual = await vi.importActual<typeof import('../../../../js/api')>('../../../../js/api');
  return {
    ...actual,
    api: {
      setReviewedPrice: vi.fn().mockResolvedValue({ success: true, reviewedAt: '2026-04-21T00:00:00Z' }),
      listPurchaseOnDH: vi.fn().mockResolvedValue({ listed: 1, synced: 1, skipped: 0, total: 1 }),
    },
    isAPIError: actual.isAPIError,
  };
});

function makeItem(overrides?: Partial<AgingItem>): AgingItem {
  return {
    purchase: {
      id: 'pur-1',
      cardName: 'Charizard',
      gradeValue: 9,
      grader: 'PSA',
      certNumber: '99999999',
      buyCostCents: 10000,
      psaSourcingFeeCents: 0,
      purchaseDate: '2026-04-01',
      campaignId: 'camp-1',
      createdAt: '2026-04-01T00:00:00Z',
      updatedAt: '2026-04-01T00:00:00Z',
      clValueCents: 20000,
      dhInventoryId: 42,
      dhStatus: 'in stock',
      reviewedPriceCents: 0,
    } as AgingItem['purchase'],
    daysHeld: 5,
    currentMarket: {
      gradePriceCents: 25000,
      lastSoldCents: 24000,
      sourcePrices: [],
      confidence: 0.8,
      activeListings: 0,
    } as AgingItem['currentMarket'],
    ...overrides,
  } as AgingItem;
}

function renderWithProviders(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>{ui}</ToastProvider>
    </QueryClientProvider>,
  );
}

describe('ExpandedDetail combined set-and-list', () => {
  beforeEach(() => vi.clearAllMocks());

  it('calls setReviewedPrice then listPurchaseOnDH when combineWithList is true', async () => {
    const { api } = await import('../../../../js/api');
    const item = makeItem();

    const { getByRole } = renderWithProviders(
      <ExpandedDetail item={item} combineWithList />,
    );

    // With combineWithList=true, confirmLabel is 'List on DH'
    const confirmButton = getByRole('button', { name: /list on dh/i });
    await userEvent.click(confirmButton);

    await waitFor(() => expect(api.setReviewedPrice).toHaveBeenCalledWith('pur-1', expect.any(Number), expect.any(String)));
    await waitFor(() => expect(api.listPurchaseOnDH).toHaveBeenCalledWith('pur-1'));

    const setOrder = (api.setReviewedPrice as ReturnType<typeof vi.fn>).mock.invocationCallOrder[0];
    const listOrder = (api.listPurchaseOnDH as ReturnType<typeof vi.fn>).mock.invocationCallOrder[0];
    expect(setOrder).toBeLessThan(listOrder);
  });

  it('does not call listPurchaseOnDH when combineWithList is false', async () => {
    const { api } = await import('../../../../js/api');
    const item = makeItem();

    const { getByRole } = renderWithProviders(
      <ExpandedDetail item={item} />,
    );

    // Default confirmLabel is 'Confirm'
    const confirmButton = getByRole('button', { name: /confirm/i });
    await userEvent.click(confirmButton);

    await waitFor(() => expect(api.setReviewedPrice).toHaveBeenCalled());
    expect(api.listPurchaseOnDH).not.toHaveBeenCalled();
  });

  it('exposes a Set Price secondary button that saves the price without listing on DH', async () => {
    const { api } = await import('../../../../js/api');
    const item = makeItem();

    const { getByRole } = renderWithProviders(
      <ExpandedDetail item={item} combineWithList />,
    );

    // Both buttons appear when combineWithList is true.
    expect(getByRole('button', { name: /list on dh/i })).toBeInTheDocument();
    const setPriceButton = getByRole('button', { name: /^set price$/i });

    await userEvent.click(setPriceButton);

    await waitFor(() => expect(api.setReviewedPrice).toHaveBeenCalledWith('pur-1', expect.any(Number), expect.any(String)));
    expect(api.listPurchaseOnDH).not.toHaveBeenCalled();
  });
});
