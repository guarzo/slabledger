import type { SellSheetItem } from '../../types/campaigns';

export type SliceID =
  | 'psa10'
  | 'modern'
  | 'vintage'
  | 'highValue'
  | 'underOneK'
  | 'byGrade'
  | 'full';

export interface SliceResult {
  id: SliceID;
  label: string;
  description: string;
  items: SellSheetItem[];
  itemCount: number;
  totalAskCents: number;
}

export interface SliceSet {
  psa10: SliceResult;
  modern: SliceResult;
  vintage: SliceResult;
  highValue: SliceResult;
  underOneK: SliceResult;
  byGrade: SliceResult;
  full: SliceResult;
  totalItemCount: number;
  totalAskCents: number;
  unparseableYearCount: number;
}

const HIGH_VALUE_CENTS = 100000;
const ERA_CUTOFF_YEAR = 2020;
const LEADING_YEAR_RE = /^(\d{4})/;

export function parseCardYear(input: string | undefined | null): number | null {
  if (!input) return null;
  const m = LEADING_YEAR_RE.exec(input);
  return m ? parseInt(m[1], 10) : null;
}

function totals(items: SellSheetItem[]): { itemCount: number; totalAskCents: number } {
  return {
    itemCount: items.length,
    totalAskCents: items.reduce((sum, i) => sum + (i.targetSellPrice ?? 0), 0),
  };
}

function byPriceDesc(a: SellSheetItem, b: SellSheetItem): number {
  return (b.targetSellPrice ?? 0) - (a.targetSellPrice ?? 0);
}

function bySetThenNumber(a: SellSheetItem, b: SellSheetItem): number {
  const setCmp = (a.setName ?? '').localeCompare(b.setName ?? '');
  if (setCmp !== 0) return setCmp;
  return (a.cardNumber ?? '').localeCompare(b.cardNumber ?? '', undefined, { numeric: true });
}

function byGradeThenPriceDesc(a: SellSheetItem, b: SellSheetItem): number {
  const g = (b.grade ?? 0) - (a.grade ?? 0);
  if (g !== 0) return g;
  return byPriceDesc(a, b);
}

function makeSlice(
  id: SliceID,
  label: string,
  description: string,
  items: SellSheetItem[],
): SliceResult {
  const t = totals(items);
  return { id, label, description, items, ...t };
}

export function computeSlices(input: SellSheetItem[]): SliceSet {
  const psa10Items = input
    .filter((i) => i.grader === 'PSA' && i.grade === 10)
    .slice()
    .sort(byPriceDesc);

  const modernItems: SellSheetItem[] = [];
  const vintageItems: SellSheetItem[] = [];
  let unparseableYearCount = 0;
  for (const it of input) {
    const yr = parseCardYear(it.cardYear);
    if (yr === null) {
      unparseableYearCount++;
      continue;
    }
    if (yr >= ERA_CUTOFF_YEAR) modernItems.push(it);
    else vintageItems.push(it);
  }
  modernItems.sort(bySetThenNumber);
  vintageItems.sort(bySetThenNumber);

  const highValueItems = input
    .filter((i) => (i.targetSellPrice ?? 0) >= HIGH_VALUE_CENTS)
    .slice()
    .sort(byPriceDesc);

  const underOneKItems = input
    .filter((i) => (i.targetSellPrice ?? 0) < HIGH_VALUE_CENTS)
    .slice()
    .sort(byPriceDesc);

  const byGradeItems = input.slice().sort(byGradeThenPriceDesc);

  const fullItems = input.slice().sort(bySetThenNumber);

  const overall = totals(input);

  return {
    psa10: makeSlice('psa10', 'PSA 10s', 'Every PSA 10, priced high to low', psa10Items),
    modern: makeSlice('modern', `Modern (${ERA_CUTOFF_YEAR}+)`, `Cards from ${ERA_CUTOFF_YEAR} or later, by set`, modernItems),
    vintage: makeSlice('vintage', `Vintage (pre-${ERA_CUTOFF_YEAR})`, `Cards before ${ERA_CUTOFF_YEAR}, by set`, vintageItems),
    highValue: makeSlice('highValue', 'High-Value ($1,000+)', 'Cards asking $1,000 or more', highValueItems),
    underOneK: makeSlice('underOneK', 'Under $1,000', 'Cards asking under $1,000', underOneKItems),
    byGrade: makeSlice('byGrade', 'By Grade (local card store)', 'Sorted grade desc, then price', byGradeItems),
    full: makeSlice('full', 'Full List', 'Every item, by set', fullItems),
    totalItemCount: overall.itemCount,
    totalAskCents: overall.totalAskCents,
    unparseableYearCount,
  };
}
