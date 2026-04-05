import type { AgingItem } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';
import { costBasis, bestPrice } from './utils';

interface MobileSellSheetRowProps {
  item: AgingItem;
  onTap: () => void;
}

export default function MobileSellSheetRow({ item, onTap }: MobileSellSheetRowProps) {
  const cb = costBasis(item.purchase);
  const snap = item.currentMarket;
  const market = snap ? bestPrice(snap) : 0;
  const clValue = item.purchase.clValueCents ?? 0;
  const recPrice = item.recommendedPriceCents ?? item.purchase.reviewedPriceCents ?? 0;
  const recProfitable = recPrice > 0 && recPrice > cb;

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onTap}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onTap(); } }}
      className="grid items-center border-b border-[rgba(255,255,255,0.04)] active:bg-[rgba(255,255,255,0.04)] cursor-pointer transition-colors"
      style={{
        gridTemplateColumns: '1fr 24px 52px 52px 52px 56px',
        padding: '7px 10px',
        fontSize: '10px',
      }}
    >
      <div className="min-w-0">
        <div className="font-medium text-[var(--text)] truncate" style={{ fontSize: '10px' }}>
          {item.purchase.cardName}
        </div>
        <div className="text-[var(--text-muted)] truncate" style={{ fontSize: '8px' }}>
          {item.purchase.setName && <>{item.purchase.setName}</>}
          {item.purchase.cardNumber && <> &middot; #{item.purchase.cardNumber}</>}
          {item.purchase.certNumber && (
            <> &middot; <span className="font-mono text-[var(--text-secondary,#94a3b8)]">{item.purchase.certNumber}</span></>
          )}
        </div>
      </div>
      <span className="text-center text-[var(--text)]">
        {item.purchase.gradeValue % 1 === 0 ? item.purchase.gradeValue.toFixed(0) : item.purchase.gradeValue}
      </span>
      <span className="text-right tabular-nums text-[var(--text)]">{formatCents(cb)}</span>
      <span className="text-right tabular-nums text-[var(--text)]">
        {market > 0 ? formatCents(market) : <span className="text-[var(--text-muted)]">-</span>}
      </span>
      <span className="text-right tabular-nums text-[var(--text)]">
        {clValue > 0 ? formatCents(clValue) : <span className="text-[var(--text-muted)]">&mdash;</span>}
      </span>
      <span className={`text-right tabular-nums font-semibold ${
        recPrice > 0
          ? recProfitable ? 'text-[var(--success)]' : 'text-[var(--danger)]'
          : 'text-[var(--text-muted)]'
      }`}>
        {recPrice > 0 ? formatCents(recPrice) : <span className="italic">&mdash;</span>}
      </span>
    </div>
  );
}
