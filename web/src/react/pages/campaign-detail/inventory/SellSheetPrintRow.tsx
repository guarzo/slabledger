import { useEffect, useRef } from 'react';
import JsBarcode from 'jsbarcode';
import type { AgingItem } from '../../../../types/campaigns';
import {
  formatCardName,
  cardSubtitle,
  gradeDisplay,
  clPriceDisplayCents,
  formatLastSaleDate,
  dollars,
} from '../../../utils/sellSheetHelpers';
import { mostRecentSale } from './utils';

interface Props {
  item: AgingItem;
  rowNumber: number;
}

export default function SellSheetPrintRow({ item, rowNumber }: Props) {
  const { purchase, recommendedPriceCents } = item;
  const barcodeRef = useRef<SVGSVGElement | null>(null);

  useEffect(() => {
    if (!barcodeRef.current || !purchase.certNumber) return;
    JsBarcode(barcodeRef.current, purchase.certNumber, {
      format: 'CODE128',
      width: 1.4,
      height: 24,
      displayValue: false,
      margin: 0,
    });
  }, [purchase.certNumber]);

  const cl = clPriceDisplayCents({
    clValueCents: purchase.clValueCents,
    recommendedPriceCents,
  });
  const clText = cl
    ? (cl.estimated ? `~${dollars(cl.cents)}` : dollars(cl.cents))
    : '—';

  // Prefer the most-recent realized comp sale (compSummary), fall back to
  // the snapshot's last-sold figure so we still surface a number whenever
  // the data exists.
  const recent = mostRecentSale(item);
  const lastSoldCents = recent?.cents ?? 0;
  const lastSoldDate = formatLastSaleDate(recent?.date);

  return (
    <div className="sell-sheet-print-row">
      <div className="sell-sheet-print-cell" data-cell="num">{rowNumber}</div>
      <div className="sell-sheet-print-cell" data-cell="card">
        <div className="sell-sheet-print-name">{formatCardName(purchase.cardName)}</div>
        <div className="sell-sheet-print-sub">
          {cardSubtitle({ setName: purchase.setName, cardNumber: purchase.cardNumber })}
        </div>
      </div>
      <div className="sell-sheet-print-cell" data-cell="grade">
        {gradeDisplay({ grader: purchase.grader, grade: purchase.gradeValue })}
      </div>
      <div className="sell-sheet-print-cell" data-cell="cert">
        <div className="sell-sheet-print-cert">{purchase.certNumber}</div>
        {purchase.certNumber && (
          <svg ref={barcodeRef} className="sell-sheet-print-barcode" />
        )}
      </div>
      <div className="sell-sheet-print-cell" data-cell="cl">{clText}</div>
      <div className="sell-sheet-print-cell" data-cell="last-sale">
        {lastSoldCents > 0 && (
          <>
            <div>{dollars(lastSoldCents)}</div>
            {lastSoldDate && <div className="sell-sheet-print-date">{lastSoldDate}</div>}
          </>
        )}
      </div>
      <div className="sell-sheet-print-cell" data-cell="agreed" />
    </div>
  );
}
