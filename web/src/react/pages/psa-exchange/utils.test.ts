import { describe, expect, it } from 'vitest';
import type { PsaExchangeOpportunity } from '../../../types/psaExchange';
import {
  applyFilters,
  applySort,
  daysToSell,
  defaultDirFor,
  defaultFilters,
  edgeBucketClass,
  daysBucketClass,
  groupByDescription,
  topDecileThreshold,
} from './utils';

function row(overrides: Partial<PsaExchangeOpportunity> = {}): PsaExchangeOpportunity {
  return {
    cert: '111',
    name: 'Sample',
    description: 'Sample card',
    grade: '10',
    listPrice: 1000,
    targetOffer: 800,
    maxOfferPct: 0.65,
    comp: 1200,
    lastSalePrice: 1000,
    lastSaleDate: '2026-04-01T00:00:00Z',
    velocityMonth: 4,
    velocityQuarter: 12,
    confidence: 5,
    population: 100,
    edgeAtOffer: 0.5,
    score: 1.0,
    listRunwayPct: 0.2,
    mayTakeAtList: false,
    frontImage: '',
    backImage: '',
    indexId: 'sample',
    tier: 'default',
    ...overrides,
  };
}

describe('daysToSell', () => {
  it('uses monthly velocity when present', () => {
    expect(daysToSell(row({ velocityMonth: 30, velocityQuarter: 0 }))).toBe(1);
    expect(daysToSell(row({ velocityMonth: 6, velocityQuarter: 0 }))).toBe(5);
  });

  it('falls back to quarterly velocity when monthly is zero', () => {
    expect(daysToSell(row({ velocityMonth: 0, velocityQuarter: 9 }))).toBe(10);
  });

  it('returns +Infinity when neither window has sales', () => {
    expect(daysToSell(row({ velocityMonth: 0, velocityQuarter: 0 }))).toBe(Number.POSITIVE_INFINITY);
  });
});

describe('color buckets', () => {
  it('edgeBucketClass tiers by 0.30 / 0.15', () => {
    expect(edgeBucketClass(0.4)).toContain('success');
    expect(edgeBucketClass(0.2)).toContain('warning');
    expect(edgeBucketClass(0.05)).toContain('text-muted');
  });

  it('daysBucketClass tiers by 6 / 15 days', () => {
    expect(daysBucketClass(3)).toContain('success');
    expect(daysBucketClass(10)).toContain('warning');
    expect(daysBucketClass(30)).toContain('text-muted');
    expect(daysBucketClass(Number.POSITIVE_INFINITY)).toContain('text-muted');
  });
});

describe('applyFilters', () => {
  const rows = [
    row({ cert: 'A', description: 'pikachu charizard', grade: '10', edgeAtOffer: 0.4, mayTakeAtList: true, tier: 'high_liquidity' }),
    row({ cert: 'B', description: 'mew', grade: '9', edgeAtOffer: 0.1, mayTakeAtList: false, tier: 'default' }),
    row({ cert: 'C', description: 'pikachu', grade: '10', edgeAtOffer: 0.2, mayTakeAtList: false, tier: 'default' }),
  ];

  it('filters by takeAtList quick view', () => {
    const out = applyFilters(rows, defaultFilters, 'takeAtList');
    expect(out.map((r) => r.cert)).toEqual(['A']);
  });

  it('filters by highLiquidity quick view', () => {
    const out = applyFilters(rows, defaultFilters, 'highLiquidity');
    expect(out.map((r) => r.cert)).toEqual(['A']);
  });

  it('filters by grade chips', () => {
    const out = applyFilters(rows, { ...defaultFilters, grades: [10] }, 'all');
    expect(out.map((r) => r.cert).sort()).toEqual(['A', 'C']);
  });

  it('filters by min edge', () => {
    const out = applyFilters(rows, { ...defaultFilters, minEdgePct: 0.25 }, 'all');
    expect(out.map((r) => r.cert)).toEqual(['A']);
  });

  it('searches description and cert case-insensitively', () => {
    expect(applyFilters(rows, { ...defaultFilters, search: 'PIKACHU' }, 'all').map((r) => r.cert)).toEqual(['A', 'C']);
    expect(applyFilters(rows, { ...defaultFilters, search: 'B' }, 'all').map((r) => r.cert)).toEqual(['B']);
  });
});

describe('applySort', () => {
  const rows = [
    row({ cert: 'A', score: 0.5, listPrice: 100 }),
    row({ cert: 'B', score: 1.0, listPrice: 300 }),
    row({ cert: 'C', score: 0.7, listPrice: 200 }),
  ];

  it('sorts numeric desc', () => {
    expect(applySort(rows, 'score', 'desc').map((r) => r.cert)).toEqual(['B', 'C', 'A']);
  });

  it('sorts numeric asc', () => {
    expect(applySort(rows, 'listPrice', 'asc').map((r) => r.cert)).toEqual(['A', 'C', 'B']);
  });

  it('sorts daysToSell with infinity at the bottom in asc', () => {
    const r2 = [
      row({ cert: 'A', velocityMonth: 0, velocityQuarter: 0 }),
      row({ cert: 'B', velocityMonth: 30 }),
      row({ cert: 'C', velocityMonth: 6 }),
    ];
    expect(applySort(r2, 'daysToSell', 'asc').map((r) => r.cert)).toEqual(['B', 'C', 'A']);
  });

  it('sorts strings lexicographically', () => {
    const r2 = [row({ cert: 'b' }), row({ cert: 'a' }), row({ cert: 'c' })];
    expect(applySort(r2, 'cert', 'asc').map((r) => r.cert)).toEqual(['a', 'b', 'c']);
  });
});

describe('defaultDirFor', () => {
  it('numeric-bigger-is-better columns default to desc', () => {
    expect(defaultDirFor('score')).toBe('desc');
    expect(defaultDirFor('edgeAtOffer')).toBe('desc');
  });
  it('time/string columns default to asc', () => {
    expect(defaultDirFor('cert')).toBe('asc');
    expect(defaultDirFor('description')).toBe('asc');
    expect(defaultDirFor('daysToSell')).toBe('asc');
  });
});

describe('topDecileThreshold', () => {
  it('returns score of the 10th percentile from the top', () => {
    const scores = Array.from({ length: 10 }, (_, i) => i + 1); // 1..10
    expect(topDecileThreshold(scores)).toBe(10);
  });

  it('handles empty input', () => {
    expect(topDecileThreshold([])).toBe(Number.POSITIVE_INFINITY);
  });
});

describe('groupByDescription', () => {
  it('collapses identical descriptions and picks the highest-score primary', () => {
    const rows = [
      row({ cert: '1', description: 'Pikachu Promo', score: 0.5 }),
      row({ cert: '2', description: 'pikachu promo', score: 0.9 }),
      row({ cert: '3', description: 'Charizard', score: 0.6 }),
    ];
    const groups = groupByDescription(rows);
    expect(groups).toHaveLength(2);
    const pikachu = groups.find((g) => g.key.includes('pikachu'))!;
    expect(pikachu.primary.cert).toBe('2');
    expect(pikachu.members).toHaveLength(2);
  });
});
