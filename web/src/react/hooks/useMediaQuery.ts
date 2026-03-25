import { useState, useEffect } from 'react';

/**
 * Custom hook for responsive design using media queries
 * Returns true if the media query matches, false otherwise
 * SSR-safe with proper cleanup
 *
 * @param query - Media query string (e.g., '(max-width: 768px)')
 * @returns True if media query matches
 *
 * @example
 * const isMobile = useMediaQuery('(max-width: 768px)');
 * const isTablet = useMediaQuery('(min-width: 769px) and (max-width: 1024px)');
 * const prefersLight = useMediaQuery('(prefers-color-scheme: light)');
 * const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)');
 *
 * // Common breakpoints
 * const breakpoints = {
 *   mobile: useMediaQuery('(max-width: 768px)'),
 *   tablet: useMediaQuery('(min-width: 769px) and (max-width: 1024px)'),
 *   desktop: useMediaQuery('(min-width: 1025px)')
 * };
 */
export function useMediaQuery(query: string): boolean {
  // Initialize state with false for SSR safety
  const [matches, setMatches] = useState<boolean>(() => {
    if (typeof window === 'undefined') {
      return false;
    }
    return window.matchMedia(query).matches;
  });

  useEffect(() => {
    // SSR check
    if (typeof window === 'undefined') {
      return;
    }

    const mediaQuery = window.matchMedia(query);

    // Update state with current match status
    setMatches(mediaQuery.matches);

    // Event handler for media query changes
    const handleChange = (event: MediaQueryListEvent) => {
      setMatches(event.matches);
    };

    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, [query]);

  return matches;
}

export default useMediaQuery;
