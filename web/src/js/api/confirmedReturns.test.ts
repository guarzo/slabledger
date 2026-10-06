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
    const request = { returnConfirmed: true as const, expectedSaleId: null, operationId: 'persisted-operation',
      expectedTarget: { dhInventoryId: 42, certNumber: 'cert', grader: 'PSA' } };
    await client.confirmPurchaseReturn('purchase', request);
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(fetcher.mock.calls[0][1].body).toBe(fetcher.mock.calls[1][1].body);
    expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual(request);
  });

  it.each([
    { name: 'a 503 after dispatch', first: () => Promise.resolve(new Response(JSON.stringify({ error: 'Uncertain' }), { status: 503 })) },
    { name: 'a dropped connection', first: () => Promise.reject(new TypeError('Connection lost')) },
  ])('does not resend a new unkeyed confirmation after $name', async ({ first }) => {
    const client = new APIClient('/api');
    client.maxRetries = 3;
    client.retryDelay = 0;
    const fetcher = vi.fn().mockImplementationOnce(first)
      .mockResolvedValueOnce(new Response(JSON.stringify(state), { status: 200 }));
    vi.stubGlobal('fetch', fetcher);
    const request = { returnConfirmed: true as const, expectedSaleId: null,
      expectedTarget: { dhInventoryId: 42, certNumber: 'cert', grader: 'PSA' } };
    await expect(client.confirmPurchaseReturn('purchase', request)).rejects.toThrow();
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0][0]).toBe('/api/purchases/purchase/confirm-return');
    expect(JSON.parse(fetcher.mock.calls[0][1].body)).toEqual(request);
  });

  it('reads durable state with an escaped purchase ID', async () => {
    const transport = vi.spyOn(api, 'fetchWithRetry').mockResolvedValue(
      new Response(JSON.stringify(state), { status: 200 }),
    );
    expect(await api.getConfirmedReturnState('purchase/1')).toEqual(state);
    expect(transport.mock.calls[0][0]).toBe('/api/purchases/purchase%2F1/confirmed-return');
  });

  it('reads the on-demand DH sale check with an escaped purchase ID', async () => {
    const check = { status: 'sold', resolvable: true, reason: '', target: { dhInventoryId: 42, certNumber: 'cert', grader: 'PSA' } };
    const transport = vi.spyOn(api, 'fetchWithRetry').mockResolvedValue(new Response(JSON.stringify(check), { status: 200 }));
    expect(await api.getDHSaleCheck('purchase/1')).toEqual(check);
    expect(transport.mock.calls[0][0]).toBe('/api/purchases/purchase%2F1/dh-sale-check');
    expect(transport.mock.calls[0][1]?.method).toBeUndefined(); // APIClient.get uses the default GET
  });

  it.each([
    { expectedSaleId: null },
    { expectedSaleId: 'original-sale', operationId: 'server-operation' },
  ])('sends explicit confirmation and the unchanged captured precondition: %j', async (precondition) => {
    const transport = vi.spyOn(api, 'fetchWithRetry').mockResolvedValue(
      new Response(JSON.stringify(state), { status: 200 }),
    );
    const request = { returnConfirmed: true as const, ...precondition,
      expectedTarget: { dhInventoryId: 42, certNumber: 'cert', grader: 'PSA' } };
    expect(await api.confirmPurchaseReturn('purchase/1', request)).toEqual(state);
    const [url, options] = transport.mock.calls[0];
    expect(url).toBe('/api/purchases/purchase%2F1/confirm-return');
    expect(options?.method).toBe('POST');
    expect(JSON.parse(options?.body as string)).toEqual(request);
    expect(JSON.parse(options?.body as string)).not.toHaveProperty('idempotencyKey');
    expect(JSON.parse(options?.body as string)).not.toHaveProperty('dhInventoryId');
  });
});
