import type { AgingItem } from '../../../../types/campaigns';
import { formatCents, daysHeldColor } from '../../../utils/formatters';
import { Button, GradeBadge } from '../../../ui';
import { DropdownMenu } from 'radix-ui';
import MarketplaceLinks from './MarketplaceLinks';
import {
  costBasis, bestPrice, unrealizedPL,
  formatPL,
  getReviewStatus, statusBorderColor, isHotSeller, formatReceivedDate,
  referencePricesTooltip,
  syncDotProps, hasCanonicalPriceSignal,
} from './utils';
import { wasUnlistedFromDH, deriveActionIntent, canDismiss, type ActionIntent } from './inventoryCalcs';
import { dhBadgeFor, DH_BADGE_COLORS } from './dhBadge';
import InlinePriceEdit from './InlinePriceEdit';

const DH_BADGE_TITLES: Record<string, string> = {
  sold:              'Sold on DoubleHolo',
  listed:            'Currently listed on DoubleHolo marketplace',
  'in stock':        'In DoubleHolo inventory — not yet listed',
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

export default function DesktopRow({ item, selected, onToggle, onExpand, onRecordSale, onFixPricing, onFixDHMatch, onSetPrice, onDelete, onListOnDH, onInlinePriceSave, onRemoveFromSellSheet, onDismiss, onUndismiss, onUnmatchDH, onRetryDHMatch, dhListingLoading, dhListedOverride, showCampaignColumn, isOnSellSheet }: DesktopRowProps) {
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

  const reviewStatus = getReviewStatus(item);
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

  const actionIntent = deriveActionIntent(item);
  const showDismiss = canDismiss(actionIntent);

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
            {hotSeller && <span className="text-[var(--warning)] mr-1" title="High demand">★</span>}
            {isOnSellSheet && <span className="text-gray-400 mr-1 text-xs" title="On sell sheet">&#9864;</span>}
            {item.purchase.cardName}
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
      <div className="glass-table-td flex-shrink-0 text-center" style={{ width: '48px' }}>
        <GradeBadge grader={item.purchase.grader || 'PSA'} grade={item.purchase.gradeValue} size="sm" />
      </div>
      <div className="glass-table-td flex-shrink-0 text-right text-[var(--text)] tabular-nums" style={{ width: '72px' }}>{formatCents(cb)}</div>
      <div className="glass-table-td flex-shrink-0 text-right" style={{ width: '140px' }}>
        <div className="flex flex-col items-end gap-[1px]">
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
      {/* DH status — read-only signal, no actions (actions live in the ⋯ menu) */}
      <div className="glass-table-td flex-shrink-0 text-center !px-1 print-hide-actions" style={{ width: '64px' }}>
        <div className="flex flex-col items-center gap-0.5">
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
            const badge = dhBadgeFor(item.purchase.dhPushStatus, item.purchase.dhStatus, item.purchase.receivedAt);
            if (badge === 'unenrolled') return null;
            return (
              <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${DH_BADGE_COLORS[badge]}`} title={DH_BADGE_TITLES[badge] ?? badge}>
                {badge}
              </span>
            );
          })()}
        </div>
      </div>
      {/* Hairline divider — separates signal columns from the action cluster */}
      <div aria-hidden="true" className="glass-table-td flex-shrink-0 self-stretch !p-0 print-hide-actions" style={{ width: '1px' }}>
        <div className="w-px h-full bg-white/[0.06]" />
      </div>
      {/* Primary action — Sell */}
      <div className="glass-table-td flex-shrink-0 text-center !px-1 print-hide-actions" style={{ width: '64px' }} onClick={e => e.stopPropagation()}>
        <Button variant="primary" size="sm" onClick={onRecordSale} aria-label={`Record sale of ${item.purchase.cardName}`}>
          Sell
        </Button>
      </div>
      {/* Overflow menu — secondary actions */}
      <div className="glass-table-td flex-shrink-0 text-center !px-1 print-hide-actions" style={{ width: '28px' }}>
        <RowOverflowMenu
          item={item}
          actionIntent={actionIntent}
          showDismiss={showDismiss}
          dhListingLoading={dhListingLoading}
          onSetPrice={onSetPrice}
          onFixPricing={onFixPricing}
          onFixDHMatch={onFixDHMatch}
          onUnmatchDH={onUnmatchDH}
          onRetryDHMatch={onRetryDHMatch}
          onListOnDH={onListOnDH}
          onUndismiss={onUndismiss}
          onDismiss={onDismiss}
          isOnSellSheet={!!isOnSellSheet}
          onRemoveFromSellSheet={onRemoveFromSellSheet}
          onDelete={onDelete}
        />
      </div>
    </div>
  );
}

interface RowOverflowMenuProps {
  item: AgingItem;
  actionIntent: ActionIntent;
  showDismiss: boolean;
  dhListingLoading?: boolean;
  onSetPrice?: () => void;
  onFixPricing?: () => void;
  onFixDHMatch?: () => void;
  onUnmatchDH?: () => void;
  onRetryDHMatch?: () => void;
  onListOnDH?: (purchaseId: string) => void;
  onUndismiss?: () => void;
  onDismiss?: () => void;
  isOnSellSheet: boolean;
  onRemoveFromSellSheet?: () => void;
  onDelete?: () => void;
}

const ITEM_BASE = 'px-3 py-2 text-sm outline-none cursor-default';
const ITEM_DEFAULT = `${ITEM_BASE} text-[var(--text-muted)] hover:bg-[rgba(255,255,255,0.04)] hover:text-[var(--text)]`;
const ITEM_PRIMARY = `${ITEM_BASE} text-[var(--brand-300)] bg-[var(--brand-500)]/10 hover:bg-[var(--brand-500)]/20`;
const ITEM_DANGER = `${ITEM_BASE} text-[var(--danger)] hover:bg-[var(--danger-bg)] hover:text-[var(--danger)]`;

function RowOverflowMenu({
  item, actionIntent, showDismiss, dhListingLoading,
  onSetPrice, onFixPricing, onFixDHMatch, onUnmatchDH, onRetryDHMatch,
  onListOnDH, onUndismiss, onDismiss,
  isOnSellSheet, onRemoveFromSellSheet, onDelete,
}: RowOverflowMenuProps) {
  // The item highlighted at the top is the row's contextual primary action
  // (driven by deriveActionIntent). Surfacing it as the first menu entry keeps
  // workflows fast — the user opens the menu and the recommended next step is
  // already the visually-accented option.
  const primary = (() => {
    if (actionIntent === 'list' && onListOnDH) {
      return {
        label: dhListingLoading ? 'Listing…' : 'List on DH',
        disabled: !!dhListingLoading,
        onSelect: () => onListOnDH(item.purchase.id),
      };
    }
    if (actionIntent === 'set_and_list' && onSetPrice) {
      return { label: 'Set Price', onSelect: onSetPrice };
    }
    if (actionIntent === 'fix_match' && onFixDHMatch) {
      return { label: 'Fix DH Match', onSelect: onFixDHMatch };
    }
    if (actionIntent === 'restore' && onUndismiss) {
      return { label: 'Restore to DH', onSelect: onUndismiss };
    }
    return null;
  })();

  // Standard items are the always-available secondary actions. We omit the
  // entry that is currently surfaced as `primary` to avoid duplicates.
  const showSetPrice = onSetPrice && primary?.label !== 'Set Price';
  const showFixDHMatch = onFixDHMatch && primary?.label !== 'Fix DH Match';

  return (
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
          className="w-44 py-1 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-lg shadow-lg z-50 data-[state=open]:animate-[fadeIn_150ms_ease-out]"
        >
          {primary && (
            <>
              <DropdownMenu.Item
                disabled={primary.disabled}
                onSelect={primary.onSelect}
                className={ITEM_PRIMARY}
              >
                {primary.label}
              </DropdownMenu.Item>
              <DropdownMenu.Separator className="my-1 h-px bg-[var(--surface-2)]" />
            </>
          )}
          {showSetPrice && (
            <DropdownMenu.Item onSelect={onSetPrice} className={ITEM_DEFAULT}>Set Price</DropdownMenu.Item>
          )}
          {onFixPricing && (
            <DropdownMenu.Item onSelect={onFixPricing} className={ITEM_DEFAULT}>Fix Pricing</DropdownMenu.Item>
          )}
          {showFixDHMatch && (
            <DropdownMenu.Item onSelect={onFixDHMatch} className={ITEM_DEFAULT}>Fix DH Match</DropdownMenu.Item>
          )}
          {onUnmatchDH && (
            <DropdownMenu.Item onSelect={onUnmatchDH} className={ITEM_DEFAULT}>Remove DH Match</DropdownMenu.Item>
          )}
          {onRetryDHMatch && (
            <DropdownMenu.Item onSelect={onRetryDHMatch} className={ITEM_DEFAULT}>Retry DH Match</DropdownMenu.Item>
          )}
          {showDismiss && onDismiss && (
            <DropdownMenu.Item
              onSelect={() => {
                if (window.confirm('Dismiss this item from DH listing?')) onDismiss();
              }}
              className={ITEM_DEFAULT}
            >
              Dismiss from DH
            </DropdownMenu.Item>
          )}
          {isOnSellSheet && onRemoveFromSellSheet && (
            <DropdownMenu.Item onSelect={onRemoveFromSellSheet} className={ITEM_DEFAULT}>
              Remove from Sell Sheet
            </DropdownMenu.Item>
          )}
          {onDelete && (
            <>
              <DropdownMenu.Separator className="my-1 h-px bg-[var(--surface-2)]" />
              <DropdownMenu.Item
                onSelect={() => {
                  if (window.confirm('Delete this item? This cannot be undone.')) onDelete();
                }}
                className={ITEM_DANGER}
              >
                Delete
              </DropdownMenu.Item>
            </>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
