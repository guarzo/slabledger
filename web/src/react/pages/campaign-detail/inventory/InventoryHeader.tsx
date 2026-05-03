import { useMemo } from 'react';
import type { AgingItem, EVPortfolio } from '../../../../types/campaigns';
import type { TabCounts, FilterTab } from './inventoryCalcs';
import { formatCents, formatPct } from '../../../utils/formatters';
import { formatPL } from './utils';
import { Button } from '../../../ui';
import CrackCandidatesBanner from './CrackCandidatesBanner';
import { SellSheetActions } from '../SellSheetView';
import BulkSelectionMissingCLWarning from './BulkSelectionMissingCLWarning';

export interface InventoryHeaderProps {
  isMobile: boolean;
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
  pageSellSheetCount: number;
  debouncedSearch: string;
  sellSheetActive: boolean;
  selected: ReadonlySet<string>;
  campaignId?: string;
  isPrinting: boolean;
  onAddToSellSheet: (ids: string[]) => void;
  onRemoveFromSellSheet: (ids: string[]) => void;
  onRecordSale: (items: AgingItem[]) => void;
  onBulkListOnDH: (ids: string[]) => void;
  onClearSelected: () => void;
  onPrint: () => void;
  onDeselectMissingCL: (purchaseIds: string[]) => void;
  onHighlightMissingCL: (purchaseIds: string[]) => void;
}

export default function InventoryHeader({
  isMobile, items, filteredCount,
  totalCost, totalMarket, totalPL,
  fullInventoryTotals,
  showEV, evPortfolio,
  searchQuery, setSearchQuery,
  showAll, setShowAll, filterTab, setFilterTab,
  tabCounts, pageSellSheetCount, debouncedSearch,
  sellSheetActive, selected,
  campaignId, isPrinting,
  onAddToSellSheet, onRemoveFromSellSheet,
  onRecordSale, onBulkListOnDH, onClearSelected, onPrint,
  onDeselectMissingCL, onHighlightMissingCL,
}: InventoryHeaderProps) {
  const missingCLIds = useMemo(() => {
    if (selected.size === 0) return [];
    return items.filter(i => selected.has(i.purchase.id) && !i.purchase.clValueCents).map(i => i.purchase.id);
  }, [items, selected]);

  const primary = useMemo(() => [
    { key: 'needs_attention' as const, label: 'Needs Attention', count: tabCounts.needs_attention, alwaysShow: true },
    { key: 'ready_to_list' as const, label: 'Pending DH Listing', count: tabCounts.ready_to_list, alwaysShow: false },
    { key: 'sell_sheet' as const, label: 'Sell Sheet', count: pageSellSheetCount, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts, pageSellSheetCount]);
  const secondary = useMemo(() => [
    { key: 'all' as const, label: 'All', count: tabCounts.all, alwaysShow: true },
    { key: 'dh_listed' as const, label: 'DH Listed', count: tabCounts.dh_listed, alwaysShow: false },
    { key: 'pending_dh_match' as const, label: 'Pending DH Match', count: tabCounts.pending_dh_match, alwaysShow: false },
    { key: 'pending_price' as const, label: 'Pending Price', count: tabCounts.pending_price, alwaysShow: false },
    { key: 'skipped' as const, label: 'Skipped on DH Listing', count: tabCounts.skipped, alwaysShow: false },
    { key: 'awaiting_intake' as const, label: 'Awaiting Intake', count: tabCounts.awaiting_intake, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts]);

  if (isMobile && sellSheetActive) return null;

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

  // Headline P/L gets a larger, bolder treatment; supporting metrics sit below.
  const plPositive = totalPL >= 0;
  const plPctSuffix = totalCost > 0
    ? ` (${plPositive ? '+' : ''}${formatPct(totalPL / totalCost)})`
    : '';

  const showNeedsHeadline = !showAll
    && !debouncedSearch?.trim()
    && filterTab !== 'needs_attention'
    && tabCounts.needs_attention > 0;

  return (
    <>
      {/* Totals — P/L is the headline when we have market data; otherwise show cards + cost */}
      <div className="mb-4 sell-sheet-no-print">
        {totalMarket > 0 ? (
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <span
              className={`text-2xl font-bold tabular-nums tracking-tight ${plPositive ? 'text-[var(--success)]' : 'text-[var(--state-problem)]'}`}
              aria-label={`Unrealized ${plPositive ? 'gain' : 'loss'} ${formatPL(totalPL)}`}
            >
              {formatPL(totalPL)}
              <span className="text-base opacity-80">{plPctSuffix}</span>
            </span>
            <span className="text-sm text-[var(--text-muted)]">unrealized</span>
          </div>
        ) : (
          <div className="text-2xl font-bold tabular-nums tracking-tight text-[var(--text)]">
            {filteredCount} {filteredCount === 1 ? 'card' : 'cards'}
          </div>
        )}
        <div className="mt-1 flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm">
          {totalMarket > 0 && (
            <>
              <span className="text-[var(--text-muted)] tabular-nums">
                {filteredCount} cards
              </span>
              <span className="text-[var(--text-muted)]">·</span>
            </>
          )}
          <span className="text-[var(--text-muted)] tabular-nums">
            Cost <span className="text-[var(--text)]">{formatCents(totalCost)}</span>
          </span>
          {totalMarket > 0 && (
            <>
              <span className="text-[var(--text-muted)]">·</span>
              <span className="text-[var(--text-muted)] tabular-nums">
                Market <span className="text-[var(--text)]">{formatCents(totalMarket)}</span>
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
        </div>
        {(filterTab !== 'all' || debouncedSearch?.trim()) && filteredCount !== items.length && (
          <div className="mt-1 text-xs text-[var(--text-subtle)] tabular-nums">
            All {items.length} cards · Cost {formatCents(fullInventoryTotals.totalCost)}
            {fullInventoryTotals.totalMarket > 0 && (
              <> · Market {formatCents(fullInventoryTotals.totalMarket)}</>
            )}
          </div>
        )}
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

      {selected.size > 0 || (sellSheetActive && pageSellSheetCount > 0) ? (
        <SellSheetActions
          selected={selected}
          sellSheetActive={sellSheetActive}
          items={items}
          onAddToSellSheet={onAddToSellSheet}
          onRemoveFromSellSheet={onRemoveFromSellSheet}
          onRecordSale={onRecordSale}
          onBulkListOnDH={onBulkListOnDH}
          onClearSelected={onClearSelected}
          isPrinting={isPrinting}
          pageSellSheetCount={pageSellSheetCount}
          onPrint={onPrint}
        />
      ) : null}

      {/* Crack Candidates Banner */}
      {campaignId && <div className="sell-sheet-no-print"><CrackCandidatesBanner campaignId={campaignId} /></div>}

      {/* Filter pills + inline search/Show All — replaces the heavy ReviewSummaryBar panel */}
      <div className="flex flex-col gap-2 mb-3 sell-sheet-no-print">
        <div className="flex flex-wrap items-center gap-2">
          {!showAll && primary.map(tab => {
            const isActive = filterTab === tab.key;
            return (
              <button key={tab.key} type="button" onClick={() => setFilterTab(tab.key)} className={pillClass(isActive, 'primary')}>
                {tab.label}
                <span className={countClass(isActive, 'primary')}>{tab.count}</span>
              </button>
            );
          })}
          <div className={`flex items-center gap-2 ${isMobile ? 'w-full' : 'ml-auto'}`}>
            <input
              type="text"
              placeholder="Search cards…"
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
                <button key={tab.key} type="button" onClick={() => setFilterTab(tab.key)} className={pillClass(isActive, 'secondary')}>
                  {tab.label}
                  <span className={countClass(isActive, 'secondary')}>{tab.count}</span>
                </button>
              );
            })}
          </div>
        )}
      </div>

      {debouncedSearch && (
        <div className="text-xs text-[var(--text-subtle)] mb-2 pl-1 sell-sheet-no-print">
          {filteredCount} of {items.length} cards
        </div>
      )}

      {sellSheetActive && filteredCount === 0 && (
        <div className="text-center py-12">
          <div className="text-[var(--text-muted)] text-sm">No items on your sell sheet.</div>
          <div className="text-[var(--text-muted)] text-xs mt-1">Select items from any tab and click &ldquo;Add to Sell Sheet&rdquo;.</div>
        </div>
      )}
    </>
  );
}
