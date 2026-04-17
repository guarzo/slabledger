import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
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

// Imports after vi.mock declarations receive the mocked modules.
import { useInventoryState } from './useInventoryState';
import { api, APIError } from '../../../../js/api';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
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

    const { result } = renderHook(() => useInventoryState(EMPTY_ITEMS, 'camp-1'), { wrapper: makeWrapper() });

    await act(async () => {
      await result.current.handleListOnDH('purchase-1');
    });

    expect(result.current.dhListedOptimistic.has('purchase-1')).toBe(false);
    expect(result.current.dhListingInFlight.size).toBe(0);
  });

  it('sets dhListedOptimistic when 409 reports already listed', async () => {
    vi.mocked(api.listPurchaseOnDH).mockRejectedValue(
      new APIError('Purchase already listed on DH', 409, undefined, { error: 'Purchase already listed on DH' })
    );

    const { result } = renderHook(() => useInventoryState(EMPTY_ITEMS, 'camp-1'), { wrapper: makeWrapper() });

    await act(async () => {
      await result.current.handleListOnDH('purchase-2');
    });

    expect(result.current.dhListedOptimistic.has('purchase-2')).toBe(true);
    expect(result.current.dhListingInFlight.size).toBe(0);
  });
});
