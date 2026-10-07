import { afterEach, expect, it, vi } from 'vitest';
import { api } from '../api';

afterEach(() => vi.restoreAllMocks());

it.each([
  { name: 'ordinary review', options: undefined, body: { priceCents: 12000, source: 'market' } },
  { name: 'intake explicit list', options: { manualList: true }, body: { priceCents: 12000, source: 'market', manualList: true } },
  { name: 'set price without listing', options: { priceOnly: true }, body: { priceCents: 12000, source: 'market', priceOnly: true } },
])('sends $name intent with the reviewed price', async ({ options, body }) => {
  const transport = vi.spyOn(api, 'fetchWithRetry').mockResolvedValue(
    new Response(JSON.stringify({ success: true, reviewedAt: '2026-10-07T00:00:00Z' }), { status: 200 }),
  );
  await api.setReviewedPrice('purchase/1', 12000, 'market', options);
  const [url, request] = transport.mock.calls[0];
  expect(url).toBe('/api/purchases/purchase%2F1/review-price');
  expect(request?.method).toBe('PATCH');
  expect(JSON.parse(request?.body as string)).toEqual(body);
});
