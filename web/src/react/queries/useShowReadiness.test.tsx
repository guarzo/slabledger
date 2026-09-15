import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { useShowReadiness } from './useShowReadiness';
import { useShowListWrites, useShowEvaluations } from './useShowPrepQueries';
import { ready } from './showReadiness.test-support';
import type { ShowEvaluation } from '../../types/showprep';

beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-09-14T12:00:00Z')); });
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
const flush = async () => { await act(async () => { for (let i = 0; i < 100; i++) { await Promise.resolve(); await vi.advanceTimersByTimeAsync(0); } }); };
function mount(initial: ShowEvaluation[], active = true) {
  const values = new Map(initial.map(e => [e.purchaseId, e]));
  const calls: { url: string; ids: string[] }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit) => {
    const ids: string[] = JSON.parse(String(options.body)).purchaseIds; calls.push({ url, ids });
    if (url.endsWith('/refresh')) ids.forEach(id => values.set(id, ready(Number(id.slice(3)), 'current')));
    return new Response(JSON.stringify({ evaluations: ids.map(id => values.get(id)) }));
  }));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const ids = initial.map(e => e.purchaseId);
  const hook = renderHook(({ enabled, selected }) => {
    const query = useShowEvaluations(ids);
    const readiness = useShowReadiness({ active: enabled, cohortIds: ids, evaluations: query.data?.evaluations ?? {}, fetching: query.isFetching, selectedCount: selected });
    return { query, readiness };
  }, { wrapper, initialProps: { enabled: active, selected: 0 } });
  return { ...hook, values, calls, qc };
}
it('keeps normal reads source-free; cold workflow activation checks identities, publishes readback', async () => {
  const hook = mount([ready(1), ready(2)], false); await flush();
  expect(hook.calls.map(c => c.url)).toEqual(['/api/show-prep/evaluate']);
  hook.rerender({ enabled: true, selected: 0 }); await flush();
  expect(hook.calls.filter(c => c.url.endsWith('/refresh')).map(c => c.ids)).toEqual([['id-1', 'id-2']]);
  expect(hook.result.current.query.data?.evaluations['id-1'].status).toBe('supported');
  expect(hook.result.current.readiness.incomplete).toBe(false);
  hook.unmount(); hook.qc.clear();
});
it('does not auto-check malformed metadata, fresh evidence, or missing listed prices', async () => {
  const hook = mount([ready(1, 'current'), ready(2, 'not_checked', { listedPriceCents: 0 }), ready(3, 'not_checked', { readiness: { state: 'not_checked' } })]);
  await flush(); expect(hook.calls).toHaveLength(1);
  expect(hook.result.current.readiness.incomplete).toBe(true);
  expect(hook.result.current.readiness.counts).toMatchObject({ current: 1, not_checked: 1, unknown: 1 });
  expect(hook.result.current.readiness.missingPriceCount).toBe(1);
  hook.unmount(); hook.qc.clear();
});
function running(retryAt: string) {
  return ready(1, 'not_checked', { readiness: { state: 'running', refreshEligibility: 'wait', identityKey: '1'.padStart(64, '0'), expiresAt: '', retryAt } });
}
it.each(['current', 'interrupted'])('observes retryAt without focus or POST, exposing %s', async state => {
  const bound = '2026-09-14T12:02:00Z';
  const hook = mount([running(bound)]); await flush();
  expect(hook.result.current.readiness.counts.running).toBe(1);
  hook.values.set('id-1', state === 'current' ? ready(1, 'current') : { ...running(bound), readiness: { ...(running(bound).readiness as object), state: 'interrupted', refreshEligibility: 'retry_only' } });
  await act(async () => { await vi.advanceTimersByTimeAsync(120000); }); await flush();
  expect(hook.calls.map(c => c.url)).toEqual(['/api/show-prep/evaluate', '/api/show-prep/evaluate']);
  expect(hook.result.current.query.data?.evaluations['id-1'].readiness).toMatchObject({ state });
  await act(async () => { await vi.advanceTimersByTimeAsync(10000); }); await flush();
  expect(hook.calls).toHaveLength(2);
  hook.unmount(); hook.qc.clear();
});
it('replaces an obsolete retry timer, catches up once after hidden sleep, and cannot loop at past bounds', async () => {
  const hook = mount([running('2026-09-14T12:01:00Z')]); await flush();
  hook.values.set('id-1', running('2026-09-14T12:03:00Z'));
  await act(async () => { await hook.result.current.query.refetch(); }); await flush();
  await act(async () => { await vi.advanceTimersByTimeAsync(60000); }); await flush();
  expect(hook.calls).toHaveLength(2);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
  await act(async () => { document.dispatchEvent(new Event('visibilitychange')); await vi.advanceTimersByTimeAsync(240000); });
  expect(hook.calls).toHaveLength(2);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  await act(async () => { document.dispatchEvent(new Event('visibilitychange')); window.dispatchEvent(new Event('focus')); }); await flush();
  expect(hook.calls).toHaveLength(3);
  await act(async () => { await vi.advanceTimersByTimeAsync(10000); }); await flush();
  expect(hook.calls).toHaveLength(3);
  hook.unmount(); hook.qc.clear();
});
it('reads expiry before acquisition, including exact current-at-expiry without a timer loop', async () => {
  const equal = ready(1, 'current', { readiness: { state: 'current', refreshEligibility: 'not_needed', identityKey: '1'.padStart(64, '0'), expiresAt: '2026-09-14T12:00:00Z', retryAt: '' } });
  const hook = mount([equal]); await flush();
  await act(async () => { await vi.advanceTimersByTimeAsync(10000); }); await flush();
  expect(hook.calls.map(c => c.url)).toEqual(['/api/show-prep/evaluate', '/api/show-prep/evaluate']);
  hook.values.set('id-1', ready(1));
  await act(async () => { window.dispatchEvent(new Event('focus')); }); await flush();
  expect(hook.calls.slice(2).map(c => c.url)).toEqual(['/api/show-prep/evaluate', '/api/show-prep/refresh', '/api/show-prep/evaluate']);
  hook.unmount(); hook.qc.clear();
});
it('selection defers acquisition at UTC rollover, while readback changes versions immediately', async () => {
  const hook = mount([ready(1, 'current')]); await flush(); hook.rerender({ enabled: true, selected: 1 });
  hook.values.set('id-1', ready(1, 'not_checked', { version: 'new-window' }));
  await act(async () => { await vi.advanceTimersByTimeAsync(12 * 60 * 60 * 1000); }); await flush();
  expect(hook.calls.filter(c => c.url.endsWith('/refresh'))).toHaveLength(0);
  expect(hook.result.current.query.data?.evaluations['id-1'].version).toBe('new-window');
  hook.rerender({ enabled: true, selected: 0 }); await flush();
  expect(hook.calls.filter(c => c.url.endsWith('/refresh'))).toHaveLength(1);
  hook.unmount(); hook.qc.clear();
});

it('rejects add/pack writes at the hook boundary while the shared refresh body is pending', async () => {
  const hook = mount([ready(1)], false); await flush();
  const qc = hook.qc;
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const writes = renderHook(() => useShowListWrites(), { wrapper });
  const calls: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    calls.push(url);
    return url.endsWith('/refresh') ? new Response(new ReadableStream({ start(c) { c.enqueue(new TextEncoder().encode('{')); } })) : new Response(JSON.stringify({ evaluations: [ready(1)] }));
  }));
  hook.rerender({ enabled: true, selected: 0 }); await flush();
  expect(hook.result.current.readiness.busy).toBe(true);
  await act(async () => {
    await expect(writes.result.current.add.mutateAsync({ id: 'list', items: [{ purchaseId: 'id-1', evaluationVersion: 'eval-1' }] })).rejects.toThrow(/checked|checking/i);
  });
  expect(calls.filter(url => url.includes('/items'))).toHaveLength(0);
  act(() => hook.result.current.readiness.cancel()); await flush();
  writes.unmount(); hook.unmount(); qc.clear();
});

it('re-reads on workflow activation before trusting an old cached evaluation', async () => {
  const hook = mount([ready(1, 'current')], false); await flush();
  hook.values.set('id-1', ready(1));
  hook.rerender({ enabled: true, selected: 0 }); await flush();
  expect(hook.calls.map(c => c.url)).toEqual(['/api/show-prep/evaluate', '/api/show-prep/evaluate', '/api/show-prep/refresh', '/api/show-prep/evaluate']);
  hook.unmount(); hook.qc.clear();
});
it('does not strand observation when workflow is deactivated during a slow read', async () => {
  const hook = mount([ready(1, 'current')]); await flush();
  let release!: (response: Response) => void;
  vi.stubGlobal('fetch', () => new Promise<Response>(resolve => { release = resolve; }));
  act(() => window.dispatchEvent(new Event('focus'))); await flush();
  hook.rerender({ enabled: false, selected: 0 });
  await act(async () => { release(new Response(JSON.stringify({ evaluations: [ready(1)] }))); }); await flush();
  const calls: string[] = [];
  vi.stubGlobal('fetch', async (url: string) => { calls.push(url); return new Response(JSON.stringify({ evaluations: [ready(1, url.endsWith('/refresh') ? 'current' : 'not_checked')] })); });
  hook.rerender({ enabled: true, selected: 0 }); await flush();
  expect(calls).toContain('/api/show-prep/refresh');
  hook.unmount(); hook.qc.clear();
});

it('pauses future automatic batches while hidden, and resumes only after a visible read', async () => {
  const hook = mount(Array.from({ length: 12 }, (_, i) => ready(i)), false); await flush();
  let release!: () => void;
  const sourceIds: string[][] = [];
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    const ids: string[] = JSON.parse(String(options.body)).purchaseIds;
    if (url.endsWith('/refresh')) {
      sourceIds.push(ids);
      if (sourceIds.length === 1) await new Promise<void>(resolve => { release = resolve; });
      ids.forEach(id => hook.values.set(id, ready(Number(id.slice(3)), 'current')));
    }
    return new Response(JSON.stringify({ evaluations: ids.map(id => hook.values.get(id)) }));
  });
  hook.rerender({ enabled: true, selected: 0 }); await flush();
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
  act(() => document.dispatchEvent(new Event('visibilitychange')));
  await act(async () => release()); await flush();
  expect(sourceIds).toHaveLength(1);
  expect(hook.result.current.readiness.phase).toBe('paused');
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  act(() => document.dispatchEvent(new Event('visibilitychange'))); await flush();
  expect(sourceIds.map(ids => ids.length)).toEqual([10, 2]);
  hook.unmount(); hook.qc.clear();
});
