import { afterEach, expect, it, vi } from 'vitest';
import { priceReviewAPI } from './priceReview';
import { evaluation, purchaseId } from '../../react/pages/show-preparation/fixtures.test-support';

function preview(trialPriceCents = 240000) {
  const e = evaluation();
  return {
    purchaseId, currentPriceCents: 280000, trialPriceCents, status: 'supported', reason: 'Recent sales support asking',
    evidenceNeedsReview: false, evidenceReason: '', evidenceVersion: e.evidenceVersion,
    policyVersion: e.policyVersion, recent: e.recent,
  };
}
function respond(body: unknown, status = 200) {
  const fetcher = vi.fn(async () => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

it.each([1, 240000, Number.MAX_SAFE_INTEGER])('posts exact integer cents with credentials and validates the preview (%s)', async price => {
  const fetcher = respond(preview(price));
  expect(await priceReviewAPI.preview(purchaseId, price)).toEqual(preview(price));
  expect(fetcher).toHaveBeenCalledExactlyOnceWith('/api/show-prep/preview', expect.objectContaining({
    method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ purchaseId, priceCents: price }), signal: expect.any(AbortSignal),
  }));
});

it.each([
  ['', 1], ['bad', 1], [purchaseId.replace(/-/g, ''), 1], ['AAAAAAAA-1111-4111-8111-111111111111', 1],
  [purchaseId, 0], [purchaseId, -1], [purchaseId, 1.1], [purchaseId, Number.MAX_SAFE_INTEGER + 1],
  [purchaseId, NaN], [purchaseId, Infinity],
] as const)('rejects invalid request %s / %s before fetching', async (id, price) => {
  const fetcher = respond(preview());
  await expect(priceReviewAPI.preview(id, price)).rejects.toMatchObject({ code: 'INVALID_REQUEST' });
  expect(fetcher).not.toHaveBeenCalled();
});

const malformed: [string, unknown][] = [
  ['null', null], ['array', []], ['missing fields', {}],
  ['wrong purchase', { ...preview(), purchaseId: '22222222-2222-4222-8222-222222222222' }],
  ['wrong trial', { ...preview(), trialPriceCents: 254000 }],
  ['fractional current', { ...preview(), currentPriceCents: 1.5 }],
  ['unsafe current', { ...preview(), currentPriceCents: Number.MAX_SAFE_INTEGER + 1 }],
  ['unknown status', { ...preview(), status: 'approved' }],
  ['invalid reason', { ...preview(), reason: null }],
  ['review flag', { ...preview(), evidenceNeedsReview: 'false' }],
  ['review reason', { ...preview(), evidenceReason: false }],
  ['evidence version', { ...preview(), evidenceVersion: null }],
  ['policy missing', { ...preview(), policyVersion: '' }],
  ...['version', 'canAdd', 'canPack', 'extra'].map(key => [key, { ...preview(), [key]: true }] as [string, unknown]),
  ...Object.keys(preview()).map(key => ['absent ' + key, Object.fromEntries(Object.entries(preview()).filter(([name]) => name !== key))] as [string, unknown]),
  ...[
    { windowStart: '2026-02-30' }, { windowEnd: null }, { latestSaleDate: 1 }, { saleIds: [1] },
    { count: -1 }, { medianCents: 1.5 }, { latestSaleCount: '2' }, { latestSaleMinCents: null },
    { latestSaleMaxCents: Number.MAX_SAFE_INTEGER + 1 }, { gapPct: '5' }, { gapPct: undefined },
  ].map(recent => ['invalid recent ' + Object.keys(recent)[0], { ...preview(), recent: { ...preview().recent, ...recent } }] as [string, unknown]),
];
it.each(malformed)('rejects malformed or mismatched response: %s', async (_name, body) => {
  respond(body);
  await expect(priceReviewAPI.preview(purchaseId, 240000)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
});

it('accepts missing cached evidence without inventing a version or gap', async () => {
  const body = { ...preview(), currentPriceCents: 0, status: 'needs_review', evidenceNeedsReview: true,
    evidenceReason: 'No verified CardLadder evidence', evidenceVersion: '', recent: {
      windowStart: '2026-09-08', windowEnd: '2026-09-14', saleIds: [], count: 0, medianCents: 0,
      latestSaleDate: '', latestSaleCount: 0, latestSaleMinCents: 0, latestSaleMaxCents: 0, gapPct: null,
    } };
  respond(body);
  expect(await priceReviewAPI.preview(purchaseId, 240000)).toEqual(body);
});

it.each([400, 401, 404, 429, 500, 503])('preserves structured HTTP %s errors without nested retries', async status => {
  const fetcher = respond({ error: 'Preview failed', code: 'STORAGE', details: { retry: 'read' } }, status);
  await expect(priceReviewAPI.preview(purchaseId, 240000)).rejects.toMatchObject({
    status, code: 'STORAGE', message: 'Preview failed', data: { details: { retry: 'read' } },
  });
  expect(fetcher).toHaveBeenCalledTimes(1);
});
it('does not retry network errors or invalid JSON', async () => {
  const fetcher = vi.fn().mockRejectedValueOnce(new TypeError('offline')).mockResolvedValueOnce(new Response('{'));
  vi.stubGlobal('fetch', fetcher);
  await expect(priceReviewAPI.preview(purchaseId, 240000)).rejects.toMatchObject({ code: 'NETWORK_ERROR' });
  expect(fetcher).toHaveBeenCalledTimes(1);
  await expect(priceReviewAPI.preview(purchaseId, 240000)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  expect(fetcher).toHaveBeenCalledTimes(2);
});

it.each([200, 503])('bounds stalled success/error bodies by deadline (%s)', async status => {
  vi.useFakeTimers();
  const cancel = vi.fn();
  const fetcher = vi.fn(async () => new Response(new ReadableStream({
    start(c) { c.enqueue(new TextEncoder().encode('{')); }, cancel,
  }), { status }));
  vi.stubGlobal('fetch', fetcher);
  const result = priceReviewAPI.preview(purchaseId, 240000, { timeoutMs: 50 }).catch(error => error);
  await vi.advanceTimersByTimeAsync(51);
  expect(await result).toMatchObject({ code: 'TIMEOUT' });
  expect(cancel).toHaveBeenCalledOnce(); expect(fetcher).toHaveBeenCalledOnce();
  expect(vi.getTimerCount()).toBe(0);
});
it.each([200, 503])('cancels a locked, stalled preview body (%s)', async status => {
  const cancel = vi.fn();
  let headers!: () => void;
  const received = new Promise<void>(resolve => { headers = resolve; });
  const fetcher = vi.fn(async () => {
    headers();
    return new Response(new ReadableStream({ start(c) { c.enqueue(new TextEncoder().encode('{')); }, cancel }), { status });
  });
  vi.stubGlobal('fetch', fetcher);
  const abort = new AbortController();
  const result = priceReviewAPI.preview(purchaseId, 240000, { signal: abort.signal }).catch(error => error);
  await received; await Promise.resolve(); await Promise.resolve(); abort.abort();
  expect(await result).toMatchObject({ code: 'CANCELLED' });
  expect(cancel).toHaveBeenCalledOnce(); expect(fetcher).toHaveBeenCalledOnce();
});
it('does not fetch when already canceled', async () => {
  const fetcher = respond(preview());
  const abort = new AbortController(); abort.abort();
  await expect(priceReviewAPI.preview(purchaseId, 240000, { signal: abort.signal })).rejects.toMatchObject({ code: 'CANCELLED' });
  expect(fetcher).not.toHaveBeenCalled();
});
