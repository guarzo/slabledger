import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';
import PolicyDrawer from './PolicyDrawer';
import type { PsaExchangePolicySettings } from '../../../types/psaExchange';
import { api } from '../../../js/api';

const settings: PsaExchangePolicySettings = {
  active: {
    highLiquidityVelocity: 5,
    highLiquidityConfidence: 5,
    highLiquidityOfferPct: 0.75,
    defaultOfferPct: 0.65,
    minConfidence: 3,
    minQuarterVelocity: 1,
  },
  defaults: {
    highLiquidityVelocity: 5,
    highLiquidityConfidence: 5,
    highLiquidityOfferPct: 0.75,
    defaultOfferPct: 0.65,
    minConfidence: 3,
    minQuarterVelocity: 1,
  },
};

function renderDrawer(props?: Partial<React.ComponentProps<typeof PolicyDrawer>>) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <PolicyDrawer open={true} onOpenChange={() => {}} settings={settings} {...props} />
    </QueryClientProvider>,
  );
}

describe('PolicyDrawer', () => {
  it('seeds the form with the active policy', () => {
    renderDrawer();
    expect((screen.getByLabelText(/^Velocity ≥/i) as HTMLInputElement).value).toBe('5');
    expect((screen.getByLabelText(/^High liquidity %/i) as HTMLInputElement).value).toBe('75');
  });

  it('disables Save when high % drops below default %', () => {
    renderDrawer();
    const hi = screen.getByLabelText(/^High liquidity %/i) as HTMLInputElement;
    fireEvent.change(hi, { target: { value: '50' } });
    const save = screen.getByRole('button', { name: /^Save/ }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    expect(screen.getByText(/must be ≥ default/i)).toBeInTheDocument();
  });

  it('rejects offer percentages outside (0, 100]', () => {
    renderDrawer();
    fireEvent.change(screen.getByLabelText(/^Default %/i), { target: { value: '0' } });
    expect((screen.getByRole('button', { name: /^Save/ }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/^Default %/i), { target: { value: '150' } });
    expect((screen.getByRole('button', { name: /^Save/ }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('Reset to defaults restores the defaults snapshot', () => {
    renderDrawer({
      settings: {
        ...settings,
        active: { ...settings.active, highLiquidityOfferPct: 0.9 },
      },
    });
    expect((screen.getByLabelText(/^High liquidity %/i) as HTMLInputElement).value).toBe('90');
    fireEvent.click(screen.getByRole('button', { name: /reset to defaults/i }));
    expect((screen.getByLabelText(/^High liquidity %/i) as HTMLInputElement).value).toBe('75');
  });

  it('saves a valid policy through the api client and closes', async () => {
    const onOpenChange = vi.fn();
    const updateSpy = vi
      .spyOn(api, 'updatePsaExchangePolicy')
      .mockResolvedValue({ active: settings.active, defaults: settings.defaults });

    renderDrawer({ onOpenChange });
    fireEvent.change(screen.getByLabelText(/^High liquidity %/i), { target: { value: '78' } });
    fireEvent.click(screen.getByRole('button', { name: /^Save/ }));

    await waitFor(() => expect(updateSpy).toHaveBeenCalled());
    const sent = updateSpy.mock.calls[0][0];
    expect(sent.highLiquidityOfferPct).toBeCloseTo(0.78, 5);
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    updateSpy.mockRestore();
  });
});
