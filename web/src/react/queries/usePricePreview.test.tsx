import { afterEach, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState, type ReactNode } from 'react';
import { usePricePreview } from './usePricePreview';
import { showPrepKeys } from './showPrepKeys';
import { detail, evaluation, listId, purchaseId } from '../pages/show-preparation/fixtures.test-support';
import type { PricePreview, ShowEvaluation } from '../../types/showprep';

const otherID = '33333333-3333-4333-8333-333333333333';
function response(id = purchaseId, cents = 240000, current = 280000): PricePreview {
  const e = evaluation();
  return { purchaseId: id, currentPriceCents: current, trialPriceCents: cents, status: 'supported', reason: 'Recent sales support asking',
    evidenceNeedsReview: false, evidenceReason: '', evidenceVersion: e.evidenceVersion, policyVersion: e.policyVersion, recent: e.recent };
}
function setup() {
  // A global retry default must not override the preview's single-attempt contract.
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 3, retryDelay: 1 } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  return { qc, wrapper };
}
function deferredFetch() {
  const requests: { id: string; cents: number; signal: AbortSignal; release: (value: PricePreview) => void }[] = [];
  vi.stubGlobal('fetch', vi.fn((_url: string, options: RequestInit) => new Promise<Response>(resolve => {
    const { purchaseId: id, priceCents: cents } = JSON.parse(String(options.body));
    requests.push({ id, cents, signal: options.signal as AbortSignal, release: value => resolve(new Response(JSON.stringify(value))) });
  })));
  return requests;
}
afterEach(() => vi.unstubAllGlobals());

it('keeps 240000 current when 254000 returns later, without publishing to saved evaluation or list caches', async () => {
  const { qc, wrapper } = setup();
  const saved = evaluation({ localPriceCents: 280000 });
  const evaluationKey = [...showPrepKeys.evaluations, [purchaseId]];
  const aggregate = { evaluations: { [purchaseId]: saved }, errors: {} };
  const savedEvidence = { evaluation: saved, sales: [] };
  const savedList = detail();
  qc.setQueryData(evaluationKey, aggregate);
  qc.setQueryData(showPrepKeys.evidence(purchaseId), savedEvidence);
  qc.setQueryData(showPrepKeys.detail(listId), savedList);
  qc.setQueryData(showPrepKeys.lists, [savedList.list]);
  const requests = deferredFetch();
  const hook = renderHook(({ cents }) => usePricePreview(purchaseId, cents, saved), { wrapper, initialProps: { cents: 254000 } });
  try {
    await waitFor(() => expect(requests).toHaveLength(1));
    hook.rerender({ cents: 240000 });
    expect(hook.result.current.data).toBeUndefined();
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[0].signal.aborted).toBe(true);
    await act(async () => { requests[1].release(response()); });
    await waitFor(() => expect(hook.result.current.data?.trialPriceCents).toBe(240000));
    await act(async () => { requests[0].release(response(purchaseId, 254000)); });
    expect(hook.result.current.data?.trialPriceCents).toBe(240000);
    expect(qc.getQueryData(evaluationKey)).toEqual(aggregate);
    expect(qc.getQueryData(showPrepKeys.evidence(purchaseId))).toEqual(savedEvidence);
    expect(qc.getQueryData(showPrepKeys.detail(listId))).toEqual(savedList);
    expect(qc.getQueryData(showPrepKeys.lists)).toEqual([savedList.list]);
    expect(qc.getQueryState(evaluationKey)?.isInvalidated).toBe(false);
  } finally { hook.unmount(); qc.clear(); }
});

it('does not show a completed previous target while the next price is pending', async () => {
  const { qc, wrapper } = setup();
  const requests = deferredFetch();
  const hook = renderHook(({ cents }) => usePricePreview(purchaseId, cents, evaluation()), { wrapper, initialProps: { cents: 254000 } });
  try {
    await waitFor(() => expect(requests).toHaveLength(1));
    await act(async () => { requests[0].release(response(purchaseId, 254000)); });
    await waitFor(() => expect(hook.result.current.isSuccess).toBe(true));
    hook.rerender({ cents: 240000 });
    expect(hook.result.current.data).toBeUndefined();
    expect(hook.result.current.isPending).toBe(true);
    await waitFor(() => expect(requests).toHaveLength(2));
    await act(async () => { requests[1].release(response()); });
    await waitFor(() => expect(hook.result.current.data?.trialPriceCents).toBe(240000));
  } finally { hook.unmount(); qc.clear(); }
});

it('isolates a switched card and cancels the old card request', async () => {
  const { qc, wrapper } = setup();
  const requests = deferredFetch();
  const hook = renderHook(({ id }) => usePricePreview(id, 240000, evaluation({ purchaseId: id })), { wrapper, initialProps: { id: purchaseId } });
  try {
    await waitFor(() => expect(requests).toHaveLength(1));
    hook.rerender({ id: otherID });
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[0].signal.aborted).toBe(true);
    await act(async () => { requests[1].release(response(otherID)); requests[0].release(response()); });
    await waitFor(() => expect(hook.result.current.data?.purchaseId).toBe(otherID));
  } finally { hook.unmount(); qc.clear(); }
});

it.each(['version', 'evidenceVersion', 'policyVersion'] as const)('reads a new preview when saved %s changes at identical trial cents', async field => {
  const { qc, wrapper } = setup();
  const requests = deferredFetch();
  const saved = evaluation({ localPriceCents: 280000 });
  const hook = renderHook(({ saved }) => usePricePreview(purchaseId, 240000, saved), { wrapper, initialProps: { saved } });
  try {
    await waitFor(() => expect(requests).toHaveLength(1));
    await act(async () => { requests[0].release(response()); });
    await waitFor(() => expect(hook.result.current.isSuccess).toBe(true));
    const changed = { ...saved, [field]: 'new-opaque-value', localPriceCents: field === 'version' ? 270000 : 280000 };
    hook.rerender({ saved: changed });
    expect(hook.result.current.data).toBeUndefined();
    await waitFor(() => expect(requests).toHaveLength(2));
    await act(async () => { requests[1].release(response(purchaseId, 240000, changed.localPriceCents)); });
    await waitFor(() => expect(hook.result.current.data?.currentPriceCents).toBe(changed.localPriceCents));
    expect(qc.getQueryCache().findAll({ queryKey: [...showPrepKeys.all, 'price-preview'] })).toHaveLength(2);
  } finally { hook.unmount(); qc.clear(); }
});

it.each([
  { id: '', cents: 240000, saved: evaluation() }, { id: purchaseId, cents: 240000, saved: undefined },
  ...[null, 0, -1, 1.5, NaN, Infinity, Number.MAX_SAFE_INTEGER + 1, 30000].map(cents => ({ id: purchaseId, cents, saved: evaluation() })),
])('does not preview absent, invalid or unchanged input ($id / $cents)', async props => {
  const { qc, wrapper } = setup();
  const fetcher = vi.fn(); vi.stubGlobal('fetch', fetcher);
  const hook = renderHook(() => usePricePreview(props.id, props.cents, props.saved), { wrapper });
  try {
    await act(async () => { await Promise.resolve(); });
    expect(hook.result.current.fetchStatus).toBe('idle');
    expect(fetcher).not.toHaveBeenCalled();
  } finally { hook.unmount(); qc.clear(); }
});

it('retains the caller draft after preview failure, with one attempt and no saved-cache publication', async () => {
  const { qc, wrapper } = setup();
  const saved = evaluation({ localPriceCents: 280000 });
  const key = [...showPrepKeys.evaluations, [purchaseId]];
  const aggregate = { evaluations: { [purchaseId]: saved }, errors: {} };
  qc.setQueryData(key, aggregate);
  const fetcher = vi.fn(async () => new Response('{"error":"Preview unavailable"}', { status: 503 }));
  vi.stubGlobal('fetch', fetcher);
  const hook = renderHook(() => {
    const [draft, setDraft] = useState<number | null>(240000);
    return { draft, setDraft, preview: usePricePreview(purchaseId, draft, saved) };
  }, { wrapper });
  try {
    await waitFor(() => expect(hook.result.current.preview.isError).toBe(true));
    expect(hook.result.current.draft).toBe(240000);
    expect(hook.result.current.preview.error).toMatchObject({ message: 'Preview unavailable' });
    expect(hook.result.current.preview.data).toBeUndefined();
    expect(fetcher).toHaveBeenCalledOnce(); expect(qc.getQueryData(key)).toEqual(aggregate);
  } finally { hook.unmount(); qc.clear(); }
});

it('cancels a stalled body on unmount without publishing a hypothetical result', async () => {
  const { qc, wrapper } = setup();
  const cancel = vi.fn();
  const fetcher = vi.fn(async () => new Response(new ReadableStream({ start(c) { c.enqueue(new TextEncoder().encode('{')); }, cancel })));
  vi.stubGlobal('fetch', fetcher);
  const hook = renderHook(() => usePricePreview(purchaseId, 240000, evaluation()), { wrapper });
  try {
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());
    hook.unmount();
    await waitFor(() => expect(cancel).toHaveBeenCalledOnce());
    expect(qc.getQueryCache().getAll().every(query => query.state.data === undefined)).toBe(true);
  } finally { hook.unmount(); qc.clear(); }
});

it('exposes a changed saved price only on the preview, leaving reconciliation to the workspace', async () => {
  const { qc, wrapper } = setup();
  const saved: ShowEvaluation = evaluation({ localPriceCents: 280000 });
  const key = [...showPrepKeys.evaluations, [purchaseId]];
  qc.setQueryData(key, { evaluations: { [purchaseId]: saved }, errors: {} });
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(response(purchaseId, 240000, 270000)))));
  const hook = renderHook(() => usePricePreview(purchaseId, 240000, saved), { wrapper });
  try {
    await waitFor(() => expect(hook.result.current.data?.currentPriceCents).toBe(270000));
    expect(qc.getQueryData(key)).toEqual({ evaluations: { [purchaseId]: saved }, errors: {} });
    expect(saved.localPriceCents).toBe(280000);
  } finally { hook.unmount(); qc.clear(); }
});
