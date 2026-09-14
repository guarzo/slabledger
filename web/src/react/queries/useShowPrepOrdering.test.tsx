import { afterEach, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import type { InventoryEvaluations } from '../../js/api/showprep';
import { showPrepKeys, useShowEvaluations, useShowEvidence } from './useShowPrepQueries';
import { evaluation, purchaseId } from '../pages/show-preparation/fixtures.test-support';

// Fingerprints are opaque, deliberately not lexically or numerically ordered.
const versions = { A: 'f09a', B: '013b', C: 'ad7c' };
afterEach(() => vi.unstubAllGlobals());

it.each(Array.from({ length: 22 }, (_, delay) => delay))('cannot publish aggregate A after detail B when their JSON bodies settle %i microtasks apart', async delay => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const queryKey = [...showPrepKeys.evaluations, [purchaseId]];
  qc.setQueryData(queryKey, { evaluations: { [purchaseId]: evaluation({ version: versions.A }) }, errors: {} }, { updatedAt: 1 });
  const aggregateBodies: ((body: unknown) => void)[] = [];
  let evidenceBody!: (body: unknown) => void;
  // Exercise the actual hooks/APIClient/retryer: headers arrive immediately,
  // but each JSON body can finish independently of transport cancellation.
  vi.stubGlobal('fetch', async (url: string) => {
    const body = new Promise(resolve => {
      if (url.includes('/evidence/')) evidenceBody = resolve;
      else aggregateBodies.push(resolve);
    });
    return { ok: true, json: () => body };
  });
  const writes: string[] = [];
  const unsubscribe = qc.getQueryCache().subscribe(event => {
    if (event.type === 'updated' && event.action.type === 'success' && event.query.queryKey[1] === 'evaluations') {
      writes.push((event.query.state.data as InventoryEvaluations).evaluations[purchaseId].version);
    }
  });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const hook = renderHook(() => ({ summary: useShowEvaluations([purchaseId]), detail: useShowEvidence(purchaseId, true, versions.A) }), { wrapper });
  try {
    await waitFor(() => { expect(aggregateBodies).toHaveLength(1); expect(evidenceBody).toBeTypeOf('function'); });
    await act(async () => {
      aggregateBodies[0]({ evaluations: [evaluation({ version: versions.A })] });
      // Three turns hits resolved-before-publication in query-core 5.102.8;
      // the surrounding matrix covers both sides of that scheduling boundary.
      for (let i = 0; i < delay; i++) await Promise.resolve();
      evidenceBody({ evaluation: evaluation({ version: versions.B }), sales: [] });
      for (let i = 0; i < 30; i++) await Promise.resolve();
    });
    const firstDetailWrite = writes.indexOf(versions.B);
    expect(firstDetailWrite).toBeGreaterThanOrEqual(0);
    expect.soft(writes.slice(firstDetailWrite)).not.toContain(versions.A);
    expect.soft(qc.getQueryData<InventoryEvaluations>(queryKey)?.evaluations[purchaseId].version).toBe(versions.B);
    expect(aggregateBodies.length).toBeLessThanOrEqual(2);
    // An old completion must not mark a still-pending replacement idle.
    expect.soft(qc.getQueryState(queryKey)?.fetchStatus).toBe(aggregateBodies.length === 2 ? 'fetching' : 'idle');
    await act(async () => {
      for (const release of aggregateBodies.slice(1)) release({ evaluations: [evaluation({ version: versions.B })] });
    });
    await waitFor(() => expect(qc.isFetching()).toBe(0));
    expect(qc.getQueryData<InventoryEvaluations>(queryKey)?.evaluations[purchaseId].version).toBe(versions.B);
  } finally {
    hook.unmount(); qc.clear(); unsubscribe();
    for (const release of aggregateBodies) release({ evaluations: [evaluation({ version: versions.B })] });
  }
});

it('converges to server C when detail B arrives before the target HTTP read in batch four of 805 cards', async () => {
  const ids = Array.from({ length: 805 }, (_, i) => `90000000-0000-4000-8000-${String(i).padStart(12, '0')}`);
  const target = ids[650];
  const value = (id: string, version: string) => evaluation({ purchaseId: id, version,
    status: version === versions.C ? 'below_target' : 'supported' });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const queryKey = [...showPrepKeys.evaluations, ids];
  qc.setQueryData(queryKey, { evaluations: Object.fromEntries(ids.map(id => [id, value(id, versions.A)])), errors: {} }, { updatedAt: 1 });
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  let serverVersion = versions.A;
  const batches: string[][] = [];
  const targetReads: string[] = [];
  let inFlight = 0;
  let maxInFlight = 0;
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    if (url.includes('/evidence/')) return new Response(JSON.stringify({ evaluation: value(target, serverVersion), sales: [] }));
    const { purchaseIds } = JSON.parse(String(options.body)) as { purchaseIds: string[] };
    batches.push(purchaseIds);
    inFlight++;
    maxInFlight = Math.max(maxInFlight, inFlight);
    let finished = false;
    const finish = () => { if (!finished) { finished = true; inFlight--; } };
    options.signal!.addEventListener('abort', finish, { once: true });
    // Capture the server at each HTTP request, not at the aggregate start.
    const response = { evaluations: purchaseIds.map(id => value(id, serverVersion)) };
    if (purchaseIds.includes(target)) targetReads.push(serverVersion);
    try {
      if (purchaseIds[0] < ids[600]) await new Promise<void>((resolve, reject) => {
        gate.then(resolve);
        options.signal!.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true });
      });
      return new Response(JSON.stringify(response));
    } finally { finish(); options.signal!.removeEventListener('abort', finish); }
  }));
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const hook = renderHook(({ open }) => {
    const summary = useShowEvaluations(ids);
    const detail = useShowEvidence(target, open, summary.data?.evaluations[target]?.version);
    return { summary, detail };
  }, { wrapper, initialProps: { open: false } });
  try {
    await waitFor(() => expect(batches).toHaveLength(3));
    serverVersion = versions.B;
    hook.rerender({ open: true });
    await waitFor(() => expect(hook.result.current.summary.data?.evaluations[target].version).toBe(versions.B));
    expect(targetReads).toEqual([]);
    serverVersion = versions.C;
    await act(async () => { release(); });
    await waitFor(() => expect(qc.isFetching()).toBe(0));
    expect(targetReads).toEqual([versions.C]);
    expect(hook.result.current.summary.data?.evaluations[target].version).toBe(versions.C);
    expect(hook.result.current.summary.data?.evaluations[target].status).toBe('below_target');
    expect(Object.keys(hook.result.current.summary.data!.evaluations)).toHaveLength(805);
    expect(hook.result.current.summary.data?.errors).toEqual({});
    expect(hook.result.current.summary.unresolvedCount).toBe(0);
    expect(batches.every(batch => batch.length <= 200)).toBe(true);
    expect(maxInFlight).toBeLessThanOrEqual(3);
    // Once C is observed, repeated unchanged detail reads must not restart the
    // aggregate or create a badge-version -> detail -> aggregate refetch loop.
    const convergedBatches = batches.length;
    expect(convergedBatches).toBeLessThanOrEqual(8);
    for (let i = 0; i < 3; i++) await act(async () => { await hook.result.current.detail.refetch(); });
    expect(qc.isFetching()).toBe(0);
    expect(batches).toHaveLength(convergedBatches);
  } finally {
    release(); hook.unmount(); qc.clear();
  }
});

it('does not cancel or restart an aggregate for repeated unchanged detail A', async () => {
  const ids = Array.from({ length: 201 }, (_, i) => `90000000-0000-4000-8000-${String(i).padStart(12, '0')}`);
  const target = ids[0];
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData([...showPrepKeys.evaluations, ids], { evaluations: { [target]: evaluation({ purchaseId: target, version: versions.A }) }, errors: {} }, { updatedAt: 1 });
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  let batches = 0;
  const signals: AbortSignal[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    if (url.includes('/evidence/')) return new Response(JSON.stringify({ evaluation: evaluation({ purchaseId: target, version: versions.A }), sales: [] }));
    batches++; signals.push(options.signal!);
    const { purchaseIds } = JSON.parse(String(options.body));
    await gate;
    return new Response(JSON.stringify({ evaluations: purchaseIds.map((id: string) => evaluation({ purchaseId: id, version: versions.A })) }));
  }));
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const hook = renderHook(() => ({ summary: useShowEvaluations(ids), detail: useShowEvidence(target, true, versions.A) }), { wrapper });
  try {
    await waitFor(() => expect(hook.result.current.detail.isSuccess).toBe(true));
    for (let i = 0; i < 3; i++) await act(async () => { await hook.result.current.detail.refetch(); });
    expect(batches).toBe(2);
    expect(signals.every(signal => !signal.aborted)).toBe(true);
    await act(async () => { release(); });
    await waitFor(() => expect(qc.isFetching()).toBe(0));
    expect(Object.keys(hook.result.current.summary.data!.evaluations)).toHaveLength(201);
    expect(hook.result.current.summary.data?.evaluations[target].version).toBe(versions.A);
    expect(batches).toBe(2);
  } finally { release(); hook.unmount(); qc.clear(); }
});
