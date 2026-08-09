import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ToastProvider } from '../../contexts/ToastContext';
import { CardLadderTab } from './CardLadderTab';
import type { CLStatusResponse, IntegrationFailuresReport } from '../../../types/admin';

let status: CLStatusResponse;
let failures: IntegrationFailuresReport | undefined;

vi.mock('../../queries/useAdminQueries', () => ({
  useCardLadderStatus: () => ({ data: status, isLoading: false, error: null }),
  useSaveCardLadderConfig: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useTriggerCardLadderRefresh: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useSyncCardLadderCollection: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCardLadderFailures: () => ({ data: failures }),
}));

function makeLastRun(overrides: Partial<NonNullable<CLStatusResponse['lastRun']>> = {}) {
  return {
    lastRunAt: '2026-07-01T10:00:00Z',
    durationMs: 1200,
    totalPurchases: 100,
    updated: 5,
    resolved: 3,
    noCert: 0,
    certResolveFailed: 0,
    noValue: 0,
    cardsPushed: 0,
    cardsRemoved: 0,
    ...overrides,
  };
}

function renderTab() {
  return render(
    <ToastProvider>
      <CardLadderTab enabled />
    </ToastProvider>,
  );
}

describe('CardLadderTab diagnostics', () => {
  beforeEach(() => {
    status = { configured: true, email: 'ops@example.com', collectionId: 'abc123' };
    failures = undefined;
  });

  it('renders the unprocessed bucket even when the last run recorded no failures', () => {
    // "Unprocessed" is the silent-miss bucket: rows with no value and no error
    // tag. It comes from the failures report, not lastRun, which is exactly why
    // it must be fetched eagerly rather than only on modal open.
    status.lastRun = makeLastRun();
    failures = { byReason: { unprocessed: 7 }, samples: null };
    renderTab();

    expect(screen.getByText('Value Gaps')).toBeInTheDocument();
    expect(screen.getByText('Unprocessed (no value, no error tag)')).toBeInTheDocument();
    expect(screen.getByText('7')).toBeInTheDocument();
  });

  it('renders each lastRun bucket that is non-zero and omits the zero ones', () => {
    status.lastRun = makeLastRun({ certResolveFailed: 4, noValue: 2, noCert: 0 });
    failures = { byReason: {}, samples: null };
    renderTab();

    expect(screen.getByText(/Cert resolve failed/)).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
    expect(screen.getByText(/Resolved, no value/)).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.queryByText(/No cert on purchase/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Unprocessed/)).not.toBeInTheDocument();
  });

  it('hides the whole diagnostics block when every bucket is zero', () => {
    status.lastRun = makeLastRun();
    failures = { byReason: {}, samples: null };
    renderTab();

    expect(screen.queryByText('Value Gaps')).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /view failure breakdown/i }),
    ).not.toBeInTheDocument();
  });

  it('opens the failure breakdown modal and passes it the report', () => {
    status.lastRun = makeLastRun({ certResolveFailed: 4 });
    failures = { byReason: { no_value: 4 }, samples: null };
    renderTab();

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /view failure breakdown/i }));

    const dialog = screen.getByRole('dialog');
    expect(dialog).toBeInTheDocument();
    // Proves `report` was threaded through rather than left null: the modal
    // renders a description per reason key it was given.
    expect(
      screen.getByText('CL collection and cards catalog both reported $0'),
    ).toBeInTheDocument();
  });
});
