import { createServer, type ServerResponse } from 'node:http';
import { once } from 'node:events';
import { afterEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { ToastProvider } from '../../contexts/ToastContext';
import InventoryTab from '../campaign-detail/InventoryTab';
import { inventoryItem } from './fixtures.test-support';
import { ready } from '../../queries/showReadiness.test-support';
import { showPrepKeys } from '../../queries/showPrepKeys';

vi.mock('@tanstack/react-virtual', () => ({ useVirtualizer: ({ count }: { count: number }) => ({
  getTotalSize: () => count * 160, getVirtualItems: () => Array.from({ length: count }, (_, index) => ({ index, start: index * 160 })), measureElement() {}, measure() {},
}) }));
afterEach(() => vi.unstubAllGlobals());
it.each(['settle', 'cancel'])('selection during a real streamed evaluation %s keeps its observed version', async outcome => {
  const initial = ready(0, 'current', { certNumber: 'cert-0' });
  const changed = { ...initial, version: 'server-changed', status: 'below_target' };
  const items = [inventoryItem(initial)];
  const calls: string[] = []; let stream!: ServerResponse;
  const server = createServer((req, res) => {
    calls.push(req.url!); res.writeHead(200, { 'Content-Type': 'application/json' });
    if (req.url === '/api/show-prep/coverage') {
      res.end(JSON.stringify({ enabled: false, configured: false, state: 'disabled', lastSweepAt: '', retryAt: '', error: '', eligibleIdentities: 1, currentIdentities: 1, missingIdentities: 0, staleIdentities: 0, failedIdentities: 0, eligibleCards: 1, currentCards: 1, unresolvedCards: 0 }));
      return;
    }
    stream = res; res.write('{"evaluations":');
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const address = server.address(); if (!address || typeof address === 'string') throw new Error('No address');
  const nativeFetch = globalThis.fetch;
  vi.stubGlobal('fetch', (url: string, options: RequestInit) => nativeFetch(new URL(url, `http://127.0.0.1:${address.port}`), options));
  vi.stubGlobal('scrollTo', vi.fn());
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData([...showPrepKeys.evaluations, [initial.purchaseId]], { evaluations: { [initial.purchaseId]: initial }, errors: {} });
  const view = render(<QueryClientProvider client={qc}><MemoryRouter><ToastProvider><InventoryTab items={items} isLoading={false} /></ToastProvider></MemoryRouter></QueryClientProvider>);
  try {
    act(() => window.dispatchEvent(new Event('focus'))); await waitFor(() => expect(stream).toBeDefined());
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select cert-0' }));
    expect(screen.getByRole('checkbox', { name: 'Select cert-0' })).toBeChecked();
    expect([...calls].sort()).toEqual(['/api/show-prep/coverage', '/api/show-prep/evaluate']);
    if (outcome === 'cancel') {
      await act(async () => qc.cancelQueries({ queryKey: showPrepKeys.evaluations }));
      stream.end(`${JSON.stringify([changed])}}`);
      expect(screen.queryByText(/Review and reselect/)).not.toBeInTheDocument();
    } else {
      stream.end(`${JSON.stringify([changed])}}`);
      expect(await screen.findByText(/Review and reselect/)).toHaveTextContent('cert-0');
      expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled();
    }
    await waitFor(() => expect(qc.isFetching()).toBe(0));
    expect(screen.getByRole('checkbox', { name: 'Select cert-0' })).toBeChecked(); expect([...calls].sort()).toEqual(['/api/show-prep/coverage', '/api/show-prep/evaluate']);
  } finally {
    view.unmount(); qc.clear(); server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve()));
  }
});
