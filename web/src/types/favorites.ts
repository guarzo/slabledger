/**
 * Types for the Favorites feature
 */

/**
 * Represents a user's saved card/opportunity
 */
export interface Favorite {
  id: number;
  user_id: number;
  card_name: string;
  set_name: string;
  card_number: string;
  image_url?: string;
  notes?: string;
  created_at: string;
}

/**
 * Input data for creating/toggling a favorite
 */
export interface FavoriteInput {
  card_name: string;
  set_name: string;
  card_number: string;
  image_url?: string;
  notes?: string;
}

/**
 * Paginated list of favorites from the API
 */
export interface FavoritesList {
  favorites: Favorite[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

/**
 * Response from toggle endpoint
 */
export interface ToggleFavoriteResponse {
  is_favorite: boolean;
}
