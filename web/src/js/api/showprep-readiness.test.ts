import { afterEach, describe, expect, it, vi } from 'vitest';
import * as api from './showprep';
import { detail, evaluation, purchaseId, listId } from '../../react/pages/show-preparation/fixtures.test-support';

const identityKey = 'a'.repeat(64);
const current = { state: 'current', refreshEligibility: 'not_needed', identityKey, expiresAt: '2026-09-15T00:00:00Z', retryAt: '' };
const cold = { ...current, state: 'not_checked', refreshEligibility: 'needed', expiresAt: '' };
const running = { ...cold, state: 'running', refreshEligibility: 'wait', retryAt: '2026-09-14T12:02:00Z' };
const unavailable = { ...cold, state: 'unavailable', refreshEligibility: 'unavailable', identityKey: '' };
const invalid = [
  undefined, null, false, 0, 'current', [], {},
  { ...current, state: 'future_state' }, { ...current, state: 1 },
  { ...current, refreshEligibility: 'unknown' }, { ...current, refreshEligibility: null },
  { ...current, refreshEligibility: 'needed' },
  { ...current, state: 'failed', refreshEligibility: 'needed' },
  { ...current, identityKey: undefined }, { ...current, identityKey: null },
  { ...current, identityKey: 123 }, { ...current, identityKey: '' },
  { ...current, identityKey: ' ' }, { ...current, identityKey: 'a'.repeat(63) },
  { ...current, identityKey: 'z'.repeat(64) }, { ...current, identityKey: 'A'.repeat(64) },
  { ...current, expiresAt: undefined }, { ...current, expiresAt: null },
  { ...current, expiresAt: 123 }, { ...current, expiresAt: '' },
  { ...current, expiresAt: '2026-09-15' }, { ...current, expiresAt: '2026-09-15T00:00:00' },
  { ...current, expiresAt: '2026-02-30T00:00:00Z' }, { ...current, expiresAt: '2026-09-15T24:00:00Z' },
  { ...current, expiresAt: '2026-09-15T00:00:00+04:00' },
  { ...current, expiresAt: '2026-09-15T00:00:00.1234567890Z' },
  { ...current, retryAt: undefined }, { ...current, retryAt: null }, { ...current, retryAt: 123 },
  { ...current, retryAt: '2026-09-15T00:00:00Z' },
  { ...running, retryAt: '' }, { ...running, retryAt: 'not-a-time' },
  { ...running, retryAt: '2026-02-30T00:00:00Z' },
  { ...running, expiresAt: '2026-09-15T00:00:00Z' },
  { ...cold, expiresAt: '2026-09-15T00:00:00Z' },
  { ...unavailable, identityKey: 'bad' },
];

afterEach(() => vi.unstubAllGlobals());

describe('validated optional show readiness', () => {
  it.each([
    cold, current, { ...current, state: 'stale', refreshEligibility: 'needed' }, running,
    { ...running, state: 'interrupted', refreshEligibility: 'retry_only' },
    { ...cold, state: 'failed', refreshEligibility: 'retry_only' },
    { ...cold, state: 'invalid', refreshEligibility: 'retry_only' }, unavailable,
    { ...unavailable, identityKey },
    { ...current, expiresAt: '2026-09-15T00:00:00.123456789Z' },
    { ...running, retryAt: '2026-09-14T12:02:00.000000001Z' },
  ])('accepts server contract %j', readiness => {
    expect(api).toHaveProperty('getShowReadiness', expect.any(Function));
    expect(api.getShowReadiness({ readiness })).toEqual(readiness);
  });

  it.each(invalid.map(readiness => [readiness]))('ignores missing/unknown/malformed metadata %j', readiness => {
    expect(api).toHaveProperty('getShowReadiness', expect.any(Function));
    expect(api.getShowReadiness({ readiness })).toBeUndefined();
  });

  it('handles a missing evaluation without manufacturing cold evidence', () => {
    expect(api).toHaveProperty('getShowReadiness', expect.any(Function));
    expect(api.getShowReadiness(undefined)).toBeUndefined();
    expect(api.getShowReadiness(null)).toBeUndefined();
    expect(api.getShowReadiness({})).toBeUndefined();
  });

  it.each(invalid.map(readiness => [readiness]))('preserves otherwise valid evaluation and manual operations for %j', async readiness => {
    const value = { ...evaluation(), readiness };
    const list = detail();
    list.items[0].evaluation = value;
    const fetcher = vi.fn(async (url: string) => new Response(JSON.stringify(
      url.includes('/evidence/') ? { evaluation: value, sales: [] }
        : url.includes('/lists/') ? list : { evaluations: [value] },
    ), { status: 200 }));
    vi.stubGlobal('fetch', fetcher);
    expect(api.isShowEvaluation(value)).toBe(true);
    const inventory = await api.evaluateInventory([purchaseId]);
    expect(inventory.errors).toEqual({});
    expect(inventory.evaluations[purchaseId].status).toBe('supported');
    expect((await api.showPrepAPI.evidence(purchaseId)).evaluation.status).toBe('supported');
    expect((await api.showPrepAPI.evaluate([purchaseId])).evaluations[0].status).toBe('supported');
    expect((await api.showPrepAPI.detail(listId)).items[0].evaluation.status).toBe('supported');
    await api.showPrepAPI.addItems(listId, [{ purchaseId, evaluationVersion: value.version }]);
    expect(fetcher).toHaveBeenLastCalledWith(`/api/show-prep/lists/${listId}/items`, expect.objectContaining({
      body: JSON.stringify({ items: [{ purchaseId, evaluationVersion: value.version }] }),
    }));
  });
});
