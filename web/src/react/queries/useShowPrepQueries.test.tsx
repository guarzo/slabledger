import { afterEach, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { showPrepKeys, useShowEvaluations, useShowEvidence } from './useShowPrepQueries';
import { evaluation, purchaseId } from '../pages/show-preparation/fixtures.test-support';

afterEach(() => vi.unstubAllGlobals());

it('resumes an interrupted initial 201-card aggregate after detail publication and immediate remount', async () => {
  const ids = [purchaseId, ...Array.from({ length: 200 }, (_, i) => `90000000-0000-4000-8000-${String(i).padStart(12, '0')}`)];
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  let batches = 0;
  let aborted = false;
  let observed = false;
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    if (url.includes('/evidence/')) {
      observed = true;
      return new Response(JSON.stringify({ evaluation: evaluation({ version: 'eval-2', status: 'below_target' }), sales: [] }));
    }
    const { purchaseIds } = JSON.parse(String(options.body));
    batches++;
    if (batches === 2) return new Promise<Response>((_resolve, reject) => {
      options.signal!.addEventListener('abort', () => {
        aborted = true;
        reject(new DOMException('Aborted', 'AbortError'));
      });
    });
    return new Response(JSON.stringify({ evaluations: purchaseIds.map((id: string) => evaluation({ purchaseId: id,
      ...(observed && id === purchaseId ? { version: 'eval-2', status: 'below_target' } : {}),
    })) }));
  }));
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const first = renderHook(({ open }) => ({ summary: useShowEvaluations(ids), detail: useShowEvidence(purchaseId, open, 'eval-1') }),
    { wrapper, initialProps: { open: false } });
  await waitFor(() => expect(batches).toBe(2));
  first.rerender({ open: true });
  await waitFor(() => expect(first.result.current.summary.data?.evaluations[purchaseId].version).toBe('eval-2'));
  expect(Object.keys(first.result.current.summary.data!.evaluations)).toHaveLength(1);
  expect(first.result.current.summary.isFetching).toBe(true);
  first.unmount();
  await waitFor(() => expect(aborted).toBe(true));
  const second = renderHook(() => useShowEvaluations(ids), { wrapper });
  await waitFor(() => expect(batches).toBe(4));
  await waitFor(() => expect(second.result.current.isFetching).toBe(false));
  expect(Object.keys(second.result.current.data!.evaluations)).toHaveLength(201);
  expect(second.result.current.data?.evaluations[purchaseId].version).toBe('eval-2');
  expect(second.result.current.data?.errors).toEqual({});
  expect(second.result.current.isStale).toBe(false);
});

it.each([true, false])('does not let an older in-flight batch replace a newer detail observation (cached=%s)', async cached => {
  const ids = [purchaseId, ...Array.from({ length: 200 }, (_, i) => `90000000-0000-4000-8000-${String(i).padStart(12, '0')}`)];
  const queryKey = [...showPrepKeys.evaluations, [...ids].sort()];
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  if (cached) qc.setQueryData(queryKey, { evaluations: { [purchaseId]: evaluation() }, errors: {} }, { updatedAt: 1 });
  let release!: (response: Response) => void;
  const lastBatch = new Promise<Response>(resolve => { release = resolve; });
  let batches = 0;
  let rechecking = false;
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    if (url.includes('/evidence/')) return new Response(JSON.stringify({
      evaluation: evaluation({ version: 'eval-2', status: 'below_target', listedPriceCents: 35000 }), sales: [],
    }));
    const { purchaseIds } = JSON.parse(String(options.body));
    batches++;
    if (!rechecking && batches === 2) return lastBatch;
    return new Response(JSON.stringify({ evaluations: purchaseIds.map((id: string) => evaluation({ purchaseId: id,
      ...(rechecking && id === purchaseId ? { version: 'eval-3', status: 'thin_evidence', compCount: 1 } : {}),
    })) }));
  }));
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const { result, rerender } = renderHook(({ open }) => ({
    summary: useShowEvaluations(ids), detail: useShowEvidence(purchaseId, open, 'eval-1'),
  }), { wrapper, initialProps: { open: false } });
  await waitFor(() => expect(batches).toBe(2));
  rerender({ open: true });
  await waitFor(() => expect(result.current.detail.data?.evaluation.version).toBe('eval-2'));
  // The first HTTP batch already captured v1, but the aggregate is still waiting
  // for a different card. Finish that old read only after v2 was observed.
  await act(async () => { release(new Response(JSON.stringify({ evaluations: [evaluation({ purchaseId: ids[200] })] }))); });
  await waitFor(() => expect(result.current.summary.isFetching).toBe(false));
  expect(result.current.summary.data?.evaluations[purchaseId].version).toBe('eval-2');
  expect(result.current.summary.data?.evaluations[purchaseId].status).toBe('below_target');
  expect(result.current.summary.data?.errors[purchaseId]).toBeUndefined();
  expect(Object.keys(result.current.summary.data!.evaluations)).toHaveLength(201);
  expect(result.current.summary.data?.evaluations[ids[200]].version).toBe('eval-1');
  // New reads begun after the detail observation must still advance normally;
  // protecting a detail from an older read must not pin it forever.
  rechecking = true;
  await act(async () => { await result.current.summary.refetch(); });
  await waitFor(() => expect(result.current.summary.data?.evaluations[purchaseId].version).toBe('eval-3'));
});
