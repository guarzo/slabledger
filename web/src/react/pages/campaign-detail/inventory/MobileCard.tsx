import type { AgingItem, ExpectedValue } from '../../../../types/campaigns';
import { formatCents, daysHeldColor, signalLabel, signalBgColor } from '../../../utils/formatters';
import { TrendArrow, ConfidenceIndicator, GradeBadge } from '../../../ui';
import MarketplaceLinks from './MarketplaceLinks';
import TruncatedCardName from '../../../ui/TruncatedCardName';
import {
  costBasis, bestPrice, unrealizedPL, marketTrend, velocityLabel,
  getSourceByType, fmtDateShort, plColor, formatPL, mostRecentSale,
  deriveSignalDirection, deriveSignalDelta, isHotSeller, formatReceivedDate,
} from './utils';
import { wasUnlistedFromDH } from './inventoryCalcs';
import { dhBadgeFor, DH_BADGE_COLORS } from './dhBadge';
import RowActions from './RowActions';
import { resolveContextualPrimary, resolveOverflowActions, ACTION_LABELS } from './rowActions';

interface MobileCardProps {
  item: AgingItem;
  selected: boolean;
  onToggle: () => void;
  onRecordSale: () => void;
  onFixPricing?: () => void;
  onFixDHMatch?: () => void;
  onSetPrice?: () => void;
  onDelete?: () => void;
  onListOnDH?: (purchaseId: string) => void;
  onRemoveFromSellSheet?: () => void;
  onUnmatchDH?: () => void;
  onRetryDHMatch?: () => void;
  onDismiss?: () => void;
  onUndismiss?: () => void;
  dhListingLoading?: boolean;
  dhListedOverride?: boolean;
  ev?: ExpectedValue;
  showCampaignColumn?: boolean;
  isOnSellSheet?: boolean;
}

export default function MobileCard({
  item, selected, onToggle, onRecordSale,
  onFixPricing, onFixDHMatch, onSetPrice, onDelete,
  onListOnDH, onRemoveFromSellSheet, onUnmatchDH, onRetryDHMatch,
  onDismiss, onUndismiss, dhListingLoading, dhListedOverride,
  ev, showCampaignColumn, isOnSellSheet,
}: MobileCardProps) {
  const cb = costBasis(item.purchase);
  const snap = item.currentMarket;
  const daysColor = daysHeldColor(item.daysHeld);
  const price = bestPrice(item);
  const pl = unrealizedPL(cb, item);
  const trend = snap ? marketTrend(snap) : null;
  const velocity = snap ? velocityLabel(snap) : null;
  const direction = deriveSignalDirection(item);
  const deltaPct = deriveSignalDelta(item);
  const hotSeller = isHotSeller(item);

  const handlers = {
    onRecordSale,
    onSetPrice,
    onFixPricing,
    onFixDHMatch,
    onUnmatchDH,
    onRetryDHMatch,
    onListOnDH,
    onDismiss,
    onUndismiss,
    onRemoveFromSellSheet,
    onDelete,
  };
  const flags = { dhListingLoading, isOnSellSheet };
  const primary = resolveContextualPrimary(item, handlers, flags);
  const fallbackPrimary = { key: 'sell', label: ACTION_LABELS.sell, onSelect: onRecordSale };
  const overflow = resolveOverflowActions(item, handlers, flags, primary);

  const inHandLabel = item.purchase.receivedAt
    ? `In hand · ${item.daysHeld}d`
    : `${item.daysHeld}d · awaiting intake`;
  const inHandTitle = item.purchase.receivedAt
    ? `In hand since ${formatReceivedDate(item.purchase.receivedAt)}`
    : 'Awaiting intake';

  return (
    <div className={`p-3 bg-[var(--surface-1)] rounded-xl border ${selected ? 'border-[var(--brand-500)]' : 'border-[var(--surface-2)]'}`}>
      <div className="flex items-start justify-between mb-2">
        <div className="flex items-start gap-2">
          <input type="checkbox" checked={selected} onChange={onToggle} className="rounded mt-0.5" />
          {item.purchase.frontImageUrl && (
            <img
              src={item.purchase.frontImageUrl}
              alt=""
              className="w-10 h-14 object-cover rounded shrink-0 bg-[var(--surface-2)]"
              loading="lazy"
            />
          )}
          <div>
            <div className="text-sm font-medium text-[var(--text)]">
              {hotSeller && <span className="text-amber-400 mr-1" title="High demand">★</span>}
              {isOnSellSheet && <span className="text-gray-400 mr-1 text-xs" title="On sell sheet">&#9864;</span>}
              <TruncatedCardName name={item.purchase.cardName} className="text-[var(--text)] font-medium" />
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
              Cert #{item.purchase.certNumber} &middot; <GradeBadge grader={item.purchase.grader || 'PSA'} grade={item.purchase.gradeValue} size="sm" />
            </div>
            <div className={`text-[11px] mt-0.5 tabular-nums ${daysColor}`} title={inHandTitle}>
              {inHandLabel}
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
        </div>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs ml-6">
        <div><span className="text-[var(--text-muted)]">Cost:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(cb)}</span></div>
        {item.purchase.clValueCents > 0 && (
          <div><span className="text-[var(--text-muted)]">CL:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(item.purchase.clValueCents)}</span></div>
        )}
        {price > 0 && (() => {
          const ebaySource = snap ? getSourceByType(snap.sourcePrices, 'ebay') : undefined;
          const estSource = snap ? getSourceByType(snap.sourcePrices, 'estimate') : undefined;
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
                <ConfidenceIndicator confidence={snap?.confidence ?? null} size="sm" />
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
        {(() => {
          const recent = mostRecentSale(item);
          if (!recent) return null;
          return (
            <div>
              <span className="text-[var(--text-muted)]">Last sold:</span>{' '}
              <span className="text-[var(--text)] tabular-nums">
                {formatCents(recent.cents)}
                {recent.date && <span className="text-[var(--text-muted)]"> ({fmtDateShort(recent.date)})</span>}
              </span>
            </div>
          );
        })()}
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
      <div className="mt-3 ml-6 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-1.5 flex-wrap">
          {wasUnlistedFromDH(item) && (
            <span
              className="text-[10px] font-medium px-1.5 py-0.5 rounded bg-[var(--warning)]/15 text-[var(--warning)]"
              title="Item was removed from DH — will be re-pushed + listed"
            >
              Re-list
            </span>
          )}
          {dhListedOverride ? (
            <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${DH_BADGE_COLORS.listed}`} title="DH: listed">listed</span>
          ) : (() => {
            const badge = dhBadgeFor(item.purchase.dhPushStatus, item.purchase.dhStatus, item.purchase.receivedAt, item.purchase.psaShipDate);
            if (badge === 'unenrolled') return null;
            return (
              <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${DH_BADGE_COLORS[badge]}`} title={`DH: ${badge}`}>
                {badge}
              </span>
            );
          })()}
        </div>
        <RowActions primary={primary} fallbackPrimary={fallbackPrimary} overflow={overflow} variant="mobile" />
      </div>
    </div>
  );
}
