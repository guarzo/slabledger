import React, { useState } from 'react';
import { clsx } from 'clsx';
import { useAuth } from '../contexts/AuthContext';
import { useFavorites } from '../contexts/FavoritesContext';
import { reportError } from '../../js/errors';
import type { FavoriteInput } from '../../types/favorites';

/**
 * Heart icon SVG component
 * Marked as decorative since the button provides the accessible name
 */
const HeartIcon: React.FC<{ filled: boolean; className?: string }> = ({ filled, className }) => (
  <svg
    className={className}
    viewBox="0 0 24 24"
    fill={filled ? 'currentColor' : 'none'}
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z" />
  </svg>
);

export interface FavoriteButtonProps {
  /** Card name */
  cardName: string;
  /** Set name */
  setName: string;
  /** Card number */
  cardNumber: string;
  /** Optional image URL to store with favorite */
  imageUrl?: string;
  /** Button size */
  size?: 'sm' | 'md' | 'lg';
  /** Additional CSS classes */
  className?: string;
}

/**
 * FavoriteButton - Heart icon button for toggling favorites
 *
 * Features:
 * - Displays heart icon (outlined when not favorited, filled when favorited)
 * - Requires authentication (disabled when not logged in)
 * - Shows loading animation during toggle
 * - Prevents event propagation (safe to use inside clickable cards)
 *
 * @example
 * ```tsx
 * <FavoriteButton
 *   cardName="Charizard ex"
 *   setName="Obsidian Flames"
 *   cardNumber="125"
 *   size="md"
 * />
 * ```
 */
export const FavoriteButton: React.FC<FavoriteButtonProps> = ({
  cardName,
  setName,
  cardNumber,
  imageUrl,
  size = 'md',
  className = '',
}) => {
  const { user } = useAuth();
  const { isFavorite, toggleFavorite } = useFavorites();
  const [isToggling, setIsToggling] = useState(false);

  const favorited = isFavorite(cardName, setName, cardNumber);

  const handleClick = async (e: React.MouseEvent) => {
    e.stopPropagation(); // Prevent card click
    e.preventDefault();

    if (!user) {
      // Not logged in - could show tooltip or login prompt
      return;
    }

    if (isToggling) return;

    setIsToggling(true);
    try {
      const input: FavoriteInput = {
        card_name: cardName,
        set_name: setName,
        card_number: cardNumber,
        image_url: imageUrl,
      };
      await toggleFavorite(input);
    } catch (err) {
      reportError('FavoriteButton/toggle', err);
    } finally {
      setIsToggling(false);
    }
  };

  const sizeClasses = {
    sm: 'p-0.5',
    md: 'p-1',
    lg: 'p-1.5',
  };

  const iconSizeClasses = {
    sm: 'w-4 h-4',
    md: 'w-5 h-5',
    lg: 'w-6 h-6',
  };

  return (
    <button
      type="button"
      className={clsx(
        // Base styles
        'inline-flex items-center justify-center',
        'border-none bg-transparent cursor-pointer',
        'rounded-full transition-all duration-200',
        // Size
        sizeClasses[size],
        // Color states - using design tokens
        favorited
          ? 'text-[var(--danger)] hover:text-[var(--danger-hover)]'
          : 'text-[var(--text-subtle)] hover:text-[var(--danger)]',
        // Hover effect
        'hover:bg-[var(--danger-subtle)]',
        // Focus styles
        'focus:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2',
        // Disabled state
        (!user || isToggling) && 'opacity-50 cursor-not-allowed',
        className
      )}
      onClick={handleClick}
      disabled={!user || isToggling}
      aria-label={favorited ? 'Remove from favorites' : 'Add to favorites'}
      aria-pressed={favorited}
      title={!user ? 'Sign in to save favorites' : favorited ? 'Remove from favorites' : 'Add to favorites'}
    >
      <HeartIcon
        filled={favorited}
        className={clsx(
          iconSizeClasses[size],
          'transition-transform duration-200',
          'hover:scale-110',
          isToggling && 'animate-pulse'
        )}
      />
    </button>
  );
};

export default FavoriteButton;
