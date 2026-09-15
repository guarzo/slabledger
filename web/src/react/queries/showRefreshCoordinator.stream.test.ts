import { createServer, type ServerResponse } from 'node:http';
import { once } from 'node:events';
import { setTimeout as delay } from 'node:timers/promises';
import { afterEach, expect, it, vi } from 'vitest';
import { QueryClient } from '@tanstack/react-query';
import { getShowRefreshCoordinator } from './showRefreshCoordinator';
import { ready } from './showReadiness.test-support';

afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });
async function until(predicate: () => boolean) {
  for (let i = 0; i < 400 && !predicate(); i++) await delay(5);
  expect(predicate()).toBe(true);
}
it.each([200, 503])('five-minute run deadline releases stalled real HTTP body (%s), not just headers', async status => {
  const responses: { ids: string[]; stream: ServerResponse }[] = [];
  const server = createServer((req, res) => {
    let body = ''; req.on('data', chunk => { body += chunk; });
    req.on('end', () => {
      const ids: string[] = JSON.parse(body).purchaseIds;
      responses.push({ ids, stream: res });
      res.writeHead(responses.length < 3 ? 200 : status, { 'Content-Type': 'application/json' }); res.write('{');
    });
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const address = server.address(); if (!address || typeof address === 'string') throw new Error('No address');
  const nativeFetch = globalThis.fetch;
  const signals: AbortSignal[] = [];
  let headers = 0;
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    signals.push(options.signal!);
    const response = await nativeFetch(new URL(url, `http://127.0.0.1:${address.port}`), options); headers++; return response;
  });
  vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout'] });
  vi.setSystemTime(new Date('2026-09-14T12:00:00Z'));
  const qc = new QueryClient(); const c = getShowRefreshCoordinator(qc); const owner = Symbol(); c.attach(owner);
  let settled = false;
  const task = c.check(Array.from({ length: 21 }, (_, i) => ready(i)), owner, true).finally(() => { settled = true; });
  try {
    for (let i = 0; i < 2; i++) {
      await until(() => headers > i);
      await vi.advanceTimersByTimeAsync(110000);
      responses[i].stream.end(`"evaluations":${JSON.stringify(responses[i].ids.map(id => ready(Number(id.slice(3)), 'current')))}}`);
    }
    await until(() => headers === 3);
    await vi.advanceTimersByTimeAsync(80000);
    await until(() => settled); await task;
    expect(signals[2].aborted).toBe(true);
    expect(c.getSnapshot()).toMatchObject({ phase: 'deadline', done: 20, busy: false, remaining: ['id-20'] });
    responses[2].stream.end('"evaluations":[]}');
    await delay(10);
    expect(c.getSnapshot()).toMatchObject({ phase: 'deadline', done: 20 });
    expect(responses).toHaveLength(3);
    c.detach(owner); const next = Symbol(); c.attach(next);
    await c.check([ready(50)], next, true);
    expect(responses).toHaveLength(3);
  } finally {
    c.cancel(); await task; qc.clear(); vi.useRealTimers();
    server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve()));
  }
});
