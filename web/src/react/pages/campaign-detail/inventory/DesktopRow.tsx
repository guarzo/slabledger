import type { AgingItem } from '../../../../types/campaigns';
import { formatCents, daysHeldColor } from '../../../utils/formatters';
import { GradeBadge } from '../../../ui';
import MarketplaceLinks from './MarketplaceLinks';
import TruncatedCardName from '../../../ui/TruncatedCardName';
import {
  costBasis, bestPrice, unrealizedPL,
  formatPL,
  isHotSeller, formatReceivedDate,
  referencePricesTooltip,
  syncDotProps, hasCanonicalPriceSignal,
} from './utils';
import { wasUnlistedFromDH } from './inventoryCalcs';
import { dhBadgeFor, DH_BADGE_COLORS } from './dhBadge';
import InlinePriceEdit from './InlinePriceEdit';
import RowActions from './RowActions';
import { resolveContextualPrimary, resolveOverflowActions, ACTION_LABELS } from './rowActions';

const DH_BADGE_TITLES: Record<string, string> = {
  sold:              'Sold on DoubleHolo',
  listed:            'Currently listed on DoubleHolo marketplace',
  'in stock':        'In DoubleHolo inventory — not yet listed',
  shipped:           'Shipped from PSA — not yet received',
  held:              'DoubleHolo push is on hold pending review',
  'no DH match':     'No matching DoubleHolo card found — use "Fix DH Match" to map manually',
  dismissed:         'DoubleHolo listing was dismissed',
  'matching DH':     'Searching DoubleHolo for a matching card',
  'awaiting intake': 'Enrolled in DoubleHolo push pipeline — waiting for cert to be scanned in',
  pushed:            'Submitted to DoubleHolo — awaiting confirmation',
  unenrolled:        'Not enrolled in DoubleHolo',
};

const BADGE_COLORS = [
  { bg: 'rgba(99,102,241,0.1)', text: '#818cf8' },
  { bg: 'rgba(168,85,247,0.1)', text: '#c084fc' },
  { bg: 'rgba(236,72,153,0.1)', text: '#f472b6' },
  { bg: 'rgba(20,184,166,0.1)', text: '#2dd4bf' },
  { bg: 'rgba(245,158,11,0.1)', text: '#fbbf24' },
  { bg: 'rgba(34,211,153,0.1)', text: '#6ee7b7' },
];

function campaignColor(name: string) {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = ((hash << 5) - hash + name.charCodeAt(i)) | 0;
  return BADGE_COLORS[Math.abs(hash) % BADGE_COLORS.length];
}

interface DesktopRowProps {
  item: AgingItem;
  selected: boolean;
  onToggle: () => void;
  onExpand: () => void;
  onRecordSale: () => void;
  onFixPricing?: () => void;
  onFixDHMatch?: () => void;
  onSetPrice?: () => void;
  onDelete?: () => void;
  onListOnDH?: (purchaseId: string) => void;
  onInlinePriceSave?: (purchaseId: string, priceCents: number) => Promise<void>;
  onRemoveFromSellSheet?: () => void;
  onDismiss?: () => void;
  onUndismiss?: () => void;
  onUnmatchDH?: () => void;
  onRetryDHMatch?: () => void;
  dhListingLoading?: boolean;
  dhListedOverride?: boolean;
  showCampaignColumn?: boolean;
  isOnSellSheet?: boolean;
}

export default function DesktopRow({
  item, selected, onToggle, onExpand, onRecordSale,
  onFixPricing, onFixDHMatch, onSetPrice, onDelete,
  onListOnDH, onInlinePriceSave, onRemoveFromSellSheet,
  onDismiss, onUndismiss, onUnmatchDH, onRetryDHMatch,
  dhListingLoading, dhListedOverride, showCampaignColumn, isOnSellSheet,
}: DesktopRowProps) {
  const cb = costBasis(item.purchase);
  const snap = item.currentMarket;
  const daysColor = daysHeldColor(item.daysHeld);
  const listCents = item.purchase.reviewedPriceCents ?? item.purchase.dhListingPriceCents ?? 0;
  const recommendedCents = item.recommendedPriceCents ?? bestPrice(item);
  const pl = unrealizedPL(cb, item);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    const target = e.target as HTMLElement;
    if (target.closest('input,button,a,select,textarea,[role="button"],[role="checkbox"]')) return;
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onExpand();
    }
  };

  const hotSeller = isHotSeller(item);
  const dot = syncDotProps({
    clSyncedAt: item.purchase.clSyncedAt,
    mmValueUpdatedAt: item.purchase.mmValueUpdatedAt,
    dhLastSyncedAt: item.purchase.dhLastSyncedAt,
    clHasValue: (item.purchase.clValueCents ?? 0) > 0,
    hasMMValue: (item.purchase.mmValueCents ?? 0) > 0,
    hasDHPrice: (snap?.gradePriceCents ?? 0) > 0 || (snap?.lastSoldCents ?? 0) > 0,
    clLastError: item.purchase.clLastError,
  });

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
    : `${item.daysHeld}d`;
  const inHandTitle = item.purchase.receivedAt
    ? `In hand since ${formatReceivedDate(item.purchase.receivedAt)}`
    : 'Awaiting intake';

  return (
    <div
      className="flex items-center cursor-pointer focus-visible:outline focus-visible:outline-[3px] focus-visible:outline-[var(--color-focus)] focus-visible:outline-offset-[-2px]"
      role="row"
      tabIndex={0}
      onClick={onExpand}
      onKeyDown={handleKeyDown}
    >
      <div className="glass-table-td flex-shrink-0 !px-1" style={{ width: '28px' }} onClick={e => e.stopPropagation()}>
        <input type="checkbox" checked={selected} onChange={onToggle} onKeyDown={e => e.stopPropagation()} className="rounded accent-[var(--brand-500)]" />
      </div>

      {/* Card */}
      <div className="glass-table-td flex-1 min-w-0 max-w-[320px]" title={item.purchase.cardName}>
        <div className="flex items-center gap-1.5 min-w-0">
          {item.purchase.frontImageUrl && (
            <img
              src={item.purchase.frontImageUrl}
              alt=""
              className="w-8 h-11 object-cover rounded shrink-0 bg-[var(--surface-2)]"
              loading="lazy"
            />
          )}
          {showCampaignColumn && item.campaignName && (() => {
            const color = campaignColor(item.campaignName);
            return (
              <span className="shrink-0 text-[10px] font-medium px-1.5 py-0.5 rounded truncate max-w-[80px]"
                    style={{ background: color.bg, color: color.text }}
                    title={item.campaignName}>
                {item.campaignName}
              </span>
            );
          })()}
          <span className="text-[var(--text)] truncate flex items-center gap-1 min-w-0">
            {hotSeller && <span className="text-[var(--warning)] mr-1" title="High demand">★</span>}
            {isOnSellSheet && <span className="text-gray-400 mr-1 text-xs" title="On sell sheet">&#9864;</span>}
            <TruncatedCardName name={item.purchase.cardName} className="text-[var(--text)] font-medium" />
          </span>
          {item.priceAnomaly && (
            <span className="shrink-0 text-xs text-[var(--warning)]" title={item.anomalyReason || 'Pricing may be inaccurate'}>&#9888;</span>
          )}
          {item.purchase.cardName && item.purchase.setName && (
            <MarketplaceLinks
              cardName={item.purchase.cardName}
              setName={item.purchase.setName}
              cardNumber={item.purchase.cardNumber ?? ''}
              gradeValue={item.purchase.gradeValue}
              variant="inline"
              stopPropagation
            />
          )}
        </div>
        <div className="text-xs text-[var(--text-muted)] truncate leading-tight">
          {item.purchase.setName && <>{item.purchase.setName}</>}
          {item.purchase.cardNumber && <> &middot; #{item.purchase.cardNumber}</>}
          {item.purchase.certNumber && <> &middot; {item.purchase.certNumber}</>}
        </div>
      </div>

      {/* Grade */}
      <div className="glass-table-td flex-shrink-0 text-center" style={{ width: '64px' }}>
        <GradeBadge grader={item.purchase.grader || 'PSA'} grade={item.purchase.gradeValue} size="sm" />
      </div>

      {/* Cost */}
      <div className="glass-table-td flex-shrink-0 text-right text-[var(--text-muted)] tabular-nums" style={{ width: '72px' }}>
        {formatCents(cb)}
      </div>

      {/* Price (with sync dot inline) */}
      <div className="glass-table-td flex-shrink-0 text-right" style={{ width: '180px' }}>
        <div className="flex flex-col items-end gap-[1px]">
          <div className="flex items-center gap-1.5 justify-end">
            <span
              title={dot.tooltip}
              aria-label="Sync freshness"
              style={{ color: dot.color, fontSize: '8px', lineHeight: 1 }}
            >&#9679;</span>
            {listCents === 0 && !hasCanonicalPriceSignal(item) ? (
              <button
                type="button"
                onClick={e => { e.stopPropagation(); (onFixPricing ?? onExpand)(); }}
                className="text-[11px] font-medium text-[var(--warning)] hover:underline inline-flex items-center gap-1"
                title="No CL, DH, or last-sold price data — click to investigate"
              >
                <span aria-hidden="true">&#9888;</span>
                no price data
              </button>
            ) : onInlinePriceSave ? (
              <InlinePriceEdit
                purchaseId={item.purchase.id}
                currentCents={listCents}
                costBasisCents={cb}
                onSave={onInlinePriceSave}
              />
            ) : (
              <span className="tabular-nums text-[var(--text)]">
                {listCents > 0 ? formatCents(listCents) : <span className="text-[var(--text-muted)]">—</span>}
              </span>
            )}
          </div>
          {recommendedCents > 0 && recommendedCents !== listCents && (
            <span
              className="text-[10px] text-[var(--text-muted)] tabular-nums leading-none cursor-help"
              title={referencePricesTooltip(item)}
            >
              rec {formatCents(recommendedCents)}
            </span>
          )}
        </div>
      </div>

      {/* P/L */}
      <div className="glass-table-td flex-shrink-0 text-right tabular-nums print-hide-col" style={{ width: '80px' }}>
        {pl != null ? (
          <span className={`text-xs font-medium px-2 py-[3px] rounded-md ${
            pl > 0 ? 'bg-[var(--success-bg)] text-[var(--success)]' :
            pl < 0 ? 'bg-[var(--danger-bg)] text-[var(--danger)]' :
            'text-[var(--text-muted)]'
          }`}>{formatPL(pl)}</span>
        ) : (
          <span className="text-xs text-[var(--text-muted)]">—</span>
        )}
      </div>

      {/* Status: in-hand label + DH badge (signals zone, end) */}
      <div className="glass-table-td flex-shrink-0 print-hide-col" style={{ width: '120px' }}>
        <div className="flex flex-col gap-0.5 items-center">
          <span className={`text-[11px] tabular-nums ${daysColor}`} title={inHandTitle}>
            {inHandLabel}
          </span>
          {wasUnlistedFromDH(item) && (
            <span
              className="text-[9px] font-medium px-1 py-0.5 rounded bg-[var(--warning)]/15 text-[var(--warning)] leading-none"
              title="Item was removed from DH — will be re-pushed + listed"
            >
              Re-list
            </span>
          )}
          {(() => {
            if (dhListedOverride) {
              return <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${DH_BADGE_COLORS.listed}`} title={DH_BADGE_TITLES['listed']}>listed</span>;
            }
            const badge = dhBadgeFor(item.purchase.dhPushStatus, item.purchase.dhStatus, item.purchase.receivedAt, item.purchase.psaShipDate);
            if (badge === 'unenrolled') return null;
            return (
              <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${DH_BADGE_COLORS[badge]}`} title={DH_BADGE_TITLES[badge] ?? badge}>
                {badge}
              </span>
            );
          })()}
        </div>
      </div>

      {/* Actions zone — separated from signals by border + spacing */}
      <div className="glass-table-td flex-shrink-0 print-hide-actions ml-2 pl-3 border-l border-white/[0.14]" style={{ minWidth: '200px' }}>
        <RowActions primary={primary} fallbackPrimary={fallbackPrimary} overflow={overflow} variant="desktop" />
      </div>
    </div>
  );
}
