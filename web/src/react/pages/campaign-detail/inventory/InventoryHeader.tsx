import { useMemo } from 'react';
import type { AgingItem, EVPortfolio } from '../../../../types/campaigns';
import type { ReviewStats } from '../../../../types/campaigns/priceReview';
import type { TabCounts, FilterTab } from './inventoryCalcs';
import { formatCents, formatPct } from '../../../utils/formatters';
import { formatPL } from './utils';
import ReviewSummaryBar from './ReviewSummaryBar';
import type { StatClickTarget } from './ReviewSummaryBar';
import CrackCandidatesBanner from './CrackCandidatesBanner';
import { SellSheetActions } from '../SellSheetView';

export interface InventoryHeaderProps {
  isMobile: boolean;
  items: AgingItem[];
  filteredCount: number;
  totalCost: number;
  totalMarket: number;
  totalPL: number;
  statsExpanded: boolean;
  setStatsExpanded: React.Dispatch<React.SetStateAction<boolean>>;
  showEV: boolean;
  evPortfolio: EVPortfolio | null | undefined;
  reviewStats: ReviewStats;
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
  selected: Set<string>;
  campaignId?: string;
  isPrinting: boolean;
  onStatClick: (target: StatClickTarget) => void;
  onAddToSellSheet: (ids: string[]) => void;
  onRemoveFromSellSheet: (ids: string[]) => void;
  onRecordSale: (items: AgingItem[]) => void;
  onBulkListOnDH: (ids: string[]) => void;
  onClearSelected: () => void;
  onPrint: () => void;
}

export default function InventoryHeader({
  isMobile, items, filteredCount,
  totalCost, totalMarket, totalPL,
  statsExpanded, setStatsExpanded,
  showEV, evPortfolio,
  reviewStats, searchQuery, setSearchQuery,
  showAll, setShowAll, filterTab, setFilterTab,
  tabCounts, pageSellSheetCount, debouncedSearch,
  sellSheetActive, selected,
  campaignId, isPrinting,
  onStatClick, onAddToSellSheet, onRemoveFromSellSheet,
  onRecordSale, onBulkListOnDH, onClearSelected, onPrint,
}: InventoryHeaderProps) {
  const primary = useMemo(() => [
    { key: 'needs_attention' as const, label: 'Needs Attention', count: tabCounts.needs_attention, alwaysShow: true },
    { key: 'ready_to_list' as const, label: 'Pending DH Listing', count: tabCounts.ready_to_list, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts]);
  const secondary = useMemo(() => [
    { key: 'all' as const, label: 'All', count: tabCounts.all, alwaysShow: true },
    { key: 'in_hand' as const, label: 'In Hand', count: tabCounts.in_hand, alwaysShow: false },
    { key: 'awaiting_intake' as const, label: 'Awaiting Intake', count: tabCounts.awaiting_intake, alwaysShow: false },
    { key: 'sell_sheet' as const, label: 'Sell Sheet', count: pageSellSheetCount, alwaysShow: false },
  ].filter(t => t.alwaysShow || t.count > 0), [tabCounts, pageSellSheetCount]);

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

  return (
    <>
      {/* Summary stat cards — collapsible on mobile */}
      {isMobile ? (
        <div className="mb-4 sell-sheet-no-print">
          <button
            type="button"
            onClick={() => setStatsExpanded(prev => !prev)}
            aria-expanded={statsExpanded}
            aria-controls="inventory-stats-panel"
            className="flex items-center justify-between w-full bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] px-3 py-2.5 text-left"
          >
            <div className="flex items-center gap-3 min-w-0">
              <span className="text-xs text-[var(--text-muted)]">{items.length} cards</span>
              <span className="text-xs font-semibold text-[var(--text)]">{formatCents(totalCost)}</span>
              {totalMarket > 0 && (
                <span className={`text-xs font-semibold ${totalPL > 0 ? 'text-[var(--success)]' : totalPL < 0 ? 'text-[var(--danger)]' : 'text-[var(--text)]'}`}>
                  {formatPL(totalPL)}
                </span>
              )}
            </div>
            <svg className={`w-4 h-4 text-[var(--text-muted)] transition-transform ${statsExpanded ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </button>
          {statsExpanded && (
            <div id="inventory-stats-panel" className="mt-3 pb-4 border-b border-[rgba(255,255,255,0.05)]">
              <div className="mb-2">
                <div className="text-[11px] font-semibold text-[var(--brand-400)] uppercase tracking-wider mb-0.5">Unrealized P/L</div>
                <div className={`text-2xl font-extrabold tracking-tight ${totalPL >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                  {totalMarket > 0 ? formatPL(totalPL) : '-'}
                </div>
                {totalMarket > 0 && totalCost > 0 && (
                  <div className={`text-xs mt-0.5 ${totalPL >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
                    {totalPL > 0 ? '+' : ''}{formatPct(totalPL / totalCost)} return
                  </div>
                )}
              </div>
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Cards</div>
                  <div className="text-sm font-semibold text-[var(--text-secondary,#cbd5e1)]">{items.length}</div>
                </div>
                <div>
                  <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Cost Basis</div>
                  <div className="text-sm font-semibold text-[var(--text-secondary,#cbd5e1)]">{formatCents(totalCost)}</div>
                </div>
                <div>
                  <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Market</div>
                  <div className="text-sm font-semibold text-[var(--text-secondary,#cbd5e1)]">{totalMarket > 0 ? formatCents(totalMarket) : '-'}</div>
                </div>
              </div>
            </div>
          )}
        </div>
      ) : (
        <div className="mb-7 pb-6 border-b border-[rgba(255,255,255,0.05)] sell-sheet-no-print">
          <div className="flex items-end gap-7">
            <div>
              <div className="text-[11px] font-semibold text-[var(--brand-400)] uppercase tracking-wider mb-0.5">
                Unrealized P/L
              </div>
              <div className={`text-[32px] font-extrabold tracking-tight leading-none ${totalPL >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                {totalMarket > 0 ? formatPL(totalPL) : '-'}
              </div>
              {totalMarket > 0 && totalCost > 0 && (
                <div className={`text-xs mt-1 ${totalPL >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
                  {totalPL > 0 ? '+' : ''}{formatPct(totalPL / totalCost)} return
                </div>
              )}
            </div>
            <div className="flex gap-6 pb-1">
              <div>
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Cards</div>
                <div className="text-base font-semibold text-[var(--text-secondary,#cbd5e1)]">{items.length}</div>
              </div>
              <div>
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Cost Basis</div>
                <div className="text-base font-semibold text-[var(--text-secondary,#cbd5e1)]">{formatCents(totalCost)}</div>
              </div>
              <div>
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Market Value</div>
                <div className="text-base font-semibold text-[var(--text-secondary,#cbd5e1)]">{totalMarket > 0 ? formatCents(totalMarket) : '-'}</div>
              </div>
              {showEV && evPortfolio && (
                <div>
                  <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">Portfolio EV</div>
                  <div className={`text-base font-semibold ${evPortfolio.totalEvCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                    {formatPL(evPortfolio.totalEvCents)}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

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

      {/* Review Summary Bar */}
      <div className="mb-4 sell-sheet-no-print">
        <ReviewSummaryBar
          stats={reviewStats}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          showAll={showAll}
          onToggleShowAll={() => setShowAll(prev => !prev)}
          onStatClick={onStatClick}
        />
      </div>

      {/* Filter tabs */}
      {!showAll && (
        <div className="flex flex-col gap-2 mb-3 sell-sheet-no-print">
          <div className="flex items-center gap-2 overflow-x-auto scrollbar-none">
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
            <div className="flex items-center gap-1.5 overflow-x-auto scrollbar-none">
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

      {sellSheetActive && filteredCount === 0 && (
        <div className="text-center py-12">
          <div className="text-[var(--text-muted)] text-sm">No items on your sell sheet.</div>
          <div className="text-[var(--text-muted)] text-xs mt-1">Select items from any tab and click &ldquo;Add to Sell Sheet&rdquo;.</div>
        </div>
      )}
    </>
  );
}
