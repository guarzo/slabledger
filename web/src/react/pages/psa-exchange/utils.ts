import type { PsaExchangeOpportunity } from '../../../types/psaExchange';

export type SortKey =
  | 'cert'
  | 'description'
  | 'grade'
  | 'listPrice'
  | 'targetOffer'
  | 'comp'
  | 'velocityMonth'
  | 'daysToSell'
  | 'edgeAtOffer'
  | 'listRunwayPct'
  | 'score'
  | 'confidence'
  | 'population';

export type SortDir = 'asc' | 'desc';

export type QuickView = 'all' | 'takeAtList' | 'highLiquidity';

export interface Filters {
  search: string;
  grades: number[];
  minEdgePct: number;
  takeAtListOnly: boolean;
}

export const defaultFilters: Filters = {
  search: '',
  grades: [],
  minEdgePct: 0,
  takeAtListOnly: false,
};

// Days until next sale at observed velocity. Falls back to quarterly velocity
// when monthly is zero. Returns +Infinity when neither window has any sales.
export function daysToSell(o: PsaExchangeOpportunity): number {
  if (o.velocityMonth > 0) return 30 / o.velocityMonth;
  if (o.velocityQuarter > 0) return 90 / o.velocityQuarter;
  return Number.POSITIVE_INFINITY;
}

// Color buckets — return Tailwind class strings matching the design tokens.
export function edgeBucketClass(edge: number): string {
  if (edge >= 0.30) return 'text-[var(--success)] font-semibold';
  if (edge >= 0.15) return 'text-[var(--warning)]';
  return 'text-[var(--text-muted)]';
}

export function daysBucketClass(days: number): string {
  if (!Number.isFinite(days)) return 'text-[var(--text-muted)]';
  if (days <= 6) return 'text-[var(--success)] font-semibold';
  if (days <= 15) return 'text-[var(--warning)]';
  return 'text-[var(--text-muted)]';
}

export function velocityBucketClass(velMonth: number): string {
  if (velMonth >= 10) return 'text-[var(--success)] font-semibold';
  if (velMonth >= 3) return 'text-[var(--warning)]';
  return 'text-[var(--text-muted)]';
}

// True if score is in the top decile of the supplied set; used to glow rows.
export function topDecileThreshold(scores: number[]): number {
  if (scores.length === 0) return Number.POSITIVE_INFINITY;
  const sorted = [...scores].sort((a, b) => b - a);
  const idx = Math.max(0, Math.floor(sorted.length * 0.1) - 1);
  return sorted[idx];
}

// Confidence color: 1-10 scale from upstream cardladder.
export function confidenceColorClass(conf: number): string {
  if (conf >= 7) return 'text-[var(--success)]';
  if (conf >= 5) return 'text-[var(--warning)]';
  return 'text-[var(--text-muted)]';
}

export function applyFilters(
  rows: PsaExchangeOpportunity[],
  f: Filters,
  qv: QuickView,
): PsaExchangeOpportunity[] {
  const search = f.search.trim().toLowerCase();
  return rows.filter((r) => {
    if (qv === 'takeAtList' && !r.mayTakeAtList) return false;
    if (qv === 'highLiquidity' && r.tier !== 'high_liquidity') return false;
    if (f.takeAtListOnly && !r.mayTakeAtList) return false;
    if (f.grades.length > 0) {
      const g = Number(r.grade);
      if (!f.grades.includes(g)) return false;
    }
    if (f.minEdgePct > 0 && r.edgeAtOffer < f.minEdgePct) return false;
    if (search) {
      const desc = (r.description || '').toLowerCase();
      const name = (r.name || '').toLowerCase();
      const cert = (r.cert || '').toLowerCase();
      if (!desc.includes(search) && !name.includes(search) && !cert.includes(search)) {
        return false;
      }
    }
    return true;
  });
}

export function applySort(
  rows: PsaExchangeOpportunity[],
  key: SortKey,
  dir: SortDir,
): PsaExchangeOpportunity[] {
  const mult = dir === 'asc' ? 1 : -1;
  return [...rows].sort((a, b) => {
    const av = sortValue(a, key);
    const bv = sortValue(b, key);
    if (typeof av === 'string' && typeof bv === 'string') {
      return av.localeCompare(bv) * mult;
    }
    return ((av as number) - (bv as number)) * mult;
  });
}

function sortValue(o: PsaExchangeOpportunity, key: SortKey): number | string {
  switch (key) {
    case 'cert':
      return o.cert;
    case 'description':
      return (o.description || o.name || '').toLowerCase();
    case 'grade':
      return Number(o.grade) || 0;
    case 'listPrice':
      return o.listPrice;
    case 'targetOffer':
      return o.targetOffer;
    case 'comp':
      return o.comp;
    case 'velocityMonth':
      return o.velocityMonth;
    case 'daysToSell': {
      const d = daysToSell(o);
      return Number.isFinite(d) ? d : Number.MAX_SAFE_INTEGER;
    }
    case 'edgeAtOffer':
      return o.edgeAtOffer;
    case 'listRunwayPct':
      return o.listRunwayPct;
    case 'score':
      return o.score;
    case 'confidence':
      return o.confidence;
    case 'population':
      return o.population;
  }
}

// Default direction when first selecting a column. Numeric-bigger-is-better
// columns sort desc; everything else asc.
export function defaultDirFor(key: SortKey): SortDir {
  switch (key) {
    case 'cert':
    case 'description':
    case 'daysToSell':
    case 'listRunwayPct':
      return 'asc';
    default:
      return 'desc';
  }
}

export interface OpportunityGroup {
  key: string;
  primary: PsaExchangeOpportunity;
  members: PsaExchangeOpportunity[];
}

// Group by normalized description; tiebreak picks the highest-score row as
// the displayed primary so the group inherits the best member's signals.
export function groupByDescription(rows: PsaExchangeOpportunity[]): OpportunityGroup[] {
  const groups = new Map<string, OpportunityGroup>();
  for (const r of rows) {
    const key = (r.description || r.name || r.cert).trim().toLowerCase();
    let g = groups.get(key);
    if (!g) {
      g = { key, primary: r, members: [r] };
      groups.set(key, g);
      continue;
    }
    g.members.push(r);
    if (r.score > g.primary.score) g.primary = r;
  }
  return Array.from(groups.values());
}
