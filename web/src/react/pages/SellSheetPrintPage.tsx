import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { SellSheet } from '../../types/campaigns';
import { formatCardName, gradeDisplay, marginCode, isHotSellerFromSellSheet } from '../utils/sellSheetHelpers';
import { api } from '../../js/api';
import PokeballLoader from '../PokeballLoader';

function formatDollars(cents: number): string {
  return `$${Math.round(cents / 100)}`;
}

export default function SellSheetPrintPage() {
  const [searchParams] = useSearchParams();
  const [sheet, setSheet] = useState<SellSheet | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const idsParam = searchParams.get('ids');
    const campaignId = searchParams.get('campaignId');
    if (!idsParam) {
      setError('No purchase IDs provided');
      setLoading(false);
      return;
    }

    const ids = idsParam.split(',').filter(Boolean);
    if (ids.length === 0) {
      setError('No purchase IDs provided');
      setLoading(false);
      return;
    }

    const fetchSheet = async () => {
      try {
        let result: SellSheet;
        if (campaignId) {
          result = await api.generateSellSheet(campaignId, ids);
        } else {
          result = await api.generateSelectedSellSheet(ids);
        }
        setSheet(result);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to generate sell sheet');
      } finally {
        setLoading(false);
      }
    };
    fetchSheet();
  }, [searchParams]);

  if (loading) return <div className="flex items-center justify-center min-h-screen"><PokeballLoader /></div>;
  if (error) return <div className="p-8 text-center text-[var(--danger)]">{error}</div>;
  if (!sheet || !sheet.items?.length) return <div className="p-8 text-center text-[var(--text-muted)]">No items for sell sheet.</div>;

  const hotItems = sheet.items.filter(isHotSellerFromSellSheet);
  const regularItems = sheet.items.filter(i => !isHotSellerFromSellSheet(i));
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
        <div key={item.certNumber} className="sell-sheet-row sell-sheet-row--hot">
          <span className="sell-sheet-title font-bold">
            ★ {formatCardName(item.cardName)} {marginCode(item.targetSellPrice, item.costBasisCents)}
          </span>
          <span className="sell-sheet-meta">
            {item.setName && <>{item.setName}</>}
            {item.cardNumber && <> #{item.cardNumber}</>}
            {' · '}{item.certNumber} · {gradeDisplay(item)}
          </span>
          <span className="sell-sheet-price">{formatDollars(item.targetSellPrice)}</span>
          <span className="sell-sheet-notes">_______________________</span>
        </div>
      ))}

      {/* Separator between hot and regular */}
      {hotItems.length > 0 && regularItems.length > 0 && (
        <div className="sell-sheet-separator" />
      )}

      {/* Regular cards */}
      {regularItems.map(item => (
        <div key={item.certNumber} className="sell-sheet-row">
          <span className="sell-sheet-title">
            {formatCardName(item.cardName)} {marginCode(item.targetSellPrice, item.costBasisCents)}
          </span>
          <span className="sell-sheet-meta">
            {item.setName && <>{item.setName}</>}
            {item.cardNumber && <> #{item.cardNumber}</>}
            {' · '}{item.certNumber} · {gradeDisplay(item)}
          </span>
          <span className="sell-sheet-price">{formatDollars(item.targetSellPrice)}</span>
          <span className="sell-sheet-notes">_______________________</span>
        </div>
      ))}

      {/* Footer */}
      <div className="sell-sheet-footer">
        <span>{sheet.items.length} cards · {today}</span>
      </div>
    </div>
  );
}
