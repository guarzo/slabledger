import React, { createContext, useContext, useCallback, ReactNode } from 'react';
import { useAuth } from './AuthContext';
import type { Favorite, FavoriteInput } from '../../types/favorites';
import { useFavoritesQuery, useFavoriteSet, useToggleFavorite } from '../queries/useFavoriteQueries';

interface FavoritesContextType {
  favorites: Favorite[];
  favoriteSet: ReadonlySet<string>;
  loading: boolean;
  total: number;
  toggleFavorite: (input: FavoriteInput) => Promise<boolean>;
  isFavorite: (cardName: string, setName: string, cardNumber: string) => boolean;
  refreshFavorites: () => Promise<void>;
}

const FavoritesContext = createContext<FavoritesContextType | undefined>(undefined);

export const FavoritesProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const { user } = useAuth();
  const { data, isLoading, refetch } = useFavoritesQuery();
  const { isFavorite, set: favSet } = useFavoriteSet();
  const toggleMutation = useToggleFavorite();

  const toggleFavorite = useCallback(async (input: FavoriteInput): Promise<boolean> => {
    if (!user) return false;
    const result = await toggleMutation.mutateAsync(input);
    return result.is_favorite;
  }, [user, toggleMutation]);

  const refreshFavorites = useCallback(async () => {
    await refetch();
  }, [refetch]);

  return (
    <FavoritesContext.Provider
      value={{
        favorites: data?.favorites ?? [],
        favoriteSet: favSet,
        loading: isLoading,
        total: data?.total ?? 0,
        toggleFavorite,
        isFavorite,
        refreshFavorites,
      }}
    >
      {children}
    </FavoritesContext.Provider>
  );
};

// eslint-disable-next-line react-refresh/only-export-components
export const useFavorites = (): FavoritesContextType => {
  const context = useContext(FavoritesContext);
  if (context === undefined) {
    throw new Error('useFavorites must be used within a FavoritesProvider');
  }
  return context;
};

export default FavoritesContext;
