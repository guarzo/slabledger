import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { QueryClient } from '@tanstack/react-query';
import { getShowRefreshCoordinator } from './showRefreshCoordinator';
import { evaluation } from '../pages/show-preparation/fixtures.test-support';

import { ready } from './showReadiness.test-support';
const flush = async () => { for (let i = 0; i < 100; i++) await Promise.resolve(); };
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-09-14T12:00:00Z')); });
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });
function setup() {
  const qc = new QueryClient();
  const coordinator = getShowRefreshCoordinator(qc);
  const owner = Symbol(); coordinator.attach(owner);
  const calls: string[][] = [];
  vi.stubGlobal('fetch', vi.fn(async (_url, options) => {
    const ids: string[] = JSON.parse(options.body).purchaseIds; calls.push(ids);
    return new Response(JSON.stringify({ evaluations: ids.map(id => ready(Number(id.slice(3)), 'current')) }));
  }));
  return { qc, coordinator, owner, calls };
}
it('shares one runner, deduplicates identities, skips fresh/missing-price/old metadata automatically', async () => {
  const { qc, coordinator: c, owner, calls } = setup();
  expect(getShowRefreshCoordinator(qc)).toBe(c);
  const cold = ready(1);
  const task = c.check([cold, { ...cold, purchaseId: 'duplicate' }, ready(2, 'current'), ready(3, 'not_checked', { listedPriceCents: 0 }), evaluation()], owner, true);
  await c.check([ready(4)], Symbol(), false);
  await task;
  expect(calls).toEqual([['id-1']]);
  expect(c.getSnapshot()).toMatchObject({ phase: 'complete', done: 1, total: 1, busy: false });
});
it('bounds automatic work at 200 identities / 20 requests across activation and UTC windows', async () => {
  const { coordinator: c, owner, calls } = setup();
  await c.check(Array.from({ length: 230 }, (_, i) => ready(i)), owner, true);
  expect(calls).toHaveLength(20); expect(calls.every(ids => ids.length === 10)).toBe(true);
  expect(c.getSnapshot()).toMatchObject({ phase: 'budget', done: 200, total: 230 });
  await c.check([ready(999)], owner, true); expect(calls).toHaveLength(20);
  vi.setSystemTime(new Date('2026-09-15T12:00:00Z'));
  // A budget stop is terminal too: midnight alone must not silently resume it.
  await c.check([ready(999)], owner, true); expect(calls).toHaveLength(20);
});
it('enforces the request budget even across many small changed cohorts', async () => {
  const { coordinator: c, owner, calls } = setup();
  for (let i = 0; i < 25; i++) await c.check([ready(i)], owner, true);
  expect(calls).toHaveLength(20); expect(c.getSnapshot().phase).toBe('budget');
});
it('pauses future batches on selection, lets active work settle, resumes without replay', async () => {
  const { coordinator: c, owner, calls } = setup();
  const task = c.check(Array.from({ length: 12 }, (_, i) => ready(i)), owner, true);
  c.pause(owner, true); await task;
  expect(calls.map(ids => ids.length)).toEqual([10]); expect(c.getSnapshot().phase).toBe('paused');
  c.pause(owner, false);
  await c.check(Array.from({ length: 12 }, (_, i) => ready(i)), owner, true);
  expect(calls).toEqual([Array.from({ length: 10 }, (_, i) => `id-${i}`), ['id-10', 'id-11']]);
});
it.each(['cancel', 'detach'])('settles stalled bodies on %s; no late completion or remount restart', async reason => {
  const { coordinator: c, owner, qc } = setup();
  let body!: ReadableStreamDefaultController<Uint8Array>;
  const fetcher = vi.fn(async () => new Response(new ReadableStream({ start(controller) { body = controller; controller.enqueue(new TextEncoder().encode('{')); } })));
  vi.stubGlobal('fetch', fetcher);
  let settled = false;
  const task = c.check([ready(1)], owner, true).finally(() => { settled = true; });
  await flush();
  if (reason === 'cancel') c.cancel();
  else c.detach(owner);
  await flush();
  expect(settled).toBe(true); await task;
  expect(c.getSnapshot()).toMatchObject({ phase: 'cancelled', done: 0, busy: false });
  expect(() => body.enqueue(new TextEncoder().encode('late'))).toThrow();
  const other = Symbol(); c.attach(other);
  await getShowRefreshCoordinator(qc).check([ready(1)], other, true);
  expect(fetcher).toHaveBeenCalledTimes(1);
});
it('guards delayed final success before progress and cache effects after cancellation', async () => {
  const { coordinator: c, owner, qc } = setup();
  let release!: (response: Response) => void;
  vi.stubGlobal('fetch', () => new Promise<Response>(resolve => { release = resolve; }));
  const task = c.check([ready(1)], owner, true);
  c.cancel(); await task;
  const invalidations = vi.spyOn(qc, 'invalidateQueries');
  release(new Response(JSON.stringify({ evaluations: [ready(1, 'current')] }))); await flush();
  expect(c.getSnapshot()).toMatchObject({ phase: 'cancelled', done: 0 });
  expect(invalidations).not.toHaveBeenCalled();
});
it.each(['partial', 'invalid', 'network'])('stops %s outcomes without automatic retry; retains explicit retry IDs', async kind => {
  const { coordinator: c, owner, calls } = setup();
  vi.stubGlobal('fetch', vi.fn(async (_url, options) => {
    const ids: string[] = JSON.parse(options.body).purchaseIds; calls.push(ids);
    if (kind === 'network') throw new TypeError('connection lost');
    return new Response(JSON.stringify({ evaluations: kind === 'invalid' ? [] : ids.map(id => ready(Number(id.slice(3)), 'failed')) }));
  }));
  await c.check(Array.from({ length: 12 }, (_, i) => ready(i)), owner, true);
  expect(c.getSnapshot()).toMatchObject({ phase: 'failed', busy: false });
  expect(c.getSnapshot().remaining).toHaveLength(12);
  await c.check([ready(100)], owner, true); expect(calls).toHaveLength(1);
  await c.check([ready(1)], owner, false); expect(calls).toHaveLength(2);
});

it('deadline aborts a real streamed final batch at five minutes, before its request timeout', async () => {
  const { coordinator: c, owner, calls } = setup();
  let finalBody!: ReadableStreamDefaultController<Uint8Array>;
  vi.stubGlobal('fetch', vi.fn(async (_url, options) => {
    const ids: string[] = JSON.parse(options.body).purchaseIds; calls.push(ids);
    if (calls.length < 3) {
      await new Promise(resolve => setTimeout(resolve, 110000));
      return new Response(JSON.stringify({ evaluations: ids.map(id => ready(Number(id.slice(3)), 'current')) }));
    }
    return new Response(new ReadableStream({ start(controller) { finalBody = controller; controller.enqueue(new TextEncoder().encode('{')); } }));
  }));
  const task = c.check(Array.from({ length: 21 }, (_, i) => ready(i)), owner, true);
  await vi.advanceTimersByTimeAsync(220000); expect(calls).toHaveLength(3);
  await vi.advanceTimersByTimeAsync(80000); await task;
  expect(c.getSnapshot()).toMatchObject({ phase: 'deadline', done: 20, busy: false, remaining: ['id-20'] });
  expect(() => finalBody.enqueue(new TextEncoder().encode('late'))).toThrow();
});
it('narrowed cohort and newly current identities are reconsidered before the next automatic batch', async () => {
  const { coordinator: c, owner, calls } = setup();
  c.setCohort(owner, Array.from({ length: 12 }, (_, i) => ready(i)));
  const task = c.check(Array.from({ length: 12 }, (_, i) => ready(i)), owner, true);
  c.setCohort(owner, [ready(10, 'current')]);
  await task;
  expect(calls).toHaveLength(1);
});
it('does not acquire automatically while a conflicting local write is pending', async () => {
  const { coordinator: c, owner, calls } = setup();
  let release!: () => void;
  const writing = c.write(() => new Promise<void>(resolve => { release = resolve; }));
  await c.check([ready(1)], owner, true); expect(calls).toHaveLength(0);
  release(); await writing; await c.check([ready(1)], owner, true); expect(calls).toHaveLength(1);
});
it('a batch crossing UTC midnight stops for a newer window rather than an outage or replay', async () => {
  const { coordinator: c, owner, calls } = setup();
  vi.setSystemTime(new Date('2026-09-14T23:59:59Z'));
  const task = c.check(Array.from({ length: 12 }, (_, i) => ready(i)), owner, true);
  vi.setSystemTime(new Date('2026-09-15T00:00:00Z'));
  await task;
  expect(c.getSnapshot()).toMatchObject({ phase: 'window', done: 10 });
  await c.check([ready(99)], owner, true); expect(calls).toHaveLength(1);
});

it('a mounted write editor blocks acquisition without treating it as a terminal cancellation', async () => {
  const { coordinator: c, owner, calls } = setup();
  c.block(owner, true);
  await c.check([ready(1)], owner, true); await c.check([ready(2)], owner, false);
  expect(calls).toHaveLength(0);
  c.block(owner, false); await c.check([ready(1)], owner, true);
  expect(calls).toHaveLength(1);
});

it('rejects duplicate or foreign refresh evaluations instead of inflating successful progress', async () => {
  const { coordinator: c, owner } = setup();
  vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ evaluations: [ready(1, 'current'), ready(1, 'current')] })));
  await c.check([ready(1)], owner, true);
  expect(c.getSnapshot()).toMatchObject({ phase: 'failed', done: 0, remaining: ['id-1'] });
});

it('explicit Continue can clear a stop after readback confirms that all interrupted work committed', async () => {
  const { coordinator: c, owner, calls } = setup();
  const run = c.check([ready(1)], owner, true); c.cancel(); await run;
  await c.check([], owner, false);
  expect(c.getSnapshot()).toMatchObject({ phase: 'complete', error: '', remaining: [] });
  await c.check([ready(2)], owner, true);
  expect(calls).toEqual([['id-1'], ['id-2']]);
});

it.each([true, false])('current comps with ambiguous price association are reviewable, not a source failure (automatic=%s)', async automatic => {
  const { coordinator: c, owner, calls } = setup();
  vi.stubGlobal('fetch', async (_url: string, options: RequestInit) => {
    const ids: string[] = JSON.parse(String(options.body)).purchaseIds; calls.push(ids);
    return new Response(JSON.stringify({ evaluations: ids.map(id => ready(Number(id.slice(3)), 'current', id === 'id-1' ? {
      status: 'needs_review', priceAssociationUnclear: true, evidenceNeedsReview: false, reason: 'DH price association unclear', evidenceReason: '',
    } : {})) }));
  });
  await c.check(Array.from({ length: 12 }, (_, i) => ready(i)), owner, automatic);
  expect(c.getSnapshot()).toMatchObject({ phase: 'complete', done: 12 });
  expect(calls.map(ids => ids.length)).toEqual([10, 2]);
  expect(c.getSnapshot().review).toHaveLength(1);
});
