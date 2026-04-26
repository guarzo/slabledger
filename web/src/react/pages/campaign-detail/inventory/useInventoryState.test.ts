import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';

// vi.mock is hoisted — factories run before imports, so we cannot reference
// imported symbols inside them. Use importOriginal to keep real isAPIError.
vi.mock('../../../../js/api', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../../../js/api')>();
  return {
    ...original,
    api: {
      listPurchaseOnDH: vi.fn(),
      resolvePriceFlag: vi.fn(),
      approveDHPush: vi.fn(),
      deletePurchase: vi.fn(),
    },
  };
});

vi.mock('../../../contexts/ToastContext', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() }),
}));

vi.mock('../../../hooks/useSellSheet', () => ({
  useSellSheet: () => ({ add: vi.fn(), remove: vi.fn(), has: vi.fn().mockReturnValue(false), clear: vi.fn(), count: 0, isLoading: false, items: [] }),
}));

vi.mock('../../../queries/useCampaignQueries', () => ({
  useExpectedValues: () => ({ data: {}, isLoading: false }),
}));

vi.mock('../../../queries/queryKeys', () => ({
  queryKeys: {
    campaigns: { inventory: (id: string) => ['campaigns', id, 'inventory'] },
    portfolio: { sellSheet: ['portfolio', 'sellSheet'] },
  },
}));

// Mock inventoryCalcs so we can drive tabCounts without building real AgingItems
const mockMeta = vi.hoisted(() => ({
  current: {
    reviewStats: { total: 0, reviewed: 0, flagged: 0, aging60d: 0 },
    tabCounts: {
      needs_attention: 0,
      awaiting_intake: 0,
      pending_dh_match: 0,
      pending_price: 0,
      ready_to_list: 0,
      dh_listed: 0,
      skipped: 0,
      in_hand: 0,
      all: 0,
    },
    summary: { totalCost: 0, totalMarket: 0, totalPL: 0 },
  },
}));

vi.mock('./inventoryCalcs', async (importOriginal) => {
  const original = await importOriginal<typeof import('./inventoryCalcs')>();
  return {
    ...original,
    computeInventoryMeta: () => mockMeta.current,
    filterAndSortItems: () => [],
  };
});

// Imports after vi.mock declarations receive the mocked modules.
import { useInventoryState } from './useInventoryState';
import { api, APIError } from '../../../../js/api';
import type { AgingItem } from '../../../../types/campaigns';

// Minimal typed factory for AgingItem — keeps tests away from `as never`
// while still allowing per-test overrides for any required field.
function mockItem(overrides: { id: string } & Partial<AgingItem['purchase']>): AgingItem {
  const { id, ...rest } = overrides;
  return {
    purchase: { id, ...rest } as AgingItem['purchase'],
    daysHeld: 0,
  } as AgingItem;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: queryClient }, children);
  return { wrapper, queryClient };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// Stable empty items array — must not be recreated on each render or the
// useEffect that clears dhListedOptimistic (dep: items) will fire and wipe
// the optimistic state before the assertion runs.
const EMPTY_ITEMS: never[] = [];

describe('useInventoryState — handleListOnDH', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('does not set dhListedOptimistic on a generic error', async () => {
    vi.mocked(api.listPurchaseOnDH).mockRejectedValue(
      new APIError('DH listing failed', 500)
    );

    const { wrapper, queryClient } = makeWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    const { result } = renderHook(() => useInventoryState(EMPTY_ITEMS, 'camp-1'), { wrapper });

    await act(async () => {
      await result.current.handleListOnDH('purchase-1');
    });

    expect(result.current.dhListedOptimistic.has('purchase-1')).toBe(false);
    expect(result.current.dhListingInFlight.size).toBe(0);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['campaigns', 'camp-1', 'inventory'] });
  });

  it('sets dhListedOptimistic when 409 reports already listed', async () => {
    vi.mocked(api.listPurchaseOnDH).mockRejectedValue(
      new APIError('Purchase already listed on DH', 409, undefined, { error: 'Purchase already listed on DH' })
    );

    const { wrapper, queryClient } = makeWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    const { result } = renderHook(() => useInventoryState(EMPTY_ITEMS, 'camp-1'), { wrapper });

    await act(async () => {
      await result.current.handleListOnDH('purchase-2');
    });

    expect(result.current.dhListedOptimistic.has('purchase-2')).toBe(true);
    expect(result.current.dhListingInFlight.size).toBe(0);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['campaigns', 'camp-1', 'inventory'] });
  });
});

describe('useInventoryState — handleBulkListOnDH', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('treats 409 already-listed rejections as success in bulk', async () => {
    vi.mocked(api.listPurchaseOnDH)
      .mockResolvedValueOnce({ listed: 1, synced: 1, skipped: 0, total: 1 })
      .mockRejectedValueOnce(
        new APIError('Purchase already listed on DH', 409, undefined, { error: 'Purchase already listed on DH' })
      );

    const { wrapper, queryClient } = makeWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    const { result } = renderHook(() => useInventoryState(EMPTY_ITEMS, 'camp-1'), { wrapper });

    await act(async () => {
      await result.current.handleBulkListOnDH(['p-a', 'p-b']);
    });

    expect(result.current.dhListedOptimistic.has('p-a')).toBe(true);
    expect(result.current.dhListedOptimistic.has('p-b')).toBe(true);
    expect(result.current.dhListingInFlight.size).toBe(0);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['campaigns', 'camp-1', 'inventory'] });
  });
});

describe('useInventoryState — default filter tab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMeta.current = {
      reviewStats: { total: 0, reviewed: 0, flagged: 0, aging60d: 0 },
      tabCounts: {
        needs_attention: 0,
        awaiting_intake: 0,
        pending_dh_match: 0,
        pending_price: 0,
        ready_to_list: 0,
        dh_listed: 0,
        skipped: 0,
        in_hand: 0,
        all: 0,
      },
      summary: { totalCost: 0, totalMarket: 0, totalPL: 0 },
    };
  });

  it('defaults to "all" when needs_attention count is 0', async () => {
    mockMeta.current = {
      ...mockMeta.current,
      tabCounts: { ...mockMeta.current.tabCounts, needs_attention: 0, all: 5 },
    };
    const items = [mockItem({ id: 'p1' })];
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useInventoryState(items, 'camp-1'), { wrapper });

    await waitFor(() => {
      expect(result.current.filterTab).toBe('all');
    });
  });

  it('defaults to "needs_attention" when count > 0', async () => {
    mockMeta.current = {
      ...mockMeta.current,
      tabCounts: { ...mockMeta.current.tabCounts, needs_attention: 3, all: 5 },
    };
    const items = [mockItem({ id: 'p1' })];
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useInventoryState(items, 'camp-1'), { wrapper });

    await waitFor(() => {
      expect(result.current.filterTab).toBe('needs_attention');
    });
  });
});
