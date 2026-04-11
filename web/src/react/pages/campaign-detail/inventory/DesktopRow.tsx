import type { AgingItem } from '../../../../types/campaigns';
import { formatCents, daysHeldColor } from '../../../utils/formatters';
import { TrendArrow, ConfidenceIndicator, GradeBadge } from '../../../ui';
import { DropdownMenu } from 'radix-ui';
import MarketplaceLinks from './MarketplaceLinks';
import {
  costBasis, bestPrice, unrealizedPL, marketTrend,
  getSourceByType, marketTooltip,
  formatPL,
  getReviewStatus, statusBorderColor, isHotSeller, formatReceivedDate,
  syncDotProps,
} from './utils';

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
  onSetPrice?: () => void;
  onDelete?: () => void;
  showCampaignColumn?: boolean;
  isOnSellSheet?: boolean;
}

export default function DesktopRow({ item, selected, onToggle, onExpand, onRecordSale, onFixPricing, onSetPrice, onDelete, showCampaignColumn, isOnSellSheet }: DesktopRowProps) {
  const cb = costBasis(item.purchase);
  const snap = item.currentMarket;
  const daysColor = daysHeldColor(item.daysHeld);
  const price = snap ? bestPrice(snap) : 0;
  const pl = unrealizedPL(cb, snap);
  const trend = snap ? marketTrend(snap) : null;

  const handleKeyDown = (e: React.KeyboardEvent) => {
    const target = e.target as HTMLElement;
    if (target.closest('input,button,a,select,textarea,[role="button"],[role="checkbox"]')) return;
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onExpand();
    }
  };

  const reviewStatus = getReviewStatus(item);
  const hotSeller = isHotSeller(item);
  const dot = syncDotProps(
    item.purchase.clSyncedAt,
    item.purchase.mmValueUpdatedAt,
    item.purchase.dhLastSyncedAt,
  );

  return (
    <div
      className="flex items-center cursor-pointer"
      role="row"
      tabIndex={0}
      onClick={onExpand}
      onKeyDown={handleKeyDown}
      style={{ borderLeft: `3px solid ${statusBorderColor(reviewStatus)}` }}
    >
      <div className="glass-table-td flex-shrink-0 !px-1" style={{ width: '28px' }} onClick={e => e.stopPropagation()}>
        <input type="checkbox" checked={selected} onChange={onToggle} onKeyDown={e => e.stopPropagation()} className="rounded accent-[var(--brand-500)]" />
      </div>
       <div className="glass-table-td flex-1 min-w-0" title={item.purchase.cardName}>
         <div className="flex items-center gap-1.5 min-w-0">
           <div style={{ position: 'relative', width: 32, height: 44, flexShrink: 0 }}>
             {item.purchase.frontImageUrl && (
               <img
                 src={item.purchase.frontImageUrl}
                 alt=""
                 className="w-8 h-11 object-cover rounded shrink-0 bg-[var(--surface-2)]"
                 loading="lazy"
               />
             )}
             {item.purchase.receivedAt && (
               <div
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
          <span className="text-[var(--text)] truncate">
            {hotSeller && <span className="text-amber-400 mr-1" title="High demand">★</span>}
            {isOnSellSheet && <span className="text-gray-400 mr-1 text-xs" title="On sell sheet">&#9864;</span>}
            {item.purchase.cardName}
          </span>
          {item.priceAnomaly && (
            <span className="shrink-0 text-xs text-amber-500" title={item.anomalyReason || 'Pricing may be inaccurate'}>&#9888;</span>
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
      <div className="glass-table-td flex-shrink-0 text-center" style={{ width: '48px' }}>
        <GradeBadge grader={item.purchase.grader || 'PSA'} grade={item.purchase.gradeValue} size="sm" />
      </div>
      <div className="glass-table-td flex-shrink-0 text-right text-[var(--text)] tabular-nums" style={{ width: '72px' }}>{formatCents(cb)}</div>
      <div className="glass-table-td flex-shrink-0 text-right" style={{ width: '120px' }}
        title={snap ? marketTooltip(snap, cb) : undefined}>
        {snap && price > 0 ? (() => {
          const displaySource = getSourceByType(snap.sourcePrices, 'ebay') || getSourceByType(snap.sourcePrices, 'estimate');
          const confidence = displaySource?.confidence ?? snap?.confidence ?? null;
          return (
            <div className="flex items-center justify-end gap-1">
              <span className="text-[var(--text)] tabular-nums">{formatCents(price)}</span>
              <ConfidenceIndicator confidence={confidence as 'high' | 'medium' | 'low' | null} size="sm" />
              <TrendArrow trend={trend} size="sm" />
            </div>
          );
        })() : (
          <span className="text-xs text-[var(--text-muted)]">-</span>
        )}
      </div>
      {/* Unrealized P/L */}
      <div className="glass-table-td flex-shrink-0 text-right tabular-nums print-hide-col" style={{ width: '72px' }}>
        {pl != null ? (
          <span className={`text-xs font-medium px-2 py-[3px] rounded-md ${
            pl > 0 ? 'bg-[rgba(52,211,153,0.1)] text-[#34d399]' :
            pl < 0 ? 'bg-[rgba(248,113,113,0.1)] text-[#f87171]' :
            'text-[var(--text-muted)]'
          }`}>{formatPL(pl)}</span>
        ) : (
          <span className="text-xs text-[var(--text-muted)]">-</span>
        )}
      </div>
      {/* Days held */}
      <div className={`glass-table-td flex-shrink-0 text-center print-hide-col ${daysColor}`} style={{ width: '40px' }}>{item.daysHeld}</div>
      {/* Sync freshness dot */}
      <div className="glass-table-td flex-shrink-0 text-center print-hide-col" style={{ width: '20px' }}>
        <span
          title={dot.tooltip}
          aria-label="Sync freshness"
          style={{ color: dot.color, fontSize: '10px', lineHeight: 1 }}
        >&#9679;</span>
      </div>
      {/* Sell button */}
      <div className="glass-table-td flex-shrink-0 text-center !px-1 print-hide-actions" style={{ width: '48px' }} onClick={e => e.stopPropagation()}>
        <button
          type="button"
          onClick={onRecordSale}
          className="text-xs font-medium px-2 py-1 rounded bg-[var(--brand-500)]/20 text-[var(--brand-400)] hover:bg-[var(--brand-500)]/40 transition-colors"
        >
          Sell
        </button>
      </div>
      {/* Actions overflow menu */}
      <div className="glass-table-td flex-shrink-0 text-center !px-1 print-hide-actions" style={{ width: '28px' }}>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              type="button"
              onClick={e => e.stopPropagation()}
              onKeyDown={e => e.stopPropagation()}
              className="p-1 rounded text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[rgba(255,255,255,0.04)] transition-colors"
              aria-label="Card actions"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <circle cx="12" cy="5" r="2" />
                <circle cx="12" cy="12" r="2" />
                <circle cx="12" cy="19" r="2" />
              </svg>
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content
              align="end"
              sideOffset={4}
              className="w-40 py-1 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-lg shadow-lg z-50
                         data-[state=open]:animate-[fadeIn_150ms_ease-out]"
            >
              {onSetPrice && (
                <DropdownMenu.Item
                  onSelect={onSetPrice}
                  className="px-3 py-2 text-sm text-[var(--text-muted)] hover:bg-[rgba(255,255,255,0.04)] hover:text-[var(--text)] outline-none cursor-default"
                >
                  Set Price
                </DropdownMenu.Item>
              )}
              {onFixPricing && (
                <DropdownMenu.Item
                  onSelect={onFixPricing}
                  className="px-3 py-2 text-sm text-[var(--text-muted)] hover:bg-[rgba(255,255,255,0.04)] hover:text-[var(--text)] outline-none cursor-default"
                >
                  Fix Pricing
                </DropdownMenu.Item>
              )}
              {onDelete && (
                <>
                  <DropdownMenu.Separator className="my-1 h-px bg-[var(--surface-2)]" />
                  <DropdownMenu.Item
                    onSelect={() => {
                      if (window.confirm('Delete this item? This cannot be undone.')) {
                        onDelete();
                      }
                    }}
                    className="px-3 py-2 text-sm text-red-400 hover:bg-[rgba(248,113,113,0.08)] hover:text-red-300 outline-none cursor-default"
                  >
                    Delete
                  </DropdownMenu.Item>
                </>
              )}
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>
    </div>
  );
}
