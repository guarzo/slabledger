import { afterEach, describe, expect, it, vi } from 'vitest';
import { showPrepAPI, evaluateInventory } from './showprep';
import { detail, evaluation, purchaseId, listId } from '../../react/pages/show-preparation/fixtures.test-support';

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

describe('show preparation transport', () => {
  it('uses exact evaluation and versioned list contracts, without financial writes', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(json({ evaluations: [evaluation()] }))
      .mockResolvedValueOnce(json(detail())).mockResolvedValueOnce(json(detail()));
    vi.stubGlobal('fetch', fetcher);
    expect((await showPrepAPI.evaluate([purchaseId])).evaluations[0].listedPriceCents).toBe(30000);
    expect(fetcher).toHaveBeenNthCalledWith(1, '/api/show-prep/evaluate', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ purchaseIds: [purchaseId] }), credentials: 'include',
    }));
    await showPrepAPI.addItems(listId, [{ purchaseId, evaluationVersion: 'eval-1' }]);
    await showPrepAPI.updateItem(listId, detail().items[0].id, { version: 1, evaluationVersion: 'eval-1', packed: true });
    expect(fetcher.mock.calls.map(c => c[0])).toEqual([
      '/api/show-prep/evaluate', `/api/show-prep/lists/${listId}/items`,
      `/api/show-prep/lists/${listId}/items/${detail().items[0].id}`,
    ]);
  });

  it.each(['network', '500', 'timeout'])('never replays a failed explicit refresh: %s', async failure => {
    vi.useFakeTimers();
    const fetcher = vi.fn().mockImplementation((_url, options) => {
      if (failure === 'network') return Promise.reject(new TypeError('Network failed'));
      if (failure === '500') return Promise.resolve(json({ error: 'Source failed' }, 500));
      return new Promise((_resolve, reject) => options.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError'))));
    });
    vi.stubGlobal('fetch', fetcher);
    const result = showPrepAPI.refresh([purchaseId]).catch((e: Error) => e);
    await vi.advanceTimersByTimeAsync(120000);
    expect(await result).toBeInstanceOf(Error);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it('batches evaluations at 200 with at most three concurrent reads and records missing results', async () => {
    const ids = Array.from({ length: 805 }, (_, i) => `id-${i}`);
    let active = 0; let peak = 0;
    const sizes: number[] = [];
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async (_url, options) => {
      const { purchaseIds } = JSON.parse(options.body);
      sizes.push(purchaseIds.length); peak = Math.max(peak, ++active);
      await new Promise(resolve => setTimeout(resolve, 2)); active--;
      return json({ evaluations: purchaseIds.filter((id: string) => id !== 'id-5').map((purchaseId: string) => evaluation({ purchaseId })) });
    }));
    const result = await evaluateInventory(ids);
    expect(sizes.sort((a, b) => a - b)).toEqual([5, 200, 200, 200, 200]);
    expect(peak).toBeLessThanOrEqual(3);
    expect(Object.keys(result.evaluations)).toHaveLength(804);
    expect(result.errors['id-5']).toMatch(/missing/i);
  });

  it('does not treat a partial evaluation object as a supported result', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({ evaluations: [{ purchaseId, version: 'eval-1', status: 'supported', availability: 'ready' }] })));
    const result = await evaluateInventory([purchaseId]);
    expect(result.evaluations[purchaseId]).toBeUndefined();
    expect(result.errors[purchaseId]).toMatch(/missing|invalid/i);
  });

  it.each([
    { evidenceNeedsReview: undefined, evidenceReason: '' },
    { evidenceNeedsReview: false, evidenceReason: undefined },
    { evidenceNeedsReview: 'false', evidenceReason: '' },
    { evidenceNeedsReview: false, evidenceReason: null },
  ])('rejects missing or incorrectly typed evidence health: %j', async health => {
    const value = { ...evaluation(), ...health };
    vi.stubGlobal('fetch', vi.fn(async (url: string) => json(url.includes('/evidence/')
      ? { evaluation: value, sales: [] } : { evaluations: [value] })));
    const result = await evaluateInventory([purchaseId]);
    expect(result.evaluations[purchaseId]).toBeUndefined();
    expect(result.errors[purchaseId]).toMatch(/missing|invalid/i);
    await expect(showPrepAPI.evidence(purchaseId)).rejects.toThrow(/invalid/i);
  });

  it('rejects malformed arrays instead of calling them empty evidence', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({ evaluations: null })));
    const result = await evaluateInventory([purchaseId]);
    expect(result.evaluations[purchaseId]).toBeUndefined();
    expect(result.errors[purchaseId]).toMatch(/invalid/i);
  });
});
