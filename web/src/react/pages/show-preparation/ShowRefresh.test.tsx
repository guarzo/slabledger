import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import ShowPreparationPage from '../ShowPreparationPage';
import ShowRefresh from './ShowRefresh';
import { evaluation, detail, member, purchaseId, listId } from './fixtures.test-support';

afterEach(() => vi.unstubAllGlobals());
function mount(ids: string[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><ShowRefresh purchaseIds={ids} /></QueryClientProvider>);
}
it('refreshes explicit selection in batches of ten, stopping partial results for explicit retry', async () => {
  const calls: string[][] = [];
  vi.stubGlobal('fetch', vi.fn(async (_url, options) => {
    const { purchaseIds } = JSON.parse(options.body); calls.push(purchaseIds);
    return new Response(JSON.stringify({ evaluations: purchaseIds.map((purchaseId: string) => evaluation({ purchaseId, status: purchaseId === 'id-2' ? 'needs_review' : 'supported', reason: purchaseId === 'id-2' ? 'Source failed' : '', evidenceNeedsReview: purchaseId === 'id-2', evidenceReason: purchaseId === 'id-2' ? 'Source failed' : '' })) }));
  }));
  mount(Array.from({ length: 23 }, (_, i) => `id-${i}`));
  expect(calls).toHaveLength(0);
  fireEvent.click(screen.getByRole('button', { name: 'Refresh selected evidence (23)' }));
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('10 of 23 checked'));
  expect(calls.map(c => c.length)).toEqual([10]);
  expect(screen.getByText(/1 need review/)).toBeVisible();
  expect(screen.getByText(/Source failed/)).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Retry refresh' }));
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]).toEqual(['id-2', ...Array.from({ length: 9 }, (_, i) => `id-${i + 10}`)]);
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
it('keeps a failed list retry scoped across actual A to B navigation and preserves its original IDs', async () => {
  const b = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'; const other = 'other-card'; const calls: string[][] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, options?: RequestInit) => {
    if (url.endsWith('/refresh')) { calls.push(JSON.parse(String(options?.body)).purchaseIds); return new Response('{"error":"Source failed"}', { status: 500 }); }
    if (url.endsWith('/lists')) return new Response(JSON.stringify({ lists: [{ ...detail().list }, { ...detail().list, id: b, name: 'B show' }] }));
    return new Response(JSON.stringify({ ...detail([member(), member({ id: 'other-item', purchaseId: other, certNumber: '87654321', evaluation: evaluation({ purchaseId: other, certNumber: '87654321' }) })]), list: { ...detail().list, id: url.endsWith(b) ? b : listId } }));
  }));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[`/shows?list=${listId}`]}><ShowPreparationPage /></MemoryRouter></QueryClientProvider>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Refresh evidence for 12345678' }));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh selected evidence (1)' }));
  await screen.findByRole('button', { name: 'Retry refresh' });
  fireEvent.change(screen.getByRole('combobox', { name: 'Show list' }), { target: { value: b } });
  await screen.findByRole('checkbox', { name: 'Refresh evidence for 87654321' });
  expect(screen.queryByRole('button', { name: 'Retry refresh' })).not.toBeInTheDocument();
  expect(calls).toEqual([[purchaseId]]);
  fireEvent.change(screen.getByRole('combobox', { name: 'Show list' }), { target: { value: listId } });
  await screen.findByRole('button', { name: 'Retry refresh' });
  fireEvent.click(screen.getByRole('checkbox', { name: 'Refresh evidence for 87654321' }));
  fireEvent.click(screen.getByRole('button', { name: 'Retry refresh' }));
  await waitFor(() => expect(calls).toEqual([[purchaseId], [purchaseId]]));
});
it('cancels active transport and never starts remaining batches', async () => {
  const fetcher = vi.fn((_url, options) => new Promise<Response>((_resolve, reject) => options.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))));
  vi.stubGlobal('fetch', fetcher); mount(Array.from({ length: 12 }, (_, i) => `id-${i}`));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh selected evidence (12)' }));
  fireEvent.click(screen.getByRole('button', { name: 'Cancel refresh' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(/Cancelled/);
  expect(fetcher).toHaveBeenCalledTimes(1);
});
