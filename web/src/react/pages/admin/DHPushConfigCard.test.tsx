import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DHPushConfigCard } from './DHPushConfigCard';
import { ToastProvider } from '../../contexts/ToastContext';
import type { DHPushConfig } from '../../../types/apiStatus';

vi.mock('../../../js/api', () => ({
  api: {
    getDHPushConfig: vi.fn(),
    saveDHPushConfig: vi.fn(),
  },
}));

import { api } from '../../../js/api';

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

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <DHPushConfigCard />
      </ToastProvider>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  vi.mocked(api.getDHPushConfig).mockReset();
  vi.mocked(api.saveDHPushConfig).mockReset();
});

describe('DHPushConfigCard', () => {
  it('renders the loaded thresholds', async () => {
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig());
    renderCard();

    const swing = await screen.findByLabelText('Price Swing %');
    expect(swing).toHaveValue(25);
    expect(screen.getByLabelText('Unreviewed CL Change Min')).toHaveValue(1000);
  });

  it('saves the edited thresholds', async () => {
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig());
    vi.mocked(api.saveDHPushConfig).mockResolvedValue(makeConfig({ swingPctThreshold: 40 }));
    renderCard();

    const swing = await screen.findByLabelText('Price Swing %');
    fireEvent.change(swing, { target: { value: '40' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(api.saveDHPushConfig).toHaveBeenCalledTimes(1));
    expect(vi.mocked(api.saveDHPushConfig).mock.calls[0][0]).toMatchObject({
      swingPctThreshold: 40,
    });
  });

  it('does not un-pause listings when only thresholds are saved', async () => {
    // Regression guard. The API has only a whole-object PUT and the store
    // upserts every column, so a card that sent its own snapshot of
    // listingsPaused would silently resume a paused marketplace. The card must
    // send only threshold keys and let useSaveDHPushConfig merge.
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig({ listingsPaused: true }));
    vi.mocked(api.saveDHPushConfig).mockResolvedValue(
      makeConfig({ listingsPaused: true, swingPctThreshold: 40 }),
    );
    renderCard();

    const swing = await screen.findByLabelText('Price Swing %');
    fireEvent.change(swing, { target: { value: '40' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(api.saveDHPushConfig).toHaveBeenCalledTimes(1));
    expect(vi.mocked(api.saveDHPushConfig).mock.calls[0][0]).toMatchObject({
      swingPctThreshold: 40,
      listingsPaused: true,
    });
  });

  it('keeps a cleared number field empty instead of snapping it to zero', async () => {
    vi.mocked(api.getDHPushConfig).mockResolvedValue(makeConfig());
    renderCard();

    const swing = await screen.findByLabelText('Price Swing %');
    fireEvent.change(swing, { target: { value: '' } });
    // The draft-state commit is not guaranteed to be flushed to the DOM by the
    // time fireEvent returns, so read the value through waitFor like every other
    // assertion here. Asserting synchronously passes on an idle machine and
    // fails under CI load, reading back the pre-edit 25.
    await waitFor(() => expect(swing).toHaveValue(null));
  });

  it('shows the error message text, not [object Object]', async () => {
    vi.mocked(api.getDHPushConfig).mockRejectedValue(new Error('boom'));
    renderCard();

    expect(await screen.findByText(/Failed to load config: boom/)).toBeInTheDocument();
  });
});
