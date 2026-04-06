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
import { FavoriteButton } from './FavoriteButton';
import { LinkDropdown } from './LinkDropdown';
import { Skeleton } from './Skeleton';
import type { GradeKey, MarketOverview, SalesVelocity } from '../../types/pricing';
import {
  defaultEbayUrl, defaultAltUrl, defaultCardLadderUrl,
  localEbayCompletedUrl, gradeRows,
} from './cardPrice/priceCardUtils';
import { PriceRow } from './cardPrice/PriceRow';
import { LegacyPriceRow } from './cardPrice/LegacyPriceRow';

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
  gradeData?: Partial<Record<GradeKey, import('../../types/pricing').GradeData>>;
  market?: MarketOverview;
  velocity?: SalesVelocity;
  lastSold?: Partial<Record<string, LastSoldEntry>>;
  conservativePrices?: { psa10?: number; psa9?: number; raw?: number };
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
