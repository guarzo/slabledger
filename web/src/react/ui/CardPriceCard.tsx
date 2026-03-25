/**
 * CardPriceCard - Unified card display component
 *
 * Used in both Price Lookup (featured variant) and Watchlist (compact variant).
 * Shows card image, name, set, graded prices with eBay links, and favorite toggle.
 *
 * Two-column layout when gradeData is available:
 *   Featured: eBay Sold (left) + Estimate (right) with confidence dots and trend arrows
 *   Compact:  condensed two-line per grade with sales count and trend
 *
 * Falls back to legacy flat-price layout when gradeData is not provided.
 */
import { clsx } from 'clsx';
import { CardShell } from './CardShell';
import { ConfidenceIndicator } from './ConfidenceIndicator';
import { FavoriteButton } from './FavoriteButton';
import { LinkDropdown } from './LinkDropdown';
import { Skeleton } from './Skeleton';
import { TrendArrow } from './TrendArrow';
import type { GradeData, GradeKey, MarketOverview, SalesVelocity } from '../../types/pricing';
import {
  defaultEbayUrl as sharedDefaultEbayUrl,
  defaultAltUrl as sharedDefaultAltUrl,
  defaultCardLadderUrl as sharedDefaultCardLadderUrl,
  ebayCompletedUrl,
} from '../utils/marketplaceUrls';
import type { SearchableCard } from '../utils/marketplaceUrls';
import { currency } from '../utils/formatters';

export interface CardPriceData {
  name: string;
  setName: string;
  number: string;
  imageUrl?: string;
}

export interface CardPrices extends Partial<Record<GradeKey, number>> {}

interface LastSoldEntry {
  lastSoldPrice: number;
  lastSoldDate: string;
  saleCount: number;
}

export interface CardPriceCardProps {
  card: CardPriceData;
  prices?: CardPrices | null;
  pricesLoading?: boolean;
  variant?: 'compact' | 'featured';
  getEbayUrl?: (card: CardPriceData, grade?: GradeKey) => string;
  getAltUrl?: (card: CardPriceData, grade?: GradeKey) => string;
  getCardLadderUrl?: (card: CardPriceData, grade?: GradeKey) => string;
  className?: string;
  // New props (all optional for backward compat)
  gradeData?: Partial<Record<GradeKey, GradeData>>;
  market?: MarketOverview;
  velocity?: SalesVelocity;
  lastSold?: Partial<Record<string, LastSoldEntry>>;
  conservativePrices?: { psa10?: number; psa9?: number; raw?: number };
}

/* ---------- Formatters ---------- */

function fmtRange(low: number, high: number): string {
  const fmtPart = (v: number): string => currency(v).replace('$', '');
  return `$${fmtPart(low)}\u2013$${fmtPart(high)}`;
}

function isNoData(price: number | null | undefined): boolean {
  return price == null || price < 0.01;
}

function formatEbayDisplay(
  ebayPrice: number | null,
  gradeDetail: GradeData | undefined,
  fallbackPrice: number | undefined,
): string {
  if (ebayPrice != null) return isNoData(ebayPrice) ? '\u2014' : currency(ebayPrice);
  if (gradeDetail) return '\u2014';
  return isNoData(fallbackPrice) ? '\u2014' : currency(fallbackPrice);
}

/* ---------- URL helpers (delegated to shared utils) ---------- */

function toSearchable(card: CardPriceData): SearchableCard {
  return { name: card.name, setName: card.setName, number: card.number };
}

function defaultEbayUrl(card: CardPriceData, grade?: GradeKey): string {
  return sharedDefaultEbayUrl(toSearchable(card), grade);
}

function defaultAltUrl(card: CardPriceData, grade?: GradeKey): string {
  return sharedDefaultAltUrl(toSearchable(card), grade);
}

function defaultCardLadderUrl(card: CardPriceData, grade?: GradeKey): string {
  return sharedDefaultCardLadderUrl(toSearchable(card), grade);
}

function localEbayCompletedUrl(card: CardPriceData, grade?: GradeKey): string {
  return ebayCompletedUrl(toSearchable(card), grade);
}

/* ---------- Constants ---------- */

const gradeRows: { key: GradeKey; label: string }[] = [
  { key: 'raw', label: 'Raw' },
  { key: 'psa8', label: 'PSA 8' },
  { key: 'psa9', label: 'PSA 9' },
  { key: 'psa10', label: 'PSA 10' },
];

const gradeBorderColors: Partial<Record<GradeKey, string>> = {
  raw: 'var(--text-muted)',
  psa8: 'var(--grade-psa8)',
  psa9: 'var(--grade-psa9)',
  psa10: 'var(--grade-psa10)',
};

/* ---------- Link helper component ---------- */

function MarketplaceLink({
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

/* ---------- PriceRow (two-column redesign) ---------- */

interface PriceRowProps {
  label: string;
  gradeKey: GradeKey;
  price: number | undefined;        // Legacy flat price (fallback)
  gradeDetail?: GradeData;          // New per-grade detail
  ebayHref: string;
  ebaySoldHref: string;
  altHref: string;
  cardLadderHref: string;
  compact?: boolean;
  // For Raw row only:
  lowestListing?: number;
  activeListings?: number;
  // Enhanced data
  lastSoldEntry?: LastSoldEntry;
  conservativePrice?: number;
}

function fmtDateShort(dateStr: string): string {
  if (!dateStr) return '\u2014';
  const parts = dateStr.split('-');
  if (parts.length < 3) return '\u2014';
  const month = parseInt(parts[1], 10);
  const day = parseInt(parts[2], 10);
  if (isNaN(month) || isNaN(day)) return '\u2014';
  return `${month}/${day}`;
}

function PriceRow({
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

  // Show row only when we have eBay or estimate data from grade details,
  // or a legacy flat price. Don't conflate sources.
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
        {/* Line 1: grade + eBay price + confidence + marketplace link */}
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
        {/* Line 2: est price + confidence + sales count + trend + 7d */}
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
        {/* Line 3: last sold (abbreviated) */}
        {lastSoldEntry && lastSoldEntry.lastSoldPrice > 0 && (
          <div className="flex items-center mt-0.5 text-[10px] text-[var(--text-muted)]">
            <span>last {currency(lastSoldEntry.lastSoldPrice)} {fmtDateShort(lastSoldEntry.lastSoldDate)}</span>
          </div>
        )}
      </div>
    );
  }

  /* --- Featured variant --- */

  // Build per-row dropdown links
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
      {/* Main row: grade label | eBay price + confidence | estimate price + confidence | dropdown */}
      <div className="flex items-center">
        {/* Grade label — colored for graded, muted for raw */}
        <span
          className={clsx('w-14 shrink-0 font-medium', isRaw && 'text-[var(--text-muted)]')}
          style={!isRaw ? { color: gradeBorderColors[gradeKey] } : undefined}
        >
          {label}
        </span>

        {/* eBay Sold column */}
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

        {/* Estimate column */}
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

        {/* Chain-link dropdown */}
        <LinkDropdown links={dropdownLinks} />
      </div>

      {/* Sub row: sales count + trend + 7d + eBay range | estimate range + floor */}
      <div className="flex items-center mt-0.5">
        <span className="w-14 shrink-0" />

        {/* eBay sub-info */}
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

        {/* Estimate range + conservative floor */}
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

      {/* Last sold sub-row */}
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

/* ---------- Legacy PriceRow (no gradeData) ---------- */

function LegacyPriceRow({
  label,
  price,
  ebayHref,
  altHref,
  cardLadderHref,
  compact,
  gradeKey,
}: {
  label: string;
  price: number | undefined;
  ebayHref: string;
  altHref: string;
  cardLadderHref: string;
  compact?: boolean;
  gradeKey: GradeKey;
}) {
  if (price == null) return null;

  const isPsa10 = gradeKey === 'psa10';
  const isRaw = gradeKey === 'raw';
  const priceEmpty = isNoData(price);

  /* --- Compact variant: keep inline text links --- */
  if (compact) {
    return (
      <div
        className={clsx(
          'flex justify-between items-center rounded border-l-2 py-1 px-1.5 -mx-1.5 text-xs',
          isPsa10 && 'font-bold',
        )}
        style={{
          borderLeftColor: gradeBorderColors[gradeKey],
          ...(isPsa10 ? { background: 'var(--gradient-psa10-highlight)' } : undefined),
        }}
      >
        <span className="text-[var(--text-muted)]">{label}</span>
        <span className="flex items-center gap-1.5">
          <span className={clsx('text-xs', priceEmpty ? 'text-[var(--text-subtle)]' : 'font-semibold text-[var(--text)]')}>
            {priceEmpty ? '\u2014' : currency(price)}
          </span>
          <MarketplaceLink href={ebayHref} label="eBay" compact />
          <MarketplaceLink href={altHref} label="Alt" compact />
          <MarketplaceLink href={cardLadderHref} label="CL" compact />
        </span>
      </div>
    );
  }

  /* --- Featured variant: chain-link dropdown + visual polish --- */
  const dropdownLinks = [
    { label: 'eBay', href: ebayHref },
    { label: 'Alt', href: altHref },
    { label: 'Card Ladder', href: cardLadderHref },
  ];

  return (
    <div
      className={clsx(
        'flex justify-between items-center rounded border-l py-2 px-2 -mx-2 border-b border-[var(--surface-0)]',
        isPsa10 ? 'text-base' : 'text-sm',
        priceEmpty && 'opacity-40',
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
      <span
        className={clsx('font-medium', isRaw && 'text-[var(--text-muted)]')}
        style={!isRaw ? { color: gradeBorderColors[gradeKey] } : undefined}
      >
        {label}
      </span>
      <span className="flex items-center gap-1.5">
        <span className={clsx(
          'tabular-nums',
          priceEmpty ? 'text-[var(--text-subtle)]' : 'font-semibold text-[var(--text)]',
        )}>
          {priceEmpty ? '\u2014' : currency(price)}
        </span>
        <LinkDropdown links={dropdownLinks} />
      </span>
    </div>
  );
}

/* ---------- Main component ---------- */

export function CardPriceCard({
  card,
  prices,
  pricesLoading,
  variant = 'compact',
  getEbayUrl = defaultEbayUrl,
  getAltUrl = defaultAltUrl,
  getCardLadderUrl = defaultCardLadderUrl,
  className,
  gradeData,
  market,
  velocity,
  lastSold,
  conservativePrices,
}: CardPriceCardProps) {
  const isCompact = variant === 'compact';
  const hasGradeData = gradeData != null;

  return (
    <CardShell
      variant="glass"
      padding="none"
      className={clsx('overflow-hidden', className)}
      ariaLabel={`${card.name} from ${card.setName}`}
    >
      {/* Image */}
      <div className="relative">
        {card.imageUrl ? (
          <img
            src={card.imageUrl}
            alt={card.name}
            className={clsx(
              'w-full object-contain bg-[var(--surface-0)]',
              isCompact ? 'h-48' : 'h-80',
            )}
            loading="lazy"
          />
        ) : (
          <div
            className={clsx(
              'w-full flex items-center justify-center bg-[var(--surface-1)] text-[var(--text-muted)]',
              isCompact ? 'h-48' : 'h-80',
            )}
          >
            No Image
          </div>
        )}

        {/* Gradient overlay for smooth transition */}
        <div className="absolute bottom-0 left-0 right-0 h-12 bg-gradient-to-t from-[var(--surface-1)] to-transparent pointer-events-none" />

        {/* Favorite button overlay */}
        <div className="absolute top-2 right-2 bg-[var(--surface-0)]/80 backdrop-blur-sm rounded-full shadow-sm">
          <FavoriteButton
            cardName={card.name}
            setName={card.setName}
            cardNumber={card.number}
            imageUrl={card.imageUrl}
            size={isCompact ? 'sm' : 'md'}
          />
        </div>
      </div>

      {/* Card info + pricing */}
      <div className={isCompact ? 'p-3' : 'p-5'}>
        <div className="flex items-center gap-1.5">
          <h3
            className={clsx(
              'font-bold truncate',
              isCompact
                ? 'text-sm mb-0.5 text-[var(--text)]'
                : 'text-lg mb-1 text-gradient text-gradient-premium',
            )}
            title={card.name}
          >
            {card.name}
          </h3>
          {!isCompact && (
            <LinkDropdown
              links={[
                { label: 'eBay', href: getEbayUrl(card) },
                { label: 'Alt', href: getAltUrl(card) },
                { label: 'Card Ladder', href: getCardLadderUrl(card) },
              ]}
              size="md"
              align="left"
            />
          )}
        </div>
        <p className={clsx('text-[var(--text-muted)]', isCompact ? 'text-xs mb-2' : 'text-sm mb-3')}>
          <span className="uppercase tracking-wide">{card.setName}</span>
          {card.number && <span> &middot; #{card.number}</span>}
        </p>

        {/* Prices */}
        {pricesLoading ? (
          <div className="space-y-2">
            {[1, 2, 3, 4].map(i => (
              <Skeleton key={i} />
            ))}
          </div>
        ) : prices ? (
            hasGradeData ? (
              <>
                {/* Column headers (featured only) */}
                {!isCompact && (
                  <div className="flex items-center text-[10px] uppercase tracking-wider font-medium text-[var(--text-subtle)] mb-2 pb-1.5 px-2 -mx-2 border-b border-[var(--surface-2)]">
                    <span className="w-14 shrink-0" />
                    <span className="flex-1">eBay Sold</span>
                    <span className="shrink-0">Estimate</span>
                  </div>
                )}

                {/* Grade rows with two-column layout */}
                <div className={isCompact ? 'space-y-0' : 'space-y-1'}>
                  {gradeRows.map(({ key, label }) => {
                    const consPrice = key === 'psa10' ? conservativePrices?.psa10
                      : key === 'psa9' ? conservativePrices?.psa9
                      : key === 'raw' ? conservativePrices?.raw
                      : undefined;
                    return (
                      <PriceRow
                        key={key}
                        label={label}
                        gradeKey={key}
                        price={prices[key]}
                        gradeDetail={gradeData[key]}
                        ebayHref={getEbayUrl(card, key)}
                        ebaySoldHref={localEbayCompletedUrl(card, key)}
                        altHref={getAltUrl(card, key)}
                        cardLadderHref={getCardLadderUrl(card, key)}
                        compact={isCompact}
                        lowestListing={key === 'raw' ? market?.lowestListing : undefined}
                        activeListings={key === 'raw' ? market?.activeListings : undefined}
                        lastSoldEntry={lastSold?.[key]}
                        conservativePrice={consPrice}
                      />
                    );
                  })}
                </div>

                {/* Bottom section: volume + velocity (featured only) */}
                {!isCompact && velocity && (
                  <div className="mt-3 pt-3 border-t border-[var(--surface-0)] text-xs text-[var(--text-muted)]">
                    30d: {market?.sales30d ?? '\u2014'} sales
                    {market?.sales90d != null && <> &middot; 90d: {market.sales90d}</>}
                    {' '}&middot; velocity: {velocity.dailyAverage.toFixed(1)}/day
                  </div>
                )}
              </>
            ) : (
              /* Legacy layout (no gradeData) */
              <div className={isCompact ? 'space-y-0' : 'space-y-1'}>
                {gradeRows.map(({ key, label }) => (
                  <LegacyPriceRow
                    key={key}
                    label={label}
                    price={prices[key]}
                    ebayHref={getEbayUrl(card, key)}
                    altHref={getAltUrl(card, key)}
                    cardLadderHref={getCardLadderUrl(card, key)}
                    compact={isCompact}
                    gradeKey={key}
                  />
                ))}
              </div>
            )
        ) : null}

      </div>
    </CardShell>
  );
}

export default CardPriceCard;
