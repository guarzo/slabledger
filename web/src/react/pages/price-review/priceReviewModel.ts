import type { AgingItem } from '../../../types/campaigns';
import type { ShowEvaluation, SupportStatus } from '../../../types/showprep';
import { isShowEvaluation } from '../../../js/api/showprep';
import { dollarsToCents } from '../../utils/formatters';

export type PriceReviewFilter = 'all' | 'above' | 'mixed' | 'limited' | 'supported' | 'unavailable' | 'unpriced';
export type PriceReviewSort = 'attention' | 'supported' | 'asking' | 'recent' | 'gap';
export interface PriceDraft { value: string; baselinePriceCents: number }

export const reviewFilters: { value: PriceReviewFilter; label: string }[] = [
  { value: 'all', label: 'All' }, { value: 'above', label: 'Above comps' }, { value: 'mixed', label: 'Mixed' },
  { value: 'limited', label: 'Limited' }, { value: 'supported', label: 'Supported' },
  { value: 'unavailable', label: 'Unavailable' }, { value: 'unpriced', label: 'Unpriced' },
];
export const assessmentGroups: Record<SupportStatus, Exclude<PriceReviewFilter, 'all'>> = {
  below_target: 'above', mixed_evidence: 'mixed', thin_evidence: 'limited', no_recent_comps: 'limited',
  supported: 'supported', needs_review: 'unavailable', no_listed_price: 'unpriced',
};
export const assessmentLabels: Record<SupportStatus, string> = {
  below_target: 'Asking above comps', mixed_evidence: 'Mixed evidence', thin_evidence: 'Limited evidence',
  no_recent_comps: 'Limited evidence', supported: 'Supported', needs_review: 'Evidence unavailable', no_listed_price: 'No asking price',
};
const attention = { above: 0, mixed: 1, limited: 2, unavailable: 3, unpriced: 4, supported: 5 };

export function priceGroup(e?: ShowEvaluation): Exclude<PriceReviewFilter, 'all'> {
  return isShowEvaluation(e) && e.availability !== 'unknown' ? assessmentGroups[e.status] : 'unavailable';
}
export function parsePriceDraft(value: string): number | null {
  const trimmed = value.trim();
  // The shared parser accepts prefixes. Financial input must be an entire USD amount.
  if (!/^(?:\d+(?:\.\d{0,2})?|\.\d{1,2})$/.test(trimmed)) return null;
  // Parse the whole-dollar portion separately so floating-point decimal parsing
  // cannot move the final cent near the JS-safe ceiling.
  const [whole, fractional = ''] = trimmed.split('.');
  const cents = dollarsToCents(whole || '0') + Number(fractional.padEnd(2, '0'));
  return Number.isSafeInteger(cents) && cents > 0 ? cents : null;
}
export function gapLabel(gap: number | null | undefined): string {
  if (gap == null) return 'Gap unavailable';
  if (gap === 0) return 'At asking';
  return `${Math.abs(gap).toFixed(1)}% ${gap > 0 ? 'below' : 'above'} asking`;
}

function knownFirst(a: number | null, b: number | null, descending: boolean): number {
  if (a === null) return b === null ? 0 : 1;
  if (b === null) return -1;
  return (descending ? -1 : 1) * (a - b);
}
function sortValue(e: ShowEvaluation | undefined, sort: PriceReviewSort): number | null {
  if (!isShowEvaluation(e) || e.availability === 'unknown') return null;
  if (sort === 'asking') return e.localPriceCents > 0 ? e.localPriceCents : null;
  if (e.evidenceNeedsReview) return null;
  if (sort === 'recent') return e.recent.count > 0 ? e.recent.medianCents : null;
  return e.recent.gapPct;
}
export function buildPriceReview(items: AgingItem[], evaluations: Record<string, ShowEvaluation>, options: {
  search: string; filter: PriceReviewFilter; sort: PriceReviewSort; descending: boolean;
}) {
  const search = options.search.trim().toLowerCase();
  const scope = items.filter(({ purchase: p, campaignName }) => !search
    || [p.cardName, p.certNumber, p.setName, campaignName].some(value => value?.toLowerCase().includes(search)));
  const counts: Record<PriceReviewFilter, number> = { all: scope.length, above: 0, mixed: 0, limited: 0, supported: 0, unavailable: 0, unpriced: 0 };
  for (const item of scope) counts[priceGroup(evaluations[item.purchase.id])]++;
  const rows = scope.filter(item => options.filter === 'all' || priceGroup(evaluations[item.purchase.id]) === options.filter);
  rows.sort((a, b) => {
    const ea = evaluations[a.purchase.id]; const eb = evaluations[b.purchase.id];
    if (options.sort === 'attention' || options.sort === 'supported') {
      const rank = (e?: ShowEvaluation) => options.sort === 'supported' && priceGroup(e) === 'supported' ? -1 : attention[priceGroup(e)];
      return rank(ea) - rank(eb) || knownFirst(sortValue(ea, 'gap'), sortValue(eb, 'gap'), true);
    }
    return knownFirst(sortValue(ea, options.sort), sortValue(eb, options.sort), options.descending);
  });
  return { rows, counts };
}
