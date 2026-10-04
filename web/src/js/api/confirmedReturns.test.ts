import { describe, it, expect, vi, afterEach } from 'vitest';
import { api } from '../api';
import { APIClient } from './client';

const state = {
  operation: null, expectedSaleId: null, awaitingListing: false,
  precedingAttempt: null, purchase: null, sale: null,
};

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe('confirmed return transport', () => {
  it('preserves the exact confirmation body across automatic transport retry', async () => {
    const client = new APIClient('/api');
    client.retryDelay = 0;
    client.maxRetries = 2;
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'Retry completion' }), { status: 503 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(state), { status: 200 }));
    vi.stubGlobal('fetch', fetcher);
    const request = { returnConfirmed: true as const, expectedSaleId: null, operationId: 'persisted-operation' };
    await client.confirmPurchaseReturn('purchase', request);
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(fetcher.mock.calls[0][1].body).toBe(fetcher.mock.calls[1][1].body);
    expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual(request);
  });

  it('reads durable state with an escaped purchase ID', async () => {
    const transport = vi.spyOn(api, 'fetchWithRetry').mockResolvedValue(
      new Response(JSON.stringify(state), { status: 200 }),
    );
    expect(await api.getConfirmedReturnState('purchase/1')).toEqual(state);
    expect(transport.mock.calls[0][0]).toBe('/api/purchases/purchase%2F1/confirmed-return');
  });

  it.each([
    { expectedSaleId: null },
    { expectedSaleId: 'original-sale', operationId: 'server-operation' },
  ])('sends explicit confirmation and the unchanged captured precondition: %j', async (precondition) => {
    const transport = vi.spyOn(api, 'fetchWithRetry').mockResolvedValue(
      new Response(JSON.stringify(state), { status: 200 }),
    );
    const request = { returnConfirmed: true as const, ...precondition };
    expect(await api.confirmPurchaseReturn('purchase/1', request)).toEqual(state);
    const [url, options] = transport.mock.calls[0];
    expect(url).toBe('/api/purchases/purchase%2F1/confirm-return');
    expect(options?.method).toBe('POST');
    expect(JSON.parse(options?.body as string)).toEqual(request);
    expect(JSON.parse(options?.body as string)).not.toHaveProperty('idempotencyKey');
    expect(JSON.parse(options?.body as string)).not.toHaveProperty('dhInventoryId');
  });
});
