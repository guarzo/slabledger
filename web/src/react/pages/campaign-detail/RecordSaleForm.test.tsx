import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import RecordSaleForm from './RecordSaleForm';
import { ToastProvider } from '../../contexts/ToastContext';
import type { AgingItem } from '../../../types/campaigns';

vi.mock('../../../js/api', () => ({
  api: {
    createSale: vi.fn(),
  },
}));

import { api } from '../../../js/api';

function makeItem(): AgingItem {
  return {
    purchase: {
      id: 'p1',
      campaignId: 'c1',
      cardName: 'Card 1',
      setName: 'Set',
      certNumber: 'cert1',
      grader: 'PSA',
      gradeValue: 10,
      cardNumber: '1',
      buyCostCents: 1000,
      psaSourcingFeeCents: 0,
      clValueCents: 5000,
      frontImageUrl: '',
      purchaseDate: '2026-01-01',
      receivedAt: '2026-01-02',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    },
    daysHeld: 10,
    campaignName: 'Campaign 1',
    currentMarket: undefined,
    signal: undefined,
    priceAnomaly: false,
  } as AgingItem;
}

function renderForm(onSuccess = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <RecordSaleForm item={makeItem()} onSuccess={onSuccess} onCancel={vi.fn()} />
      </ToastProvider>
    </QueryClientProvider>
  );
}

describe('RecordSaleForm', () => {
  beforeEach(() => {
    vi.mocked(api.createSale).mockReset();
  });

  it('includes saleReason in the payload when a non-default reason is selected', async () => {
    vi.mocked(api.createSale).mockResolvedValue({} as never);
    renderForm();

    fireEvent.change(screen.getByLabelText(/Sale Reason/i), { target: { value: 'aging_policy' } });
    fireEvent.click(screen.getByRole('button', { name: /Record Sale/i }));

    await waitFor(() => {
      expect(api.createSale).toHaveBeenCalledTimes(1);
    });
    const [, payload] = vi.mocked(api.createSale).mock.calls[0];
    expect(payload).toMatchObject({ saleReason: 'aging_policy' });
  });

  it('omits saleReason from the payload when left on the default option', async () => {
    vi.mocked(api.createSale).mockResolvedValue({} as never);
    renderForm();

    fireEvent.click(screen.getByRole('button', { name: /Record Sale/i }));

    await waitFor(() => {
      expect(api.createSale).toHaveBeenCalledTimes(1);
    });
    const [, payload] = vi.mocked(api.createSale).mock.calls[0];
    expect(payload).not.toHaveProperty('saleReason');
  });
});
