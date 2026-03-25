import { createContext, useContext, useCallback, ReactNode } from 'react';
import { useLocalStorage } from '../hooks/useLocalStorage';

export type ViewMode = 'grid' | 'list';

/**
 * Saved filter preferences for user preferences context.
 * Not to be confused with FilterState in stores/filterStore which manages
 * the active advanced filter state.
 */
export interface SavedFilterPreferences {
  roi: number | null;
  liquidity: string[];
  scarcity: string[];
  searchQuery: string;
}

export interface RecentCard {
  id: string;
  name: string;
  setName: string;
  number: string;
  imageUrl?: string;
}

export interface UserPreferences {
  /** View preferences */
  viewMode: ViewMode;
  gridColumns: number;
  sortBy: string;

  /** Filter preferences (last used filters) */
  filters: SavedFilterPreferences;

  /** Recent price checks (card objects, max 5) */
  recentPriceChecks: RecentCard[];

  /** Display preferences */
  showImages: boolean;
  compactMode: boolean;

  /** Accessibility */
  reduceMotion: boolean;
}

export interface UserPreferencesContextValue {
  /** Current preferences */
  preferences: UserPreferences;

  /** Add a card to recent price checks (max 5) */
  addRecentPriceCheck: (card: RecentCard) => void;

  /** Clear recent price checks */
  clearRecentPriceChecks: () => void;
}

/**
 * User Preferences Context
 * Manages user preferences with localStorage persistence
 */
const UserPreferencesContext = createContext<UserPreferencesContextValue | null>(null);

/**
 * Default preferences
 */
const DEFAULT_PREFERENCES: UserPreferences = {
  viewMode: 'grid',
  gridColumns: 3,
  sortBy: 'roi_desc',
  filters: {
    roi: null,
    liquidity: [],
    scarcity: [],
    searchQuery: '',
  },
  recentPriceChecks: [],
  showImages: true,
  compactMode: false,
  reduceMotion: false,
};

export interface UserPreferencesProviderProps {
  children: ReactNode;
}

/**
 * User Preferences Provider Component
 * Provides user preferences state and actions
 *
 * @example
 * <UserPreferencesProvider>
 *   <App />
 * </UserPreferencesProvider>
 */
export function UserPreferencesProvider({ children }: UserPreferencesProviderProps) {
  const [preferences, setPreferences] = useLocalStorage<UserPreferences>(
    'userPreferences',
    DEFAULT_PREFERENCES
  );

  /**
   * Add a card to recent price checks
   * Limits to 5 most recent, removes duplicates by card.id
   */
  const addRecentPriceCheck = useCallback(
    (card: RecentCard) => {
      if (!card || !card.id) return;

      setPreferences((prev) => {
        // Remove duplicates (same card id) and add to front
        const filtered = prev.recentPriceChecks.filter((c) => c.id !== card.id);
        const updated = [card, ...filtered].slice(0, 5); // Keep max 5

        return {
          ...prev,
          recentPriceChecks: updated,
        };
      });
    },
    [setPreferences]
  );

  /**
   * Clear recent price checks
   */
  const clearRecentPriceChecks = useCallback(() => {
    setPreferences((prev) => ({
      ...prev,
      recentPriceChecks: [],
    }));
  }, [setPreferences]);

  const contextValue: UserPreferencesContextValue = {
    preferences,
    addRecentPriceCheck,
    clearRecentPriceChecks,
  };

  return (
    <UserPreferencesContext.Provider value={contextValue}>
      {children}
    </UserPreferencesContext.Provider>
  );
}

/**
 * Hook to access user preferences context
 * Must be used within a UserPreferencesProvider
 *
 * @example
 * function RecentCards() {
 *   const { preferences, addRecentPriceCheck } = useUserPreferences();
 *   // Use preferences.recentPriceChecks
 * }
 */
// eslint-disable-next-line react-refresh/only-export-components
export function useUserPreferences(): UserPreferencesContextValue {
  const context = useContext(UserPreferencesContext);

  if (!context) {
    throw new Error('useUserPreferences must be used within a UserPreferencesProvider');
  }

  return context;
}

export default UserPreferencesContext;
