import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ToastProvider } from '../../contexts/ToastContext';
import { DHOperationsPanel } from './DHOperationsPanel';
import type { DHStatusResponse } from '../../../types/apiStatus';

const reconcileMutate = vi.fn().mockResolvedValue({
  scanned: 10,
  missingOnDH: 0,
  reset: 0,
  errors: null,
  resetIds: null,
});
const clearMutate = vi.fn().mockResolvedValue({ cleared: 0 });

let dhStatus: Partial<DHStatusResponse> = {};
let tombstoneCount = 0;

vi.mock('../../queries/useAdminQueries', () => ({
  useDHStatus: () => ({ data: dhStatus, isLoading: false, error: null, refetch: vi.fn() }),
  useTriggerDHReconcile: () => ({ mutateAsync: reconcileMutate, isPending: false }),
  useDHTombstoneCount: () => ({ data: { count: tombstoneCount } }),
  useClearDHTombstones: () => ({ mutateAsync: clearMutate, isPending: false }),
}));

function renderPanel() {
  return render(
    <ToastProvider>
      <DHOperationsPanel enabled />
    </ToastProvider>,
  );
}

describe('DHOperationsPanel', () => {
  beforeEach(() => {
    reconcileMutate.mockClear();
    clearMutate.mockClear();
    dhStatus = {};
    tombstoneCount = 0;
  });

  it('triggers the DH reconcile when Sync DH now is clicked', () => {
    renderPanel();
    const btn = screen.getByRole('button', { name: 'Sync DH now' });
    fireEvent.click(btn);
    expect(reconcileMutate).toHaveBeenCalledTimes(1);
  });

  it('disables Clear tombstones when the count is zero', () => {
    tombstoneCount = 0;
    renderPanel();
    expect(screen.getByRole('button', { name: 'Clear tombstones' })).toBeDisabled();
  });

  it('enables Clear tombstones when tombstones exist', () => {
    tombstoneCount = 7;
    renderPanel();
    expect(screen.getByRole('button', { name: 'Clear tombstones' })).toBeEnabled();
    expect(screen.getByText('7')).toBeInTheDocument();
  });

  it('renders the 24h orphan count when orders have been polled', () => {
    dhStatus = {
      last_orders_poll_at: '2026-08-09T12:00:00Z',
      orders_matched_count_24h: 12,
      orders_orphan_count_24h: 3,
      orders_already_sold_count_24h: 1,
    };
    renderPanel();
    expect(screen.getByText('3 orphan DH orders in the last 24h')).toBeInTheDocument();
    expect(screen.getByText('12 matched')).toBeInTheDocument();
    expect(screen.getByText('1 already sold')).toBeInTheDocument();
  });

  it('omits the orders block entirely when DH orders have never been polled', () => {
    dhStatus = { orders_orphan_count_24h: 3 };
    renderPanel();
    expect(screen.queryByText(/orphan DH orders/)).not.toBeInTheDocument();
  });
});
