import type { AgingItem, ExpectedValue } from '../../../../types/campaigns';
import { formatCents, daysHeldColor, signalLabel, signalBgColor } from '../../../utils/formatters';
import { TrendArrow, ConfidenceIndicator } from '../../../ui';
import MarketplaceLinks from './MarketplaceLinks';
import {
  costBasis, bestPrice, unrealizedPL, marketTrend, velocityLabel,
  getSourceByType, fmtDateShort, plColor, formatPL,
  deriveSignalDirection, deriveSignalDelta, isHotSeller,
} from './utils';

interface MobileCardProps {
  item: AgingItem;
  selected: boolean;
  onToggle: () => void;
  onRecordSale: () => void;
  onFixPricing?: () => void;
  onSetPrice?: () => void;
  ev?: ExpectedValue;
  showCampaignColumn?: boolean;
  isOnSellSheet?: boolean;
}

export default function MobileCard({ item, selected, onToggle, onRecordSale, onFixPricing, onSetPrice, ev, showCampaignColumn, isOnSellSheet }: MobileCardProps) {
  const cb = costBasis(item.purchase);
  const snap = item.currentMarket;
  const daysColor = daysHeldColor(item.daysHeld);
  const price = snap ? bestPrice(snap) : 0;
  const pl = unrealizedPL(cb, snap);
  const trend = snap ? marketTrend(snap) : null;
  const velocity = snap ? velocityLabel(snap) : null;
  const direction = deriveSignalDirection(item);
  const deltaPct = deriveSignalDelta(item);
  const hotSeller = isHotSeller(item);

  return (
    <div className={`p-3 bg-[var(--surface-1)] rounded-xl border ${selected ? 'border-[var(--brand-500)]' : 'border-[var(--surface-2)]'}`}>
      <div className="flex items-start justify-between mb-2">
        <div className="flex items-start gap-2">
          <input type="checkbox" checked={selected} onChange={onToggle} className="rounded mt-0.5" />
          <div>
            <div className="text-sm font-medium text-[var(--text)]">
              {hotSeller && <span className="text-amber-400 mr-1" title="High demand">★</span>}
              {isOnSellSheet && <span className="text-gray-400 mr-1 text-xs" title="On sell sheet">&#9864;</span>}
              {item.purchase.cardName}
              {item.purchase.cardName && item.purchase.setName && (
                <MarketplaceLinks
                  cardName={item.purchase.cardName}
                  setName={item.purchase.setName}
                  cardNumber={item.purchase.cardNumber ?? ''}
                  gradeValue={item.purchase.gradeValue}
                  variant="inline"
                />
              )}
            </div>
            <div className="text-xs text-[var(--text-muted)]">
              {showCampaignColumn && item.campaignName && <>{item.campaignName} &middot; </>}
              {item.purchase.setName && <>{item.purchase.setName} &middot; </>}
              Cert #{item.purchase.certNumber} &middot; {item.purchase.grader && item.purchase.grader !== 'PSA' ? `${item.purchase.grader} ${item.purchase.gradeValue}` : `PSA ${item.purchase.gradeValue}`}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {direction && (
            <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${signalBgColor(direction)}`}>
              {signalLabel(direction)}
              {deltaPct != null && ` ${deltaPct > 0 ? '+' : ''}${deltaPct.toFixed(0)}%`}
            </span>
          )}
          <span className={`text-xs font-medium ${daysColor}`}>{item.daysHeld}d</span>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs ml-6">
        <div><span className="text-[var(--text-muted)]">Cost:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(cb)}</span></div>
        {item.purchase.clValueCents > 0 && (
          <div><span className="text-[var(--text-muted)]">CL:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(item.purchase.clValueCents)}</span></div>
        )}
        {snap && price > 0 && (() => {
          const ebaySource = getSourceByType(snap.sourcePrices, 'ebay');
          const estSource = getSourceByType(snap.sourcePrices, 'estimate');
          if (ebaySource || estSource) {
            return (
              <>
                {ebaySource && (
                  <div>
                    <span className="text-[var(--text-muted)]">eBay:</span>{' '}
                    <span className="text-[var(--text)] tabular-nums inline-flex items-center gap-1">
                      {formatCents(ebaySource.priceCents)}
                      <TrendArrow trend={trend} size="sm" />
                      {ebaySource.confidence && <ConfidenceIndicator confidence={ebaySource.confidence as 'high' | 'medium' | 'low'} size="sm" />}
                    </span>
                  </div>
                )}
                {estSource && (
                  <div>
                    <span className="text-[var(--text-muted)]">Est:</span>{' '}
                    <span className="text-[var(--text)] tabular-nums inline-flex items-center gap-1">
                      {formatCents(estSource.priceCents)}
                      {estSource.confidence && <ConfidenceIndicator confidence={estSource.confidence as 'high' | 'medium' | 'low'} size="sm" />}
                    </span>
                  </div>
                )}
              </>
            );
          }
          return (
            <div>
              <span className="text-[var(--text-muted)]">Market:</span>{' '}
              <span className="text-[var(--text)] tabular-nums inline-flex items-center gap-1">
                {formatCents(price)}
                <TrendArrow trend={trend} size="sm" />
                <ConfidenceIndicator confidence={snap.confidence ?? null} size="sm" />
              </span>
            </div>
          );
        })()}
        {snap && snap.conservativeCents && snap.optimisticCents ? (
          <div>
            <span className="text-[var(--text-muted)]">Range:</span>{' '}
            <span className="text-[var(--text)] tabular-nums">{formatCents(snap.conservativeCents)} - {formatCents(snap.optimisticCents)}</span>
          </div>
        ) : null}
        {snap && snap.lastSoldCents > 0 && (
          <div>
            <span className="text-[var(--text-muted)]">Last sold:</span>{' '}
            <span className="text-[var(--text)] tabular-nums">
              {formatCents(snap.lastSoldCents)}
              {snap.lastSoldDate && <span className="text-[var(--text-muted)]"> ({fmtDateShort(snap.lastSoldDate)})</span>}
            </span>
          </div>
        )}
        {snap?.lowestListCents ? (
          <div>
            <span className="text-[var(--text-muted)]">Low list:</span>{' '}
            <span className="text-[var(--text)] tabular-nums">{formatCents(snap.lowestListCents)}</span>
            {snap.activeListings ? <span className="text-[var(--text-muted)]"> ({snap.activeListings})</span> : null}
          </div>
        ) : null}
        {velocity && (
          <div>
            <span className="text-[var(--text-muted)]">Sales:</span>{' '}
            <span className="text-[var(--text)] tabular-nums">{velocity}</span>
          </div>
        )}
        {pl != null && (
          <div>
            <span className="text-[var(--text-muted)]">P/L:</span>{' '}
            <span className={`tabular-nums ${plColor(pl)}`}>{formatPL(pl)}</span>
          </div>
        )}
        {ev && (
          <div>
            <span className="text-[var(--text-muted)]">EV:</span>{' '}
            <span className={`tabular-nums ${ev.evCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>{formatPL(ev.evCents)}</span>
          </div>
        )}
      </div>
      <div className="mt-2 ml-6 flex justify-end gap-2">
        {onSetPrice && (
          <button
            type="button"
            onClick={onSetPrice}
            className={`text-xs font-medium px-2 py-1 rounded transition-colors ${
              item.purchase.overridePriceCents ? 'bg-[var(--brand-500)]/20 text-[var(--brand-400)]' : 'bg-[var(--surface-2)] hover:bg-[var(--surface-3)]'
            }`}
            title={item.purchase.overridePriceCents ? `Override set` : 'Set price'}
            aria-label={item.purchase.overridePriceCents ? 'Edit override price' : 'Set price'}
          >
            {item.purchase.overridePriceCents ? '$OVR' : '$'}
          </button>
        )}
        {onFixPricing && (
          <button
            type="button"
            onClick={onFixPricing}
            className="text-xs text-[var(--info)] underline"
            title="Override price lookup"
          >
            Fix
          </button>
        )}
        <button
          type="button"
          onClick={onRecordSale}
          className="text-xs font-medium px-2 py-1 rounded bg-[var(--brand-500)]/20 text-[var(--brand-400)] hover:bg-[var(--brand-500)]/40 transition-colors"
        >
          Sell
        </button>
      </div>
    </div>
  );
}
