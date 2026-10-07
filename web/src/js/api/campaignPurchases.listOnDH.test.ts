import { afterEach, expect, it, vi } from 'vitest';
import { APIClient } from './client';
import '../api';

afterEach(() => vi.unstubAllGlobals());

it.each([
  { name: 'DH returns an uncertain 502', first: () => Promise.resolve(new Response(JSON.stringify({ error: 'DH listing uncertain' }), { status: 502 })) },
  { name: 'the connection drops', first: () => Promise.reject(new TypeError('Connection lost')) },
])('does not replay an unkeyed List-on-DH request when $name', async ({ first }) => {
  const client = new APIClient('/api');
  client.retryDelay = 0;
  client.maxRetries = 3;
  const fetcher = vi.fn().mockImplementationOnce(first)
    .mockResolvedValueOnce(new Response(JSON.stringify({ listed: 1 }), { status: 200 }));
  vi.stubGlobal('fetch', fetcher);

  await expect(client.listPurchaseOnDH('purchase/1')).rejects.toThrow();
  expect(fetcher).toHaveBeenCalledTimes(1);
  expect(fetcher.mock.calls[0][0]).toBe('/api/purchases/purchase%2F1/list-on-dh');
  expect(fetcher.mock.calls[0][1].method).toBe('POST');
});
