import type { SellSheetItem } from '../../types/campaigns';
import { formatCents } from '../utils/formatters';
import { formatCardName, gradeDisplay, cardSubtitle } from '../utils/sellSheetHelpers';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { useGlobalSellSheet, useGlobalInventory } from '../queries/useCampaignQueries';
import InventoryTab from './campaign-detail/InventoryTab';
import AIAnalysisWidget from '../components/advisor/AIAnalysisWidget';
import logoSrc from '../../assets/card-yeti-business-logo.png';

function SellSheetMobileCard({ item, showCampaignColumn }: { item: SellSheetItem; showCampaignColumn?: boolean }) {
  const sub = cardSubtitle(item);

  return (
    <div className="p-3 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] print:hidden">
      <div className="flex items-start justify-between mb-1">
        <div className="flex-1 min-w-0">
          <div className="text-lg font-medium text-[var(--text)] break-words">{formatCardName(item.cardName)}</div>
          {sub && (
            <div className="text-[11px] text-[var(--text-muted)]">{sub}</div>
          )}
          <div className="text-xs text-[var(--text-muted)]">Cert: {item.certNumber} | {gradeDisplay(item)}</div>
          {showCampaignColumn && item.campaignName && (
            <div className="text-[11px] text-[var(--text-muted)]">{item.campaignName}</div>
          )}
        </div>
        <div className="text-right ml-3">
          <div className="text-xl font-bold text-[var(--success)]">
            {formatCents(item.targetSellPrice ?? item.clValueCents)}
            {item.isOverridden && <span className="text-xs text-[var(--text-muted)]" title="Price override" aria-label="Price override">*</span>}
          </div>
        </div>
      </div>
    </div>
  );
}

function SellSheetTable({ items, showCampaignColumn }: { items: SellSheetItem[]; showCampaignColumn?: boolean }) {
  return (
    <table className="w-full text-sm print:text-[8pt]">
      <thead>
        <tr className="border-b-2 border-[var(--surface-2)] print:border-gray-400">
          <th className="text-left py-1.5 px-1.5 text-[var(--text-muted)] font-medium print:text-gray-600 print:py-1 print:px-1">Card</th>
          {showCampaignColumn && <th className="text-left py-1.5 px-1.5 text-[var(--text-muted)] font-medium print:text-gray-600 print:py-1 print:px-1">Campaign</th>}
          <th className="text-left py-1.5 px-1.5 text-[var(--text-muted)] font-medium print:text-gray-600 print:py-1 print:px-1">Cert</th>
          <th className="text-center py-1.5 px-1.5 text-[var(--text-muted)] font-medium print:text-gray-600 print:py-1 print:px-1">Grade</th>
          <th className="text-right py-1.5 px-1.5 text-[var(--text-muted)] font-medium print:text-gray-600 print:py-1 print:px-1">Price</th>
        </tr>
      </thead>
      <tbody>
        {items.map(item => {
          const sub = cardSubtitle(item);
          return (
            <tr key={item.certNumber} className="border-b border-[var(--surface-2)]/50 print:border-gray-200 print:break-inside-avoid">
              <td className="py-1.5 px-1.5 print:py-1 print:px-1">
                <div className="text-[var(--text)] print:text-black font-medium leading-tight">{formatCardName(item.cardName)}</div>
                {sub && <div className="text-[10px] text-[var(--text-muted)] print:text-gray-500 print:text-[7pt] leading-tight">{sub}</div>}
              </td>
              {showCampaignColumn && <td className="py-1.5 px-1.5 text-xs text-[var(--text-muted)] print:text-gray-500 print:py-1 print:px-1">{item.campaignName || ''}</td>}
              <td className="py-1.5 px-1.5 text-[var(--text-muted)] print:text-gray-500 text-xs print:py-1 print:px-1">{item.certNumber}</td>
              <td className="py-1.5 px-1.5 text-center text-[var(--text)] print:text-black print:py-1 print:px-1">{gradeDisplay(item)}</td>
              <td className="py-1.5 px-1.5 text-right font-medium text-[var(--success)] print:text-green-800 print:py-1 print:px-1">
                {formatCents(item.targetSellPrice ?? item.clValueCents)}
                {item.isOverridden && <span className="text-[var(--text-muted)] print:text-gray-400" title="Price override" aria-label="Price override">*</span>}
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

function PrintableSellSheet({ showCampaignColumn }: { showCampaignColumn?: boolean }) {
  const { data: sheet, isLoading, error } = useGlobalSellSheet();
  const isMobile = useMediaQuery('(max-width: 768px)');

  if (isLoading || error || !sheet) return null;

  const items = sheet.items ?? [];
  const totalValue = sheet.totals.totalExpectedRevenue;

  return (
    <div className="sell-sheet max-w-6xl mx-auto px-4">
      {/* Compact header: logo left, title center, stats right */}
      <div className="flex items-center justify-between mb-3 print:mb-2">
        <img src={logoSrc} alt="Card Yeti" className="print-color-adjust w-24 print:w-20" />
        <div className="text-center flex-1">
          <h1 className="text-lg font-bold text-[var(--text)] print:text-black print:text-sm">Available Inventory</h1>
          <p className="text-xs text-[var(--text-muted)] print:text-gray-500 print:text-[7pt]">
            {new Date(sheet.generatedAt).toLocaleDateString()} · card-yeti.com
          </p>
        </div>
        <div className="text-right text-xs print:text-[7pt]">
          <div className="text-[var(--text-muted)] print:text-gray-500">{sheet.totals.itemCount} items</div>
          <div className="font-bold text-[var(--text)] print:text-black">{formatCents(totalValue)}</div>
        </div>
      </div>

      {/* Mobile card layout */}
      {isMobile && (
        <div className="space-y-3 print:hidden">
          {items.length === 0 ? (
            <p className="text-center text-[var(--text-muted)] py-8">No unsold inventory to display.</p>
          ) : items.map(item => (
            <SellSheetMobileCard key={item.certNumber} item={item} showCampaignColumn={showCampaignColumn} />
          ))}
        </div>
      )}

      {/* Desktop table */}
      <div className={`overflow-x-auto ${isMobile ? 'hidden print:block' : ''}`}>
        {items.length === 0 ? (
          <p className="text-center text-[var(--text-muted)] py-8">No unsold inventory to display.</p>
        ) : (
          <SellSheetTable items={items} showCampaignColumn={showCampaignColumn} />
        )}
      </div>

      {/* Footer */}
      <div className="mt-3 pt-2 border-t border-[var(--surface-2)] print:border-gray-300 print:mt-2 print:pt-1">
        <div className="flex justify-between items-center text-[10px] text-[var(--text-muted)] print:text-gray-500 print:text-[7pt]">
          <span>{sheet.totals.itemCount} items · {formatCents(totalValue)} total value</span>
          <span>card-yeti.com</span>
        </div>
        <p className="text-[10px] text-[var(--text-muted)] print:text-gray-400 text-center mt-0.5 print:text-[7pt]">
          Prices reflect verified recent sales data. {items.some(i => i.isOverridden) && '* = price override'}
        </p>
      </div>
    </div>
  );
}

// --- Main page ---

export default function GlobalInventoryPage() {
  const { data: items = [], isLoading, isError, error } = useGlobalInventory();

  if (isError) {
    return (
      <div className="max-w-6xl mx-auto px-4 text-center py-16">
        <p className="text-[var(--danger)] mb-4">{error instanceof Error ? error.message : 'Failed to load inventory'}</p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="px-4 py-2 bg-[var(--brand-500)] text-white rounded-lg text-sm font-medium hover:bg-[var(--brand-600)] transition-colors"
        >
          Retry
        </button>
      </div>
    );
  }

  return (
    <>
      {/* Browse view — hidden when printing */}
      <div className="max-w-6xl mx-auto px-4 print:hidden">
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Inventory</h1>
          <button
            type="button"
            onClick={() => window.print()}
            className="p-2 rounded-lg text-[var(--text-muted)] hover:text-[var(--text)] bg-[rgba(255,255,255,0.04)] border border-[rgba(255,255,255,0.08)] hover:bg-[rgba(255,255,255,0.06)] transition-colors"
            title="Print Sell Sheet"
            aria-label="Print Sell Sheet"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <polyline points="6 9 6 2 18 2 18 9" />
              <path d="M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2" />
              <rect x="6" y="14" width="12" height="8" />
            </svg>
          </button>
        </div>
        <InventoryTab items={items} isLoading={isLoading} showCampaignColumn />

        {/* AI Liquidation Analysis */}
        <div className="mt-6">
          <AIAnalysisWidget
            endpoint="liquidation-analysis"
            cacheType="liquidation"
            title="Liquidation Analysis"
            buttonLabel="Analyze Liquidation"
            description="Identify cards where selling now — even below market — frees capital more efficiently than holding. Factors in credit pressure, carrying costs, trends, and liquidity."
            collapsible
          />
        </div>
      </div>

      {/* Sell sheet — visible only when printing */}
      <div className="hidden print:block">
        <PrintableSellSheet showCampaignColumn />
      </div>
    </>
  );
}
