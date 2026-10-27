import { afterEach, expect, it, vi } from 'vitest';
import { APIClient } from './client';
import '../api';

afterEach(() => vi.unstubAllGlobals());

it.each([false, true])('sends listing acknowledgement only when confirmed=%s', async confirmed => {
  const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal('fetch', fetcher);
  const client = new APIClient('/api');
  await client.deletePurchase('camp/1', 'purchase/1', confirmed);
  expect(fetcher).toHaveBeenCalledOnce();
  expect(fetcher.mock.calls[0][0]).toBe('/api/campaigns/camp%2F1/purchases/purchase%2F1' + (confirmed ? '?confirmFailedListing=true' : ''));
  expect(fetcher.mock.calls[0][1].method).toBe('DELETE');
});

it('does not retry or report success for a blocked Delete', async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'Financial work unresolved', code: 'preceding_dh_mutation_uncertain' }), { status: 409 }));
  vi.stubGlobal('fetch', fetcher);
  const client = new APIClient('/api');
  await expect(client.deletePurchase('c', 'p', true)).rejects.toMatchObject({ code: 'preceding_dh_mutation_uncertain' });
  expect(fetcher).toHaveBeenCalledOnce();
});
