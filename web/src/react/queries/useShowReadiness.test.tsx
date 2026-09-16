import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { useShowReadiness } from './useShowReadiness';
import { useShowEvaluations } from './useShowPrepQueries';
import { ready } from './showReadiness.test-support';
import type { ShowEvaluation } from '../../types/showprep';

beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-09-14T12:00:00Z')); });
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
const flush = async () => { await act(async () => { for (let i = 0; i < 100; i++) { await Promise.resolve(); await vi.advanceTimersByTimeAsync(0); } }); };
function mount(initial: ShowEvaluation[]) {
  const values = new Map(initial.map(e => [e.purchaseId, e]));
  const calls: { url: string; ids: string[] }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit) => {
    const ids: string[] = JSON.parse(String(options.body)).purchaseIds; calls.push({ url, ids });
    if (url.endsWith('/refresh')) return Response.json({ error: 'retired' }, { status: 410 });
    return Response.json({ evaluations: ids.map(id => values.get(id)) });
  }));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const ids = initial.map(e => e.purchaseId);
  const hook = renderHook(() => {
    const query = useShowEvaluations(ids);
    const readiness = useShowReadiness(ids, query.data?.evaluations ?? {});
    return { query, readiness };
  }, { wrapper });
  return { ...hook, values, calls, qc, ids, wrapper };
}
function running(retryAt: string) {
  return ready(1, 'not_checked', { readiness: { state: 'running', refreshEligibility: 'wait', identityKey: '1'.padStart(64, '0'), expiresAt: '', retryAt } });
}
it('keeps cold inventory read-only on mount and rerender', async () => {
  const hook = mount([ready(1), ready(2)]); await flush(); hook.rerender(); await flush();
  expect(hook.calls.map(c => c.url)).toEqual(['/api/show-prep/evaluate']);
  expect(hook.result.current.readiness.counts.not_checked).toBe(2);
  expect(hook.result.current.readiness.incomplete).toBe(true); hook.unmount(); hook.qc.clear();
});
it('separates malformed metadata, current evidence and missing listed prices', async () => {
  const hook = mount([ready(1, 'current'), ready(2, 'not_checked', { listedPriceCents: 0 }), ready(3, 'not_checked', { readiness: { state: 'not_checked' } })]);
  await flush(); expect(hook.calls).toHaveLength(1);
  expect(hook.result.current.readiness.counts).toMatchObject({ current: 1, not_checked: 1, unknown: 1 });
  expect(hook.result.current.readiness.missingPriceCount).toBe(1); hook.unmount(); hook.qc.clear();
});
it('settles a stalled observation, removes cached Supported and offers a read retry', async () => {
  const hook = mount([ready(1, 'current')]); await flush();
  vi.stubGlobal('fetch', async () => new Response(new ReadableStream({ start(s) { s.enqueue(new TextEncoder().encode('{')); } })));
  act(() => window.dispatchEvent(new Event('focus'))); await flush();
  await act(async () => { await vi.advanceTimersByTimeAsync(30001); }); await flush();
  expect(hook.result.current.query.isFetching).toBe(false);
  expect(hook.result.current.readiness.currentCount).toBe(0);
  expect(hook.result.current.readiness.observationError).toMatch(/read|timed out/i);
  vi.stubGlobal('fetch', async () => Response.json({ evaluations: [ready(1, 'current')] }));
  await act(async () => { await hook.result.current.readiness.retryObservation(); }); await flush();
  expect(hook.result.current.readiness.observationError).toBe('');
  expect(hook.result.current.readiness.currentCount).toBe(1); hook.unmount(); hook.qc.clear();
});
it('does not let an inactive older query error poison a current observation', async () => {
  const hook = mount([ready(1, 'current')]); await flush();
  hook.qc.setQueryData(['show-prep', 'evaluations', ['id-1', 'old-card']], { evaluations: {}, errors: { 'id-1': 'old timeout' } });
  act(() => window.dispatchEvent(new Event('focus'))); await flush();
  expect(hook.result.current.readiness.observationError).toBe('');
  expect(hook.result.current.readiness.currentCount).toBe(1); hook.unmount(); hook.qc.clear();
});
it.each(['current', 'interrupted'])('observes an old running attempt boundary as %s without acquisition', async state => {
  const bound = '2026-09-14T12:02:00Z'; const hook = mount([running(bound)]); await flush();
  hook.values.set('id-1', state === 'current' ? ready(1, 'current') : { ...running(bound), readiness: { ...(running(bound).readiness as object), state: 'interrupted', refreshEligibility: 'retry_only' } });
  await act(async () => { await vi.advanceTimersByTimeAsync(120000); }); await flush();
  expect(hook.calls.map(c => c.url)).toEqual(['/api/show-prep/evaluate', '/api/show-prep/evaluate']);
  expect(hook.result.current.query.data?.evaluations['id-1'].readiness).toMatchObject({ state });
  await act(async () => { await vi.advanceTimersByTimeAsync(10000); }); await flush();
  expect(hook.calls).toHaveLength(2); hook.unmount(); hook.qc.clear();
});
it('replaces obsolete timers and catches up once after hidden sleep without looping at past bounds', async () => {
  const hook = mount([running('2026-09-14T12:01:00Z')]); await flush();
  hook.values.set('id-1', running('2026-09-14T12:03:00Z'));
  await act(async () => { await hook.result.current.query.refetch(); }); await flush();
  await act(async () => { await vi.advanceTimersByTimeAsync(60000); }); await flush(); expect(hook.calls).toHaveLength(2);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
  await act(async () => { document.dispatchEvent(new Event('visibilitychange')); await vi.advanceTimersByTimeAsync(240000); }); expect(hook.calls).toHaveLength(2);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  await act(async () => { document.dispatchEvent(new Event('visibilitychange')); window.dispatchEvent(new Event('focus')); }); await flush(); expect(hook.calls).toHaveLength(3);
  await act(async () => { await vi.advanceTimersByTimeAsync(10000); }); await flush(); expect(hook.calls).toHaveLength(3);
  hook.unmount(); hook.qc.clear();
});
it('UTC rollover reads changed versions without acquiring after rerender', async () => {
  const hook = mount([ready(1, 'current')]); await flush();
  hook.values.set('id-1', ready(1, 'not_checked', { version: 'new-window' }));
  await act(async () => { await vi.advanceTimersByTimeAsync(12 * 60 * 60 * 1000); }); await flush();
  expect(hook.result.current.query.data?.evaluations['id-1'].version).toBe('new-window');
  hook.rerender(); await flush(); expect(hook.calls.map(c => c.url)).toEqual(Array(2).fill('/api/show-prep/evaluate'));
  hook.unmount(); hook.qc.clear();
});
it.each(['current', 'interrupted'])('follows early running-boundary reads until server reports %s', async state => {
  const bound = '2026-09-14T12:02:00Z'; const hook = mount([running(bound)]); await flush();
  await act(async () => { await vi.advanceTimersByTimeAsync(120000); }); await flush(); expect(hook.calls).toHaveLength(2);
  await act(async () => { await vi.advanceTimersByTimeAsync(29999); }); await flush(); expect(hook.calls).toHaveLength(2);
  hook.values.set('id-1', state === 'current' ? ready(1, 'current') : { ...running(bound), readiness: { ...(running(bound).readiness as object), state: 'interrupted', refreshEligibility: 'retry_only' } });
  await act(async () => { await vi.advanceTimersByTimeAsync(1); }); await flush();
  expect(hook.result.current.readiness.counts[state as 'current' | 'interrupted']).toBe(1);
  await act(async () => { await vi.advanceTimersByTimeAsync(120000); }); await flush();
  expect(hook.calls.map(c => c.url)).toEqual(Array(3).fill('/api/show-prep/evaluate')); hook.unmount(); hook.qc.clear();
});
it('follows current-at-exact-expiry to stale without polling resolved stale evidence', async () => {
  const current = ready(1, 'current', { readiness: { state: 'current', refreshEligibility: 'not_needed', identityKey: '1'.padStart(64, '0'), expiresAt: '2026-09-14T12:00:00Z', retryAt: '' } });
  const hook = mount([current]); await flush();
  await act(async () => { await vi.advanceTimersByTimeAsync(1000); }); await flush(); expect(hook.calls).toHaveLength(2);
  hook.values.set('id-1', { ...current, readiness: { ...(current.readiness as object), state: 'stale', refreshEligibility: 'needed' } });
  await act(async () => { await vi.advanceTimersByTimeAsync(30000); }); await flush(); expect(hook.result.current.readiness.counts.stale).toBe(1);
  await act(async () => { await vi.advanceTimersByTimeAsync(120000); }); await flush();
  expect(hook.calls.map(c => c.url)).toEqual(Array(3).fill('/api/show-prep/evaluate')); hook.unmount(); hook.qc.clear();
});
it('retains unresolved boundary cooldown across remount without an acquisition runner', async () => {
  const hook = mount([running('2026-09-14T12:02:00Z')]); await flush();
  await act(async () => { await vi.advanceTimersByTimeAsync(120000); }); await flush();
  expect(hook.calls).toHaveLength(2); hook.unmount();
  const remount = renderHook(() => {
    const query = useShowEvaluations(hook.ids);
    return useShowReadiness(hook.ids, query.data?.evaluations ?? {});
  }, { wrapper: hook.wrapper });
  await flush();
  await act(async () => { await vi.advanceTimersByTimeAsync(29999); }); await flush();
  expect(hook.calls).toHaveLength(2);
  hook.values.set('id-1', ready(1, 'current'));
  await act(async () => { await vi.advanceTimersByTimeAsync(1); }); await flush();
  expect(remount.result.current.counts.current).toBe(1);
  expect(hook.calls.map(c => c.url)).toEqual(Array(3).fill('/api/show-prep/evaluate'));
  remount.unmount(); hook.qc.clear();
});
it('new attempt metadata supersedes an elapsed-boundary follow-up', async () => {
  const hook = mount([running('2026-09-14T12:02:00Z')]); await flush();
  await act(async () => { await vi.advanceTimersByTimeAsync(120000); }); await flush();
  hook.values.set('id-1', running('2026-09-14T12:03:00Z'));
  await act(async () => { await hook.result.current.query.refetch(); }); await flush(); expect(hook.calls).toHaveLength(3);
  await act(async () => { await vi.advanceTimersByTimeAsync(30000); }); await flush(); expect(hook.calls).toHaveLength(3);
  hook.values.set('id-1', ready(1, 'current'));
  await act(async () => { await vi.advanceTimersByTimeAsync(30000); }); await flush();
  expect(hook.result.current.readiness.counts.current).toBe(1);
  expect(hook.calls.map(c => c.url)).toEqual(Array(4).fill('/api/show-prep/evaluate')); hook.unmount(); hook.qc.clear();
});
