import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DHTab } from './DHTab';
import { ToastProvider } from '../../contexts/ToastContext';
import type { DHPushConfig, DHStatusResponse } from '../../../types/apiStatus';

vi.mock('../../../js/api', () => ({
  api: {
    getDHStatus: vi.fn(),
    getDHPushConfig: vi.fn(),
    saveDHPushConfig: vi.fn(),
    triggerDHBulkMatch: vi.fn(),
  },
}));

import { api } from '../../../js/api';

function makeStatus(overrides: Partial<DHStatusResponse> = {}): DHStatusResponse {
  return {
    intelligence_count: 0,
    intelligence_last_fetch: '',
    suggestions_count: 0,
    suggestions_last_fetch: '',
    unmatched_count: 0,
    dismissed_count: 0,
    pending_count: 0,
    pending_received_count: 0,
    unenrolled_received_count: 0,
    mapped_count: 12,
    bulk_match_running: false,
    api_health: { total_calls: 100, failures: 0, success_rate: 1 },
    ...overrides,
  };
}

function makeConfig(overrides: Partial<DHPushConfig> = {}): DHPushConfig {
  return {
    swingPctThreshold: 25,
    swingMinCents: 500,
    disagreementPctThreshold: 30,
    unreviewedChangePctThreshold: 20,
    unreviewedChangeMinCents: 1000,
    listingsPaused: false,
    updatedAt: '2026-08-01T00:00:00Z',
    ...overrides,
  };
}

function renderTab() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <DHTab enabled />
      </ToastProvider>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  vi.mocked(api.getDHStatus).mockReset();
  vi.mocked(api.getDHPushConfig).mockReset();
  vi.mocked(api.saveDHPushConfig).mockReset();
});

describe('DHTab pause control', () => {
  it('shows the pause switch without expanding anything', async () => {
    vi.mocked(api.getDHStatus).mockResolvedValue(makeStatus());
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig());
    renderTab();

    const sw = await screen.findByRole('switch', { name: 'Pause DH listings' });
    expect(sw).toBeVisible();
    expect(sw).toHaveAttribute('aria-checked', 'false');
    expect(sw.closest('details')).toBeNull();
  });

  it('shows the paused warning on first render when listings are paused', async () => {
    vi.mocked(api.getDHStatus).mockResolvedValue(makeStatus());
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig({ listingsPaused: true }));
    renderTab();

    const warning = await screen.findByText(/Listings are currently paused/i);
    expect(warning).toBeVisible();
    expect(warning.closest('details')).toBeNull();
    expect(await screen.findByRole('switch', { name: 'Pause DH listings' }))
      .toHaveAttribute('aria-checked', 'true');
  });

  it('saves the flipped listingsPaused value when the switch is clicked', async () => {
    vi.mocked(api.getDHStatus).mockResolvedValue(makeStatus());
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig());
    vi.mocked(api.saveDHPushConfig).mockResolvedValue(makeConfig({ listingsPaused: true }));
    renderTab();

    const sw = await screen.findByRole('switch', { name: 'Pause DH listings' });
    sw.click();

    await waitFor(() => expect(api.saveDHPushConfig).toHaveBeenCalledTimes(1));
    expect(vi.mocked(api.saveDHPushConfig).mock.calls[0][0]).toMatchObject({
      listingsPaused: true,
      swingPctThreshold: 25,
    });
  });

  it('keeps the safety thresholds reachable without a disclosure widget', async () => {
    vi.mocked(api.getDHStatus).mockResolvedValue(makeStatus());
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig());
    renderTab();

    const field = await screen.findByLabelText('Price Swing %');
    expect(field).toBeVisible();
    expect(field.closest('details')).toBeNull();
  });
});

describe('DHTab mapped-cards lead block', () => {
  it('surfaces the dismissed count even when nothing is currently unmatched', async () => {
    vi.mocked(api.getDHStatus).mockResolvedValue(
      makeStatus({ unmatched_count: 0, dismissed_count: 7 }),
    );
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig());
    renderTab();

    expect(await screen.findByText('7 dismissed', { exact: false })).toBeVisible();
    expect(screen.queryByText(/still unmatched/)).toBeNull();
  });

  it('shows both counts together when both are nonzero', async () => {
    vi.mocked(api.getDHStatus).mockResolvedValue(
      makeStatus({ unmatched_count: 3, dismissed_count: 2 }),
    );
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig());
    renderTab();

    expect(await screen.findByText('3 still unmatched', { exact: false })).toBeVisible();
    expect(await screen.findByText('2 dismissed', { exact: false })).toBeVisible();
  });
});
