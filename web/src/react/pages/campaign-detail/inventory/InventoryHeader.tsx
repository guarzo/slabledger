import { useMemo } from 'react';
import type { AgingItem, EVPortfolio } from '../../../../types/campaigns';
import type { TabCounts, FilterTab } from './inventoryCalcs';
import { formatCents, formatPct } from '../../../utils/formatters';
import { formatPL } from './utils';
import { Button } from '../../../ui';
import { useMediaQuery } from '../../../hooks/useMediaQuery';
import CrackCandidatesBanner from './CrackCandidatesBanner';
import BulkSelectionMissingCLWarning from './BulkSelectionMissingCLWarning';

export interface InventoryHeaderProps {
  items: AgingItem[];
  filteredCount: number;
  totalCost: number;
  totalMarket: number;
  fullInventoryTotals: { totalCost: number; totalMarket: number; totalPL: number };
  showEV: boolean;
  evPortfolio: EVPortfolio | null | undefined;
  searchQuery: string;
  setSearchQuery: (q: string) => void;
  showAll: boolean;
  setShowAll: React.Dispatch<React.SetStateAction<boolean>>;
  filterTab: FilterTab;
  setFilterTab: (tab: FilterTab) => void;
  tabCounts: TabCounts;
  debouncedSearch: string;
  selected: ReadonlySet<string>;
  campaignId?: string;
  onDeselectMissingCL: (purchaseIds: string[]) => void;
  onHighlightMissingCL: (purchaseIds: string[]) => void;
}

export default function InventoryHeader({
  items, filteredCount,
  totalCost, totalMarket,
  fullInventoryTotals,
  showEV, evPortfolio,
  searchQuery, setSearchQuery,
  showAll, setShowAll, filterTab, setFilterTab,
  tabCounts, debouncedSearch,
  selected,
  campaignId,
  onDeselectMissingCL, onHighlightMissingCL,
}: InventoryHeaderProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  const missingCLIds = useMemo(() => {
    if (selected.size === 0) return [];
    return items.filter(i => selected.has(i.purchase.id) && !i.purchase.clValueCents).map(i => i.purchase.id);
  }, [items, selected]);

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
    { key: 'awaiting_intake' as const, label: 'Awaiting Intake', count: tabCounts.awaiting_intake, alwaysShow: false },
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

  const showNeedsHeadline = !showAll
    && !hasSearch
    && filterTab !== 'needs_attention'
    && tabCounts.needs_attention > 0;

  return (
    <>
      {/* Universe totals — always reflect the full inventory, never the filter result. */}
      <div className="mb-4 sell-sheet-no-print">
        {universe.totalMarket > 0 ? (
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <span
              className={`text-2xl font-bold tabular-nums tracking-tight ${universePlPositive ? 'text-[var(--success)]' : 'text-[var(--state-problem)]'}`}
              aria-label={`Unrealized ${universePlPositive ? 'gain' : 'loss'} ${formatPL(universe.totalPL)}`}
            >
              {formatPL(universe.totalPL)}
              <span className="text-base opacity-80">{universePlPctSuffix}</span>
            </span>
            <span className="text-sm text-[var(--text-muted)]">unrealized</span>
          </div>
        ) : (
          <div className="text-2xl font-bold tabular-nums tracking-tight text-[var(--text)]">
            {items.length} {items.length === 1 ? 'card' : 'cards'}
          </div>
        )}
        <div className="mt-1 flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm">
          {universe.totalMarket > 0 && (
            <>
              <span className="text-[var(--text-muted)] tabular-nums">
                {items.length} {items.length === 1 ? 'card' : 'cards'}
              </span>
              <span className="text-[var(--text-muted)]">·</span>
            </>
          )}
          <span className="text-[var(--text-muted)] tabular-nums">
            Cost <span className="text-[var(--text)]">{formatCents(universe.totalCost)}</span>
          </span>
          {universe.totalMarket > 0 && (
            <>
              <span className="text-[var(--text-muted)]">·</span>
              <span className="text-[var(--text-muted)] tabular-nums">
                Market <span className="text-[var(--text)]">{formatCents(universe.totalMarket)}</span>
              </span>
            </>
          )}
          {showEV && evPortfolio && (
            <>
              <span className="text-[var(--text-muted)]">·</span>
              <span className="text-[var(--text-muted)] tabular-nums">
                EV <span className={evPortfolio.totalEvCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--state-problem)]'}>{formatPL(evPortfolio.totalEvCents)}</span>
              </span>
            </>
          )}
          {/* Filter-tab summary. Search has its own dedicated summary line
              below the filter pills, so this fragment only renders for
              non-search filter narrowing — the two are mutually exclusive. */}
          {isFiltering && !hasSearch && (
            <>
              <span className="text-[var(--text-muted)]">·</span>
              <span className="text-[var(--text-subtle)] tabular-nums">
                Showing {filteredCount}
                {totalMarket > 0 && <> · Cost {formatCents(totalCost)}</>}
              </span>
            </>
          )}
        </div>
      </div>

      {/* Needs Attention call-to-action: only when there's something to do and the user isn't already there */}
      {showNeedsHeadline && (
        <button
          type="button"
          onClick={() => setFilterTab('needs_attention')}
          className="mb-4 w-full flex items-center justify-between gap-3 rounded-lg border border-[var(--warning)]/30 bg-[var(--warning)]/10 px-4 py-2.5 text-left hover:bg-[var(--warning)]/15 transition-colors sell-sheet-no-print"
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

      {/* Crack Candidates Banner */}
      {campaignId && <div className="sell-sheet-no-print"><CrackCandidatesBanner campaignId={campaignId} /></div>}

      {/* Filter pills + inline search/Show All — replaces the heavy ReviewSummaryBar panel */}
      <div className="flex flex-col gap-2 mb-3 sell-sheet-no-print">
        <div className="flex flex-wrap items-center gap-2">
          {!showAll && primary.map(tab => {
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
            <Button
              variant={showAll ? 'primary' : 'secondary'}
              size="sm"
              onClick={() => setShowAll(prev => !prev)}
              title={showAll ? 'Return to filter tabs' : 'Show every card, ignoring filter tabs'}
            >
              {showAll ? 'Use Filters' : 'Show All'}
            </Button>
          </div>
        </div>
        {!showAll && secondary.length > 0 && (
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
      </div>

      {hasSearch && (
        <div className="text-xs text-[var(--text-subtle)] mb-2 pl-1 sell-sheet-no-print">
          {filteredCount} of {items.length} cards
        </div>
      )}

    </>
  );
}
