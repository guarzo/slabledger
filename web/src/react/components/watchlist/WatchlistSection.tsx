import { Link } from 'react-router-dom';
import { useFavorites } from '../../contexts/FavoritesContext';
import { SectionErrorBoundary, CardPriceCard } from '../../ui';
import type { Favorite } from '../../../types/favorites';

function FavoriteCard({ fav }: {
  fav: Favorite;
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
        prices={null}
        pricesLoading={false}
        variant="compact"
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
          {displayFavorites.map((fav) => (
            <FavoriteCard
              key={`${fav.card_name}-${fav.set_name}-${fav.card_number}`}
              fav={fav}
            />
          ))}
        </div>
      </SectionErrorBoundary>
    </div>
  );
}
