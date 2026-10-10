import { afterEach, expect, it, vi } from 'vitest';
import { showPrepAPI } from './showprep';
import { evaluation, listId, member, purchaseId } from '../../react/pages/show-preparation/fixtures.test-support';
const command = { commandId: '55555555-5555-4555-8555-555555555555', certNumber: '00123AB', withdrawalGeneration: 2 };
const result = () => ({ original: { commandId: command.commandId, certNumber: command.certNumber, listId, outcome: 'confirmed', code: '', itemId: member().id, purchaseId, cardName: 'Card', confirmedAt: '2026-10-09T12:00:00Z', acknowledgedPriceCents: 30000, acknowledgedStatus: 'supported' }, replay: false, current: { state: 'present', evaluation: evaluation({ certNumber: command.certNumber }) } });
afterEach(() => vi.unstubAllGlobals());
it('sends the exact immutable scan command with credentials', async () => {
  const fetcher = vi.fn().mockResolvedValue(Response.json(result())); vi.stubGlobal('fetch', fetcher);
  expect(await showPrepAPI.scanConfirm(listId, command)).toEqual(result());
  expect(fetcher).toHaveBeenCalledWith(`/api/show-prep/lists/${listId}/scan-confirm`, expect.objectContaining({ body: JSON.stringify(command), method: 'POST', credentials: 'include' }));
});
it.each([
  ['command mismatch', { commandId: listId }], ['list mismatch', { listId: purchaseId }], ['cert mismatch', { certNumber: '123AB' }],
  ['bad outcome', { outcome: 'packed' }], ['fraction cents', { acknowledgedPriceCents: 1.1 }], ['negative cents', { acknowledgedPriceCents: -1 }],
  ['unsafe cents', { acknowledgedPriceCents: Number.MAX_SAFE_INTEGER + 1 }], ['missing item', { itemId: '' }], ['missing purchase', { purchaseId: '' }],
  ['missing time', { confirmedAt: '' }], ['invalid time', { confirmedAt: '2026-02-30T12:00:00Z' }], ['empty success status', { acknowledgedStatus: '' }],
  ['unknown status', { acknowledgedStatus: 'packed' }], ['success rejection code', { code: 'sold' }],
])('does not trust %s in a successful response', async (_name, patch) => {
  const data = result(); Object.assign(data.original, patch); vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(data)));
  await expect(showPrepAPI.scanConfirm(listId, command)).rejects.toThrow(/Invalid scan response/);
});
it.each([
  { replay: 'true' }, { current: { state: 'packed' } }, { current: { state: 'present', evaluation: {} } },
  { current: { state: 'present', evaluation: evaluation({ purchaseId: listId }) } },
])('rejects malformed current projections independently of original consent', async patch => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ ...result(), ...patch })));
  await expect(showPrepAPI.scanConfirm(listId, command)).rejects.toThrow(/Invalid scan response/);
});
it('accepts a replay with removed current state and no evaluation, preserving original baseline', async () => {
  const data = { ...result(), replay: true, current: { state: 'removed' } }; vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(data)));
  expect(await showPrepAPI.scanConfirm(listId, command)).toEqual(data);
});
it('accepts a committed rejection with empty identity, time and acknowledged status', async () => {
  const data = { ...result(), original: { ...result().original, outcome: 'rejected', code: 'no_match', itemId: '', purchaseId: '', cardName: '', confirmedAt: '', acknowledgedPriceCents: 0, acknowledgedStatus: '' }, current: { state: 'unknown' } };
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(data))); expect(await showPrepAPI.scanConfirm(listId, command)).toEqual(data);
});
it.each([-1, 0.5, Number.MAX_SAFE_INTEGER + 1])('refuses invalid generation %s without submitting', async withdrawalGeneration => {
  const fetcher = vi.fn(); vi.stubGlobal('fetch', fetcher);
  await expect(showPrepAPI.scanConfirm(listId, { ...command, withdrawalGeneration })).rejects.toThrow(/Invalid scan command/); expect(fetcher).not.toHaveBeenCalled();
});
it.each([{ state: 'removed' }, { state: 'present' }])('rejects non-replay success without a present full evaluation', async current => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ ...result(), current })));
  await expect(showPrepAPI.scanConfirm(listId, command)).rejects.toThrow(/Invalid scan response/);
});
it('rejects evaluation with malformed successful identity and cents', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ ...result(), current: { state: 'present', evaluation: evaluation({ certNumber: command.certNumber, localPriceCents: -1, cardName: undefined }) } })));
  await expect(showPrepAPI.scanConfirm(listId, command)).rejects.toThrow(/Invalid scan response/);
});
it('does not automatically retry typed contention', async () => {
  const fetcher = vi.fn().mockResolvedValue(Response.json({ error: 'Retry same command', code: 'scan_contended', retryable: true }, { status: 503 })); vi.stubGlobal('fetch', fetcher);
  await expect(showPrepAPI.scanConfirm(listId, command)).rejects.toMatchObject({ code: 'scan_contended', status: 503 }); expect(fetcher).toHaveBeenCalledTimes(1);
});
