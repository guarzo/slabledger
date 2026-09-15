import { createServer } from 'node:http';
import { once } from 'node:events';
import { afterEach, expect, it, vi } from 'vitest';
import { showPrepAPI } from './showprep';
import { evaluation, purchaseId } from '../../react/pages/show-preparation/fixtures.test-support';

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });
it.each([200, 503])('cancels evaluation through a stalled real HTTP body (%s)', async status => {
  const server = createServer((_req, res) => {
    res.writeHead(status, { 'Content-Type': 'application/json' });
    res.write(status === 200 ? '{"evaluations":[' : '{"error":"');
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const address = server.address();
  if (!address || typeof address === 'string') throw new Error('No address');
  const nativeFetch = globalThis.fetch;
  let headers!: () => void;
  const received = new Promise<void>(resolve => { headers = resolve; });
  let signal: AbortSignal | null | undefined;
  const fetcher = vi.fn(async (url: string, options: RequestInit) => {
    signal = options.signal;
    const response = await nativeFetch(new URL(url, `http://127.0.0.1:${address.port}`), options);
    headers(); return response;
  });
  vi.stubGlobal('fetch', fetcher);
  const abort = new AbortController();
  let settled = false;
  const result = showPrepAPI.evaluate([purchaseId], { signal: abort.signal }).catch(error => error).finally(() => { settled = true; });
  try {
    await received; await Promise.resolve(); abort.abort();
    await new Promise(resolve => setTimeout(resolve, 30));
    expect(signal?.aborted).toBe(true);
    expect(settled).toBe(true);
    expect(await result).toMatchObject({ code: 'CANCELLED' });
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0][1]).toMatchObject({ credentials: 'include', body: JSON.stringify({ purchaseIds: [purchaseId] }) });
  } finally {
    server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve()));
    await result;
  }
});

it.each([200, 503])('times out stalled streamed success/error JSON without replay (%s)', async status => {
  vi.useFakeTimers();
  let cancelled = false;
  const body = new ReadableStream({ start(c) { c.enqueue(new TextEncoder().encode('{')); }, cancel() { cancelled = true; } });
  const fetcher = vi.fn(async () => new Response(body, { status }));
  vi.stubGlobal('fetch', fetcher);
  let settled = false;
  const result = showPrepAPI.evaluate([purchaseId]).catch(error => error).finally(() => { settled = true; });
  await vi.advanceTimersByTimeAsync(30000);
  expect(settled).toBe(true);
  expect(await result).toMatchObject({ code: 'TIMEOUT' });
  expect(cancelled).toBe(true);
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it('preserves structured server errors and valid success JSON', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'Credentials missing', code: 'SOURCE', details: { provider: 'CardLadder' } }), { status: 400 })));
  await expect(showPrepAPI.evaluate([purchaseId])).rejects.toMatchObject({ status: 400, code: 'SOURCE', data: { details: { provider: 'CardLadder' } } });
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ evaluations: [evaluation()] }))));
  expect((await showPrepAPI.evaluate([purchaseId])).evaluations).toEqual([evaluation()]);
});
