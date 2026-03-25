import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { useQueries } from '@tanstack/react-query';
import { useFavorites } from '../../contexts/FavoritesContext';
import { SectionErrorBoundary, CardPriceCard } from '../../ui';
import { queryKeys } from '../../queries/queryKeys';
import { api } from '../../../js/api';
import type { Favorite } from '../../../types/favorites';
import type { CardPricingResponse } from '../../../types/pricing';

function FavoriteCard({ fav, pricing, pricingLoading }: {
  fav: Favorite;
  pricing: CardPricingResponse | undefined;
  pricingLoading: boolean;
}) {
  return (
    <div className="transition-all duration-200 hover:-translate-y-1 hover:shadow-[0_8px_24px_rgba(0,0,0,0.3)]">
      <CardPriceCard
        card={{
          name: fav.card_name,
          setName: fav.set_name,
          number: fav.card_number,
          imageUrl: fav.image_url,
        }}
        prices={pricing ? {
          raw: pricing.rawUSD,
          psa8: pricing.psa8,
          psa9: pricing.psa9,
          psa10: pricing.psa10,
        } : null}
        pricesLoading={pricingLoading}
        variant="compact"
        gradeData={pricing?.gradeData}
        market={pricing?.market}
        velocity={pricing?.velocity}
        lastSold={pricing?.lastSold}
        conservativePrices={{
          psa10: pricing?.conservativePsa10,
          psa9: pricing?.conservativePsa9,
          raw: pricing?.conservativeRaw,
        }}
      />
      {fav.notes && (
        <p className="text-xs text-[var(--text-muted)] italic mt-1 px-3 pb-2">
          {fav.notes}
        </p>
      )}
    </div>
  );
}

interface WatchlistSectionProps {
  maxItems?: number;
}

export default function WatchlistSection({ maxItems }: WatchlistSectionProps) {
  const { favorites, loading: favoritesLoading } = useFavorites();

  const pricingQueries = useQueries({
    queries: favorites.map(fav => ({
      queryKey: queryKeys.pricing.cardPrice([fav.set_name, fav.card_number, fav.card_name]),
      queryFn: () => api.getCardPricing(fav.card_name, fav.set_name, fav.card_number),
      staleTime: 5 * 60 * 1000,
      enabled: !!fav.card_name && !!fav.set_name && !!fav.card_number,
    })),
  });

  const pricingMap = useMemo(() => {
    const map = new Map<string, { data: CardPricingResponse | undefined; isLoading: boolean }>();
    favorites.forEach((fav, i) => {
      const key = `${fav.card_name}-${fav.set_name}-${fav.card_number}`;
      map.set(key, { data: pricingQueries[i]?.data, isLoading: pricingQueries[i]?.isLoading ?? true });
    });
    return map;
  }, [favorites, pricingQueries]);

  if (favoritesLoading || favorites.length === 0) return null;

  const displayFavorites = maxItems ? favorites.slice(0, maxItems) : favorites;
  const hasMore = maxItems != null && favorites.length > maxItems;

  return (
    <div className="p-4 rounded-2xl border border-[var(--surface-2)]/50" style={{ background: 'var(--glass-bg)', backdropFilter: 'blur(12px)' }}>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider">Watchlist</h2>
        {hasMore && (
          <Link to="/watchlist" className="text-xs text-[var(--brand-500)] hover:underline">
            View all {favorites.length} &rarr;
          </Link>
        )}
      </div>
      <SectionErrorBoundary sectionName="Watchlist">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
          {displayFavorites.map((fav) => {
            const key = `${fav.card_name}-${fav.set_name}-${fav.card_number}`;
            const entry = pricingMap.get(key);
            return (
              <FavoriteCard
                key={key}
                fav={fav}
                pricing={entry?.data}
                pricingLoading={entry?.isLoading ?? true}
              />
            );
          })}
        </div>
      </SectionErrorBoundary>
    </div>
  );
}
