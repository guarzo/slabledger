import { afterEach, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { ToastProvider } from '../../contexts/ToastContext';
import GlobalInventoryPage from '../GlobalInventoryPage';

afterEach(() => vi.unstubAllGlobals());
it('offers a clear show-prep action from the inventory header', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ items: [], warnings: [] })));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter><ToastProvider><GlobalInventoryPage /></ToastProvider></MemoryRouter></QueryClientProvider>);
  await screen.findByText('All cards sold!');
  expect(screen.getByRole('link', { name: 'Prepare for show' })).toHaveAttribute('href', '/shows');
});
it.each([200, 404])('keeps saved shows reachable when current inventory is empty or unavailable (%s)', async status => {
  vi.stubGlobal('scrollTo', vi.fn());
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(status === 200 ? { items: [], warnings: [] } : { error: 'Inventory unavailable' }), { status })));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter><ToastProvider><GlobalInventoryPage /></ToastProvider></MemoryRouter></QueryClientProvider>);
  if (status === 404) await screen.findByText('Inventory unavailable');
  else await screen.findByText('All cards sold!');
  expect(screen.getByRole('link', { name: 'Prepare for show' })).toHaveAttribute('href', '/shows');
});
