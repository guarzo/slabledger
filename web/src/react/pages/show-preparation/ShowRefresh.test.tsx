import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import ShowRefresh from './ShowRefresh';
import { evaluation } from './fixtures.test-support';

afterEach(() => vi.unstubAllGlobals());
function mount(ids: string[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><ShowRefresh purchaseIds={ids} /></QueryClientProvider>);
}
it('refreshes only explicit selection, in sequential batches of ten, reporting per-card failures', async () => {
  const calls: string[][] = [];
  vi.stubGlobal('fetch', vi.fn(async (_url, options) => {
    const { purchaseIds } = JSON.parse(options.body); calls.push(purchaseIds);
    return new Response(JSON.stringify({ evaluations: purchaseIds.map((purchaseId: string) => evaluation({ purchaseId, status: purchaseId === 'id-2' ? 'needs_review' : 'supported', reason: purchaseId === 'id-2' ? 'Source failed' : '', evidenceNeedsReview: purchaseId === 'id-2', evidenceReason: purchaseId === 'id-2' ? 'Source failed' : '' })) }));
  }));
  mount(Array.from({ length: 23 }, (_, i) => `id-${i}`));
  expect(calls).toHaveLength(0);
  fireEvent.click(screen.getByRole('button', { name: 'Refresh selected evidence (23)' }));
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('23 of 23 checked'));
  expect(calls.map(c => c.length)).toEqual([10, 10, 3]);
  expect(screen.getByText(/1 need review/)).toBeVisible();
  expect(screen.getByText(/Source failed/)).toBeVisible();
});
it.each([
  { name: 'failed evidence', evidenceNeedsReview: true, evidenceReason: 'CardLadder refresh failed', count: 1 },
  { name: 'healthy complete evidence', evidenceNeedsReview: false, evidenceReason: '', count: 0 },
])('reports evidence health despite no listed price: $name', async ({ evidenceNeedsReview, evidenceReason, count }) => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ evaluations: [evaluation({
    purchaseId: 'id-1', status: 'no_listed_price', reason: 'No positive DH listed price', listedPriceCents: 0,
    evidenceNeedsReview, evidenceReason,
  })] }))));
  mount(['id-1']);
  fireEvent.click(screen.getByRole('button', { name: 'Refresh selected evidence (1)' }));
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent(`1 of 1 checked · ${count} need review`));
  if (evidenceNeedsReview) expect(screen.getByText(/CardLadder refresh failed/)).toBeVisible();
  else expect(screen.queryByRole('list')).not.toBeInTheDocument();
});

it('offers explicit retry after failure, without automatic replay', async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'Refresh failed' }), { status: 500 }));
  vi.stubGlobal('fetch', fetcher); mount(['id-1']);
  fireEvent.click(screen.getByRole('button', { name: 'Refresh selected evidence (1)' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Refresh failed');
  expect(fetcher).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole('button', { name: 'Retry refresh' }));
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
});
it('cancels active transport and never starts remaining batches', async () => {
  const fetcher = vi.fn((_url, options) => new Promise<Response>((_resolve, reject) => options.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))));
  vi.stubGlobal('fetch', fetcher); mount(Array.from({ length: 12 }, (_, i) => `id-${i}`));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh selected evidence (12)' }));
  fireEvent.click(screen.getByRole('button', { name: 'Cancel refresh' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(/Cancelled/);
  expect(fetcher).toHaveBeenCalledTimes(1);
});
