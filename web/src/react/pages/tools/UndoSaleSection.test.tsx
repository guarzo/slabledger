import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, expect, it, vi } from 'vitest';
import { api } from '../../../js/api';
import { APIError } from '../../../js/api/client';
import UndoSaleSection from './UndoSaleSection';

afterEach(() => vi.restoreAllMocks());

const purchase = { id: 'p1', campaignId: 'c1', cardName: 'Spheal', certNumber: '123', setName: 'Set', gradeValue: 9 };
const sale = { id: 's1', saleChannel: 'ebay', saleDate: '2026-02-07', salePriceCents: 27273 };

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><UndoSaleSection /></QueryClientProvider>);
}
async function lookup() {
  await userEvent.type(screen.getByLabelText(/PSA cert number/i), '123');
  await userEvent.click(screen.getByRole('button', { name: 'Look up' }));
}

it('un-sells a sold card after confirmation', async () => {
  const get = vi.spyOn(api, 'getSaleByCert').mockResolvedValue({ purchase, sale });
  const del = vi.spyOn(api, 'deleteSale').mockResolvedValue();
  setup();
  await lookup();
  expect(get).toHaveBeenCalledWith('123');
  expect(await screen.findByText('$272.73')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Un-sell' }));
  expect(await screen.findByText(/Un-sell Spheal\? This deletes the .* sale of \$272\.73 on 2026-02-07/)).toBeInTheDocument();
  const buttons = screen.getAllByRole('button', { name: 'Un-sell' });
  await userEvent.click(buttons[buttons.length - 1]);
  await waitFor(() => expect(del).toHaveBeenCalledWith('c1', 'p1'));
  expect(await screen.findByText('Sale removed; card is back in inventory')).toBeInTheDocument();
});

it('shows unsold message', async () => {
  vi.spyOn(api, 'getSaleByCert').mockResolvedValue({ purchase, sale: null });
  setup();
  await lookup();
  expect(await screen.findByText("This card isn't recorded as sold.")).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Un-sell' })).toBeNull();
});

it('shows not-in-inventory on 404', async () => {
  vi.spyOn(api, 'getSaleByCert').mockRejectedValue(new APIError('not found', 404));
  setup();
  await lookup();
  expect(await screen.findByText('No card with that cert in inventory.')).toBeInTheDocument();
});

it('shows the server error when un-sell fails', async () => {
  vi.spyOn(api, 'getSaleByCert').mockResolvedValue({ purchase, sale });
  vi.spyOn(api, 'deleteSale').mockRejectedValue(new APIError('preceding_dh_mutation_uncertain', 409));
  setup();
  await lookup();
  await userEvent.click(await screen.findByRole('button', { name: 'Un-sell' }));
  const buttons = await screen.findAllByRole('button', { name: 'Un-sell' });
  await userEvent.click(buttons[buttons.length - 1]);
  expect(await screen.findByRole('alert')).toHaveTextContent('preceding_dh_mutation_uncertain');
});
