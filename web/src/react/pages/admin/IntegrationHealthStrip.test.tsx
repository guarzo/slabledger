import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { IntegrationHealthStrip } from './IntegrationHealthStrip';
import type { DHPushConfig, DHStatusResponse } from '../../../types/apiStatus';

vi.mock('../../../js/api', () => ({
  api: {
    getDHStatus: vi.fn(),
    getDHPushConfig: vi.fn(),
    getCardLadderStatus: vi.fn(),
    getPSASyncStatus: vi.fn(),
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
    mapped_count: 0,
    bulk_match_running: false,
    api_health: { total_calls: 200, failures: 0, success_rate: 1 },
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

function renderStrip() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <IntegrationHealthStrip enabled />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  vi.mocked(api.getDHStatus).mockReset();
  vi.mocked(api.getDHPushConfig).mockReset();
  vi.mocked(api.getCardLadderStatus).mockReset().mockResolvedValue({ configured: false });
  vi.mocked(api.getPSASyncStatus).mockReset().mockResolvedValue({ configured: false, interval: '' });
});

describe('IntegrationHealthStrip DH tile', () => {
  it('reports paused even when the API success rate is perfect', async () => {
    vi.mocked(api.getDHStatus).mockResolvedValue(makeStatus());
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig({ listingsPaused: true }));
    renderStrip();

    expect(await screen.findByRole('img', { name: 'DoubleHolo: Listings paused' })).toBeInTheDocument();
    expect(screen.getByText('Listings paused')).toBeInTheDocument();
  });

  it('reports healthy when nothing is paused', async () => {
    vi.mocked(api.getDHStatus).mockResolvedValue(makeStatus());
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig());
    renderStrip();

    expect(await screen.findByRole('img', { name: 'DoubleHolo: Healthy' })).toBeInTheDocument();
    expect(screen.getByText('100.0% API')).toBeInTheDocument();
  });

  it('does not claim healthy while the pause query is still in flight', async () => {
    vi.mocked(api.getDHStatus).mockResolvedValue(makeStatus());
    vi.mocked(api.getDHPushConfig).mockReturnValue(new Promise(() => {}));
    renderStrip();

    expect(await screen.findByRole('img', { name: 'DoubleHolo: Status unavailable' })).toBeInTheDocument();
    expect(screen.queryByRole('img', { name: 'DoubleHolo: Healthy' })).toBeNull();
  });
});
