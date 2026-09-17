import { act, renderHook } from '@testing-library/react';
import { expect, it } from 'vitest';
import { usePriceReviewState } from './usePriceReviewState';
import { evaluation, inventoryItem, purchaseId, otherId } from './fixtures.test-support';

function setup() {
  const a = evaluation(); const b = evaluation({ purchaseId: otherId, status: 'supported', localPriceCents: 240000,
    reason: 'Recent sales support asking.', recent: { ...a.recent, gapPct: 0 } });
  const props = { items: [inventoryItem(a), inventoryItem(b)], evaluations: { [purchaseId]: a, [otherId]: b }, search: '' };
  const hook = renderHook(p => usePriceReviewState(p.items, p.evaluations, p.search), { initialProps: props });
  return { hook, props };
}
it('retains focus and per-card dirty text through evidence changes, sort, search and filter changes', () => {
  const { hook, props } = setup();
  act(() => { hook.result.current.focus(purchaseId); hook.result.current.setDraft(purchaseId, { value: '2400', baselinePriceCents: 280000 }); });
  act(() => { hook.result.current.focus(otherId); hook.result.current.setDraft(otherId, { value: '19.', baselinePriceCents: 240000 }); });
  act(() => hook.result.current.focus(purchaseId));
  hook.rerender({ ...props, evaluations: { ...props.evaluations, [purchaseId]: evaluation({ localPriceCents: 270000, status: 'supported', version: 'new', evidenceVersion: 'new-sales',
    reason: 'Recent sales support asking.', recent: { ...evaluation().recent, medianCents: 260000, latestSaleMinCents: 250000, latestSaleMaxCents: 250000, gapPct: 3.703704 } }) } });
  act(() => { hook.result.current.setFilter('above'); hook.result.current.setSort('asking'); hook.result.current.setDescending(false); });
  expect(hook.result.current.activeId).toBe(purchaseId);
  expect(hook.result.current.outsideFilter).toBe(true);
  expect(hook.result.current.drafts).toEqual({ [purchaseId]: { value: '2400', baselinePriceCents: 280000 }, [otherId]: { value: '19.', baselinePriceCents: 240000 } });
  hook.rerender({ ...props, search: 'nothing' });
  expect(hook.result.current.activeId).toBe(purchaseId);
  expect(hook.result.current.rows).toEqual([]);
  act(() => hook.result.current.move(1));
  expect(hook.result.current.activeId).toBe(purchaseId);
});
it.each([-1, 1] as const)('chooses adjacent surviving queue item after active leaves filter, direction %s', delta => {
  const es = ['before', 'active', 'after'].map((purchaseId, n) => evaluation({ purchaseId, recent: { ...evaluation().recent, gapPct: 30 - n } }));
  const props = { items: es.map(e => inventoryItem(e)), evaluations: Object.fromEntries(es.map(e => [e.purchaseId, e])) };
  const hook = renderHook(p => usePriceReviewState(p.items, p.evaluations, ''), { initialProps: props });
  act(() => { hook.result.current.setFilter('above'); hook.result.current.focus('active'); });
  hook.rerender({ ...props, evaluations: { ...props.evaluations, active: { ...es[1], status: 'supported', localPriceCents: 240000, recent: { ...es[1].recent, gapPct: 0 } } } });
  expect(hook.result.current.outsideFilter).toBe(true);
  act(() => hook.result.current.move(delta));
  expect(hook.result.current.activeId).toBe(delta === -1 ? 'before' : 'after');
});
it('keeps an unavailable identity and its unsaved text when a card disappears, without retaining stale purchase objects', () => {
  const { hook, props } = setup();
  act(() => hook.result.current.setDraft(purchaseId, { value: '2400', baselinePriceCents: 280000 }));
  hook.rerender({ ...props, items: [props.items[1]] });
  expect(hook.result.current.activeId).toBe(purchaseId);
  expect(hook.result.current.drafts[purchaseId].value).toBe('2400');
  expect(hook.result.current.rows.map(i => i.purchase.id)).toEqual([otherId]);
  act(() => hook.result.current.move(1));
  expect(hook.result.current.activeId).toBe(otherId);
  act(() => hook.result.current.clearDraft(purchaseId));
  expect(hook.result.current.drafts[purchaseId]).toBeUndefined();
});
