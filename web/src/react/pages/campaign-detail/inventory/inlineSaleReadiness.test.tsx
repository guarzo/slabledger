import { afterEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from '../../../contexts/ToastContext';
import { getShowRefreshCoordinator } from '../../../queries/showRefreshCoordinator';
import { inventoryItem } from '../../show-preparation/fixtures.test-support';
import { useInventoryState } from './useInventoryState';
import RecordSaleForm from '../RecordSaleForm';
const items = [inventoryItem()];
let current: ReturnType<typeof useInventoryState>;
function Editor({ mounted = true }: { mounted?: boolean }) {
  current = useInventoryState(items);
  return <><button onClick={() => current.startInlineSale(items[0])}>Start sale</button>
    <button onClick={() => current.toggleExpand(items[0].purchase.id)}>Collapse row</button>
    <button onClick={() => current.setPriceBand('lt50')}>Hide row</button>
    {mounted && current.expandedId === current.inlineSaleId && current.inlineSaleId && current.filteredAndSortedItems.length > 0 && <RecordSaleForm item={items[0]} onCancel={current.cancelInlineSale} />}</>;
}
afterEach(() => vi.unstubAllGlobals());
it.each(['Collapse row', 'Hide row', 'unmount form'])('releases only the visible editor blocker on %s', async action => {
  vi.stubGlobal('scrollTo', vi.fn());
  const qc = new QueryClient(); const c = getShowRefreshCoordinator(qc);
  const tree = (mounted = true) => <QueryClientProvider client={qc}><ToastProvider><Editor mounted={mounted} /></ToastProvider></QueryClientProvider>;
  const view = render(tree()); fireEvent.click(screen.getByText('Start sale'));
  expect(c.getSnapshot().blocked).toBe(true);
  if (action === 'unmount form') view.rerender(tree(false)); else fireEvent.click(screen.getByText(action));
  await waitFor(() => expect(c.getSnapshot().blocked).toBe(false));
  if (action !== 'unmount form') expect(current.inlineSaleId).toBeNull();
  view.unmount(); qc.clear();
});
it('collapsing a submitted inline sale retains its request lease until body rejection settles', async () => {
  vi.stubGlobal('scrollTo', vi.fn());
  let body!: ReadableStreamDefaultController<Uint8Array>;
  vi.stubGlobal('fetch', async () => new Response(new ReadableStream({ start(s) { body = s; s.enqueue(new TextEncoder().encode('{')); } }), { status: 400 }));
  const qc = new QueryClient(); const c = getShowRefreshCoordinator(qc);
  const view = render(<QueryClientProvider client={qc}><ToastProvider><Editor /></ToastProvider></QueryClientProvider>);
  fireEvent.click(screen.getByText('Start sale'));
  fireEvent.click(screen.getByRole('button', { name: /Record Sale/i }));
  await waitFor(() => expect(body).toBeDefined());
  fireEvent.click(screen.getByText('Collapse row'));
  expect(current.inlineSaleId).toBeNull(); expect(c.getSnapshot().blocked).toBe(true);
  await act(async () => { body.enqueue(new TextEncoder().encode('"error":"rejected"}')); body.close(); });
  await waitFor(() => expect(c.getSnapshot().blocked).toBe(false));
  view.unmount(); qc.clear();
});
