import { createServer, type ServerResponse } from 'node:http';
import { once } from 'node:events';
import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { ToastProvider } from '../../contexts/ToastContext';
import InventoryTab from '../campaign-detail/InventoryTab';
import { inventoryItem } from './fixtures.test-support';
import { ready } from '../../queries/showReadiness.test-support';
import { getShowRefreshCoordinator } from '../../queries/showRefreshCoordinator';

vi.mock('@tanstack/react-virtual', () => ({ useVirtualizer: ({ count }: { count: number }) => ({
  getTotalSize: () => count * 160, getVirtualItems: () => Array.from({ length: count }, (_, index) => ({ index, start: index * 160 })), measureElement() {}, measure() {},
}) }));
afterEach(() => vi.unstubAllGlobals());

it.each(['settle', 'cancel'])('selection during a real streamed batch: %s keeps captured versions and prevents the next batch', async outcome => {
  let values = Array.from({ length: 12 }, (_, i) => ready(i, 'not_checked', { certNumber: `cert-${i}` }));
  const items = values.map(value => inventoryItem(value));
  const posts: string[][] = [];
  let stream!: ServerResponse;
  let headersRead = false;
  const server = createServer((req, res) => {
    let body = ''; req.on('data', chunk => { body += chunk; });
    req.on('end', () => {
      const ids: string[] = body ? JSON.parse(body).purchaseIds ?? [] : [];
      res.writeHead(200, { 'Content-Type': 'application/json' });
      if (req.url?.endsWith('/refresh')) {
        posts.push(ids); stream = res; res.write('{"evaluations":');
      } else res.end(JSON.stringify(req.url?.endsWith('/lists') ? { lists: [] } : { evaluations: values.filter(e => ids.includes(e.purchaseId)) }));
    });
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const address = server.address(); if (!address || typeof address === 'string') throw new Error('No address');
  const nativeFetch = globalThis.fetch;
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    const response = await nativeFetch(new URL(url, `http://127.0.0.1:${address.port}`), options);
    if (url.endsWith('/refresh')) headersRead = true;
    return response;
  });
  vi.stubGlobal('scrollTo', vi.fn());
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const coordinator = getShowRefreshCoordinator(qc);
  const view = render(<QueryClientProvider client={qc}><MemoryRouter><ToastProvider><InventoryTab items={items} isLoading={false} /></ToastProvider></MemoryRouter></QueryClientProvider>);
  try {
    fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
    await waitFor(() => expect(headersRead).toBe(true));
    expect(posts[0]).toHaveLength(10);
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select cert-0' }));
    expect(screen.getByRole('checkbox', { name: 'Select cert-0' })).toBeChecked();
    expect(screen.getByRole('button', { name: 'Add selected to show (1)' })).toBeDisabled();
    // Server commits may survive cancellation; readback, not the refresh body,
    // is authoritative. A real read changes versions without acknowledging them.
    values = values.map((e, i) => posts[0].includes(e.purchaseId) ? ready(i, 'current', { certNumber: `cert-${i}`, version: 'server-committed' }) : e);
    if (outcome === 'cancel') fireEvent.click(screen.getByRole('button', { name: 'Cancel checking' }));
    else stream.end(`${JSON.stringify(values.filter(e => posts[0].includes(e.purchaseId)))}}`);
    await waitFor(() => expect(coordinator.getSnapshot().busy).toBe(false));
    expect(coordinator.getSnapshot()).toMatchObject({ phase: outcome === 'cancel' ? 'cancelled' : 'paused', done: outcome === 'cancel' ? 0 : 10 });
    expect(await screen.findByText(/Review and reselect/)).toHaveTextContent('cert-0');
    expect(screen.getByRole('checkbox', { name: 'Select cert-0' })).toBeChecked();
    expect(posts).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
    fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
    window.dispatchEvent(new Event('focus'));
    await waitFor(() => expect(qc.isFetching()).toBe(0));
    expect(posts).toHaveLength(1);
    expect(screen.getByRole('button', { name: 'Add selected to show (1)' })).toBeDisabled();
  } finally {
    view.unmount(); qc.clear(); server.closeAllConnections();
    await new Promise<void>(resolve => server.close(() => resolve()));
  }
});
