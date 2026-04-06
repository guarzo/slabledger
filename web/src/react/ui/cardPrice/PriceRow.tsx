import { clsx } from 'clsx';
import { ConfidenceIndicator } from '../ConfidenceIndicator';
import { LinkDropdown } from '../LinkDropdown';
import { TrendArrow } from '../TrendArrow';
import type { GradeKey, GradeData } from '../../../types/pricing';
import { currency } from '../../utils/formatters';
import { gradeBorderColors, isNoData, formatEbayDisplay, fmtRange, fmtDateShort } from './priceCardUtils';
import type { LastSoldEntry } from './priceCardUtils';

export interface PriceRowProps {
  label: string;
  gradeKey: GradeKey;
  price: number | undefined;
  gradeDetail?: GradeData;
  ebayHref: string;
  ebaySoldHref: string;
  altHref: string;
  cardLadderHref: string;
  compact?: boolean;
  lowestListing?: number;
  activeListings?: number;
  lastSoldEntry?: LastSoldEntry;
  conservativePrice?: number;
}

export function MarketplaceLink({
  href,
  label,
  compact,
}: {
  href: string;
  label: string;
  compact?: boolean;
}) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className={clsx(
        'inline-flex items-center rounded px-1 py-0.5 text-[var(--text-muted)] hover:text-[var(--brand-400)] hover:bg-[var(--surface-hover)] transition-colors',
        compact ? 'text-[10px]' : 'text-xs',
      )}
      title={`Search on ${label}`}
    >
      {label}
    </a>
  );
}

export function PriceRow({
  label,
  gradeKey,
  price,
  gradeDetail,
  ebayHref,
  ebaySoldHref,
  altHref,
  cardLadderHref,
  compact,
  lowestListing,
  activeListings,
  lastSoldEntry,
  conservativePrice,
}: PriceRowProps) {
  const ebay = gradeDetail?.ebay ?? null;
  const estimate = gradeDetail?.estimate ?? null;
  const ebayPrice = ebay?.price ?? null;
  const isPsa10 = gradeKey === 'psa10';
  const isRaw = gradeKey === 'raw';

  // 7-day average divergence
  const avg7d = ebay?.avg7day ?? null;
  const basePrice = ebayPrice ?? price ?? null;
  const avg7dPct = (avg7d != null && basePrice != null && basePrice > 0)
    ? (avg7d - basePrice) / basePrice
    : null;
  const show7d = avg7dPct != null && Math.abs(avg7dPct) > 0.05;

  if (ebayPrice == null && price == null && !estimate) return null;

  /* --- Compact variant --- */
  if (compact) {
    return (
      <div
        className={clsx(
          'rounded border-l-2 py-1 px-1.5 -mx-1.5 text-xs',
          isPsa10 && 'font-bold',
        )}
        style={{
          borderLeftColor: gradeBorderColors[gradeKey],
          ...(isPsa10 ? { background: 'var(--gradient-psa10-highlight)' } : undefined),
        }}
      >
        <div className="flex justify-between items-center">
          <span className="flex items-center gap-1.5">
            <span className="text-[var(--text-muted)]">{label}</span>
            <span className="font-semibold text-[var(--text)] text-xs">
              {formatEbayDisplay(ebayPrice, gradeDetail, price)}
            </span>
            {ebay?.confidence && (
              <ConfidenceIndicator confidence={ebay.confidence} size="sm" />
            )}
          </span>
          <span className="flex items-center gap-1">
            {isRaw ? (
              <MarketplaceLink href={ebayHref} label="eBay" compact />
            ) : (
              <MarketplaceLink href={altHref} label="alt" compact />
            )}
            <MarketplaceLink href={cardLadderHref} label="CL" compact />
          </span>
        </div>
        {(estimate || ebay) && (
          <div className="flex justify-between items-center mt-0.5">
            <span className="flex items-center gap-1.5">
              {estimate ? (
                <>
                  <span className="text-[var(--text-muted)]">est</span>
                  <span className="text-[var(--text)] text-xs">
                    {currency(estimate.price)}
                  </span>
                  <ConfidenceIndicator confidence={estimate.confidence} size="sm" />
                </>
              ) : (
                <span className="text-[var(--text-muted)]">est {'\u2014'}</span>
              )}
            </span>
            {ebay && (
              <span className="flex items-center gap-1 text-[10px] text-[var(--text-muted)]">
                {ebay.salesCount > 0 && (
                  <span>{ebay.salesCount} sold</span>
                )}
                <TrendArrow trend={ebay.trend} size="sm" />
                {show7d && (
                  <span className={avg7dPct! > 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}>
                    7d {avg7dPct! > 0 ? '+' : ''}{(avg7dPct! * 100).toFixed(0)}%
                  </span>
                )}
              </span>
            )}
          </div>
        )}
        {lastSoldEntry && lastSoldEntry.lastSoldPrice > 0 && (
          <div className="flex items-center mt-0.5 text-[10px] text-[var(--text-muted)]">
            <span>last {currency(lastSoldEntry.lastSoldPrice)} {fmtDateShort(lastSoldEntry.lastSoldDate)}</span>
          </div>
        )}
      </div>
    );
  }

  /* --- Featured variant --- */
  const dropdownLinks = isRaw
    ? [
        { label: 'eBay', href: ebayHref },
        { label: 'Card Ladder', href: cardLadderHref },
      ]
    : [
        { label: 'eBay Sold', href: ebaySoldHref },
        { label: 'Alt', href: altHref },
        { label: 'Card Ladder', href: cardLadderHref },
      ];

  const bothNoData = isNoData(ebayPrice) && isNoData(estimate?.price) && isNoData(price);
  const ebayDisplay = formatEbayDisplay(ebayPrice, gradeDetail, price);
  const ebayIsEmpty = ebayDisplay === '\u2014';

  return (
    <div
      className={clsx(
        'rounded border-l py-2 px-2 -mx-2 border-b border-[var(--surface-0)]',
        isPsa10 ? 'text-base' : 'text-sm',
        bothNoData && 'opacity-40',
      )}
      style={{
        borderLeftColor: gradeBorderColors[gradeKey],
        borderLeftWidth: gradeKey === 'psa10' ? 4 : gradeKey === 'psa9' ? 3 : 2,
        ...(isPsa10
          ? { background: 'var(--gradient-psa10-highlight)' }
          : gradeKey === 'psa9'
          ? { background: 'var(--gradient-psa9-highlight)' }
          : undefined),
      }}
    >
      <div className="flex items-center">
        <span
          className={clsx('w-14 shrink-0 font-medium', isRaw && 'text-[var(--text-muted)]')}
          style={!isRaw ? { color: gradeBorderColors[gradeKey] } : undefined}
        >
          {label}
        </span>
        <span className="flex items-center gap-1.5 flex-1 min-w-0">
          <span className={clsx(
            'tabular-nums',
            ebayIsEmpty ? 'text-[var(--text-subtle)]' : 'font-semibold text-[var(--text)]',
          )}>
            {ebayDisplay}
          </span>
          {ebay?.confidence && (
            <ConfidenceIndicator confidence={ebay.confidence} size="sm" />
          )}
        </span>
        <span className="flex items-center gap-1.5 justify-end shrink-0">
          {estimate ? (
            <>
              <span className={clsx(
                'tabular-nums',
                isNoData(estimate.price) ? 'text-[var(--text-subtle)]' : 'font-semibold text-[var(--text)]',
              )}>
                {isNoData(estimate.price) ? '\u2014' : currency(estimate.price)}
              </span>
              <ConfidenceIndicator confidence={estimate.confidence} size="sm" />
            </>
          ) : (
            <span className="text-[var(--text-subtle)]">{'\u2014'}</span>
          )}
        </span>
        <LinkDropdown links={dropdownLinks} />
      </div>
      <div className="flex items-center mt-0.5">
        <span className="w-14 shrink-0" />
        <span className="flex items-center gap-1.5 flex-1 min-w-0 text-[11px] text-[var(--text-subtle)]">
          {isRaw && lowestListing != null && activeListings != null ? (
            <span>low {currency(lowestListing)} &middot; {activeListings} listed</span>
          ) : ebay && ebay.salesCount > 0 ? (
            <>
              <span>{ebay.salesCount} sold</span>
              <TrendArrow trend={ebay.trend} size="sm" />
              {show7d && (
                <span className={avg7dPct! > 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}>
                  &middot; 7d: {currency(avg7d!)} ({avg7dPct! > 0 ? '+' : ''}{(avg7dPct! * 100).toFixed(0)}%)
                </span>
              )}
              {ebay.min != null && ebay.max != null && ebay.min > 0 && ebay.max > 0 && (
                <span>&middot; {fmtRange(ebay.min, ebay.max)}</span>
              )}
            </>
          ) : null}
        </span>
        <span className="flex items-center gap-1.5 justify-end shrink-0 text-[11px] text-[var(--text-subtle)]">
          {estimate && estimate.low > 0 && estimate.high > 0 ? (
            <span>
              {fmtRange(estimate.low, estimate.high)}
              {conservativePrice != null && conservativePrice > 0 && (
                <> &middot; floor {currency(conservativePrice)}</>
              )}
            </span>
          ) : conservativePrice != null && conservativePrice > 0 ? (
            <span>floor {currency(conservativePrice)}</span>
          ) : null}
        </span>
      </div>
      {lastSoldEntry && lastSoldEntry.lastSoldPrice > 0 && (
        <div className="flex items-center mt-0.5">
          <span className="w-14 shrink-0" />
          <span className="text-[11px] text-[var(--text-subtle)]">
            last: {currency(lastSoldEntry.lastSoldPrice)} on {fmtDateShort(lastSoldEntry.lastSoldDate)}
            {lastSoldEntry.saleCount > 0 && <> &middot; {lastSoldEntry.saleCount} sales</>}
          </span>
        </div>
      )}
    </div>
  );
}
