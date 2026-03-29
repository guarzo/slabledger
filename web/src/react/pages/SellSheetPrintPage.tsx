import { useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { SellSheet } from '../../types/campaigns';
import { formatCardName, gradeDisplay, marginCode, isHotSellerFromSellSheet } from '../utils/sellSheetHelpers';
import { formatCents } from '../utils/formatters';
import { api } from '../../js/api';
import PokeballLoader from '../PokeballLoader';

export default function SellSheetPrintPage() {
  const [searchParams] = useSearchParams();
  const [sheet, setSheet] = useState<SellSheet | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const consumed = useRef(false);

  useEffect(() => {
    // Guard against React StrictMode double-invocation consuming sessionStorage twice
    if (consumed.current) return;
    consumed.current = true;

    // Read IDs from sessionStorage (preferred) or fall back to URL params
    let ids: string[] = [];
    let campaignId: string | null = null;

    const storedIds = sessionStorage.getItem('sellSheetIds');
    if (storedIds) {
      try { ids = JSON.parse(storedIds); } catch { /* fall through to URL params */ }
      campaignId = sessionStorage.getItem('sellSheetCampaignId') || null;
      sessionStorage.removeItem('sellSheetIds');
      sessionStorage.removeItem('sellSheetCampaignId');
    }

    // Fall back to URL params for backwards compatibility
    if (ids.length === 0) {
      const idsParam = searchParams.get('ids');
      campaignId = searchParams.get('campaignId') || null;
      if (idsParam) {
        ids = idsParam.split(',').filter(Boolean);
      }
    }

    if (ids.length === 0) {
      setError('No purchase IDs provided');
      setLoading(false);
      return;
    }

    let cancelled = false;
    const fetchSheet = async () => {
      try {
        let result: SellSheet;
        if (campaignId) {
          result = await api.generateSellSheet(campaignId, ids);
        } else {
          result = await api.generateSelectedSellSheet(ids);
        }
        if (!cancelled) setSheet(result);
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to generate sell sheet');
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    fetchSheet();
    return () => { cancelled = true; };
  }, [searchParams]);

  if (loading) return <div className="flex items-center justify-center min-h-screen"><PokeballLoader /></div>;
  if (error) return <div className="p-8 text-center text-[var(--danger)]">{error}</div>;
  if (!sheet || !sheet.items?.length) return <div className="p-8 text-center text-[var(--text-muted)]">No items for sell sheet.</div>;

  const hotItems: typeof sheet.items = [];
  const regularItems: typeof sheet.items = [];
  for (const item of sheet.items) {
    (isHotSellerFromSellSheet(item) ? hotItems : regularItems).push(item);
  }
  const today = new Date().toLocaleDateString();

  return (
    <div className="sell-sheet-print max-w-4xl mx-auto px-4 py-6 print:px-0 print:py-0 print:max-w-full">
      {/* Print button — hidden when printing */}
      <div className="mb-4 print:hidden flex justify-end">
        <button
          type="button"
          onClick={() => window.print()}
          className="px-4 py-2 bg-[var(--brand-500)] text-white rounded-lg text-sm font-medium hover:bg-[var(--brand-600)] transition-colors"
        >
          Print
        </button>
      </div>

      {/* Header */}
      <div className="text-center mb-4 print:mb-2">
        <div className="text-lg font-bold tracking-wide print:text-sm">CARD SHOW SELL SHEET</div>
        <div className="text-xs text-[var(--text-muted)] print:text-gray-500">{today} · {sheet.items.length} cards</div>
      </div>

      {/* Hot sellers section */}
      {hotItems.map(item => (
        <SellSheetRow key={item.certNumber} item={item} hot />
      ))}

      {/* Separator between hot and regular */}
      {hotItems.length > 0 && regularItems.length > 0 && (
        <div className="sell-sheet-separator" />
      )}

      {/* Regular cards */}
      {regularItems.map(item => (
        <SellSheetRow key={item.certNumber} item={item} />
      ))}

      {/* Footer */}
      <div className="sell-sheet-footer">
        <span>{sheet.items.length} cards · {today}</span>
      </div>
    </div>
  );
}

function SellSheetRow({ item, hot }: { item: SellSheet['items'][number]; hot?: boolean }) {
  return (
    <div className={`sell-sheet-row${hot ? ' sell-sheet-row--hot' : ''}`}>
      <span className={`sell-sheet-title${hot ? ' font-bold' : ''}`}>
        {hot && '\u2605 '}{formatCardName(item.cardName)} {marginCode(item.targetSellPrice, item.costBasisCents)}
      </span>
      <span className="sell-sheet-meta">
        {item.setName}
        {item.cardNumber && ` #${item.cardNumber}`}
        {' · '}{item.certNumber} · {gradeDisplay(item)}
      </span>
      <span className="sell-sheet-price">{formatCents(item.targetSellPrice)}</span>
      <span className="sell-sheet-notes">_______________________</span>
    </div>
  );
}
