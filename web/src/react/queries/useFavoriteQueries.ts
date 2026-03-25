import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useMemo } from 'react';
import { api } from '../../js/api';
import type { Favorite, FavoriteInput } from '../../types/favorites';
import { queryKeys } from './queryKeys';

const makeKey = (cardName: string, setName: string, cardNumber: string): string =>
  `${cardName}|${setName}|${cardNumber}`;

export function useFavoritesQuery() {
  return useQuery({
    queryKey: queryKeys.favorites.all,
    queryFn: async () => {
      const allFavorites: Favorite[] = [];
      let page = 1;
      const pageSize = 100;
      const first = await api.getFavorites(page, pageSize);
      allFavorites.push(...(first.favorites || []));
      while (allFavorites.length < first.total) {
        page++;
        const result = await api.getFavorites(page, pageSize);
        if (!result.favorites?.length) break;
        allFavorites.push(...result.favorites);
      }
      return { favorites: allFavorites, total: first.total };
    },
  });
}

export function useFavoriteSet() {
  const { data } = useFavoritesQuery();
  return useMemo(() => {
    const set = new Set<string>();
    data?.favorites.forEach(f => set.add(makeKey(f.card_name, f.set_name, f.card_number)));
    return {
      isFavorite: (cardName: string, setName: string, cardNumber: string) =>
        set.has(makeKey(cardName, setName, cardNumber)),
      set,
    };
  }, [data]);
}

export function useToggleFavorite() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: FavoriteInput) => api.toggleFavorite(input),
    onMutate: async (input) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.favorites.all });
      const previous = queryClient.getQueryData<{ favorites: Favorite[]; total: number }>(
        queryKeys.favorites.all,
      );
      queryClient.setQueryData<{ favorites: Favorite[]; total: number }>(
        queryKeys.favorites.all,
        (old) => {
          if (!old) return old;
          const key = makeKey(input.card_name, input.set_name, input.card_number);
          const exists = old.favorites.some(
            f => makeKey(f.card_name, f.set_name, f.card_number) === key,
          );
          if (exists) {
            return {
              favorites: old.favorites.filter(
                f => makeKey(f.card_name, f.set_name, f.card_number) !== key,
              ),
              total: old.total - 1,
            };
          }
          const newFav: Favorite = {
            id: 0,
            user_id: 0,
            card_name: input.card_name,
            set_name: input.set_name,
            card_number: input.card_number,
            image_url: input.image_url,
            notes: input.notes,
            created_at: new Date().toISOString(),
          };
          return { favorites: [newFav, ...old.favorites], total: old.total + 1 };
        },
      );
      return { previous };
    },
    onError: (_err, _input, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKeys.favorites.all, context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.favorites.all });
    },
  });
}
