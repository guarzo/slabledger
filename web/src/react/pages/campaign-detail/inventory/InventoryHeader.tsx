import { useMemo } from 'react';
import type { AgingItem } from '../../../../types/campaigns';
import type { TabCounts, FilterTab, PriceBand, PriceBandCounts } from './inventoryCalcs';
import { formatCents, formatPct } from '../../../utils/formatters';
import { formatPL } from './utils';
import { useMediaQuery } from '../../../hooks/useMediaQuery';
import BulkSelectionMissingCLWarning from './BulkSelectionMissingCLWarning';

export interface InventoryHeaderProps {
  items: AgingItem[];
  filteredCount: number;
  totalCost: number;
  totalMarket: number;
  fullInventoryTotals: { totalCost: number; totalMarket: number; totalPL: number };
  searchQuery: string;
  setSearchQuery: (q: string) => void;
  filterTab: FilterTab;
  setFilterTab: (tab: FilterTab) => void;
  tabCounts: TabCounts;
  priceBand: PriceBand;
  setPriceBand: (b: PriceBand) => void;
  priceBandCounts: PriceBandCounts;
  debouncedSearch: string;
  selected: ReadonlySet<string>;
  onDeselectMissingCL: (purchaseIds: string[]) => void;
  onHighlightMissingCL: (purchaseIds: string[]) => void;
}

export default function InventoryHeader({
  items, filteredCount,
  totalCost, totalMarket,
  fullInventoryTotals,
  searchQuery, setSearchQuery,
  filterTab, setFilterTab,
  tabCounts, priceBand, setPriceBand, priceBandCounts, debouncedSearch,
  selected,
  onDeselectMissingCL, onHighlightMissingCL,
}: InventoryHeaderProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  const missingCLIds = useMemo(() => {
    if (selected.size === 0) return [];
    return items.filter(i => selected.has(i.purchase.id) && !i.purchase.clValueCents).map(i => i.purchase.id);
  }, [items, selected]);

  const priceBands = useMemo(() => [
    { key: 'lt50' as const, label: '<$50', count: priceBandCounts.lt50 },
    { key: '50to100' as const, label: '$50–100', count: priceBandCounts['50to100'] },
    { key: '100to250' as const, label: '$100–250', count: priceBandCounts['100to250'] },
    { key: '250to500' as const, label: '$250–500', count: priceBandCounts['250to500'] },
    { key: 'gte500' as const, label: '$500+', count: priceBandCounts.gte500 },
  ], [priceBandCounts]);

  const primary = useMemo(() => [
    { key: 'needs_attention' as const, label: 'Needs Attention', count: tabCounts.needs_attention, alwaysShow: true },
    { key: 'ready_to_list' as const, label: 'Pending DH Listing', count: tabCounts.ready_to_list, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts]);
  const secondary = useMemo(() => [
    { key: 'all' as const, label: 'All', count: tabCounts.all, alwaysShow: true },
    { key: 'dh_listed' as const, label: 'DH Listed', count: tabCounts.dh_listed, alwaysShow: false },
    { key: 'pending_dh_match' as const, label: 'Pending DH Match', count: tabCounts.pending_dh_match, alwaysShow: false },
    { key: 'pending_price' as const, label: 'Pending Price', count: tabCounts.pending_price, alwaysShow: false },
    { key: 'skipped' as const, label: 'Skipped on DH Listing', count: tabCounts.skipped, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts]);

  const pillClass = (isActive: boolean, size: 'primary' | 'secondary') => {
    const base = 'shrink-0 inline-flex items-center rounded-full border transition-colors tabular-nums';
    const sizing = size === 'primary' ? 'text-xs font-semibold px-3 py-1.5' : 'text-[11px] font-medium px-2.5 py-1';
    const stateClass = isActive
      ? 'border-[var(--brand-500)] bg-[var(--brand-500)]/10 text-[var(--brand-400)]'
      : 'border-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]';
    return `${base} ${sizing} ${stateClass}`;
  };
  const countClass = (isActive: boolean, size: 'primary' | 'secondary') => {
    const base = 'ml-1.5 inline-flex items-center justify-center rounded-full text-[10px] font-semibold px-1 tabular-nums';
    const sizing = size === 'primary' ? 'min-w-[22px] h-[18px]' : 'min-w-[20px] h-[16px]';
    const stateClass = isActive
      ? 'bg-[var(--brand-500)]/20 text-[var(--brand-300)]'
      : 'bg-[rgba(255,255,255,0.06)] text-[var(--text-muted)]';
    return `${base} ${sizing} ${stateClass}`;
  };

  // Hero always reflects the universe — never the filter result. A filter chip
  // returning 0 must not zero out the page-level totals.
  const universe = fullInventoryTotals;
  const universePlPositive = universe.totalPL >= 0;
  const universePlPctSuffix = universe.totalCost > 0
    ? ` (${universePlPositive ? '+' : ''}${formatPct(universe.totalPL / universe.totalCost)})`
    : '';
  // Normalized search flag — whitespace-only queries shouldn't count as
  // an active search anywhere downstream.
  const hasSearch = !!debouncedSearch?.trim();
  const isFiltering = (filterTab !== 'all' || hasSearch) && filteredCount !== items.length;

  const showNeedsHeadline = !hasSearch
    && filterTab !== 'needs_attention'
    && tabCounts.needs_attention > 0;

  return (
    <>
      {/* Universe totals — always reflect the full inventory, never the filter result.
          Mirrors the dashboard's Slab Terminal hero treatment: an editorial-display
          headline (Fraunces) with the supporting stats demoted to a tabular mono
          strip, plus a small cost-vs-market bar that visualizes where the unrealized
          delta sits without needing historical data. */}
      <div className="mb-5">
        {universe.totalMarket > 0 ? (
          <>
            <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--brand-400)] mb-1">
              Unrealized P&amp;L
            </div>
            <div className="flex flex-wrap items-end gap-x-6 gap-y-2">
              <span
                className={`tabular-nums leading-[0.95] tracking-[-0.035em] ${universePlPositive ? 'text-[var(--success)]' : 'text-[var(--state-problem)]'}`}
                style={{
                  fontFamily: 'var(--font-display)',
                  fontSize: 'clamp(40px, 6vw, 72px)',
                  fontWeight: 500,
                  fontFeatureSettings: '"ss01", "ss02", "tnum" 1, "lnum" 1',
                }}
                aria-label={`Unrealized ${universePlPositive ? 'gain' : 'loss'} ${formatPL(universe.totalPL)}`}
              >
                {formatPL(universe.totalPL)}
                <span className="ml-2 text-[0.42em] opacity-80 align-baseline">{universePlPctSuffix}</span>
              </span>
              {/* Cost-vs-market bar: cost is the anchor, market sits to the left
                  (loss) or right (gain) by an amount proportional to the delta.
                  Capped visually at ±60% of cost so giant outliers don't break
                  the scale. */}
              <CostVsMarketBar
                costCents={universe.totalCost}
                marketCents={universe.totalMarket}
                positive={universePlPositive}
              />
            </div>
          </>
        ) : (
          <div className="text-2xl font-bold tabular-nums tracking-tight text-[var(--text)]">
            {items.length} {items.length === 1 ? 'card' : 'cards'}
          </div>
        )}
        <div className="mt-2 flex flex-wrap items-baseline gap-x-4 gap-y-1 text-xs">
          {universe.totalMarket > 0 && (
            <>
              <StatStrip label="Cards" value={`${items.length}`} />
              <StatStrip label="Cost" value={formatCents(universe.totalCost)} />
              <StatStrip label="Market" value={formatCents(universe.totalMarket)} />
            </>
          )}
          {tabCounts.awaiting_intake > 0 && (
            <StatStrip
              label="Awaiting Intake"
              value={`${tabCounts.awaiting_intake}`}
            />
          )}
          {/* Filter-tab summary. Search has its own dedicated summary line
              below the filter pills, so this fragment only renders for
              non-search filter narrowing — the two are mutually exclusive. */}
          {isFiltering && !hasSearch && (
            <span className="text-[var(--text-subtle)] tabular-nums">
              Showing {filteredCount}
              {totalMarket > 0 && <> · Cost {formatCents(totalCost)}</>}
            </span>
          )}
        </div>
      </div>

      {/* Needs Attention call-to-action: only when there's something to do and the user isn't already there */}
      {showNeedsHeadline && (
        <button
          type="button"
          onClick={() => setFilterTab('needs_attention')}
          className="mb-4 w-full flex items-center justify-between gap-3 rounded-lg border border-[var(--warning)]/30 bg-[var(--warning)]/10 px-4 py-2.5 text-left hover:bg-[var(--warning)]/15 transition-colors"
        >
          <span className="text-sm text-[var(--text)]">
            <span className="font-semibold tabular-nums">{tabCounts.needs_attention}</span>{' '}
            {tabCounts.needs_attention === 1 ? 'card needs' : 'cards need'} attention
          </span>
          <span className="text-xs text-[var(--text-muted)]">Review →</span>
        </button>
      )}

      <BulkSelectionMissingCLWarning
        missingCLIds={missingCLIds}
        selectedCount={selected.size}
        onDeselect={onDeselectMissingCL}
        onHighlight={onHighlightMissingCL}
      />

      {/* Filter pills + inline search — replaces the heavy ReviewSummaryBar panel */}
      <div className="flex flex-col gap-2 mb-3">
        <div className="flex flex-wrap items-center gap-2">
          {primary.map(tab => {
            const isActive = filterTab === tab.key;
            return (
              <button key={tab.key} type="button" onClick={() => setFilterTab(tab.key)} aria-pressed={isActive} className={pillClass(isActive, 'primary')}>
                {tab.label}
                <span className={countClass(isActive, 'primary')}>{tab.count}</span>
              </button>
            );
          })}
          <div className={`flex items-center gap-2 ${isMobile ? 'w-full' : 'ml-auto'}`}>
            <input
              type="text"
              aria-label="Search cards"
              placeholder="Search by cert, card, or set…"
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              className={`${isMobile ? 'flex-1' : 'w-48'} px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:border-[var(--accent)]`}
            />
          </div>
        </div>
        {secondary.length > 0 && (
          <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1.5">
            {secondary.map(tab => {
              const isActive = filterTab === tab.key;
              return (
                <button key={tab.key} type="button" onClick={() => setFilterTab(tab.key)} aria-pressed={isActive} className={pillClass(isActive, 'secondary')}>
                  {tab.label}
                  <span className={countClass(isActive, 'secondary')}>{tab.count}</span>
                </button>
              );
            })}
          </div>
        )}
        {/* Price-band pill row — independent of tab/search; composes with all of them.
            Built for in-person liquidation triage where the operator wants to surface
            "everything in this dollar tier" fast. Bands hide at count=0 unless active.
            The whole row is hidden when no band has any items AND no band is active,
            so an empty/priceless inventory doesn't show a label-only row. */}
        {(priceBands.some(b => b.count > 0) || priceBand !== 'all') && (
          <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1.5">
            <span className="text-[10px] uppercase tracking-[0.08em] font-medium text-[var(--text-muted)] mr-1">Price</span>
            {priceBands.map(band => {
              const isActive = priceBand === band.key;
              if (band.count === 0 && !isActive) return null;
              return (
                <button
                  key={band.key}
                  type="button"
                  onClick={() => setPriceBand(isActive ? 'all' : band.key)}
                  aria-pressed={isActive}
                  className={pillClass(isActive, 'secondary')}
                >
                  {band.label}
                  <span className={countClass(isActive, 'secondary')}>{band.count}</span>
                </button>
              );
            })}
            {priceBand !== 'all' && (
              <button
                type="button"
                onClick={() => setPriceBand('all')}
                className="text-[11px] text-[var(--text-muted)] hover:text-[var(--text)] underline-offset-2 hover:underline ml-1"
              >
                Clear
              </button>
            )}
          </div>
        )}
      </div>

      {hasSearch && (
        <div className="text-xs text-[var(--text-subtle)] mb-2 pl-1">
          {filteredCount} of {items.length} cards
        </div>
      )}

    </>
  );
}

/** Single label/value cell in the demoted stat strip beneath the hero. */
function StatStrip({ label, value, tone }: { label: string; value: string; tone?: 'success' | 'problem' }) {
  const valueColor = tone === 'success'
    ? 'text-[var(--success)]'
    : tone === 'problem'
      ? 'text-[var(--state-problem)]'
      : 'text-[var(--text)]';
  return (
    <span className="inline-flex items-baseline gap-1.5">
      <span className="text-[10px] uppercase tracking-[0.08em] font-medium text-[var(--text-muted)]">{label}</span>
      <span className={`tabular-nums ${valueColor}`}>{value}</span>
    </span>
  );
}

/** Cost-vs-market diverging bar.
    The cost basis is the anchor at the centre; the market value extends a
    coloured fill to the right (gain) or left (loss) proportional to the
    unrealized delta. Capped at ±60% of cost so a single 10x outlier doesn't
    flatten the scale for everyone else. Uses a single inline SVG so the bar
    composes with the page's existing CSS without bringing in a chart lib. */
function CostVsMarketBar({ costCents, marketCents, positive }: {
  costCents: number;
  marketCents: number;
  positive: boolean;
}) {
  if (costCents <= 0) return null;
  const delta = marketCents - costCents;
  const ratio = Math.max(-0.6, Math.min(0.6, delta / costCents));
  const halfWidth = 80; // px each side of the anchor
  const fillWidth = Math.abs(ratio / 0.6) * halfWidth;
  const fillX = ratio >= 0 ? halfWidth : halfWidth - fillWidth;
  const fillColor = positive ? 'var(--success)' : 'var(--state-problem)';
  return (
    <svg
      width={halfWidth * 2}
      height={20}
      viewBox={`0 0 ${halfWidth * 2} 20`}
      role="img"
      aria-label={`Market value ${positive ? 'above' : 'below'} cost basis`}
      className="shrink-0"
    >
      {/* Track */}
      <rect x={0} y={9} width={halfWidth * 2} height={2} fill="rgba(255,255,255,0.06)" rx={1} />
      {/* Delta fill */}
      <rect x={fillX} y={7} width={fillWidth} height={6} fill={fillColor} opacity={0.55} rx={1} />
      {/* Cost anchor — small notch at the centre */}
      <rect x={halfWidth - 1} y={3} width={2} height={14} fill="var(--text-muted)" rx={1} />
    </svg>
  );
}
