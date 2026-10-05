import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, expect, it, vi } from 'vitest';
import { api } from '../../../js/api';
import { ToastProvider } from '../../contexts/ToastContext';
import ImportSalesTab from './ImportSalesTab';

afterEach(() => vi.restoreAllMocks());

it('confirms the matched order identity, own price and date rather than the returned order for the same cert', async () => {
  vi.spyOn(api, 'importOrdersSales').mockResolvedValue({
    matched: [{ orderId: 'ext-99999', certNumber: '162787413', productTitle: 'Spheal', cardName: 'Spheal',
      saleChannel: 'ebay', saleDate: '2026-02-07', salePriceCents: 27273, saleFeeCents: 0,
      purchaseId: 'p1', campaignId: 'c1', buyCostCents: 1000, netProfitCents: 26273 }],
    skipped: [{ orderId: 'ext-848', certNumber: '162787413', productTitle: 'Spheal', reason: 'returned_order' }],
    alreadySold: [], notFound: [],
  });
  const confirm = vi.spyOn(api, 'confirmOrdersSales').mockResolvedValue({ created: 1, failed: 0 });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const { container } = render(<QueryClientProvider client={queryClient}><ToastProvider><ImportSalesTab /></ToastProvider></QueryClientProvider>);
  const input = container.querySelector<HTMLInputElement>('input[type="file"]');
  expect(input).not.toBeNull();
  fireEvent.change(input!, { target: { files: [new File(['fixture'], 'orders.csv', { type: 'text/csv' })] } });
  await userEvent.click(await screen.findByRole('button', { name: 'Confirm 1 Sale' }));
  await waitFor(() => expect(confirm).toHaveBeenCalledWith([{ purchaseId: 'p1', saleChannel: 'ebay',
    saleDate: '2026-02-07', salePriceCents: 27273, orderId: 'ext-99999' }]));
});
