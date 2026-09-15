import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { evaluateInventory, showPrepAPI } from './showprep';
import { ready } from '../../react/queries/showReadiness.test-support';
beforeEach(() => vi.useFakeTimers());
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });
it.each([200, 503])('bounds full evaluate %s bodies without replaying timed out reads', async status => {
  const cancel = vi.fn();
  const fetcher = vi.fn(async () => new Response(new ReadableStream({ start(s) { s.enqueue(new TextEncoder().encode('{')); }, cancel }), { status }));
  vi.stubGlobal('fetch', fetcher);
  let outcome: unknown;
  void showPrepAPI.evaluate(['id-1'], { timeoutMs: 50 }).catch(error => { outcome = error; });
  await vi.advanceTimersByTimeAsync(51);
  expect(outcome).toMatchObject({ code: 'TIMEOUT' });
  expect(cancel).toHaveBeenCalledOnce(); expect(fetcher).toHaveBeenCalledOnce();
});
it('cancels body consumption and rejects an aggregate rather than publishing late values', async () => {
  let release!: (response: Response) => void;
  vi.stubGlobal('fetch', () => new Promise<Response>(resolve => { release = resolve; }));
  const abort = new AbortController(); let result: unknown; let error: unknown;
  void evaluateInventory(['id-1'], abort.signal).then(value => { result = value; }, value => { error = value; });
  abort.abort(); await vi.advanceTimersByTimeAsync(0);
  expect(error).toBeDefined(); expect(result).toBeUndefined();
  release(new Response(JSON.stringify({ evaluations: [ready(1, 'current')] })));
  await vi.advanceTimersByTimeAsync(0); expect(result).toBeUndefined();
});
it.each(['network', '503', '429'])('retains three read attempts with backoff for %s failures', async kind => {
  const fetcher = vi.fn(async () => { if (kind === 'network') throw new TypeError('offline'); return new Response('{}', { status: Number(kind) }); });
  vi.stubGlobal('fetch', fetcher);
  const result = showPrepAPI.evaluate(['id-1']).catch(error => error);
  await vi.advanceTimersByTimeAsync(3001);
  expect(await result).toBeInstanceOf(Error); expect(fetcher).toHaveBeenCalledTimes(3);
});
