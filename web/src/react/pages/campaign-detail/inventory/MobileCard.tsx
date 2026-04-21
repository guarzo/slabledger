import type { AgingItem, ExpectedValue } from '../../../../types/campaigns';
import { formatCents, daysHeldColor, signalLabel, signalBgColor } from '../../../utils/formatters';
import { TrendArrow, ConfidenceIndicator, GradeBadge } from '../../../ui';
import MarketplaceLinks from './MarketplaceLinks';
import {
  costBasis, bestPrice, unrealizedPL, marketTrend, velocityLabel,
  getSourceByType, fmtDateShort, plColor, formatPL, mostRecentSale,
  deriveSignalDirection, deriveSignalDelta, isHotSeller, formatReceivedDate,
} from './utils';
import { wasUnlistedFromDH, deriveActionIntent, canDismiss } from './inventoryCalcs';
import { dhBadgeFor, DH_BADGE_COLORS } from './dhBadge';

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
  onDismiss?: () => void;
  onUndismiss?: () => void;
  dhListingLoading?: boolean;
  dhListedOverride?: boolean;
  ev?: ExpectedValue;
  showCampaignColumn?: boolean;
  isOnSellSheet?: boolean;
}

export default function MobileCard({ item, selected, onToggle, onRecordSale, onFixPricing, onFixDHMatch, onSetPrice, onDelete, onListOnDH, onRemoveFromSellSheet, onUnmatchDH, onDismiss, onUndismiss, dhListingLoading, dhListedOverride, ev, showCampaignColumn, isOnSellSheet }: MobileCardProps) {
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

  const actionIntent = deriveActionIntent(item);
  const showDismiss = canDismiss(actionIntent);

  return (
    <div className={`p-3 bg-[var(--surface-1)] rounded-xl border ${selected ? 'border-[var(--brand-500)]' : 'border-[var(--surface-2)]'}`}>
      <div className="flex items-start justify-between mb-2">
        <div className="flex items-start gap-2">
          <input type="checkbox" checked={selected} onChange={onToggle} className="rounded mt-0.5" />
          <div style={{ position: 'relative', width: 40, height: 56, flexShrink: 0 }}>
            {item.purchase.frontImageUrl && (
              <img
                src={item.purchase.frontImageUrl}
                alt=""
                className="w-10 h-14 object-cover rounded shrink-0 bg-[var(--surface-2)]"
                loading="lazy"
              />
            )}
            {item.purchase.receivedAt && (
              <div
                data-testid="in-hand-indicator"
                role="img"
                aria-label={`In hand since ${formatReceivedDate(item.purchase.receivedAt)}`}
                title={`In hand since ${formatReceivedDate(item.purchase.receivedAt)}`}
                style={{
                  position: 'absolute',
                  top: -3,
                  right: -3,
                  width: 10,
                  height: 10,
                  borderRadius: '50%',
                  background: '#34d399',
                  border: '2px solid var(--surface-1)',
                  flexShrink: 0,
                }}
              />
            )}
          </div>
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
              Cert #{item.purchase.certNumber} &middot; <GradeBadge grader={item.purchase.grader || 'PSA'} grade={item.purchase.gradeValue} size="sm" />
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
      <div className="mt-2 ml-6 flex flex-wrap justify-end gap-2">
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
        {onFixDHMatch && (
          <button
            type="button"
            onClick={onFixDHMatch}
            className="text-xs text-[var(--info)] underline"
            title="Re-map to correct DH card"
            aria-label="Re-map to correct DH card"
          >
            Fix DH
          </button>
        )}
        {onUnmatchDH && (
          <button
            type="button"
            onClick={onUnmatchDH}
            className="text-xs text-[var(--info)] underline"
            title="Remove DH match"
            aria-label="Remove DH match"
          >
            Remove DH
          </button>
        )}
        {isOnSellSheet && onRemoveFromSellSheet && (
          <button
            type="button"
            onClick={onRemoveFromSellSheet}
            className="text-xs text-[var(--text-muted)] underline"
            title="Remove from sell sheet"
            aria-label="Remove from sell sheet"
          >
            Remove
          </button>
        )}
        {wasUnlistedFromDH(item) && (
          <span
            className="text-[10px] font-medium px-1.5 py-0.5 rounded bg-[var(--warning)]/15 text-[var(--warning)]"
            title="Item was removed from DH — will be re-pushed + listed"
          >
            Re-list (removed from DH)
          </span>
        )}
        {dhListedOverride ? (
          <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${DH_BADGE_COLORS.listed}`} title="DH: listed">listed</span>
        ) : actionIntent === 'fix_match' && onFixDHMatch ? (
          <button
            type="button"
            onClick={onFixDHMatch}
            className="text-xs font-medium px-2 py-1 rounded bg-[var(--warning)]/15 text-[var(--warning)] hover:bg-[var(--warning)]/30 transition-colors"
            title="Paste the correct DoubleHolo URL to fix the match"
            aria-label="Fix DH Match"
          >
            Fix Match
          </button>
        ) : actionIntent === 'list' && onListOnDH ? (
          <button
            type="button"
            onClick={() => onListOnDH(item.purchase.id)}
            disabled={dhListingLoading}
            className={`text-xs font-medium px-2 py-1 rounded transition-colors ${
              dhListingLoading
                ? 'bg-[var(--surface-2)] text-[var(--text-muted)] cursor-wait'
                : 'bg-[var(--success)]/15 text-[var(--success)] hover:bg-[var(--success)]/25'
            }`}
            title="Publish this item on DH"
          >
            {dhListingLoading ? 'Listing…' : 'List'}
          </button>
        ) : actionIntent === 'set_and_list' && onSetPrice ? (
          <button
            type="button"
            onClick={onSetPrice}
            className="text-xs font-medium px-2 py-1 rounded bg-[var(--warning)]/15 text-[var(--warning)] hover:bg-[var(--warning)]/30 transition-colors"
            title="Set a reviewed price before listing on DH"
            aria-label="Set reviewed price before listing on DH"
          >
            Set Price
          </button>
        ) : actionIntent === 'restore' && onUndismiss ? (
          <button
            type="button"
            onClick={onUndismiss}
            className="text-xs font-medium px-2 py-1 rounded bg-[var(--brand-500)]/15 text-[var(--brand-400)] hover:bg-[var(--brand-500)]/30 transition-colors"
            title="Restore to DH pipeline"
            aria-label="Restore to DH pipeline"
          >
            Restore
          </button>
        ) : (() => {
          const badge = dhBadgeFor(item.purchase.dhPushStatus, item.purchase.dhStatus, item.purchase.receivedAt);
          if (badge === 'unenrolled') return null;
          return (
            <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${DH_BADGE_COLORS[badge]}`} title={`DH: ${badge}`}>
              {badge}
            </span>
          );
        })()}
        {showDismiss && onDismiss && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              if (window.confirm('Dismiss this item from DH listing?')) onDismiss();
            }}
            className="text-[10px] text-[var(--text-muted)] hover:text-[var(--danger)] underline underline-offset-2"
            title="Skip DH for this item"
            aria-label="Dismiss this item from DH listing"
          >
            Dismiss
          </button>
        )}
        <button
          type="button"
          onClick={onRecordSale}
          className="text-xs font-medium px-2 py-1 rounded bg-[var(--brand-500)]/20 text-[var(--brand-400)] hover:bg-[var(--brand-500)]/40 transition-colors"
        >
          Sell
        </button>
        {onDelete && (
          <button
            type="button"
            onClick={onDelete}
            className="ml-2 text-[var(--text-muted)] hover:text-[var(--danger)] hover:bg-[var(--danger)]/10 transition-colors rounded p-2"
            aria-label="Delete"
            title="Delete"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
              <path fillRule="evenodd" d="M8.75 1A2.75 2.75 0 0 0 6 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 1 0 .23 1.482l.149-.022.841 10.518A2.75 2.75 0 0 0 7.596 19h4.807a2.75 2.75 0 0 0 2.742-2.53l.841-10.52.149.023a.75.75 0 0 0 .23-1.482A41.03 41.03 0 0 0 14 4.193V3.75A2.75 2.75 0 0 0 11.25 1h-2.5ZM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4ZM8.58 7.72a.75.75 0 0 0-1.5.06l.3 7.5a.75.75 0 1 0 1.5-.06l-.3-7.5Zm4.34.06a.75.75 0 1 0-1.5-.06l-.3 7.5a.75.75 0 1 0 1.5.06l.3-7.5Z" clipRule="evenodd" />
            </svg>
          </button>
        )}
      </div>
    </div>
  );
}
