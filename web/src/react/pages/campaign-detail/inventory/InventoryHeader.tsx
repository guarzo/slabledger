import { useMemo } from 'react';
import type { AgingItem, EVPortfolio } from '../../../../types/campaigns';
import type { TabCounts, FilterTab } from './inventoryCalcs';
import { formatCents, formatPct } from '../../../utils/formatters';
import { formatPL } from './utils';
import ReviewSummaryBar from './ReviewSummaryBar';
import CrackCandidatesBanner from './CrackCandidatesBanner';
import BulkSelectionMissingCLWarning from './BulkSelectionMissingCLWarning';

export interface InventoryHeaderProps {
  items: AgingItem[];
  filteredCount: number;
  totalCost: number;
  totalMarket: number;
  totalPL: number;
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
  totalCost, totalMarket, totalPL,
  fullInventoryTotals,
  showEV, evPortfolio,
  searchQuery, setSearchQuery,
  showAll, setShowAll, filterTab, setFilterTab,
  tabCounts, debouncedSearch,
  selected,
  campaignId,
  onDeselectMissingCL, onHighlightMissingCL,
}: InventoryHeaderProps) {
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

  return (
    <>
      {/* Compact one-line breadcrumb of inventory totals */}
      <div className="mb-4 flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm sell-sheet-no-print">
        <span className="text-[var(--text)] font-semibold tabular-nums">
          {filteredCount} cards
        </span>
        <span className="text-[var(--text-muted)]">·</span>
        <span className="text-[var(--text-muted)] tabular-nums">
          Cost <span className="text-[var(--text)] font-medium">{formatCents(totalCost)}</span>
        </span>
        {totalMarket > 0 && (
          <>
            <span className="text-[var(--text-muted)]">·</span>
            <span className="text-[var(--text-muted)] tabular-nums">
              Market <span className="text-[var(--text)] font-medium">{formatCents(totalMarket)}</span>
            </span>
            <span className="text-[var(--text-muted)]">·</span>
            <span
              className={`tabular-nums font-semibold ${totalPL >= 0 ? 'text-[var(--success)]' : 'text-[var(--state-problem)]'}`}
              aria-label={`Unrealized ${totalPL >= 0 ? 'gain' : 'loss'} ${formatPL(totalPL)}`}
            >
              {formatPL(totalPL)} unrealized
              {totalCost > 0 && (
                <span className="ml-1 opacity-80">
                  ({totalPL >= 0 ? '+' : ''}{formatPct(totalPL / totalCost)})
                </span>
              )}
            </span>
          </>
        )}
        {showEV && evPortfolio && (
          <>
            <span className="text-[var(--text-muted)]">·</span>
            <span className="text-[var(--text-muted)] tabular-nums">
              EV <span className={`font-medium ${evPortfolio.totalEvCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--state-problem)]'}`}>{formatPL(evPortfolio.totalEvCents)}</span>
            </span>
          </>
        )}
      </div>

      {(filterTab !== 'all' || debouncedSearch?.trim()) && filteredCount !== items.length && (
        <div className="mb-4 -mt-3 text-xs text-[var(--text-subtle)] tabular-nums sell-sheet-no-print">
          All {items.length} cards · Cost {formatCents(fullInventoryTotals.totalCost)}
          {fullInventoryTotals.totalMarket > 0 && (
            <> · Market {formatCents(fullInventoryTotals.totalMarket)}</>
          )}
        </div>
      )}

      <BulkSelectionMissingCLWarning
        missingCLIds={missingCLIds}
        selectedCount={selected.size}
        onDeselect={onDeselectMissingCL}
        onHighlight={onHighlightMissingCL}
      />

      {/* Crack Candidates Banner */}
      {campaignId && <div className="sell-sheet-no-print"><CrackCandidatesBanner campaignId={campaignId} /></div>}

      {/* Review Summary Bar */}
      <div className="mb-4 sell-sheet-no-print">
        <ReviewSummaryBar
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          showAll={showAll}
          onToggleShowAll={() => setShowAll(prev => !prev)}
        />
      </div>

      {/* Filter tabs */}
      {!showAll && (
        <div className="flex flex-col gap-2 mb-3 sell-sheet-no-print">
          <div className="flex flex-wrap items-center gap-2">
            {primary.map(tab => {
              const isActive = filterTab === tab.key;
              return (
                <button key={tab.key} type="button" onClick={() => setFilterTab(tab.key)} className={pillClass(isActive, 'primary')}>
                  {tab.label}
                  <span className={countClass(isActive, 'primary')}>{tab.count}</span>
                </button>
              );
            })}
          </div>
          {secondary.length > 0 && (
            <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1.5">
              {secondary.map(tab => {
                const isActive = filterTab === tab.key;
                return (
                  <button key={tab.key} type="button" onClick={() => setFilterTab(tab.key)} className={pillClass(isActive, 'secondary')}>
                    {tab.label}
                    <span className={countClass(isActive, 'secondary')}>{tab.count}</span>
                  </button>
                );
              })}
            </div>
          )}
        </div>
      )}

      {debouncedSearch && (
        <div className="text-xs text-[var(--text-subtle)] mb-2 pl-1 sell-sheet-no-print">
          {filteredCount} of {items.length} cards
        </div>
      )}

    </>
  );
}
