import { describe, it, expect } from 'vitest';
import { computeSlices, parseCardYear } from './sellSheetSlices';
import type { SellSheetItem } from '../../types/campaigns';

function item(overrides: Partial<SellSheetItem> = {}): SellSheetItem {
  return {
    purchaseId: 'p1',
    certNumber: '12345',
    cardName: 'Charizard',
    setName: 'Base Set',
    cardNumber: '4',
    grade: 9,
    grader: 'PSA',
    buyCostCents: 10000,
    costBasisCents: 11000,
    clValueCents: 20000,
    recommendation: 'stable',
    targetSellPrice: 50000,
    minimumAcceptPrice: 45000,
    ...overrides,
  };
}

describe('parseCardYear', () => {
  it('extracts a leading 4-digit year', () => {
    expect(parseCardYear('1999')).toBe(1999);
    expect(parseCardYear('1999-2000')).toBe(1999);
    expect(parseCardYear('2022 Pokemon')).toBe(2022);
  });

  it('returns null when no leading 4-digit run', () => {
    expect(parseCardYear('')).toBeNull();
    expect(parseCardYear(undefined)).toBeNull();
    expect(parseCardYear('Pokemon 1999')).toBeNull();
    expect(parseCardYear('99-00')).toBeNull();
  });
});

describe('computeSlices', () => {
  const all: SellSheetItem[] = [
    item({ purchaseId: 'a', grader: 'PSA', grade: 10, targetSellPrice: 150000 }),
    item({ purchaseId: 'b', grader: 'PSA', grade: 10, targetSellPrice: 50000 }),
    item({ purchaseId: 'c', grader: 'BGS', grade: 10, targetSellPrice: 80000 }),
    item({ purchaseId: 'd', grader: 'PSA', grade: 9,  targetSellPrice: 200000 }),
  ];

  it('PSA10s slice: only PSA grader at grade 10, sorted by price desc', () => {
    const slices = computeSlices(all);
    const ids = slices.psa10.items.map((i) => i.purchaseId);
    expect(ids).toEqual(['a', 'b']);
  });

  it('PSA10s totals reflect the filtered subset', () => {
    const slices = computeSlices(all);
    expect(slices.psa10.itemCount).toBe(2);
    expect(slices.psa10.totalAskCents).toBe(150000 + 50000);
  });

  it('high-value slice: targetSellPrice >= $1000 (100000c), sorted desc', () => {
    const slices = computeSlices(all);
    const ids = slices.highValue.items.map((i) => i.purchaseId);
    expect(ids).toEqual(['d', 'a']);
  });

  it('under-1k slice: targetSellPrice < 100000c', () => {
    const slices = computeSlices(all);
    const ids = slices.underOneK.items.map((i) => i.purchaseId);
    expect(ids).toEqual(['c', 'b']);
  });

  it('era split uses cardYear regex; missing year excluded from both eras', () => {
    const items: SellSheetItem[] = [
      item({ purchaseId: 'm1', cardYear: '2022', setName: 'B', cardNumber: '2' }),
      item({ purchaseId: 'm2', cardYear: '2020-2021', setName: 'A', cardNumber: '1' }),
      item({ purchaseId: 'v1', cardYear: '1999', setName: 'C', cardNumber: '4' }),
      item({ purchaseId: 'v2', cardYear: '2019', setName: 'D', cardNumber: '3' }),
      item({ purchaseId: 'x', cardYear: '' }),
    ];
    const slices = computeSlices(items);
    expect(slices.modern.items.map((i) => i.purchaseId)).toEqual(['m2', 'm1']);
    expect(slices.vintage.items.map((i) => i.purchaseId)).toEqual(['v1', 'v2']);
    expect(slices.unparseableYearCount).toBe(1);
  });

  it('byGrade slice: all items, grade desc then price desc', () => {
    const slices = computeSlices(all);
    const ids = slices.byGrade.items.map((i) => i.purchaseId);
    expect(ids).toEqual(['a', 'c', 'b', 'd']);
  });

  it('full slice: all items, set asc then card number asc', () => {
    const items: SellSheetItem[] = [
      item({ purchaseId: '1', setName: 'B', cardNumber: '1' }),
      item({ purchaseId: '2', setName: 'A', cardNumber: '10' }),
      item({ purchaseId: '3', setName: 'A', cardNumber: '2' }),
    ];
    const slices = computeSlices(items);
    expect(slices.full.items.map((i) => i.purchaseId)).toEqual(['3', '2', '1']);
  });

  it('overall total reflects the full input', () => {
    const slices = computeSlices(all);
    expect(slices.totalItemCount).toBe(4);
    expect(slices.totalAskCents).toBe(150000 + 50000 + 80000 + 200000);
  });
});
