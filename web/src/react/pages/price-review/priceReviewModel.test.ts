import { describe, expect, it } from 'vitest';
import type { ShowEvaluation, SupportStatus } from '../../../types/showprep';
import { buildPriceReview, priceGroup, parsePriceDraft } from './priceReviewModel';
import { evaluation, inventoryItem } from './fixtures.test-support';

it.each<[SupportStatus, string]>([
  ['below_target', 'above'], ['mixed_evidence', 'mixed'], ['thin_evidence', 'limited'],
  ['no_recent_comps', 'limited'], ['supported', 'supported'], ['needs_review', 'unavailable'], ['no_listed_price', 'unpriced'],
])('groups %s using only the server judgment, never requalifying facts', (status, group) => {
  // Deliberately contradictory numbers would break a local price assessor.
  expect(priceGroup(evaluation({ status, recent: { ...evaluation().recent, medianCents: 1, count: 1 } }))).toBe(group);
});
it('separates missing/invalid evaluation from healthy sparse evidence', () => {
  expect(priceGroup(undefined)).toBe('unavailable');
  expect(priceGroup({ ...evaluation(), recent: undefined } as unknown as ShowEvaluation)).toBe('unavailable');
  expect(priceGroup(evaluation({ status: 'needs_review', evidenceNeedsReview: true }))).toBe('unavailable');
  expect(priceGroup(evaluation({ status: 'no_recent_comps' }))).toBe('limited');
});

const cohort = (statuses: SupportStatus[]) => {
  const es = statuses.map((status, n) => evaluation({ purchaseId: `card-${n}`, status }));
  return { items: es.map(e => inventoryItem(e)), evaluations: Object.fromEntries(es.map(e => [e.purchaseId, e])) };
};
const ids = (result: ReturnType<typeof buildPriceReview>) => result.rows.map(i => i.purchase.id);
const defaults = { search: '', filter: 'all' as const, sort: 'attention' as const, descending: true };

it('orders attention above, mixed, limited, unavailable, unpriced, supported; supported-first retains other precedence', () => {
  const data = cohort(['supported', 'no_listed_price', 'needs_review', 'thin_evidence', 'mixed_evidence', 'below_target']);
  expect(ids(buildPriceReview(data.items, data.evaluations, defaults))).toEqual(['card-5', 'card-4', 'card-3', 'card-2', 'card-1', 'card-0']);
  expect(ids(buildPriceReview(data.items, data.evaluations, { ...defaults, sort: 'supported' }))).toEqual(['card-0', 'card-5', 'card-4', 'card-3', 'card-2', 'card-1']);
});
it.each(['  AURORA  ', '00000001', 'northern lights', 'Autumn collection'])('searches card/cert/set/campaign and scopes counts to %s', search => {
  const e = evaluation();
  const other = evaluation({ purchaseId: 'other', cardName: 'Orbit Fox', certNumber: '00000002', status: 'supported' });
  const item = inventoryItem(other); item.campaignName = 'Summer'; item.purchase.setName = 'Solar';
  const result = buildPriceReview([inventoryItem(e), item], { [e.purchaseId]: e, other }, { ...defaults, search, filter: 'supported' });
  expect(result.rows).toEqual([]);
  expect(result.counts).toEqual({ all: 1, above: 1, mixed: 0, limited: 0, supported: 0, unavailable: 0, unpriced: 0 });
});
it('asking sorts the displayed local cents rather than DH prices or market references; missing stays last both ways', () => {
  const data = cohort(['supported', 'supported', 'supported']);
  data.evaluations['card-0'].localPriceCents = 200;
  data.evaluations['card-0'].listedPriceCents = 999999;
  data.evaluations['card-1'].localPriceCents = 300;
  data.evaluations['card-2'].localPriceCents = 0;
  expect(ids(buildPriceReview(data.items, data.evaluations, { ...defaults, sort: 'asking' }))).toEqual(['card-1', 'card-0', 'card-2']);
  expect(ids(buildPriceReview(data.items, data.evaluations, { ...defaults, sort: 'asking', descending: false }))).toEqual(['card-0', 'card-1', 'card-2']);
});
it('uses precise server gaps, orders larger known gaps first within an attention group and leaves unknown last', () => {
  const data = cohort(['thin_evidence', 'thin_evidence', 'thin_evidence', 'below_target']);
  data.evaluations['card-0'].recent.gapPct = null;
  data.evaluations['card-1'].recent.gapPct = 10.000001;
  data.evaluations['card-2'].recent.gapPct = 10.000002;
  data.evaluations['card-3'].recent.gapPct = -1;
  expect(ids(buildPriceReview(data.items, data.evaluations, defaults))).toEqual(['card-3', 'card-2', 'card-1', 'card-0']);
  expect(ids(buildPriceReview(data.items, data.evaluations, { ...defaults, sort: 'gap' }))).toEqual(['card-2', 'card-1', 'card-3', 'card-0']);
  expect(ids(buildPriceReview(data.items, data.evaluations, { ...defaults, sort: 'gap', descending: false }))).toEqual(['card-3', 'card-1', 'card-2', 'card-0']);
});
it('recent sort never treats empty or unhealthy evidence as a zero-dollar bargain', () => {
  const data = cohort(['thin_evidence', 'thin_evidence', 'needs_review']);
  data.evaluations['card-0'].recent.count = 0;
  data.evaluations['card-1'].recent.medianCents = 10000;
  data.evaluations['card-2'].evidenceNeedsReview = true;
  expect(ids(buildPriceReview(data.items, data.evaluations, { ...defaults, sort: 'recent', descending: false }))).toEqual(['card-1', 'card-0', 'card-2']);
});

describe('safe dollar drafts', () => {
  it.each(['', '0', '-1', '1e3', 'Infinity', '12usd', '1.001', '1,000', '90071992547409.92'])('rejects %s', value => {
    expect(parsePriceDraft(value)).toBeNull();
  });
  it.each([['2400', 240000], ['0.01', 1], ['12.34', 1234], [' 25.10 ', 2510], ['.50', 50],
    ['90071992547409.90', 9007199254740990], ['90071992547409.91', 9007199254740991]] as const)('parses %s as integer cents', (value, cents) => {
    expect(parsePriceDraft(value)).toBe(cents);
  });
});
